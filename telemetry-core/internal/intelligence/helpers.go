package intelligence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// BuildContext creates a rich context string from the current RaceState.
// Shared by CommsGate, analyst, and any future components.
func BuildContext(state *models.RaceState) string {
	if state == nil {
		return "No telemetry data available yet."
	}

	var parts []string

	// Session identity
	track := trackName(int(state.TrackID))
	sessType := sessionTypeName(int(state.SessionType))
	parts = append(parts, fmt.Sprintf("SESSION: %s at %s, Lap %d/%d",
		sessType, track, state.CurrentLap, state.TotalLaps))

	// Position & gaps
	parts = append(parts, fmt.Sprintf("Position: P%d (started P%d), Pit stops: %d",
		state.Position, state.GridPosition, state.NumPitStops))
	if state.DeltaToFrontMs > 0 {
		parts = append(parts, fmt.Sprintf("Gap to car ahead: +%.3fs", float64(state.DeltaToFrontMs)/1000))
	}
	if state.DeltaToLeaderMs > 0 && state.Position > 1 {
		parts = append(parts, fmt.Sprintf("Gap to leader: +%.3fs", float64(state.DeltaToLeaderMs)/1000))
	}
	if state.LastLapTimeMs > 0 {
		parts = append(parts, fmt.Sprintf("Last lap: %.3fs", float64(state.LastLapTimeMs)/1000))
	}
	if state.CurrentLapTimeMs > 0 {
		parts = append(parts, fmt.Sprintf("Current lap: %.3fs", float64(state.CurrentLapTimeMs)/1000))
	}

	// Car location
	pitSt := pitStatusName(state.PitStatus)
	driverSt := driverStatusName(state.DriverStatus)
	lapPct := 0.0
	if state.TrackLength > 0 {
		lapPct = float64(state.LapDistance) / float64(state.TrackLength) * 100
	}
	parts = append(parts, fmt.Sprintf(
		"Location: Sector %d, %.0f%% into lap (%.0fm / %dm), Status: %s, Driver: %s",
		state.Sector+1, lapPct, state.LapDistance, state.TrackLength, pitSt, driverSt,
	))

	// Basic telemetry
	parts = append(parts, fmt.Sprintf("Speed: %dkm/h, Gear: %d, RPM: %d",
		state.Speed, state.Gear, state.EngineRPM))

	// Tires
	compound := compoundName(state.ActualCompound)
	parts = append(parts, fmt.Sprintf("Tire: %s (Age: %d laps)", compound, state.TyresAgeLaps))
	parts = append(parts, fmt.Sprintf("Tire Wear - FL:%.1f%%, FR:%.1f%%, RL:%.1f%%, RR:%.1f%%",
		state.TyresWear[2], state.TyresWear[3], state.TyresWear[0], state.TyresWear[1]))

	// Fuel & ERS
	fuelMix := fuelMixName(state.FuelMix)
	ersPct := float64(state.ERSStoreEnergy) / 4_000_000.0 * 100
	ersMode := ersModeName(state.ERSDeployMode)
	parts = append(parts, fmt.Sprintf("Fuel: %.1fkg (%.1f laps remaining, Mix: %s)",
		state.FuelInTank, state.FuelRemainingLaps, fuelMix))
	parts = append(parts, fmt.Sprintf("ERS: %.0f%% (%s)", ersPct, ersMode))
	if state.DRSAllowed > 0 {
		parts = append(parts, "DRS: Available")
	}

	// Damage
	var dmg []string
	addDmg := func(name string, val uint8) {
		if val > 0 {
			dmg = append(dmg, fmt.Sprintf("%s:%d%%", name, val))
		}
	}
	addDmg("FL Wing", state.FrontLeftWingDmg)
	addDmg("FR Wing", state.FrontRightWingDmg)
	addDmg("Rear Wing", state.RearWingDmg)
	addDmg("Floor", state.FloorDmg)
	addDmg("Diffuser", state.DiffuserDmg)
	addDmg("Sidepod", state.SidepodDmg)
	addDmg("Gearbox", state.GearBoxDmg)
	addDmg("Engine", state.EngineDmg)
	if len(dmg) > 0 {
		parts = append(parts, "DAMAGE: "+strings.Join(dmg, ", "))
	}

	// Session conditions
	weather := weatherName(state.Weather)
	parts = append(parts, fmt.Sprintf("Weather: %s, Track: %dC, Air: %dC, Rain: %d%%",
		weather, state.TrackTemp, state.AirTemp, state.RainPercentage))
	if state.SafetyCarStatus > 0 {
		parts = append(parts, fmt.Sprintf("SAFETY CAR: %s", safetyCarName(state.SafetyCarStatus)))
	}

	// Pit window
	if state.PitWindowIdeal > 0 || state.PitWindowLatest > 0 {
		parts = append(parts, fmt.Sprintf("Pit window: ideal lap %d, latest lap %d",
			state.PitWindowIdeal, state.PitWindowLatest))
	}

	// Session time
	if state.SessionTimeLeft > 0 {
		parts = append(parts, fmt.Sprintf("Time left: %dm %ds",
			state.SessionTimeLeft/60, state.SessionTimeLeft%60))
	}

	return strings.Join(parts, ", ")
}

// --- enum helpers ---

func compoundName(c uint8) string {
	switch c {
	case 16:
		return "Soft"
	case 17:
		return "Medium"
	case 18:
		return "Hard"
	case 7:
		return "Inter"
	case 8:
		return "Wet"
	default:
		return "Unknown"
	}
}

func fuelMixName(m uint8) string {
	switch m {
	case 0:
		return "Lean"
	case 1:
		return "Standard"
	case 2:
		return "Rich"
	case 3:
		return "Max"
	default:
		return "Unknown"
	}
}

func ersModeName(m uint8) string {
	switch m {
	case 0:
		return "None"
	case 1:
		return "Medium"
	case 2:
		return "Hotlap"
	case 3:
		return "Overtake"
	default:
		return "Unknown"
	}
}

func weatherName(w uint8) string {
	switch w {
	case 0:
		return "Clear"
	case 1:
		return "Light Cloud"
	case 2:
		return "Overcast"
	case 3:
		return "Light Rain"
	case 4:
		return "Heavy Rain"
	case 5:
		return "Storm"
	default:
		return "Unknown"
	}
}

func pitStatusName(s uint8) string {
	switch s {
	case 0:
		return "On Track"
	case 1:
		return "Pit Lane"
	case 2:
		return "In Pit Area"
	default:
		return "Unknown"
	}
}

func driverStatusName(s uint8) string {
	switch s {
	case 0:
		return "In Garage"
	case 1:
		return "Flying Lap"
	case 2:
		return "In Lap"
	case 3:
		return "Out Lap"
	case 4:
		return "On Track"
	default:
		return "Unknown"
	}
}

func safetyCarName(s uint8) string {
	switch s {
	case 0:
		return "None"
	case 1:
		return "Full"
	case 2:
		return "Virtual"
	case 3:
		return "Formation Lap"
	default:
		return "Unknown"
	}
}

func trackName(id int) string {
	names := map[int]string{
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

func sessionTypeName(t int) string {
	names := map[int]string{
		0: "Unknown", 1: "P1", 2: "P2", 3: "P3", 4: "Short P",
		5: "Q1", 6: "Q2", 7: "Q3", 8: "Short Q", 9: "OSQ",
		10: "Race", 11: "Race 2", 12: "Race 3", 13: "Time Trial",
	}
	if n, ok := names[t]; ok {
		return n
	}
	return fmt.Sprintf("Session-%d", t)
}

// loadFile reads a file from the current directory, returning fallback on error.
func loadFile(filename, fallback string) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		log.Warn().Str("file", filename).Msg("Could not load personality file, using default")
		return fallback
	}
	return string(data)
}

// loadFileWithFallback tries workspace/primary first, then root/secondary, then returns fallback.
func loadFileWithFallback(workspaceDir, primary, secondary, fallback string) string {
	// Try workspace directory first (e.g. workspace/soul.md)
	path := filepath.Join(workspaceDir, primary)
	data, err := os.ReadFile(path)
	if err == nil {
		log.Info().Str("file", path).Msg("Loaded personality file from workspace")
		return string(data)
	}

	// Fall back to project root (e.g. SOUL.md)
	data, err = os.ReadFile(secondary)
	if err == nil {
		log.Info().Str("file", secondary).Msg("Loaded personality file from project root")
		return string(data)
	}

	log.Warn().Str("primary", path).Str("secondary", secondary).Msg("Could not load personality file, using default")
	return fallback
}
