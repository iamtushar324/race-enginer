package insights

import (
	"sync"
	"time"
)

// CooldownManager tracks per-rule cooldown state and debounce timers.
// Every field is guarded by mu so the engine goroutine and (future) reset
// callers stay race-free.
type CooldownManager struct {
	mu sync.Mutex

	// --- Per-lap flags (reset on each new lap) ---
	lastLap            uint8
	tireWarnIssued     bool
	fuelWarnIssued     bool // < 5 laps
	fuelCriticalIssued bool // < 3 laps
	ersLowWarned       bool
	brakeTempWarned    bool
	tireTempWarned     bool
	pitEntryWarned     bool

	// --- Session-scoped flags (reset on condition clear) ---
	rainWarned        bool
	safetyCarNotified bool
	damageWarned      map[string]bool // component name -> warned

	// --- Position tracking ---
	lastPosition uint8
	overtakeLap  uint8 // one notification per lap per direction

	// --- Braking zone debounce ---
	brakingInZone        bool
	brakingWarnedThisZone bool

	// --- Rate limiters ---
	lastCarStatusTime     time.Time // for fuel + ERS (1 Hz)
	lastPositionCheckTime time.Time // for position changes (1 Hz)
	lastCarTelemetryTime  time.Time // for brake/tire temp (0.5 Hz)

	// --- Event dedup ---
	lastEventCode string
}

// NewCooldownManager returns an initialised CooldownManager.
func NewCooldownManager() *CooldownManager {
	return &CooldownManager{
		damageWarned: make(map[string]bool),
	}
}

// ResetForNewLap clears every flag that should reset at the start of a new lap.
func (c *CooldownManager) ResetForNewLap() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tireWarnIssued = false
	c.brakeTempWarned = false
	c.tireTempWarned = false
	c.ersLowWarned = false
	c.pitEntryWarned = false
}

// ShouldRateLimit returns true if the caller should skip this evaluation round.
//
// Supported kinds:
//
//	"car_status"    - 1 Hz (fuel, ERS)
//	"position"      - 1 Hz
//	"car_telemetry" - 0.5 Hz (brake/tire temps)
func (c *CooldownManager) ShouldRateLimit(kind string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	switch kind {
	case "car_status":
		if now.Sub(c.lastCarStatusTime) < 1*time.Second {
			return true
		}
		c.lastCarStatusTime = now
	case "position":
		if now.Sub(c.lastPositionCheckTime) < 1*time.Second {
			return true
		}
		c.lastPositionCheckTime = now
	case "car_telemetry":
		if now.Sub(c.lastCarTelemetryTime) < 2*time.Second {
			return true
		}
		c.lastCarTelemetryTime = now
	}
	return false
}

// ---------- Lap ----------

func (c *CooldownManager) LastLap() uint8 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastLap
}

func (c *CooldownManager) SetLastLap(lap uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastLap = lap
}

// ---------- Tire Wear ----------

func (c *CooldownManager) TireWarnIssued() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tireWarnIssued
}

func (c *CooldownManager) SetTireWarnIssued(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tireWarnIssued = v
}

// ---------- Fuel ----------

func (c *CooldownManager) FuelWarnIssued() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fuelWarnIssued
}

func (c *CooldownManager) SetFuelWarnIssued(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fuelWarnIssued = v
}

func (c *CooldownManager) FuelCriticalIssued() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fuelCriticalIssued
}

func (c *CooldownManager) SetFuelCriticalIssued(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fuelCriticalIssued = v
}

// ---------- ERS ----------

func (c *CooldownManager) ERSLowWarned() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ersLowWarned
}

func (c *CooldownManager) SetERSLowWarned(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ersLowWarned = v
}

// ---------- Rain ----------

func (c *CooldownManager) RainWarned() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rainWarned
}

func (c *CooldownManager) SetRainWarned(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rainWarned = v
}

// ---------- Safety Car ----------

func (c *CooldownManager) SafetyCarNotified() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.safetyCarNotified
}

func (c *CooldownManager) SetSafetyCarNotified(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.safetyCarNotified = v
}

// ---------- Damage ----------

func (c *CooldownManager) DamageWarned(component string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.damageWarned[component]
}

func (c *CooldownManager) SetDamageWarned(component string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.damageWarned[component] = true
}

// ---------- Brake Temp ----------

func (c *CooldownManager) BrakeTempWarned() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.brakeTempWarned
}

func (c *CooldownManager) SetBrakeTempWarned(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.brakeTempWarned = v
}

// ---------- Tire Temp ----------

func (c *CooldownManager) TireTempWarned() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tireTempWarned
}

func (c *CooldownManager) SetTireTempWarned(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tireTempWarned = v
}

// ---------- Position ----------

func (c *CooldownManager) LastPosition() uint8 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastPosition
}

func (c *CooldownManager) SetLastPosition(p uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPosition = p
}

func (c *CooldownManager) OvertakeLap() uint8 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.overtakeLap
}

func (c *CooldownManager) SetOvertakeLap(lap uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.overtakeLap = lap
}

// ---------- Braking Zone ----------

func (c *CooldownManager) BrakingInZone() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.brakingInZone
}

func (c *CooldownManager) SetBrakingInZone(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.brakingInZone = v
}

func (c *CooldownManager) BrakingWarnedThisZone() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.brakingWarnedThisZone
}

func (c *CooldownManager) SetBrakingWarnedThisZone(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.brakingWarnedThisZone = v
}

// ---------- Event Dedup ----------

func (c *CooldownManager) LastEventCode() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastEventCode
}

func (c *CooldownManager) SetLastEventCode(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastEventCode = code
}

// ---------- Pit Entry ----------

func (c *CooldownManager) PitEntryWarned() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pitEntryWarned
}

func (c *CooldownManager) SetPitEntryWarned(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pitEntryWarned = v
}
