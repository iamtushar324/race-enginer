package ingestion

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/config"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/storage"
)

// ---------------------------------------------------------------------------
// TestMockIngestion — verifies mock mode generates data that reaches DuckDB.
// ---------------------------------------------------------------------------

func TestMockIngestion(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")

	store, err := storage.NewStorage(dbPath, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{
		UDPHost:    "127.0.0.1",
		UDPPort:    0,
		BatchSize:  5,
		SampleRate: 1, // every packet written
	}
	cfg.MockMode.Store(true)

	ingester := NewIngester(cfg, store)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ingester.Start(ctx, &wg)

	// Mock runs at 20Hz — 2s gives ~40 packets.
	time.Sleep(2 * time.Second)
	cancel()
	wg.Wait()

	// Mock only generates CarTelemetry (ID 6) which writes to telemetry + car_telemetry_ext.
	assertTableHasRows(t, store, "telemetry")
	assertTableHasRows(t, store, "car_telemetry_ext")

	// Spot-check: mock speed range is 200-299.
	var speed float64
	if err := store.Writer().QueryRow("SELECT speed FROM telemetry LIMIT 1").Scan(&speed); err != nil {
		t.Fatal(err)
	}
	if speed < 200 || speed >= 300 {
		t.Errorf("expected speed in [200,300), got %.1f", speed)
	}
	t.Logf("spot-check speed = %.1f", speed)
}

// ---------------------------------------------------------------------------
// TestRealUDPIngestion — sends valid binary packets over UDP for all 8
// handled types and verifies data lands in each DuckDB table.
// ---------------------------------------------------------------------------

func TestRealUDPIngestion(t *testing.T) {
	port := freeUDPPort(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")

	store, err := storage.NewStorage(dbPath, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{
		UDPHost:    "127.0.0.1",
		UDPPort:    port,
		BatchSize:  5,
		SampleRate: 1,
	}
	cfg.MockMode.Store(false)
	cfg.SetRuntimeUDPPort(port)

	ingester := NewIngester(cfg, store)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ingester.Start(ctx, &wg)

	// Give the UDP listener time to bind.
	time.Sleep(300 * time.Millisecond)

	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	const N = 30
	for i := 0; i < N; i++ {
		mustWrite(t, conn, buildMotionPacket(0, float32(100+i), float32(200+i), float32(300+i)))
		mustWrite(t, conn, buildSessionPacket(0, 30, 50, 0))
		mustWrite(t, conn, buildLapDataPacket(0, uint8(i%50+1), 3, 1))
		mustWrite(t, conn, buildEventPacket("FTLP"))
		mustWrite(t, conn, buildCarTelemetryPacket(0, uint16(250+i%50), 0.8, 0.1))
		mustWrite(t, conn, buildCarStatusPacket(0, 50.0, 2000000.0))
		mustWrite(t, conn, buildCarDamagePacket(0, [4]float32{10.0, 11.0, 12.0, 13.0}))
		mustWrite(t, conn, buildSessionHistoryPacket(0, 3, []uint32{90000, 91000, 92000}))
	}

	// Let the pipeline process + periodic flush.
	time.Sleep(1500 * time.Millisecond)
	cancel()
	wg.Wait()

	// The 8 buffered tables that should have rows:
	// (session_history rows are constructed but discarded in handleSessionHistory)
	tables := []string{
		"telemetry",
		"car_telemetry_ext",
		"session_data",
		"lap_data",
		"motion_data",
		"car_status",
		"car_damage",
		"race_events",
	}
	for _, tbl := range tables {
		assertTableHasRows(t, store, tbl)
	}

	// Spot-check: the speeds we sent were 250..279.
	var speed float64
	if err := store.Writer().QueryRow("SELECT speed FROM telemetry ORDER BY timestamp DESC LIMIT 1").Scan(&speed); err != nil {
		t.Fatal(err)
	}
	if speed < 250 || speed > 300 {
		t.Errorf("expected speed in [250,300], got %.1f", speed)
	}
	t.Logf("spot-check speed = %.1f", speed)

	// Spot-check: motion world_position_x should be in 100..129.
	var posX float64
	if err := store.Writer().QueryRow("SELECT world_position_x FROM motion_data LIMIT 1").Scan(&posX); err != nil {
		t.Fatal(err)
	}
	if posX < 100 || posX > 130 {
		t.Errorf("expected world_position_x in [100,130], got %.1f", posX)
	}
	t.Logf("spot-check motion posX = %.1f", posX)

	t.Logf("packets received by ingester: %d", ingester.PacketsReceived())
}

// ---------------------------------------------------------------------------
// TestSamplingReducesVolume — sampleRate=10 should keep ~1/10 of packets.
// ---------------------------------------------------------------------------

func TestSamplingReducesVolume(t *testing.T) {
	port := freeUDPPort(t)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")

	store, err := storage.NewStorage(dbPath, 200) // large batch → no auto-flush during send
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{
		UDPHost:    "127.0.0.1",
		UDPPort:    port,
		BatchSize:  200,
		SampleRate: 10,
	}
	cfg.MockMode.Store(false)
	cfg.SetRuntimeUDPPort(port)

	ingester := NewIngester(cfg, store)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ingester.Start(ctx, &wg)

	time.Sleep(300 * time.Millisecond)

	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	const total = 100
	for i := 0; i < total; i++ {
		mustWrite(t, conn, buildCarTelemetryPacket(0, 250, 0.8, 0.1))
	}

	time.Sleep(1500 * time.Millisecond)
	cancel()
	wg.Wait()

	var count int
	if err := store.Writer().QueryRow("SELECT count(*) FROM telemetry").Scan(&count); err != nil {
		t.Fatal(err)
	}

	// Expect ~10 rows (100 / sampleRate 10). Allow ±2 for timing.
	if count < 8 || count > 12 {
		t.Errorf("expected ~10 rows in telemetry (100/10), got %d", count)
	}
	t.Logf("telemetry rows with sampleRate=10: %d (expected ~10)", count)
}

// ---------------------------------------------------------------------------
// TestAtomicCacheUpdates — verify the LiveCache is populated in mock mode.
// ---------------------------------------------------------------------------

func TestAtomicCacheUpdates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.duckdb")

	store, err := storage.NewStorage(dbPath, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{
		UDPHost:    "127.0.0.1",
		UDPPort:    0,
		BatchSize:  5,
		SampleRate: 1,
	}
	cfg.MockMode.Store(true)

	ingester := NewIngester(cfg, store)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	ingester.Start(ctx, &wg)

	time.Sleep(1 * time.Second)

	state := store.Cache().Load()
	if state == nil {
		t.Fatal("expected non-nil cache state after 1s of mock mode")
	}
	if state.Speed < 200 || state.Speed >= 300 {
		t.Errorf("expected speed in [200,300), got %d", state.Speed)
	}
	t.Logf("cache speed = %d, gear = %d, engine_temp = %d", state.Speed, state.Gear, state.EngineTemp)

	cancel()
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertTableHasRows(t *testing.T, store *storage.Storage, table string) {
	t.Helper()
	var count int
	if err := store.Writer().QueryRow(fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count); err != nil {
		t.Errorf("querying %s: %v", table, err)
		return
	}
	if count == 0 {
		t.Errorf("%s: expected rows, got 0", table)
	}
	t.Logf("%s: %d rows", table, count)
}

func mustWrite(t *testing.T, conn net.Conn, data []byte) {
	t.Helper()
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("UDP write: %v", err)
	}
}
