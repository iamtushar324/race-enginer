package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// Generic batch buffer
// ---------------------------------------------------------------------------

// TableBuffer is a generic batch buffer that accumulates rows of type T and
// flushes them to DuckDB in bulk when the batch size threshold is reached.
type TableBuffer[T any] struct {
	mu        sync.Mutex
	items     []T
	batchSize int
	flushFn   func(ctx context.Context, items []T) error
	tableName string // for logging
}

// NewTableBuffer creates a new TableBuffer for the given table.
func NewTableBuffer[T any](name string, batchSize int, flushFn func(ctx context.Context, items []T) error) *TableBuffer[T] {
	return &TableBuffer[T]{
		items:     make([]T, 0, batchSize),
		batchSize: batchSize,
		flushFn:   flushFn,
		tableName: name,
	}
}

// Add appends an item to the buffer. If the buffer reaches batchSize it is
// automatically flushed in a background goroutine to avoid blocking the caller.
func (b *TableBuffer[T]) Add(item T) {
	b.mu.Lock()
	b.items = append(b.items, item)
	shouldFlush := len(b.items) >= b.batchSize
	b.mu.Unlock()

	if shouldFlush {
		go func() {
			if err := b.Flush(context.Background()); err != nil {
				log.Error().Err(err).Str("table", b.tableName).Msg("auto-flush failed")
			}
		}()
	}
}

// Flush writes all buffered items to DuckDB and resets the buffer.
func (b *TableBuffer[T]) Flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.items) == 0 {
		b.mu.Unlock()
		return nil
	}
	// Swap out the slice so we can release the lock while flushing.
	batch := b.items
	b.items = make([]T, 0, b.batchSize)
	b.mu.Unlock()

	start := time.Now()
	if err := b.flushFn(ctx, batch); err != nil {
		return fmt.Errorf("flush %s: %w", b.tableName, err)
	}
	log.Debug().
		Str("table", b.tableName).
		Int("rows", len(batch)).
		Dur("duration", time.Since(start)).
		Msg("flushed buffer")
	return nil
}

// Len returns the current number of buffered items.
func (b *TableBuffer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// ---------------------------------------------------------------------------
// Row types — one per buffered table, plus SessionHistoryRow (unbuffered).
// Field names use short forms that match the ingestion writer's usage.
// ---------------------------------------------------------------------------

// TelemetryRow matches the legacy "telemetry" table columns.
type TelemetryRow struct {
	Speed    float64
	Gear     int
	Throttle float64
	Brake    float64
	Steering float64
	RPM      int
	WearFL   float64
	WearFR   float64
	WearRL   float64
	WearRR   float64
	Lap      int
	TrackPos float64
	Sector   int
}

// SessionRow matches the "session_data" table columns.
type SessionRow struct {
	Weather      int
	TrackTemp    int
	AirTemp      int
	TotalLaps    int
	TrackLength  int
	SessionType  int
	TrackID      int
	TimeLeft     int
	SafetyCar    int
	RainPct      int
	PitIdealLap  int
	PitLatestLap int
}

// LapDataRow matches the "lap_data" table columns.
type LapDataRow struct {
	CarIndex      int
	LastLapMs     int
	CurrentLapMs  int
	Sector1Ms     int
	Sector2Ms     int
	Position      int
	LapNum        int
	PitStatus     int
	NumPitStops   int
	Sector        int
	Penalties     int
	DriverStatus  int
	ResultStatus  int
	DeltaFrontMs  int
	DeltaLeaderMs int
	SpeedTrap     float64
	GridPosition  int
}

// CarStatusRow matches the "car_status" table columns.
type CarStatusRow struct {
	CarIndex   int
	FuelMix    int
	FuelInTank float64
	FuelRemLaps float64
	ERSStore   float64
	ERSMode    int
	ERSMguk    float64
	ERSMguh    float64
	ERSDeployed float64
	ActualComp int
	VisualComp int
	TyresAge   int
	DRSAllowed int
	FIAFlags   int
	PowerICE   float64
	PowerMGUK  float64
}

// CarDamageRow matches the "car_damage" table columns.
type CarDamageRow struct {
	CarIndex int
	WearRL   float64
	WearRR   float64
	WearFL   float64
	WearFR   float64
	DmgRL    int
	DmgRR    int
	DmgFL    int
	DmgFR    int
	FLWing   int
	FRWing   int
	RearWing int
	Floor    int
	Diffuser int
	Sidepod  int
	Gearbox  int
	Engine   int
	MGUHWear int
	ESWear   int
	CEWear   int
	ICEWear  int
	MGUKWear int
	TCWear   int
	DRSFault int
	ERSFault int
}

// TelemetryExtRow matches the "car_telemetry_ext" table columns.
type TelemetryExtRow struct {
	CarIndex        int
	BrakeTempRL     int
	BrakeTempRR     int
	BrakeTempFL     int
	BrakeTempFR     int
	TyreSurfTempRL  int
	TyreSurfTempRR  int
	TyreSurfTempFL  int
	TyreSurfTempFR  int
	TyreInnerTempRL int
	TyreInnerTempRR int
	TyreInnerTempFL int
	TyreInnerTempFR int
	EngineTemp      int
	TyrePressureRL  float64
	TyrePressureRR  float64
	TyrePressureFL  float64
	TyrePressureFR  float64
	DRS             int
	Clutch          int
	SuggestedGear   int
}

// RaceEventRow matches the "race_events" table columns.
type RaceEventRow struct {
	EventCode  string
	VehicleIdx int
	DetailText string
}

// MotionRow matches the "motion_data" table columns.
type MotionRow struct {
	CarIndex  int
	WorldPosX float64
	WorldPosY float64
	WorldPosZ float64
	GForceLat float64
	GForceLon float64
	GForceVer float64
	Yaw       float64
	Pitch     float64
	Roll      float64
}

// SessionHistoryRow matches the "session_history" table columns.
// This is not buffered (written directly) but the type is used by the
// ingestion writer.
type SessionHistoryRow struct {
	CarIndex  int
	LapNum    int
	LapTimeMs int
	S1Ms      int
	S2Ms      int
	S3Ms      int
	LapValid  int
}

// ---------------------------------------------------------------------------
// BufferSet — holds all eight table buffers
// ---------------------------------------------------------------------------

// BufferSet aggregates the batch buffers for every buffered table.
type BufferSet struct {
	Telemetry    *TableBuffer[TelemetryRow]
	Session      *TableBuffer[SessionRow]
	LapData      *TableBuffer[LapDataRow]
	CarStatus    *TableBuffer[CarStatusRow]
	CarDamage    *TableBuffer[CarDamageRow]
	TelemetryExt *TableBuffer[TelemetryExtRow]
	RaceEvents   *TableBuffer[RaceEventRow]
	Motion       *TableBuffer[MotionRow]
}

// NewBufferSet creates all eight table buffers wired to the given DuckDB writer
// connection with the specified batch size.
func NewBufferSet(db *sql.DB, batchSize int) *BufferSet {
	return &BufferSet{
		Telemetry:    NewTableBuffer[TelemetryRow]("telemetry", batchSize, makeTelemetryFlusher(db)),
		Session:      NewTableBuffer[SessionRow]("session_data", batchSize, makeSessionFlusher(db)),
		LapData:      NewTableBuffer[LapDataRow]("lap_data", batchSize, makeLapDataFlusher(db)),
		CarStatus:    NewTableBuffer[CarStatusRow]("car_status", batchSize, makeCarStatusFlusher(db)),
		CarDamage:    NewTableBuffer[CarDamageRow]("car_damage", batchSize, makeCarDamageFlusher(db)),
		TelemetryExt: NewTableBuffer[TelemetryExtRow]("car_telemetry_ext", batchSize, makeTelemetryExtFlusher(db)),
		RaceEvents:   NewTableBuffer[RaceEventRow]("race_events", batchSize, makeRaceEventFlusher(db)),
		Motion:       NewTableBuffer[MotionRow]("motion_data", batchSize, makeMotionFlusher(db)),
	}
}

// FlushAll flushes every buffer. Errors are collected; the first is returned.
func (bs *BufferSet) FlushAll(ctx context.Context) error {
	var firstErr error
	collect := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	collect(bs.Telemetry.Flush(ctx))
	collect(bs.Session.Flush(ctx))
	collect(bs.LapData.Flush(ctx))
	collect(bs.CarStatus.Flush(ctx))
	collect(bs.CarDamage.Flush(ctx))
	collect(bs.TelemetryExt.Flush(ctx))
	collect(bs.RaceEvents.Flush(ctx))
	collect(bs.Motion.Flush(ctx))
	return firstErr
}

// ---------------------------------------------------------------------------
// Flush functions — one per table. Each performs a bulk INSERT inside a tx.
// ---------------------------------------------------------------------------

func makeTelemetryFlusher(db *sql.DB) func(ctx context.Context, items []TelemetryRow) error {
	return func(ctx context.Context, items []TelemetryRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO telemetry
			(speed, gear, throttle, brake, steering, engine_rpm,
			 tire_wear_fl, tire_wear_fr, tire_wear_rl, tire_wear_rr,
			 lap, track_position, sector)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.Speed, r.Gear, r.Throttle, r.Brake, r.Steering, r.RPM,
				r.WearFL, r.WearFR, r.WearRL, r.WearRR,
				r.Lap, r.TrackPos, r.Sector,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeSessionFlusher(db *sql.DB) func(ctx context.Context, items []SessionRow) error {
	return func(ctx context.Context, items []SessionRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO session_data
			(weather, track_temperature, air_temperature,
			 total_laps, track_length, session_type, track_id,
			 session_time_left, safety_car_status, rain_percentage,
			 pit_stop_window_ideal_lap, pit_stop_window_latest_lap)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.Weather, r.TrackTemp, r.AirTemp,
				r.TotalLaps, r.TrackLength, r.SessionType, r.TrackID,
				r.TimeLeft, r.SafetyCar, r.RainPct,
				r.PitIdealLap, r.PitLatestLap,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeLapDataFlusher(db *sql.DB) func(ctx context.Context, items []LapDataRow) error {
	return func(ctx context.Context, items []LapDataRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO lap_data
			(car_index, last_lap_time_in_ms, current_lap_time_in_ms,
			 sector1_time_in_ms, sector2_time_in_ms,
			 car_position, current_lap_num, pit_status,
			 num_pit_stops, sector, penalties,
			 driver_status, result_status,
			 delta_to_car_in_front_in_ms, delta_to_race_leader_in_ms,
			 speed_trap_fastest_speed, grid_position)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.CarIndex, r.LastLapMs, r.CurrentLapMs,
				r.Sector1Ms, r.Sector2Ms,
				r.Position, r.LapNum, r.PitStatus,
				r.NumPitStops, r.Sector, r.Penalties,
				r.DriverStatus, r.ResultStatus,
				r.DeltaFrontMs, r.DeltaLeaderMs,
				r.SpeedTrap, r.GridPosition,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeCarStatusFlusher(db *sql.DB) func(ctx context.Context, items []CarStatusRow) error {
	return func(ctx context.Context, items []CarStatusRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO car_status
			(car_index, fuel_mix, fuel_in_tank,
			 fuel_remaining_laps, ers_store_energy, ers_deploy_mode,
			 ers_harvested_this_lap_mguk, ers_harvested_this_lap_mguh,
			 ers_deployed_this_lap, actual_tyre_compound,
			 visual_tyre_compound, tyres_age_laps, drs_allowed,
			 vehicle_fia_flags, engine_power_ice, engine_power_mguk)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.CarIndex, r.FuelMix, r.FuelInTank,
				r.FuelRemLaps, r.ERSStore, r.ERSMode,
				r.ERSMguk, r.ERSMguh,
				r.ERSDeployed, r.ActualComp,
				r.VisualComp, r.TyresAge, r.DRSAllowed,
				r.FIAFlags, r.PowerICE, r.PowerMGUK,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeCarDamageFlusher(db *sql.DB) func(ctx context.Context, items []CarDamageRow) error {
	return func(ctx context.Context, items []CarDamageRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO car_damage
			(car_index,
			 tyres_wear_rl, tyres_wear_rr, tyres_wear_fl, tyres_wear_fr,
			 tyres_damage_rl, tyres_damage_rr, tyres_damage_fl, tyres_damage_fr,
			 front_left_wing_damage, front_right_wing_damage,
			 rear_wing_damage, floor_damage, diffuser_damage,
			 sidepod_damage, gear_box_damage, engine_damage,
			 engine_mguh_wear, engine_es_wear, engine_ce_wear,
			 engine_ice_wear, engine_mguk_wear, engine_tc_wear,
			 drs_fault, ers_fault)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.CarIndex,
				r.WearRL, r.WearRR, r.WearFL, r.WearFR,
				r.DmgRL, r.DmgRR, r.DmgFL, r.DmgFR,
				r.FLWing, r.FRWing,
				r.RearWing, r.Floor, r.Diffuser,
				r.Sidepod, r.Gearbox, r.Engine,
				r.MGUHWear, r.ESWear, r.CEWear,
				r.ICEWear, r.MGUKWear, r.TCWear,
				r.DRSFault, r.ERSFault,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeTelemetryExtFlusher(db *sql.DB) func(ctx context.Context, items []TelemetryExtRow) error {
	return func(ctx context.Context, items []TelemetryExtRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO car_telemetry_ext
			(car_index,
			 brakes_temp_rl, brakes_temp_rr, brakes_temp_fl, brakes_temp_fr,
			 tyres_surface_temp_rl, tyres_surface_temp_rr,
			 tyres_surface_temp_fl, tyres_surface_temp_fr,
			 tyres_inner_temp_rl, tyres_inner_temp_rr,
			 tyres_inner_temp_fl, tyres_inner_temp_fr,
			 engine_temperature,
			 tyres_pressure_rl, tyres_pressure_rr,
			 tyres_pressure_fl, tyres_pressure_fr,
			 drs, clutch, suggested_gear)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.CarIndex,
				r.BrakeTempRL, r.BrakeTempRR, r.BrakeTempFL, r.BrakeTempFR,
				r.TyreSurfTempRL, r.TyreSurfTempRR,
				r.TyreSurfTempFL, r.TyreSurfTempFR,
				r.TyreInnerTempRL, r.TyreInnerTempRR,
				r.TyreInnerTempFL, r.TyreInnerTempFR,
				r.EngineTemp,
				r.TyrePressureRL, r.TyrePressureRR,
				r.TyrePressureFL, r.TyrePressureFR,
				r.DRS, r.Clutch, r.SuggestedGear,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeRaceEventFlusher(db *sql.DB) func(ctx context.Context, items []RaceEventRow) error {
	return func(ctx context.Context, items []RaceEventRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO race_events
			(event_code, vehicle_idx, detail_text)
			VALUES (?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.EventCode, r.VehicleIdx, r.DetailText,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}

func makeMotionFlusher(db *sql.DB) func(ctx context.Context, items []MotionRow) error {
	return func(ctx context.Context, items []MotionRow) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO motion_data
			(car_index,
			 world_position_x, world_position_y, world_position_z,
			 g_force_lateral, g_force_longitudinal, g_force_vertical,
			 yaw, pitch, roll)
			VALUES (?,?,?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, r := range items {
			if _, err := stmt.ExecContext(ctx,
				r.CarIndex,
				r.WorldPosX, r.WorldPosY, r.WorldPosZ,
				r.GForceLat, r.GForceLon, r.GForceVer,
				r.Yaw, r.Pitch, r.Roll,
			); err != nil {
				return err
			}
		}
		return tx.Commit()
	}
}
