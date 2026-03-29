// Package workspace maintains a compact insights.md file that accumulates
// findings from the insight engine and telemetry state. The file serves as
// persistent memory for LLM prompts — any agent or model can GET /api/workspace
// to read what the race engineer has observed so far.
package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

const (
	maxEntries     = 100          // ring-buffer size for insight log
	snapshotPeriod = 10 * time.Second // how often we rewrite the file
)

// entry is a single timestamped finding.
type entry struct {
	Time    time.Time
	Type    string // warning, info, encouragement, strategy
	Message string
}

// Writer accumulates insights and periodically flushes them to insights.md.
type Writer struct {
	dir         string
	stateLoader func() *models.RaceState

	mu      sync.Mutex
	entries []entry
	session sessionMeta
}

// sessionMeta captures one-time session facts written to the header.
type sessionMeta struct {
	set       bool
	trackID   int8
	sessType  uint8
	totalLaps uint8
	trackLen  uint16
}

// NewWriter creates a workspace writer that will write to dir/insights.md.
func NewWriter(dir string, stateLoader func() *models.RaceState) *Writer {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("Could not create workspace dir")
	}
	return &Writer{
		dir:         dir,
		stateLoader: stateLoader,
		entries:     make([]entry, 0, maxEntries),
	}
}

// RecordInsight appends an insight to the in-memory ring buffer.
func (w *Writer) RecordInsight(insight models.DrivingInsight) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, entry{
		Time:    time.Now(),
		Type:    insight.Type,
		Message: insight.Message,
	})
	if len(w.entries) > maxEntries {
		w.entries = w.entries[len(w.entries)-maxEntries:]
	}
}

// Run starts the periodic flush loop. Blocks until ctx is done.
func (w *Writer) Run(ctx context.Context) {
	ticker := time.NewTicker(snapshotPeriod)
	defer ticker.Stop()

	log.Info().Str("dir", w.dir).Msg("Workspace writer started")
	for {
		select {
		case <-ctx.Done():
			w.flush() // final write
			log.Info().Msg("Workspace writer stopped")
			return
		case <-ticker.C:
			w.flush()
		}
	}
}

// Content returns the current insights.md content (used by the API handler).
func (w *Writer) Content() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.render()
}

// flush writes insights.md to disk atomically.
func (w *Writer) flush() {
	w.mu.Lock()
	content := w.render()
	w.mu.Unlock()

	path := filepath.Join(w.dir, "insights.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Warn().Err(err).Msg("Failed to write insights.md")
	}
}

// render builds the markdown string. Must be called with mu held.
func (w *Writer) render() string {
	var b strings.Builder
	b.WriteString("# Race Engineer Insights\n\n")

	// Session header from live state
	state := w.stateLoader()
	if state != nil {
		if !w.session.set {
			w.session.set = true
			w.session.trackID = state.TrackID
			w.session.sessType = state.SessionType
			w.session.totalLaps = state.TotalLaps
			w.session.trackLen = state.TrackLength
		}

		b.WriteString("## Session\n")
		b.WriteString(fmt.Sprintf("Track: %s | Type: %s | Laps: %d | Length: %dm\n\n",
			trackName(w.session.trackID), sessionTypeName(w.session.sessType),
			w.session.totalLaps, w.session.trackLen))

		// Compact live snapshot
		b.WriteString("## Live State\n")
		b.WriteString(fmt.Sprintf("Lap %d P%d | %dkm/h G%d | T:%.0f%% B:%.0f%%\n",
			state.CurrentLap, state.Position, state.Speed, state.Gear,
			state.Throttle*100, state.Brake*100))
		b.WriteString(fmt.Sprintf("Wear FL:%.1f FR:%.1f RL:%.1f RR:%.1f\n",
			state.TyresWear[2], state.TyresWear[3], state.TyresWear[0], state.TyresWear[1]))
		b.WriteString(fmt.Sprintf("Fuel: %.1fkg (%.1f laps) | ERS: %.0f%%\n",
			state.FuelInTank, state.FuelRemainingLaps,
			state.ERSStoreEnergy/4_000_000*100))
		b.WriteString(fmt.Sprintf("Weather: %s | Track: %dC Air: %dC | Rain: %d%%\n",
			weatherName(state.Weather), state.TrackTemp, state.AirTemp, state.RainPercentage))

		// Damage (only non-zero)
		dmg := w.damageLine(state)
		if dmg != "" {
			b.WriteString(fmt.Sprintf("Damage: %s\n", dmg))
		}
		b.WriteByte('\n')
	}

	// Insight log (newest first)
	if len(w.entries) > 0 {
		b.WriteString("## Findings\n")
		// Write newest first, max 50 for LLM context window
		limit := len(w.entries)
		if limit > 50 {
			limit = 50
		}
		for i := len(w.entries) - 1; i >= len(w.entries)-limit; i-- {
			e := w.entries[i]
			b.WriteString(fmt.Sprintf("- [%s] %s: %s\n",
				e.Time.Format("15:04:05"), strings.ToUpper(e.Type), e.Message))
		}
	}

	return b.String()
}

func (w *Writer) damageLine(state *models.RaceState) string {
	parts := []string{}
	add := func(name string, val uint8) {
		if val > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d%%", name, val))
		}
	}
	add("FLW", state.FrontLeftWingDmg)
	add("FRW", state.FrontRightWingDmg)
	add("RW", state.RearWingDmg)
	add("Floor", state.FloorDmg)
	add("Diff", state.DiffuserDmg)
	add("Side", state.SidepodDmg)
	add("GB", state.GearBoxDmg)
	add("Eng", state.EngineDmg)
	return strings.Join(parts, " ")
}

// --- enum maps (compact subset for the file) ---

func trackName(id int8) string {
	names := map[int8]string{
		0: "Melbourne", 1: "Paul Ricard", 2: "Shanghai", 3: "Bahrain",
		4: "Catalunya", 5: "Monaco", 6: "Montreal", 7: "Silverstone",
		8: "Hockenheim", 9: "Hungaroring", 10: "Spa", 11: "Monza",
		12: "Singapore", 13: "Suzuka", 14: "Abu Dhabi", 15: "Austin",
		16: "Interlagos", 17: "Red Bull Ring", 18: "Sochi", 19: "Mexico City",
		20: "Baku", 21: "Sakhir Short", 22: "Silverstone Short",
		23: "Austin Short", 24: "Suzuka Short", 25: "Hanoi",
		26: "Zandvoort", 27: "Imola", 28: "Portimao", 29: "Jeddah",
		30: "Miami", 31: "Las Vegas", 32: "Losail",
	}
	if n, ok := names[id]; ok {
		return n
	}
	return fmt.Sprintf("Track-%d", id)
}

func sessionTypeName(t uint8) string {
	names := map[uint8]string{
		0: "Unknown", 1: "P1", 2: "P2", 3: "P3", 4: "Short P",
		5: "Q1", 6: "Q2", 7: "Q3", 8: "Short Q", 9: "OSQ",
		10: "Race", 11: "Race 2", 12: "Race 3", 13: "Time Trial",
	}
	if n, ok := names[t]; ok {
		return n
	}
	return fmt.Sprintf("Session-%d", t)
}

func weatherName(w uint8) string {
	names := map[uint8]string{
		0: "Clear", 1: "Light Cloud", 2: "Overcast",
		3: "Light Rain", 4: "Heavy Rain", 5: "Storm",
	}
	if n, ok := names[w]; ok {
		return n
	}
	return "Unknown"
}
