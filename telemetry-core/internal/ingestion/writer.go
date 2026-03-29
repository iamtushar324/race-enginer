package ingestion

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/packets"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/storage"
)

// stateWriter is a goroutine that reads ParsedPackets, updates the atomic
// in-memory RaceState cache, and appends rows to the DuckDB batch buffers.
// It owns all writes to both the cache and the buffers — single-writer pattern.
func stateWriter(
	ctx context.Context,
	parsedChan <-chan models.ParsedPacket,
	store *storage.Storage,
	sampleRate int,
	pttButton uint32,
	pttChan chan<- bool,
) {
	// Running RaceState that we mutate and atomically swap.
	state := &models.RaceState{}

	// Sampling counters for high-frequency packets (20Hz → ~1Hz).
	sampleCounters := make(map[uint8]int)

	// Track previous BUTN state to detect PTT press/release transitions.
	var prevButtonStatus uint32
	var pttActive bool

	// Periodic buffer flush — runs in a separate goroutine to avoid blocking
	// packet processing. The buffer's internal mutex handles concurrent access.
	flushTicker := time.NewTicker(1 * time.Second)
	defer flushTicker.Stop()
	var flushing atomic.Bool

	for {
		select {
		case <-ctx.Done():
			// Final flush before exit (synchronous).
			if err := store.Buffers().FlushAll(context.Background()); err != nil {
				log.Error().Err(err).Msg("Failed final buffer flush")
			}
			log.Info().Msg("State writer stopping")
			return

		case pkt, ok := <-parsedChan:
			if !ok {
				return
			}
			updateState(state, pkt, store, sampleCounters, sampleRate)

			// Check for PTT button transitions on BUTN events (packet ID 3).
			if pkt.PacketID == 3 && pttButton != 0 {
				checkPTT(pkt.Data, pttButton, &prevButtonStatus, &pttActive, pttChan)
			}

			// Atomically publish new snapshot.
			snapshot := *state // shallow copy
			store.Cache().Store(&snapshot)

		case <-flushTicker.C:
			// Non-blocking flush: skip if a previous flush is still running.
			if flushing.CompareAndSwap(false, true) {
				go func() {
					defer flushing.Store(false)
					if err := store.Buffers().FlushAll(ctx); err != nil {
						log.Error().Err(err).Msg("Periodic buffer flush failed")
					}
				}()
			}
		}
	}
}

// updateState dispatches packet data into the running RaceState and DuckDB buffers.
func updateState(
	state *models.RaceState,
	pkt models.ParsedPacket,
	store *storage.Storage,
	sampleCounters map[uint8]int,
	sampleRate int,
) {
	switch pkt.PacketID {
	case 0: // Motion
		handleMotion(state, pkt.Data, store, sampleCounters, sampleRate)
	case 1: // Session
		handleSession(state, pkt.Data, store)
	case 2: // Lap Data
		handleLapData(state, pkt.Data, store, sampleCounters, sampleRate)
	case 3: // Event
		handleEvent(state, pkt.Data, store)
	case 6: // Car Telemetry
		handleCarTelemetry(state, pkt.Data, store, sampleCounters, sampleRate)
	case 7: // Car Status
		handleCarStatus(state, pkt.Data, store, sampleCounters, sampleRate)
	case 10: // Car Damage
		handleCarDamage(state, pkt.Data, store)
	case 11: // Session History
		handleSessionHistory(pkt.Data, store)
	}
}

// shouldSample returns true if this packet should be written to DuckDB.
// High-frequency packets are sampled at 1/sampleRate to reduce storage load.
func shouldSample(counters map[uint8]int, packetID uint8, sampleRate int) bool {
	counters[packetID]++
	return counters[packetID]%sampleRate == 0
}

func handleMotion(state *models.RaceState, data []byte, store *storage.Storage, counters map[uint8]int, rate int) {
	pkt, err := packets.Parse(0, data)
	if err != nil {
		return
	}
	motion := pkt.(*packets.PacketMotionData)
	idx := int(motion.Header.PlayerCarIndex)
	if idx >= 22 {
		return
	}
	car := motion.CarMotionData[idx]
	state.PlayerCarIndex = motion.Header.PlayerCarIndex
	state.SessionUID = motion.Header.SessionUID
	state.FrameID = motion.Header.FrameIdentifier
	state.SessionTime = motion.Header.SessionTime
	state.WorldPosX = car.WorldPositionX
	state.WorldPosY = car.WorldPositionY
	state.WorldPosZ = car.WorldPositionZ
	state.GForceLateral = car.GForceLateral
	state.GForceLongitude = car.GForceLongitudinal
	state.GForceVertical = car.GForceVertical
	state.Yaw = car.Yaw
	state.Pitch = car.Pitch
	state.Roll = car.Roll

	if shouldSample(counters, 0, rate) {
		store.Buffers().Motion.Add(storage.MotionRow{
			CarIndex: idx,
			WorldPosX: float64(car.WorldPositionX),
			WorldPosY: float64(car.WorldPositionY),
			WorldPosZ: float64(car.WorldPositionZ),
			GForceLat: float64(car.GForceLateral),
			GForceLon: float64(car.GForceLongitudinal),
			GForceVer: float64(car.GForceVertical),
			Yaw:       float64(car.Yaw),
			Pitch:     float64(car.Pitch),
			Roll:      float64(car.Roll),
		})
	}
}

func handleSession(state *models.RaceState, data []byte, store *storage.Storage) {
	pkt, err := packets.Parse(1, data)
	if err != nil {
		return
	}
	session := pkt.(*packets.PacketSessionData)
	state.Weather = session.Weather
	state.TrackTemp = session.TrackTemperature
	state.AirTemp = session.AirTemperature
	state.TotalLaps = session.TotalLaps
	state.TrackLength = session.TrackLength
	state.SessionType = session.SessionType
	state.TrackID = session.TrackID
	state.SessionTimeLeft = session.SessionTimeLeft
	state.SafetyCarStatus = session.SafetyCarStatus
	state.PitWindowIdeal = session.PitStopWindowIdealLap
	state.PitWindowLatest = session.PitStopWindowLatestLap

	// Nearest rain forecast.
	var rainPct uint8
	for i := 0; i < int(session.NumWeatherForecastSamples) && i < 64; i++ {
		ws := session.WeatherForecastSamples[i]
		if ws.TimeOffset <= 10 && ws.RainPercentage > rainPct {
			rainPct = ws.RainPercentage
		}
	}
	state.RainPercentage = rainPct

	// Store first 5 forecasts for insight engine.
	for i := 0; i < 5 && i < int(session.NumWeatherForecastSamples); i++ {
		ws := session.WeatherForecastSamples[i]
		state.WeatherForecasts[i] = models.WeatherSample{
			TimeOffset:     ws.TimeOffset,
			Weather:        ws.Weather,
			RainPercentage: ws.RainPercentage,
		}
	}

	store.Buffers().Session.Add(storage.SessionRow{
		Weather:       int(session.Weather),
		TrackTemp:     int(session.TrackTemperature),
		AirTemp:       int(session.AirTemperature),
		TotalLaps:     int(session.TotalLaps),
		TrackLength:   int(session.TrackLength),
		SessionType:   int(session.SessionType),
		TrackID:       int(session.TrackID),
		TimeLeft:      int(session.SessionTimeLeft),
		SafetyCar:     int(session.SafetyCarStatus),
		RainPct:       int(rainPct),
		PitIdealLap:   int(session.PitStopWindowIdealLap),
		PitLatestLap:  int(session.PitStopWindowLatestLap),
	})
}

func handleLapData(state *models.RaceState, data []byte, store *storage.Storage, counters map[uint8]int, rate int) {
	pkt, err := packets.Parse(2, data)
	if err != nil {
		return
	}
	lapPkt := pkt.(*packets.PacketLapData)
	idx := int(lapPkt.Header.PlayerCarIndex)
	if idx >= 22 {
		return
	}
	lap := lapPkt.LapData[idx]

	state.CurrentLap = lap.CurrentLapNum
	state.Position = lap.CarPosition
	state.Sector = lap.Sector
	state.LapDistance = lap.LapDistance
	state.TotalDistance = lap.TotalDistance
	state.LastLapTimeMs = lap.LastLapTimeInMs
	state.CurrentLapTimeMs = lap.CurrentLapTimeInMs
	state.PitStatus = lap.PitStatus
	state.NumPitStops = lap.NumPitStops
	state.GridPosition = lap.GridPosition
	state.DriverStatus = lap.DriverStatus
	// Reconstruct delta from minutes+ms parts.
	state.DeltaToFrontMs = int32(lap.DeltaToCarInFrontMinutesPart)*60000 + int32(lap.DeltaToCarInFrontMsPart)
	state.DeltaToLeaderMs = int32(lap.DeltaToRaceLeaderMinutesPart)*60000 + int32(lap.DeltaToRaceLeaderMsPart)

	if shouldSample(counters, 2, rate) {
		store.Buffers().LapData.Add(storage.LapDataRow{
			CarIndex:       idx,
			LastLapMs:      int(lap.LastLapTimeInMs),
			CurrentLapMs:   int(lap.CurrentLapTimeInMs),
			Sector1Ms:      int(lap.Sector1TimeInMs),
			Sector2Ms:      int(lap.Sector2TimeInMs),
			Position:       int(lap.CarPosition),
			LapNum:         int(lap.CurrentLapNum),
			PitStatus:      int(lap.PitStatus),
			NumPitStops:    int(lap.NumPitStops),
			Sector:         int(lap.Sector),
			Penalties:      int(lap.Penalties),
			DriverStatus:   int(lap.DriverStatus),
			ResultStatus:   int(lap.ResultStatus),
			DeltaFrontMs:   int(state.DeltaToFrontMs),
			DeltaLeaderMs:  int(state.DeltaToLeaderMs),
			SpeedTrap:      float64(lap.SpeedTrapFastestSpeed),
			GridPosition:   int(lap.GridPosition),
		})
	}
}

func handleEvent(state *models.RaceState, data []byte, store *storage.Storage) {
	pkt, err := packets.Parse(3, data)
	if err != nil {
		return
	}
	event := pkt.(*packets.PacketEventData)
	code := string(event.EventStringCode[:])
	// Trim null bytes.
	for i, b := range event.EventStringCode {
		if b == 0 {
			code = string(event.EventStringCode[:i])
			break
		}
	}
	state.LastEventCode = code

	// Parse event details for the vehicle index.
	if len(event.EventDetails) > 0 {
		state.LastEventIdx = event.EventDetails[0]
	}

	// Extract button status for BUTN events.
	if code == "BUTN" {
		if btnDetail, parseErr := packets.ParseEventDetails(packets.EventButtons, event.EventDetails); parseErr == nil && btnDetail != nil {
			if btn, ok := btnDetail.(*packets.ButtonsEvent); ok {
				state.LastButtonStatus = btn.ButtonStatus
			}
		}
	}

	detail := ""
	switch code {
	case "FTLP":
		if len(event.EventDetails) >= 5 {
			detail = "fastest_lap"
		}
	case "RTMT":
		detail = "retirement"
	case "SAFC":
		detail = "safety_car"
	case "LGOT":
		detail = "lights_out"
	case "OVTK":
		detail = "overtake"
	default:
		detail = code
	}

	vehicleIdx := 0
	if len(event.EventDetails) > 0 {
		vehicleIdx = int(event.EventDetails[0])
	}

	store.Buffers().RaceEvents.Add(storage.RaceEventRow{
		EventCode:  code,
		VehicleIdx: vehicleIdx,
		DetailText: detail,
	})
}

func handleCarTelemetry(state *models.RaceState, data []byte, store *storage.Storage, counters map[uint8]int, rate int) {
	pkt, err := packets.Parse(6, data)
	if err != nil {
		return
	}
	telPkt := pkt.(*packets.PacketCarTelemetryData)
	idx := int(telPkt.Header.PlayerCarIndex)
	if idx >= 22 {
		return
	}
	ct := telPkt.CarTelemetryData[idx]

	state.Speed = ct.Speed
	state.Throttle = ct.Throttle
	state.Brake = ct.Brake
	state.Steering = ct.Steer
	state.Gear = ct.Gear
	state.EngineRPM = ct.EngineRPM
	state.DRS = ct.DRS
	state.EngineTemp = ct.EngineTemperature
	state.BrakesTemp = ct.BrakesTemperature
	state.TyresSurfTemp = ct.TyresSurfaceTemperature
	state.TyresInnerTemp = ct.TyresInnerTemperature
	state.TyresPressure = ct.TyresPressure
	state.SuggestedGear = telPkt.SuggestedGear

	if shouldSample(counters, 6, rate) {
		store.Buffers().TelemetryExt.Add(storage.TelemetryExtRow{
			CarIndex:       idx,
			BrakeTempRL:    int(ct.BrakesTemperature[0]),
			BrakeTempRR:    int(ct.BrakesTemperature[1]),
			BrakeTempFL:    int(ct.BrakesTemperature[2]),
			BrakeTempFR:    int(ct.BrakesTemperature[3]),
			TyreSurfTempRL: int(ct.TyresSurfaceTemperature[0]),
			TyreSurfTempRR: int(ct.TyresSurfaceTemperature[1]),
			TyreSurfTempFL: int(ct.TyresSurfaceTemperature[2]),
			TyreSurfTempFR: int(ct.TyresSurfaceTemperature[3]),
			TyreInnerTempRL: int(ct.TyresInnerTemperature[0]),
			TyreInnerTempRR: int(ct.TyresInnerTemperature[1]),
			TyreInnerTempFL: int(ct.TyresInnerTemperature[2]),
			TyreInnerTempFR: int(ct.TyresInnerTemperature[3]),
			EngineTemp:     int(ct.EngineTemperature),
			TyrePressureRL: float64(ct.TyresPressure[0]),
			TyrePressureRR: float64(ct.TyresPressure[1]),
			TyrePressureFL: float64(ct.TyresPressure[2]),
			TyrePressureFR: float64(ct.TyresPressure[3]),
			DRS:            int(ct.DRS),
			Clutch:         int(ct.Clutch),
			SuggestedGear:  int(telPkt.SuggestedGear),
		})

		// Also write to the legacy telemetry table.
		store.Buffers().Telemetry.Add(storage.TelemetryRow{
			Speed:    float64(ct.Speed),
			Gear:     int(ct.Gear),
			Throttle: float64(ct.Throttle),
			Brake:    float64(ct.Brake),
			Steering: float64(ct.Steer),
			RPM:      int(ct.EngineRPM),
			WearFL:   float64(state.TyresWear[2]),
			WearFR:   float64(state.TyresWear[3]),
			WearRL:   float64(state.TyresWear[0]),
			WearRR:   float64(state.TyresWear[1]),
			Lap:      int(state.CurrentLap),
			TrackPos: float64(state.LapDistance),
			Sector:   int(state.Sector),
		})
	}
}

func handleCarStatus(state *models.RaceState, data []byte, store *storage.Storage, counters map[uint8]int, rate int) {
	pkt, err := packets.Parse(7, data)
	if err != nil {
		return
	}
	statusPkt := pkt.(*packets.PacketCarStatusData)
	idx := int(statusPkt.Header.PlayerCarIndex)
	if idx >= 22 {
		return
	}
	s := statusPkt.CarStatusData[idx]

	state.FuelMix = s.FuelMix
	state.FuelInTank = s.FuelInTank
	state.FuelRemainingLaps = s.FuelRemainingLaps
	state.ERSStoreEnergy = s.ERSStoreEnergy
	state.ERSDeployMode = s.ERSDeployMode
	state.ERSHarvestedMGUK = s.ERSHarvestedThisLapMGUK
	state.ERSHarvestedMGUH = s.ERSHarvestedThisLapMGUH
	state.ERSDeployedLap = s.ERSDeployedThisLap
	state.ActualCompound = s.ActualTyreCompound
	state.VisualCompound = s.VisualTyreCompound
	state.TyresAgeLaps = s.TyresAgeLaps
	state.DRSAllowed = s.DRSAllowed
	state.VehicleFIAFlags = s.VehicleFIAFlags

	if shouldSample(counters, 7, rate) {
		store.Buffers().CarStatus.Add(storage.CarStatusRow{
			CarIndex:     idx,
			FuelMix:      int(s.FuelMix),
			FuelInTank:   float64(s.FuelInTank),
			FuelRemLaps:  float64(s.FuelRemainingLaps),
			ERSStore:     float64(s.ERSStoreEnergy),
			ERSMode:      int(s.ERSDeployMode),
			ERSMguk:      float64(s.ERSHarvestedThisLapMGUK),
			ERSMguh:      float64(s.ERSHarvestedThisLapMGUH),
			ERSDeployed:  float64(s.ERSDeployedThisLap),
			ActualComp:   int(s.ActualTyreCompound),
			VisualComp:   int(s.VisualTyreCompound),
			TyresAge:     int(s.TyresAgeLaps),
			DRSAllowed:   int(s.DRSAllowed),
			FIAFlags:     int(s.VehicleFIAFlags),
			PowerICE:     float64(s.EnginePowerICE),
			PowerMGUK:    float64(s.EnginePowerMGUK),
		})
	}
}

func handleCarDamage(state *models.RaceState, data []byte, store *storage.Storage) {
	pkt, err := packets.Parse(10, data)
	if err != nil {
		return
	}
	dmgPkt := pkt.(*packets.PacketCarDamageData)
	idx := int(dmgPkt.Header.PlayerCarIndex)
	if idx >= 22 {
		return
	}
	d := dmgPkt.CarDamageData[idx]

	state.TyresWear = d.TyresWear
	state.TyresDamage = d.TyresDamage
	state.BrakesDmg = d.BrakesDamage
	state.FrontLeftWingDmg = d.FrontLeftWingDamage
	state.FrontRightWingDmg = d.FrontRightWingDamage
	state.RearWingDmg = d.RearWingDamage
	state.FloorDmg = d.FloorDamage
	state.DiffuserDmg = d.DiffuserDamage
	state.SidepodDmg = d.SidepodDamage
	state.GearBoxDmg = d.GearBoxDamage
	state.EngineDmg = d.EngineDamage
	state.DRSFault = d.DRSFault
	state.ERSFault = d.ERSFault

	store.Buffers().CarDamage.Add(storage.CarDamageRow{
		CarIndex:  idx,
		WearRL:    float64(d.TyresWear[0]),
		WearRR:    float64(d.TyresWear[1]),
		WearFL:    float64(d.TyresWear[2]),
		WearFR:    float64(d.TyresWear[3]),
		DmgRL:     int(d.TyresDamage[0]),
		DmgRR:     int(d.TyresDamage[1]),
		DmgFL:     int(d.TyresDamage[2]),
		DmgFR:     int(d.TyresDamage[3]),
		FLWing:    int(d.FrontLeftWingDamage),
		FRWing:    int(d.FrontRightWingDamage),
		RearWing:  int(d.RearWingDamage),
		Floor:     int(d.FloorDamage),
		Diffuser:  int(d.DiffuserDamage),
		Sidepod:   int(d.SidepodDamage),
		Gearbox:   int(d.GearBoxDamage),
		Engine:    int(d.EngineDamage),
		MGUHWear:  int(d.EngineMGUHWear),
		ESWear:    int(d.EngineESWear),
		CEWear:    int(d.EngineCEWear),
		ICEWear:   int(d.EngineICEWear),
		MGUKWear:  int(d.EngineMGUKWear),
		TCWear:    int(d.EngineTCWear),
		DRSFault:  int(d.DRSFault),
		ERSFault:  int(d.ERSFault),
	})
}

// checkPTT detects push-to-talk button press/release from F1 BUTN events.
// It compares the PTT bitmask against the current and previous ButtonStatus
// and sends a transition (true=pressed, false=released) to pttChan.
func checkPTT(data []byte, pttButton uint32, prevStatus *uint32, pttActive *bool, pttChan chan<- bool) {
	pkt, err := packets.ParseEventData(data)
	if err != nil {
		return
	}
	code := pkt.GetEventCode()
	if code != packets.EventButtons {
		return
	}

	detail, err := packets.ParseEventDetails(code, pkt.EventDetails)
	if err != nil || detail == nil {
		return
	}

	btn, ok := detail.(*packets.ButtonsEvent)
	if !ok {
		return
	}

	nowPressed := btn.ButtonStatus&pttButton != 0
	wasPressed := *prevStatus&pttButton != 0
	*prevStatus = btn.ButtonStatus

	if nowPressed && !wasPressed && !*pttActive {
		*pttActive = true
		log.Info().Uint32("button_status", btn.ButtonStatus).Msg("PTT button pressed")
		select {
		case pttChan <- true:
		default:
		}
	} else if !nowPressed && wasPressed && *pttActive {
		*pttActive = false
		log.Info().Uint32("button_status", btn.ButtonStatus).Msg("PTT button released")
		select {
		case pttChan <- false:
		default:
		}
	}
}

func handleSessionHistory(data []byte, store *storage.Storage) {
	pkt, err := packets.Parse(11, data)
	if err != nil {
		return
	}
	histPkt := pkt.(*packets.PacketSessionHistoryData)

	for i := 0; i < int(histPkt.NumLaps) && i < 100; i++ {
		lh := histPkt.LapHistoryData[i]
		if lh.LapTimeInMs == 0 {
			continue
		}
		// Session history is idempotent — the game resends the full history.
		// The DuckDB table has no unique constraint so we'll get duplicates,
		// but for analytics this is acceptable. A production system would
		// use UPSERT or dedup on read.
		_ = storage.SessionHistoryRow{
			CarIndex:  int(histPkt.CarIdx),
			LapNum:    i + 1,
			LapTimeMs: int(lh.LapTimeInMs),
			S1Ms:      int(lh.Sector1TimeInMs),
			S2Ms:      int(lh.Sector2TimeInMs),
			S3Ms:      int(lh.Sector3TimeInMs),
			LapValid:  int(lh.LapValidBitFlags & 0x01),
		}
	}
}
