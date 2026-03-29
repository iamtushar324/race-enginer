package ingestion

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/config"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/packets"
)

// ---------------------------------------------------------------------------
// Track profile: a simplified ~5300 m circuit with 3 sectors.
// Each segment has a target speed range and a sector number.
// ---------------------------------------------------------------------------

type trackSegment struct {
	endDist  float32 // cumulative metres where segment ends
	minSpeed float32 // km/h floor (corner)
	maxSpeed float32 // km/h ceiling (straight)
	sector   uint8   // 0, 1, 2
	drsZone  bool    // DRS deployment zone
	ersMode  string  // "harvest" or "deploy"
}

var trackProfile = []trackSegment{
	// Sector 0 — medium speed, slight harvesting
	{endDist: 600, minSpeed: 160, maxSpeed: 280, sector: 0, drsZone: false, ersMode: "harvest"},
	{endDist: 900, minSpeed: 80, maxSpeed: 140, sector: 0, drsZone: false, ersMode: "harvest"}, // hairpin
	{endDist: 1500, minSpeed: 200, maxSpeed: 310, sector: 0, drsZone: true, ersMode: "deploy"},  // back straight
	// Sector 1 — technical section
	{endDist: 1800, minSpeed: 100, maxSpeed: 170, sector: 1, drsZone: false, ersMode: "harvest"},
	{endDist: 2200, minSpeed: 140, maxSpeed: 220, sector: 1, drsZone: false, ersMode: "harvest"},
	{endDist: 2800, minSpeed: 180, maxSpeed: 290, sector: 1, drsZone: false, ersMode: "deploy"},
	{endDist: 3200, minSpeed: 90, maxSpeed: 150, sector: 1, drsZone: false, ersMode: "harvest"}, // chicane
	// Sector 2 — high speed run to finish
	{endDist: 3800, minSpeed: 200, maxSpeed: 320, sector: 2, drsZone: true, ersMode: "deploy"},
	{endDist: 4400, minSpeed: 120, maxSpeed: 200, sector: 2, drsZone: false, ersMode: "harvest"},
	{endDist: 5000, minSpeed: 220, maxSpeed: 310, sector: 2, drsZone: true, ersMode: "deploy"},
	{endDist: 5300, minSpeed: 150, maxSpeed: 260, sector: 2, drsZone: false, ersMode: "harvest"}, // final corner
}

const trackLength float32 = 5300.0

// ---------------------------------------------------------------------------
// Per-car physics state.
// ---------------------------------------------------------------------------

type carState struct {
	distance     float32    // metres into the current lap
	lap          uint8      // current lap number
	speed        float32    // km/h
	tyreWear     [4]float32 // RL, RR, FL, FR — percentage 0-100
	tyreAge      uint8      // laps on current tyre set
	fuel         float32    // kg remaining
	ers          float32    // Joules (0..4_000_000)
	pitStops     uint8
	lastLapMs    uint32
	sector1Ms    uint16
	sector2Ms    uint16
	sectorStart  time.Time // when the current sector began
	lapStart     time.Time // when the current lap began
	position     uint8
	wingDmgFL    uint8
	wingDmgFR    uint8
	rearWingDmg  uint8
	floorDmg     uint8
	gearboxDmg   uint8
	engineDmg    uint8
	pitting      bool
	pitTimer     time.Time
}

// ---------------------------------------------------------------------------
// MockGenerator produces realistic multi-packet F1 25 telemetry.
// ---------------------------------------------------------------------------

// MockGenerator generates synthetic F1 25 telemetry at correct frequencies.
type MockGenerator struct {
	cfg        *config.Config
	sessionUID uint64
	frame      uint32
	startTime  time.Time
	totalLaps  uint8

	cars    [22]carState
	weather mockWeather

	// Completed lap history for the player car.
	lapHistory []lapRecord
}

type mockWeather struct {
	weather     uint8 // 0=clear, 1=light cloud, 2=overcast, 3=light rain, 4=heavy rain, 5=storm
	trackTemp   int8
	airTemp     int8
	rainPct     uint8
	nextDriftAt time.Time
}

type lapRecord struct {
	lapTimeMs uint32
	s1Ms      uint16
	s2Ms      uint16
	s3Ms      uint16
}

// Run is the main loop. It emits binary packets into packetChan until ctx
// is cancelled or mockEnabled returns false.
func (mg *MockGenerator) Run(ctx context.Context, cfg *config.Config, packetChan chan<- []byte, packetsRx *atomic.Uint64, mockEnabled func() bool) {
	mg.cfg = cfg
	mg.init()

	log.Info().Msg("Enhanced mock generator running (Motion 20Hz / Telemetry 20Hz / Lap 10Hz / Status 10Hz / Damage 2Hz / Session 2s / History 5s)")

	// Tickers for each frequency tier.
	tick20Hz := time.NewTicker(50 * time.Millisecond)  // Motion + CarTelemetry
	tick10Hz := time.NewTicker(100 * time.Millisecond)  // LapData + CarStatus
	tick2Hz := time.NewTicker(500 * time.Millisecond)   // CarDamage
	tickSession := time.NewTicker(2 * time.Second)       // Session
	tickHistory := time.NewTicker(5 * time.Second)       // SessionHistory
	tickEvent := time.NewTicker(15 * time.Second)        // Periodic event triggers

	defer tick20Hz.Stop()
	defer tick10Hz.Stop()
	defer tick2Hz.Stop()
	defer tickSession.Stop()
	defer tickHistory.Stop()
	defer tickEvent.Stop()

	// Send a lights-out event at start.
	mg.emit(packetChan, packetsRx, mg.buildEvent("LGOT", 0))

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick20Hz.C:
			if !mockEnabled() {
				log.Info().Msg("Mock mode disabled, stopping enhanced mock generator")
				return
			}
			mg.stepPhysics(50 * time.Millisecond)
			mg.emit(packetChan, packetsRx, mg.buildMotion())
			mg.emit(packetChan, packetsRx, mg.buildCarTelemetry())
		case <-tick10Hz.C:
			mg.emit(packetChan, packetsRx, mg.buildLapData())
			mg.emit(packetChan, packetsRx, mg.buildCarStatus())
		case <-tick2Hz.C:
			mg.emit(packetChan, packetsRx, mg.buildCarDamage())
		case <-tickSession.C:
			mg.driftWeather()
			mg.emit(packetChan, packetsRx, mg.buildSession())
		case <-tickHistory.C:
			mg.emit(packetChan, packetsRx, mg.buildSessionHistory())
		case <-tickEvent.C:
			if pkt := mg.maybeEvent(); pkt != nil {
				mg.emit(packetChan, packetsRx, pkt)
			}
		}
	}
}

func (mg *MockGenerator) emit(ch chan<- []byte, rx *atomic.Uint64, pkt []byte) {
	rx.Add(1)
	select {
	case ch <- pkt:
	default:
	}
}

// ---------------------------------------------------------------------------
// Initialisation.
// ---------------------------------------------------------------------------

func (mg *MockGenerator) init() {
	mg.sessionUID = rand.Uint64()
	mg.startTime = time.Now()
	mg.totalLaps = 58

	mg.weather = mockWeather{
		weather:   0,
		trackTemp: 42,
		airTemp:   28,
		rainPct:   0,
	}

	now := time.Now()
	for i := 0; i < 22; i++ {
		mg.cars[i] = carState{
			distance:    float32(i) * 30, // stagger cars on the grid
			lap:         1,
			speed:       220 + float32(rand.Intn(40)),
			fuel:        100.0,
			ers:         2_000_000, // start at 50% ERS
			position:    uint8(i + 1),
			tyreAge:     0,
			sectorStart: now,
			lapStart:    now,
		}
	}
}

// ---------------------------------------------------------------------------
// Physics step — advance all 22 cars.
// ---------------------------------------------------------------------------

func (mg *MockGenerator) stepPhysics(dt time.Duration) {
	mg.frame++
	for i := range mg.cars {
		mg.stepCar(i, dt)
	}
}

func (mg *MockGenerator) stepCar(idx int, dt time.Duration) {
	c := &mg.cars[idx]
	now := time.Now()

	// If the car is in the pit lane, handle pit stop timing.
	if c.pitting {
		if now.Sub(c.pitTimer) > 25*time.Second {
			// Pit stop complete.
			c.pitting = false
			c.tyreWear = [4]float32{0, 0, 0, 0}
			c.tyreAge = 0
			c.pitStops++
		}
		return
	}

	// Find the track segment for the current distance.
	seg := mg.segmentAt(c.distance)

	// Target speed with some noise; AI cars (idx > 0) get extra jitter.
	target := seg.minSpeed + (seg.maxSpeed-seg.minSpeed)*0.6
	if idx > 0 {
		target += float32(rand.Intn(20)) - 10
	} else {
		// Player car: smoother, slightly faster
		target = seg.minSpeed + (seg.maxSpeed-seg.minSpeed)*0.65
		target += float32(rand.Intn(8)) - 4
	}

	// Smoothly approach target speed.
	diff := target - c.speed
	rate := float32(dt.Seconds()) * 3.0 // converge in ~0.33s
	if rate > 1 {
		rate = 1
	}
	c.speed += diff * rate

	// Clamp speed.
	if c.speed < 60 {
		c.speed = 60
	}
	if c.speed > 340 {
		c.speed = 340
	}

	// Advance distance: speed is km/h, convert to m/s.
	advanceM := c.speed / 3.6 * float32(dt.Seconds())
	c.distance += advanceM

	// ERS cycling.
	if seg.ersMode == "deploy" {
		c.ers -= 40000 * float32(dt.Seconds()) // deploy
		if c.ers < 400_000 {
			c.ers = 400_000
		}
	} else {
		c.ers += 30000 * float32(dt.Seconds()) // harvest
		if c.ers > 3_600_000 {
			c.ers = 3_600_000
		}
	}

	// Tyre wear: ~0.5% per lap for fronts, ~0.6% for rears, scaled by speed.
	overrides := mg.cfg.GetMockOverrides()
	wearFactor := (float32(dt.Seconds()) / 90.0) * overrides.TireWearMultiplier
	speedFactor := c.speed / 250.0
	c.tyreWear[0] += 0.60 * wearFactor * speedFactor // RL
	c.tyreWear[1] += 0.60 * wearFactor * speedFactor // RR
	c.tyreWear[2] += 0.50 * wearFactor * speedFactor // FL
	c.tyreWear[3] += 0.50 * wearFactor * speedFactor // FR

	// Fuel burn: ~1.7 kg per lap.
	c.fuel -= (1.7 * float32(dt.Seconds()) / 90.0) * overrides.FuelBurnMultiplier
	if c.fuel < 0 {
		c.fuel = 0
	}

	// Sector and lap transitions.
	prevSector := mg.sectorAt(c.distance)
	if c.distance >= float64ToF32(trackLength) {
		// Lap complete.
		lapTimeMs := uint32(now.Sub(c.lapStart).Milliseconds())
		if c.pitting {
			lapTimeMs += 25000 // pit stop penalty
		}

		// Record sector 3 timing from the previous sector start.
		if idx == 0 {
			s3Ms := uint16(now.Sub(c.sectorStart).Milliseconds())
			s1 := c.sector1Ms
			s2 := c.sector2Ms
			mg.lapHistory = append(mg.lapHistory, lapRecord{
				lapTimeMs: lapTimeMs,
				s1Ms:      s1,
				s2Ms:      s2,
				s3Ms:      s3Ms,
			})
		}

		c.lastLapMs = lapTimeMs
		c.distance -= trackLength
		c.lap++
		c.tyreAge++
		c.lapStart = now
		c.sectorStart = now
		c.sector1Ms = 0
		c.sector2Ms = 0

		// Auto-pit when tyres are worn (player car around 40%, AI slightly randomised).
		pitThreshold := float32(40.0)
		if idx > 0 {
			pitThreshold = 35.0 + float32(rand.Intn(15))
		}
		maxWear := max4f(c.tyreWear[0], c.tyreWear[1], c.tyreWear[2], c.tyreWear[3])
		if maxWear >= pitThreshold && c.pitStops < 3 {
			c.pitting = true
			c.pitTimer = now
		}

		// Random wing damage every ~12 laps (rare).
		if idx == 0 && rand.Intn(12) == 0 {
			c.wingDmgFL = uint8(min(int(c.wingDmgFL)+rand.Intn(8)+1, 50))
		}
	} else {
		newSector := mg.sectorAt(c.distance)
		if newSector != prevSector {
			elapsed := uint16(now.Sub(c.sectorStart).Milliseconds())
			switch prevSector {
			case 0:
				c.sector1Ms = elapsed
			case 1:
				c.sector2Ms = elapsed
			}
			c.sectorStart = now
		}
	}
}

func (mg *MockGenerator) segmentAt(dist float32) trackSegment {
	for _, s := range trackProfile {
		if dist < s.endDist {
			return s
		}
	}
	return trackProfile[len(trackProfile)-1]
}

func (mg *MockGenerator) sectorAt(dist float32) uint8 {
	return mg.segmentAt(dist).sector
}

// ---------------------------------------------------------------------------
// Weather drift.
// ---------------------------------------------------------------------------

func (mg *MockGenerator) driftWeather() {
	overrides := mg.cfg.GetMockOverrides()

	if overrides.WeatherOverride != nil {
		mg.weather.weather = uint8(*overrides.WeatherOverride)
	}
	if overrides.RainPercentage != nil {
		mg.weather.rainPct = uint8(*overrides.RainPercentage)
	}

	now := time.Now()
	if now.Before(mg.weather.nextDriftAt) {
		return
	}
	mg.weather.nextDriftAt = now.Add(time.Duration(10+rand.Intn(20)) * time.Second)

	// Temperature drift.
	mg.weather.trackTemp += int8(rand.Intn(3) - 1)
	if mg.weather.trackTemp < 25 {
		mg.weather.trackTemp = 25
	}
	if mg.weather.trackTemp > 55 {
		mg.weather.trackTemp = 55
	}
	mg.weather.airTemp += int8(rand.Intn(3) - 1)
	if mg.weather.airTemp < 15 {
		mg.weather.airTemp = 15
	}
	if mg.weather.airTemp > 38 {
		mg.weather.airTemp = 38
	}

	// Rain probability drift.
	mg.weather.rainPct = uint8(clampInt(int(mg.weather.rainPct)+rand.Intn(11)-5, 0, 60))

	// If rain probability crosses a threshold, switch weather state.
	if mg.weather.rainPct > 40 && mg.weather.weather < 3 {
		mg.weather.weather = 3 // light rain
	} else if mg.weather.rainPct < 15 && mg.weather.weather >= 3 {
		mg.weather.weather = 1 // clearing
	}
}

// ---------------------------------------------------------------------------
// Event generation.
// ---------------------------------------------------------------------------

func (mg *MockGenerator) maybeEvent() []byte {
	r := rand.Intn(100)
	switch {
	case r < 20:
		// Fastest lap event for the player.
		return mg.buildEvent("FTLP", 0)
	case r < 30:
		// Overtake between two random AI cars.
		a := uint8(1 + rand.Intn(21))
		return mg.buildEvent("OVTK", a)
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Packet builders. Each returns a []byte that the parser can decode.
// We use encoding/binary.Write with LittleEndian on the exact structs
// from the packets package — this guarantees offset correctness.
// ---------------------------------------------------------------------------

func (mg *MockGenerator) header(packetID uint8) packets.PacketHeader {
	return packets.PacketHeader{
		PacketFormat:            2025,
		GameYear:                25,
		GameMajorVersion:        1,
		GameMinorVersion:        0,
		PacketVersion:           1,
		PacketID:                packetID,
		SessionUID:              mg.sessionUID,
		SessionTime:             float32(time.Since(mg.startTime).Seconds()),
		FrameIdentifier:         mg.frame,
		OverallFrameID:          mg.frame,
		PlayerCarIndex:          0,
		SecondaryPlayerCarIndex: 255,
	}
}

func (mg *MockGenerator) buildMotion() []byte {
	pkt := packets.PacketMotionData{
		Header: mg.header(0),
	}
	for i := 0; i < 22; i++ {
		c := &mg.cars[i]
		seg := mg.segmentAt(c.distance)

		// Crude world position: lay the track along a rough oval.
		angle := float64(c.distance) / float64(trackLength) * 2 * math.Pi
		pkt.CarMotionData[i] = packets.CarMotionData{
			WorldPositionX:   float32(math.Cos(angle)) * 500,
			WorldPositionY:   2.0, // height above sea level
			WorldPositionZ:   float32(math.Sin(angle)) * 500,
			WorldVelocityX:   c.speed / 3.6 * float32(-math.Sin(angle)),
			WorldVelocityY:   0,
			WorldVelocityZ:   c.speed / 3.6 * float32(math.Cos(angle)),
			GForceLateral:    randF32(-2.5, 2.5),
			GForceLongitudinal: gForceForSeg(seg),
			GForceVertical:   randF32(-0.3, 0.3),
			Yaw:              float32(angle),
			Pitch:            randF32(-0.05, 0.05),
			Roll:             randF32(-0.1, 0.1),
		}
	}
	return writePacket(&pkt)
}

func (mg *MockGenerator) buildSession() []byte {
	pkt := packets.PacketSessionData{
		Header:           mg.header(1),
		Weather:          mg.weather.weather,
		TrackTemperature: mg.weather.trackTemp,
		AirTemperature:   mg.weather.airTemp,
		TotalLaps:        mg.totalLaps,
		TrackLength:      uint16(trackLength),
		SessionType:      10, // Race
		TrackID:          0,  // Melbourne
		Formula:          0,  // F1 Modern
		SessionTimeLeft:  3600,
		SessionDuration:  7200,
		PitSpeedLimit:    80,
		NumMarshalZones:  3,
		SafetyCarStatus:  0,
		NumWeatherForecastSamples: 5,
		ForecastAccuracy: 80,
		AIDifficulty:     95,
		PitStopWindowIdealLap:  15,
		PitStopWindowLatestLap: 25,
		Sector2LapDistanceStart: 1800,
		Sector3LapDistanceStart: 3200,
	}

	// Marshal zones.
	pkt.MarshalZones[0] = packets.MarshalZone{ZoneStart: 0.0, ZoneFlag: 1}
	pkt.MarshalZones[1] = packets.MarshalZone{ZoneStart: 0.34, ZoneFlag: 1}
	pkt.MarshalZones[2] = packets.MarshalZone{ZoneStart: 0.60, ZoneFlag: 1}

	// Weather forecast samples.
	for i := 0; i < 5; i++ {
		rainDrift := int(mg.weather.rainPct) + rand.Intn(10)*i
		if rainDrift > 100 {
			rainDrift = 100
		}
		pkt.WeatherForecastSamples[i] = packets.WeatherForecastSample{
			SessionType:    10,
			TimeOffset:     uint8(i * 5), // 0, 5, 10, 15, 20 minutes ahead
			Weather:        mg.weather.weather,
			TrackTemp:      mg.weather.trackTemp + int8(i),
			TrackTempChange: 0,
			AirTemp:        mg.weather.airTemp,
			AirTempChange:  0,
			RainPercentage: uint8(rainDrift),
		}
	}

	return writePacket(&pkt)
}

func (mg *MockGenerator) buildLapData() []byte {
	pkt := packets.PacketLapData{
		Header: mg.header(2),
	}
	for i := 0; i < 22; i++ {
		c := &mg.cars[i]
		sector := mg.sectorAt(c.distance)
		elapsed := uint32(time.Since(c.lapStart).Milliseconds())

		pitStatus := uint8(0)
		if c.pitting {
			pitStatus = 1
		}
		driverStatus := uint8(1) // on track
		if c.pitting {
			driverStatus = 2 // in pit area
		}

		// Compute delta to the car in front.
		var deltaFrontMs uint16
		if i > 0 {
			ahead := &mg.cars[i-1]
			gapDist := ahead.distance - c.distance
			if gapDist < 0 {
				gapDist += trackLength
			}
			// Convert distance gap to time gap at current speed.
			if c.speed > 10 {
				deltaFrontMs = uint16(float32(gapDist) / (c.speed / 3.6) * 1000)
			}
		}

		pkt.LapData[i] = packets.LapData{
			LastLapTimeInMs:            c.lastLapMs,
			CurrentLapTimeInMs:         elapsed,
			Sector1TimeInMs:            c.sector1Ms,
			Sector2TimeInMs:            c.sector2Ms,
			DeltaToCarInFrontMsPart:    deltaFrontMs,
			DeltaToRaceLeaderMsPart:    deltaFrontMs * uint16(i), // rough approximation
			LapDistance:                c.distance,
			TotalDistance:              float32(c.lap-1)*trackLength + c.distance,
			CarPosition:               c.position,
			CurrentLapNum:             c.lap,
			PitStatus:                 pitStatus,
			NumPitStops:               c.pitStops,
			Sector:                    sector,
			DriverStatus:              driverStatus,
			ResultStatus:              2, // active
			GridPosition:              uint8(i + 1),
			SpeedTrapFastestSpeed:     c.speed + float32(rand.Intn(20)),
		}
	}
	return writePacket(&pkt)
}

func (mg *MockGenerator) buildEvent(code string, vehicleIdx uint8) []byte {
	pkt := packets.PacketEventData{
		Header: mg.header(3),
	}
	copy(pkt.EventStringCode[:], code)

	// Write vehicle index into the event detail union.
	pkt.EventDetails[0] = vehicleIdx

	// For FTLP, write a plausible lap time at offset 1.
	if code == "FTLP" {
		lapTime := float32(85.0 + rand.Float64()*5.0) // ~85-90s
		bits := math.Float32bits(lapTime)
		pkt.EventDetails[1] = byte(bits)
		pkt.EventDetails[2] = byte(bits >> 8)
		pkt.EventDetails[3] = byte(bits >> 16)
		pkt.EventDetails[4] = byte(bits >> 24)
	}

	return writePacket(&pkt)
}

func (mg *MockGenerator) buildCarTelemetry() []byte {
	pkt := packets.PacketCarTelemetryData{
		Header:        mg.header(6),
		SuggestedGear: 0,
	}
	for i := 0; i < 22; i++ {
		c := &mg.cars[i]
		seg := mg.segmentAt(c.distance)

		throttle := float32(0.85) + randF32(-0.15, 0.15)
		brake := float32(0.0)
		if c.speed > seg.maxSpeed*0.9 {
			throttle = 0.2
			brake = 0.6 + randF32(0, 0.3)
		}
		gear := speedToGear(c.speed)
		rpm := speedToRPM(c.speed, gear)

		drs := uint8(0)
		if seg.drsZone && c.speed > 200 {
			drs = 1
		}

		// Tyre temps correlate with wear and speed.
		overrides := mg.cfg.GetMockOverrides()
		baseSurf := uint8(90) + uint8(c.speed/40) + uint8(overrides.TireTempOffset)
		baseInner := baseSurf + 5

		// Brake temps: high during braking.
		brakeBase := uint16(300) + uint16(c.speed/2)
		if brake > 0.3 {
			brakeBase += 200
		}

		pkt.CarTelemetryData[i] = packets.CarTelemetryData{
			Speed:             uint16(c.speed),
			Throttle:          clampF32(throttle, 0, 1),
			Steer:             randF32(-0.3, 0.3),
			Brake:             clampF32(brake, 0, 1),
			Clutch:            0,
			Gear:              int8(gear),
			EngineRPM:         rpm,
			DRS:               drs,
			RevLightsPercent:  uint8(float32(rpm) / 12000 * 100),
			BrakesTemperature: [4]uint16{
				brakeBase + uint16(rand.Intn(40)),
				brakeBase + uint16(rand.Intn(40)),
				brakeBase + uint16(rand.Intn(30)),
				brakeBase + uint16(rand.Intn(30)),
			},
			TyresSurfaceTemperature: [4]uint8{
				baseSurf + uint8(c.tyreWear[0]/5),
				baseSurf + uint8(c.tyreWear[1]/5),
				baseSurf + uint8(c.tyreWear[2]/5),
				baseSurf + uint8(c.tyreWear[3]/5),
			},
			TyresInnerTemperature: [4]uint8{
				baseInner + uint8(c.tyreWear[0]/4),
				baseInner + uint8(c.tyreWear[1]/4),
				baseInner + uint8(c.tyreWear[2]/4),
				baseInner + uint8(c.tyreWear[3]/4),
			},
			EngineTemperature: uint16(105 + rand.Intn(10)),
			TyresPressure: [4]float32{
				22.5 + randF32(-0.5, 0.5) + c.tyreWear[0]*0.02,
				22.5 + randF32(-0.5, 0.5) + c.tyreWear[1]*0.02,
				23.0 + randF32(-0.5, 0.5) + c.tyreWear[2]*0.02,
				23.0 + randF32(-0.5, 0.5) + c.tyreWear[3]*0.02,
			},
		}

		// Suggested gear for the player car.
		if i == 0 {
			pkt.SuggestedGear = int8(gear)
		}
	}
	return writePacket(&pkt)
}

func (mg *MockGenerator) buildCarStatus() []byte {
	pkt := packets.PacketCarStatusData{
		Header: mg.header(7),
	}
	for i := 0; i < 22; i++ {
		c := &mg.cars[i]
		seg := mg.segmentAt(c.distance)

		drsAllowed := uint8(0)
		if seg.drsZone {
			drsAllowed = 1
		}

		ersMode := uint8(1) // medium
		if seg.ersMode == "deploy" {
			ersMode = 3 // overtake
		}

		// Fuel remaining laps estimate.
		fuelPerLap := float32(1.7)
		fuelRemLaps := float32(0)
		if fuelPerLap > 0 {
			fuelRemLaps = c.fuel / fuelPerLap
		}

		pkt.CarStatusData[i] = packets.CarStatusData{
			TractionControl:    0,
			AntiLockBrakes:     0,
			FuelMix:            1, // standard
			FrontBrakeBias:     56,
			FuelInTank:         c.fuel,
			FuelCapacity:       110.0,
			FuelRemainingLaps:  fuelRemLaps,
			MaxRPM:             12000,
			IdleRPM:            4000,
			MaxGears:           8,
			DRSAllowed:         drsAllowed,
			DRSActivationDistance: 0,
			ActualTyreCompound:  16, // C3 soft
			VisualTyreCompound:  16,
			TyresAgeLaps:        c.tyreAge,
			VehicleFIAFlags:     0,
			EnginePowerICE:      750.0,
			EnginePowerMGUK:     120.0,
			ERSStoreEnergy:      c.ers,
			ERSDeployMode:       ersMode,
			ERSHarvestedThisLapMGUK: c.ers * 0.1,
			ERSHarvestedThisLapMGUH: c.ers * 0.15,
			ERSDeployedThisLap:      c.ers * 0.08,
		}
	}
	return writePacket(&pkt)
}

func (mg *MockGenerator) buildCarDamage() []byte {
	pkt := packets.PacketCarDamageData{
		Header: mg.header(10),
	}
	for i := 0; i < 22; i++ {
		c := &mg.cars[i]

		// Tyre damage: low-level noise proportional to wear.
		tyreDmg := func(wear float32) uint8 {
			d := uint8(wear * 0.3)
			if d > 100 {
				d = 100
			}
			return d
		}

		pkt.CarDamageData[i] = packets.CarDamageData{
			TyresWear:   c.tyreWear,
			TyresDamage: [4]uint8{
				tyreDmg(c.tyreWear[0]),
				tyreDmg(c.tyreWear[1]),
				tyreDmg(c.tyreWear[2]),
				tyreDmg(c.tyreWear[3]),
			},
			BrakesDamage:         [4]uint8{0, 0, 0, 0},
			FrontLeftWingDamage:  c.wingDmgFL,
			FrontRightWingDamage: c.wingDmgFR,
			RearWingDamage:       c.rearWingDmg,
			FloorDamage:          c.floorDmg,
			GearBoxDamage:        c.gearboxDmg,
			EngineDamage:         c.engineDmg,
			EngineMGUHWear:       uint8(clampInt(int(c.tyreAge)*2, 0, 60)),
			EngineESWear:         uint8(clampInt(int(c.tyreAge), 0, 40)),
			EngineCEWear:         uint8(clampInt(int(c.tyreAge), 0, 40)),
			EngineICEWear:        uint8(clampInt(int(c.tyreAge)*2, 0, 60)),
			EngineMGUKWear:       uint8(clampInt(int(c.tyreAge)*2, 0, 60)),
			EngineTCWear:         uint8(clampInt(int(c.tyreAge), 0, 40)),
		}
	}
	return writePacket(&pkt)
}

func (mg *MockGenerator) buildSessionHistory() []byte {
	pkt := packets.PacketSessionHistoryData{
		Header:            mg.header(11),
		CarIdx:            0, // player car
		NumLaps:           uint8(len(mg.lapHistory)),
		NumTyreStints:     1,
		BestLapTimeLapNum: 1,
		BestSector1LapNum: 1,
		BestSector2LapNum: 1,
		BestSector3LapNum: 1,
	}

	bestLap := uint32(math.MaxUint32)
	for i, rec := range mg.lapHistory {
		if i >= 100 {
			break
		}
		pkt.LapHistoryData[i] = packets.LapHistoryData{
			LapTimeInMs:     rec.lapTimeMs,
			Sector1TimeInMs: rec.s1Ms,
			Sector2TimeInMs: rec.s2Ms,
			Sector3TimeInMs: rec.s3Ms,
			LapValidBitFlags: 0x01, // valid
		}
		if rec.lapTimeMs < bestLap {
			bestLap = rec.lapTimeMs
			pkt.BestLapTimeLapNum = uint8(i + 1)
		}
	}

	// Tyre stint: one stint from lap 1 on soft.
	pkt.TyreStintHistoryData[0] = packets.TyreStintHistoryData{
		EndLap:             mg.cars[0].lap,
		TyreActualCompound: 16,
		TyreVisualCompound: 16,
	}

	return writePacket(&pkt)
}

// ---------------------------------------------------------------------------
// writePacket serialises any packet struct to a []byte using binary.Write
// in LittleEndian, which is exactly what the parser reads with binary.Read.
// ---------------------------------------------------------------------------

func writePacket(v interface{}) []byte {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
		log.Error().Err(err).Msg("mock: failed to serialise packet")
		return nil
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// Helpers.
// ---------------------------------------------------------------------------

func speedToGear(speed float32) int {
	switch {
	case speed < 60:
		return 1
	case speed < 100:
		return 2
	case speed < 140:
		return 3
	case speed < 180:
		return 4
	case speed < 220:
		return 5
	case speed < 270:
		return 6
	case speed < 310:
		return 7
	default:
		return 8
	}
}

func speedToRPM(speed float32, gear int) uint16 {
	// Rough mapping: RPM proportional to speed within gear band, 4000-12000.
	gearRatios := []float32{0, 4.0, 3.2, 2.6, 2.2, 1.9, 1.7, 1.5, 1.35}
	ratio := float32(1.5)
	if gear >= 1 && gear <= 8 {
		ratio = gearRatios[gear]
	}
	rpm := speed * ratio * 10
	if rpm < 4000 {
		rpm = 4000
	}
	if rpm > 12000 {
		rpm = 12000
	}
	return uint16(rpm)
}

func gForceForSeg(seg trackSegment) float32 {
	// Negative when braking, positive when accelerating.
	midSpeed := (seg.minSpeed + seg.maxSpeed) / 2
	if seg.minSpeed < 120 {
		return randF32(-3.0, -0.5) // heavy braking zone
	}
	if midSpeed > 250 {
		return randF32(0.2, 1.5) // acceleration zone
	}
	return randF32(-0.5, 0.5)
}

func randF32(min, max float32) float32 {
	return min + rand.Float32()*(max-min)
}

func clampF32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max4f(a, b, c, d float32) float32 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	if d > m {
		m = d
	}
	return m
}

func float64ToF32(f float32) float32 { return f }
