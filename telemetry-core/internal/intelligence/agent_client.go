package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const maxAgentLogs = 100

// AgentLogEntry is a single log line surfaced to the dashboard.
type AgentLogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`   // info, warn, error, debug
	Message string `json:"message"`
}

// AgentStatus is the JSON returned by GET /api/agent/status.
type AgentStatus struct {
	Enabled   bool            `json:"enabled"`
	Healthy   bool            `json:"healthy"`
	URL       string          `json:"url"`
	SessionID string          `json:"session_id"`
	Interval  int             `json:"interval_sec"`
	Cycles    int             `json:"cycles"`
	LastCycle string          `json:"last_cycle,omitempty"` // ISO timestamp
	Logs      []AgentLogEntry `json:"logs"`
}

// AgentClient sends periodic analysis prompts to an OpenCode headless server.
// The OpenCode agent queries DuckDB via racedb and publishes insights via
// publish_insight/publish_critical, which POST back to /api/strategy.
type AgentClient struct {
	baseURL  string
	interval time.Duration
	client   *http.Client

	sessionID string // reused across ticks for conversation continuity

	// Observable state for the dashboard.
	mu        sync.RWMutex
	healthy   bool
	cycles    int
	lastCycle time.Time
	logs      []AgentLogEntry
}

// NewAgentClient creates an agent client pointing at the OpenCode server.
func NewAgentClient(baseURL string, intervalSec int) *AgentClient {
	if intervalSec <= 0 {
		intervalSec = 15
	}
	return &AgentClient{
		baseURL:  baseURL,
		interval: time.Duration(intervalSec) * time.Second,
		client: &http.Client{
			Timeout: 180 * time.Second, // agent runs multiple tool calls per cycle
		},
		logs: make([]AgentLogEntry, 0, maxAgentLogs),
	}
}

// Status returns a snapshot of the agent client state for the dashboard.
func (ac *AgentClient) Status() AgentStatus {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	var lastCycle string
	if !ac.lastCycle.IsZero() {
		lastCycle = ac.lastCycle.Format(time.RFC3339)
	}

	// Return a copy of logs (newest first — they're already stored that way).
	logsCopy := make([]AgentLogEntry, len(ac.logs))
	copy(logsCopy, ac.logs)

	return AgentStatus{
		Enabled:   true,
		Healthy:   ac.healthy,
		URL:       ac.baseURL,
		SessionID: ac.sessionID,
		Interval:  int(ac.interval.Seconds()),
		Cycles:    ac.cycles,
		LastCycle: lastCycle,
		Logs:      logsCopy,
	}
}

func (ac *AgentClient) appendLog(level, msg string) {
	entry := AgentLogEntry{
		Time:    time.Now().Format("15:04:05"),
		Level:   level,
		Message: msg,
	}
	ac.mu.Lock()
	// Prepend (newest first).
	ac.logs = append([]AgentLogEntry{entry}, ac.logs...)
	if len(ac.logs) > maxAgentLogs {
		ac.logs = ac.logs[:maxAgentLogs]
	}
	ac.mu.Unlock()
}

// Run starts the periodic analysis loop. Blocks until ctx is cancelled.
func (ac *AgentClient) Run(ctx context.Context) {
	ac.appendLog("info", fmt.Sprintf("Agent client starting — target %s, interval %s", ac.baseURL, ac.interval))
	log.Info().
		Str("url", ac.baseURL).
		Dur("interval", ac.interval).
		Msg("OpenCode agent client started")

	// Wait for the OpenCode server to become healthy before creating a session.
	if !ac.waitForHealth(ctx) {
		ac.appendLog("error", "Shutdown before OpenCode server became healthy")
		return
	}

	// Create a persistent session.
	if err := ac.createSession(ctx); err != nil {
		ac.appendLog("error", fmt.Sprintf("Failed to create session: %v", err))
		log.Error().Err(err).Msg("Failed to create OpenCode session — agent client disabled")
		return
	}

	ticker := time.NewTicker(ac.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ac.appendLog("info", "Agent client shutting down")
			log.Info().Msg("OpenCode agent client stopping")
			return
		case <-ticker.C:
			ac.sendAnalysisPrompt(ctx)
		}
	}
}

// waitForHealth polls the OpenCode /global/health endpoint until it responds OK.
func (ac *AgentClient) waitForHealth(ctx context.Context) bool {
	healthURL := ac.baseURL + "/global/health"
	ac.appendLog("info", "Waiting for OpenCode server to become healthy…")
	log.Info().Str("url", healthURL).Msg("Waiting for OpenCode server…")

	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		resp, err := ac.client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ac.mu.Lock()
				ac.healthy = true
				ac.mu.Unlock()
				ac.appendLog("info", "OpenCode server is healthy")
				log.Info().Msg("OpenCode server is healthy")
				return true
			}
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(3 * time.Second):
		}
	}
}

// createSession creates a new OpenCode session and stores the session ID.
func (ac *AgentClient) createSession(ctx context.Context) error {
	url := ac.baseURL + "/session"

	// OpenCode POST /session accepts an empty object (or optional parentID/title).
	body := []byte("{}")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ac.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	ac.sessionID = result.ID
	ac.appendLog("info", fmt.Sprintf("Session created: %s", ac.sessionID))
	log.Info().Str("session_id", ac.sessionID).Msg("OpenCode session created")
	return nil
}

// sendAnalysisPrompt sends an analysis trigger message to the OpenCode session.
func (ac *AgentClient) sendAnalysisPrompt(ctx context.Context) {
	if ac.sessionID == "" {
		ac.appendLog("warn", "No session — skipping analysis tick")
		log.Warn().Msg("No OpenCode session — skipping analysis tick")
		return
	}

	ac.appendLog("info", "Sending analysis prompt…")

	url := fmt.Sprintf("%s/session/%s/message", ac.baseURL, ac.sessionID)

	prompt := `Analyze the current race state. Follow your analysis checklist:

1. First, detect the player car index: curl -s http://localhost:8081/api/telemetry/latest | jq '.player_car_index' — use this as $IDX in all queries below. NEVER hardcode a car_index.
2. Read workspace context files (driver_profile.md, track_setup.md, past_learnings.md) if you haven't recently.
3. Query car_damage for tire wear trend (car_index = $IDX, last 5 samples).
4. Query session_data for weather and rain_percentage.
5. Query session_history for last 3-5 lap times (car_index = $IDX).
6. Query car_status for fuel state and tire compound age.
7. Query lap_data for position and gaps to cars ahead/behind.
8. Check car_damage for wing/engine component wear.

Cross-reference findings with past_learnings.md thresholds. If you find actionable insights, use publish_insight or publish_critical. If nothing notable has changed, say "No action needed."`

	// OpenCode expects: {"parts": [{"type": "text", "text": "..."}], "agent": "data-analyst"}
	type textPart struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	payload := struct {
		Parts []textPart `json:"parts"`
		Agent string     `json:"agent"`
	}{
		Parts: []textPart{{Type: "text", Text: prompt}},
		Agent: "data-analyst",
	}

	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		ac.appendLog("error", fmt.Sprintf("Failed to build request: %v", err))
		log.Error().Err(err).Msg("Failed to build OpenCode message request")
		return
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := ac.client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		ac.mu.Lock()
		ac.healthy = false
		ac.mu.Unlock()
		ac.appendLog("warn", fmt.Sprintf("Agent unreachable: %v", err))
		log.Warn().Err(err).Msg("OpenCode agent unreachable — will retry next tick")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		ac.appendLog("warn", fmt.Sprintf("Non-OK response %d: %s", resp.StatusCode, truncate(string(data), 120)))
		log.Warn().
			Int("status", resp.StatusCode).
			Str("body", string(data)).
			Msg("OpenCode agent returned non-OK")
		return
	}

	// OpenCode response: {"info": {...}, "parts": [{"type": "text", "text": "..."}, ...]}
	var result struct {
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Debug().Err(err).Msg("Could not decode OpenCode response")
	}

	// Extract text from response parts.
	var responseText string
	for _, p := range result.Parts {
		if p.Type == "text" && p.Text != "" {
			responseText = p.Text
			break
		}
	}

	ac.mu.Lock()
	ac.healthy = true
	ac.cycles++
	ac.lastCycle = time.Now()
	ac.mu.Unlock()

	summary := "No action needed"
	if responseText != "" {
		summary = truncate(responseText, 150)
	}
	ac.appendLog("info", fmt.Sprintf("Cycle #%d complete (%s): %s", ac.cycles, elapsed.Truncate(time.Millisecond), summary))
	log.Debug().Str("response", truncate(responseText, 200)).Msg("OpenCode analysis cycle complete")
}

// truncate shortens a string to maxLen, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
