package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TestBufferFlushWritesToDB — explicit flush writes buffered rows to DuckDB.
// ---------------------------------------------------------------------------

func TestBufferFlushWritesToDB(t *testing.T) {
	store := newTestStore(t, 100) // large batch → no auto-flush
	defer store.Close()

	for i := 0; i < 5; i++ {
		store.Buffers().Telemetry.Add(TelemetryRow{
			Speed:    float64(200 + i),
			Gear:     5,
			Throttle: 0.8,
			Brake:    0.0,
			Steering: 0.1,
			RPM:      10000,
			WearFL:   10.0, WearFR: 10.0, WearRL: 10.0, WearRR: 10.0,
			Lap:      3,
			TrackPos: 1000.0,
			Sector:   1,
		})
	}

	if err := store.Buffers().Telemetry.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	count := queryCount(t, store, "telemetry")
	if count != 5 {
		t.Errorf("expected 5 rows, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestBufferAutoFlushOnCapacity — adding batchSize items triggers auto-flush.
// ---------------------------------------------------------------------------

func TestBufferAutoFlushOnCapacity(t *testing.T) {
	store := newTestStore(t, 3) // batch size 3
	defer store.Close()

	for i := 0; i < 3; i++ {
		store.Buffers().Telemetry.Add(TelemetryRow{
			Speed: float64(200 + i),
			Gear:  5,
		})
	}

	// Auto-flush should have been triggered by the 3rd Add.
	count := queryCount(t, store, "telemetry")
	if count != 3 {
		t.Errorf("expected 3 rows from auto-flush, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// TestQueryFlushesFirst — FlushAll() (called internally by Store.Query)
// moves buffered rows into DuckDB.
// ---------------------------------------------------------------------------

func TestQueryFlushesFirst(t *testing.T) {
	store := newTestStore(t, 100) // large batch → no auto-flush
	defer store.Close()

	for i := 0; i < 3; i++ {
		store.Buffers().Telemetry.Add(TelemetryRow{
			Speed: float64(200 + i),
			Gear:  5,
		})
	}

	// Buffer has items, DB does not.
	if store.Buffers().Telemetry.Len() != 3 {
		t.Fatal("expected 3 items in buffer")
	}
	if c := queryCount(t, store, "telemetry"); c != 0 {
		t.Fatalf("expected 0 rows before flush, got %d", c)
	}

	// FlushAll (same call Query() makes before reading) writes rows to DuckDB.
	if err := store.FlushAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	if store.Buffers().Telemetry.Len() != 0 {
		t.Errorf("expected buffer empty after flush, got %d", store.Buffers().Telemetry.Len())
	}
	if c := queryCount(t, store, "telemetry"); c != 3 {
		t.Errorf("expected 3 rows after flush, got %d", c)
	}
}

// ---------------------------------------------------------------------------
// TestMultiTableFlush — verify FlushAll writes to multiple tables.
// ---------------------------------------------------------------------------

func TestMultiTableFlush(t *testing.T) {
	store := newTestStore(t, 100)
	defer store.Close()

	store.Buffers().Telemetry.Add(TelemetryRow{Speed: 250, Gear: 5})
	store.Buffers().Session.Add(SessionRow{Weather: 0, TrackTemp: 30, TotalLaps: 50})
	store.Buffers().Motion.Add(MotionRow{CarIndex: 0, WorldPosX: 100, WorldPosY: 200, WorldPosZ: 300})
	store.Buffers().RaceEvents.Add(RaceEventRow{EventCode: "FTLP", VehicleIdx: 0, DetailText: "fastest_lap"})

	if err := store.FlushAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, tbl := range []string{"telemetry", "session_data", "motion_data", "race_events"} {
		if c := queryCount(t, store, tbl); c != 1 {
			t.Errorf("%s: expected 1 row, got %d", tbl, c)
		}
	}
}

// ---------------------------------------------------------------------------
// TestBufferLen — Len() tracks items before and after flush.
// ---------------------------------------------------------------------------

func TestBufferLen(t *testing.T) {
	store := newTestStore(t, 100)
	defer store.Close()

	buf := store.Buffers().Telemetry

	if buf.Len() != 0 {
		t.Errorf("expected len 0, got %d", buf.Len())
	}

	buf.Add(TelemetryRow{Speed: 200})
	buf.Add(TelemetryRow{Speed: 201})

	if buf.Len() != 2 {
		t.Errorf("expected len 2, got %d", buf.Len())
	}

	if err := buf.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	if buf.Len() != 0 {
		t.Errorf("expected len 0 after flush, got %d", buf.Len())
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestStore creates a Storage backed by a temporary DuckDB file.
func newTestStore(t *testing.T, batchSize int) *Storage {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")
	store, err := NewStorage(dbPath, batchSize)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// queryCount returns the row count for a table via the writer connection.
func queryCount(t *testing.T, store *Storage, table string) int {
	t.Helper()
	var count int
	if err := store.Writer().QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// toInt64 coerces a DuckDB count(*) result to int64.
func toInt64(t *testing.T, v interface{}) int64 {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		t.Fatalf("unexpected type for count: %T (%v)", v, v)
		return 0
	}
}
