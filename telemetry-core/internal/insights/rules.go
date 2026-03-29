package insights

import (
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// Rule is a named evaluation function that inspects the current RaceState and
// returns a DrivingInsight when the rule fires, or nil when it does not.
type Rule struct {
	Name     string
	Evaluate func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight
}

// safetyCarName maps the game's SafetyCarStatus enum to a human-readable string.
func safetyCarName(status uint8) string {
	switch status {
	case 1:
		return "Safety Car"
	case 2:
		return "Virtual Safety Car"
	case 3:
		return "Formation Lap Safety Car"
	default:
		return "Safety Car"
	}
}

// DefaultRules returns all insight rules. The slice order matches evaluation priority.
func DefaultRules() []Rule {
	return []Rule{
		ruleLapStart(),
		ruleHardBraking(),
		ruleTireWear(),
		ruleFuelLow(),
		ruleERSLow(),
		ruleComponentDamage(),
		ruleRainForecast(),
		ruleSafetyCar(),
		rulePositionChange(),
		ruleBrakeTemp(),
		ruleTireTemp(),
		ruleRaceEvents(),
		rulePitEntryApproach(),
	}
}

// ---------- Rule 1: LapStart ----------

func ruleLapStart() Rule {
	return Rule{
		Name: "LapStart",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if state.CurrentLap > cd.LastLap() {
				cd.SetLastLap(state.CurrentLap)
				cd.ResetForNewLap()

				log.Info().Uint8("lap", state.CurrentLap).Msg("Detected new lap")

				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Lap %d started. Keep the momentum going.", state.CurrentLap),
					Type:     "encouragement",
					Priority: 2,
				}
			}
			return nil
		},
	}
}

// ---------- Rule 2: HardBraking ----------

func ruleHardBraking() Rule {
	return Rule{
		Name: "HardBraking",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if state.Brake > 0.5 {
				// Enter braking zone
				if !cd.BrakingInZone() {
					cd.SetBrakingInZone(true)
					cd.SetBrakingWarnedThisZone(false)
				}

				if state.Brake > 0.8 && state.Speed > 250 && !cd.BrakingWarnedThisZone() {
					cd.SetBrakingWarnedThisZone(true)
					return &models.DrivingInsight{
						Message:  "Watch the lockup, you are braking very hard into this zone.",
						Type:     "warning",
						Priority: 4,
					}
				}
			} else {
				// Released brakes - reset for next zone
				cd.SetBrakingInZone(false)
			}
			return nil
		},
	}
}

// ---------- Rule 3: TireWear ----------

func ruleTireWear() Rule {
	return Rule{
		Name: "TireWear",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			maxWear := state.TyresWear[0]
			for i := 1; i < 4; i++ {
				if state.TyresWear[i] > maxWear {
					maxWear = state.TyresWear[i]
				}
			}

			if maxWear > 60.0 && !cd.TireWarnIssued() {
				cd.SetTireWarnIssued(true)
				return &models.DrivingInsight{
					Message:  "Tires are heavily worn. You might want to consider boxing in the next few laps.",
					Type:     "strategy",
					Priority: 5,
				}
			}
			return nil
		},
	}
}

// ---------- Rule 4: FuelLow ----------

func ruleFuelLow() Rule {
	return Rule{
		Name: "FuelLow",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if cd.ShouldRateLimit("car_status") {
				return nil
			}

			// Critical first (< 3.0 laps)
			if state.FuelRemainingLaps < 3.0 && !cd.FuelCriticalIssued() {
				cd.SetFuelCriticalIssued(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Fuel is critical! Only %.1f laps of fuel remaining. Consider fuel saving.", state.FuelRemainingLaps),
					Type:     "warning",
					Priority: 5,
				}
			}

			// Low warning (< 5.0 laps)
			if state.FuelRemainingLaps < 5.0 && !cd.FuelWarnIssued() {
				cd.SetFuelWarnIssued(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Fuel is getting low - %.1f laps remaining. Keep an eye on it.", state.FuelRemainingLaps),
					Type:     "info",
					Priority: 3,
				}
			}

			return nil
		},
	}
}

// ---------- Rule 5: ERSLow ----------

func ruleERSLow() Rule {
	return Rule{
		Name: "ERSLow",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if cd.ShouldRateLimit("car_status") {
				return nil
			}

			ersPct := state.ERSStoreEnergy / 4_000_000.0 * 100.0

			if ersPct < 10.0 && !cd.ERSLowWarned() {
				cd.SetERSLowWarned(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("ERS battery at %.0f%%. Harvest more before the next overtake opportunity.", ersPct),
					Type:     "info",
					Priority: 3,
				}
			}

			// Reset when battery is sufficiently charged
			if ersPct > 30.0 {
				cd.SetERSLowWarned(false)
			}

			return nil
		},
	}
}

// ---------- Rule 6: ComponentDamage ----------

func ruleComponentDamage() Rule {
	return Rule{
		Name: "ComponentDamage",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			components := map[string]uint8{
				"Front left wing":  state.FrontLeftWingDmg,
				"Front right wing": state.FrontRightWingDmg,
				"Rear wing":        state.RearWingDmg,
				"Floor":            state.FloorDmg,
				"Diffuser":         state.DiffuserDmg,
				"Sidepod":          state.SidepodDmg,
				"Gearbox":          state.GearBoxDmg,
				"Engine":           state.EngineDmg,
			}

			for name, pct := range components {
				if pct > 50 && !cd.DamageWarned(name) {
					cd.SetDamageWarned(name)
					return &models.DrivingInsight{
						Message:  fmt.Sprintf("%s damage is at %d%%! This could affect performance significantly.", name, pct),
						Type:     "warning",
						Priority: 5,
					}
				}
			}
			return nil
		},
	}
}

// ---------- Rule 7: RainForecast ----------

func ruleRainForecast() Rule {
	return Rule{
		Name: "RainForecast",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			var rainPct uint8
			for _, f := range state.WeatherForecasts {
				if f.TimeOffset <= 10 && f.RainPercentage > 50 {
					rainPct = f.RainPercentage
					break
				}
			}

			if rainPct > 50 && !cd.RainWarned() {
				cd.SetRainWarned(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Rain forecast at %d%% probability in the next 10 minutes. Standby for potential tire change.", rainPct),
					Type:     "strategy",
					Priority: 4,
				}
			}

			if rainPct <= 20 {
				cd.SetRainWarned(false)
			}

			return nil
		},
	}
}

// ---------- Rule 8: SafetyCar ----------

func ruleSafetyCar() Rule {
	return Rule{
		Name: "SafetyCar",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if state.SafetyCarStatus > 0 && !cd.SafetyCarNotified() {
				cd.SetSafetyCarNotified(true)
				scName := safetyCarName(state.SafetyCarStatus)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("%s deployed! Manage your tires and prepare for restart.", scName),
					Type:     "warning",
					Priority: 5,
				}
			}

			if state.SafetyCarStatus == 0 {
				cd.SetSafetyCarNotified(false)
			}

			return nil
		},
	}
}

// ---------- Rule 9: PositionChange ----------

func rulePositionChange() Rule {
	return Rule{
		Name: "PositionChange",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if cd.ShouldRateLimit("position") {
				return nil
			}

			lastPos := cd.LastPosition()
			cd.SetLastPosition(state.Position)

			if lastPos == 0 {
				// First reading, nothing to compare
				return nil
			}

			// Gained a position (lower number = better)
			if state.Position < lastPos {
				if cd.OvertakeLap() != state.CurrentLap {
					cd.SetOvertakeLap(state.CurrentLap)
					return &models.DrivingInsight{
						Message:  fmt.Sprintf("Great move! You're up to P%d.", state.Position),
						Type:     "encouragement",
						Priority: 3,
					}
				}
			}

			// Lost a position
			if state.Position > lastPos {
				if cd.OvertakeLap() != state.CurrentLap {
					cd.SetOvertakeLap(state.CurrentLap)
					return &models.DrivingInsight{
						Message:  fmt.Sprintf("Lost a position, now P%d. Push to get it back.", state.Position),
						Type:     "info",
						Priority: 3,
					}
				}
			}

			return nil
		},
	}
}

// ---------- Rule 10: BrakeTemp ----------

func ruleBrakeTemp() Rule {
	return Rule{
		Name: "BrakeTemp",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if cd.ShouldRateLimit("car_telemetry") {
				return nil
			}

			var maxBrake uint16
			for _, t := range state.BrakesTemp {
				if t > maxBrake {
					maxBrake = t
				}
			}

			if maxBrake > 900 && !cd.BrakeTempWarned() {
				cd.SetBrakeTempWarned(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Brake temperatures are very high at %dC! Ease the braking or you risk brake failure.", maxBrake),
					Type:     "warning",
					Priority: 4,
				}
			}

			return nil
		},
	}
}

// ---------- Rule 11: TireTemp ----------

func ruleTireTemp() Rule {
	return Rule{
		Name: "TireTemp",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if cd.ShouldRateLimit("car_telemetry") {
				return nil
			}

			var maxSurf, minSurf uint8
			maxSurf = state.TyresSurfTemp[0]
			minSurf = state.TyresSurfTemp[0]
			for i := 1; i < 4; i++ {
				if state.TyresSurfTemp[i] > maxSurf {
					maxSurf = state.TyresSurfTemp[i]
				}
				if state.TyresSurfTemp[i] < minSurf {
					minSurf = state.TyresSurfTemp[i]
				}
			}

			if cd.TireTempWarned() {
				return nil
			}

			if maxSurf > 110 {
				cd.SetTireTempWarned(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Tire surface temperature is %dC - overheating. Manage your inputs.", maxSurf),
					Type:     "warning",
					Priority: 3,
				}
			}

			if minSurf < 80 {
				cd.SetTireTempWarned(true)
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Tire surface temperature is %dC - tires are cold. Push to build heat.", minSurf),
					Type:     "info",
					Priority: 2,
				}
			}

			return nil
		},
	}
}

// ---------- Rule 13: PitEntryApproach ----------

func rulePitEntryApproach() Rule {
	return Rule{
		Name: "PitEntryApproach",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			if state.PitStatus != 0 || state.TrackLength == 0 {
				return nil
			}
			if state.PitWindowIdeal == 0 && state.PitWindowLatest == 0 {
				return nil // no pit window data
			}
			inWindow := state.CurrentLap >= state.PitWindowIdeal &&
				state.CurrentLap <= state.PitWindowLatest
			if !inWindow {
				return nil
			}
			lapPct := float64(state.LapDistance) / float64(state.TrackLength)
			if lapPct < 0.85 {
				return nil
			}
			if cd.PitEntryWarned() {
				return nil
			}
			cd.SetPitEntryWarned(true)
			remaining := float64(state.TrackLength) * (1.0 - lapPct)
			return &models.DrivingInsight{
				Message:  fmt.Sprintf("Approaching pit entry — %.0fm remaining this lap. Pit window open (ideal lap %d, latest %d). Commit now if boxing.", remaining, state.PitWindowIdeal, state.PitWindowLatest),
				Type:     "strategy",
				Priority: 4,
			}
		},
	}
}

// ---------- Rule 12: RaceEvents ----------

func ruleRaceEvents() Rule {
	return Rule{
		Name: "RaceEvents",
		Evaluate: func(state *models.RaceState, cd *CooldownManager) *models.DrivingInsight {
			code := state.LastEventCode
			if code == "" || code == cd.LastEventCode() {
				return nil
			}
			cd.SetLastEventCode(code)

			switch code {
			case "FTLP":
				return &models.DrivingInsight{
					Message:  fmt.Sprintf("Fastest lap set by car %d!", state.LastEventIdx),
					Type:     "info",
					Priority: 2,
				}
			case "SAFC":
				return &models.DrivingInsight{
					Message:  "Safety car has been deployed.",
					Type:     "warning",
					Priority: 5,
				}
			case "LGOT":
				return &models.DrivingInsight{
					Message:  "Lights out and away we go! Good start, keep it clean.",
					Type:     "encouragement",
					Priority: 4,
				}
			case "SSTA":
				return &models.DrivingInsight{
					Message:  "The race is about to start! Give me a concise pre-race brief of the track, strategy, and other relevant info based on the context.",
					Type:     "strategy",
					Priority: 5,
				}
			default:
				return nil
			}
		},
	}
}
