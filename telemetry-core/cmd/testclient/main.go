// Command testclient sends valid binary F1 25 UDP packets to a running
// telemetry-core server and verifies data is ingested by polling the API.
//
// Usage:
//
//	go run cmd/testclient/main.go [--host 127.0.0.1] [--port 20777] [--packets 100] [--rate 20] [--api http://127.0.0.1:8081]
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/packets"
)

func main() {
	host := flag.String("host", "127.0.0.1", "Target UDP host")
	port := flag.Int("port", 20777, "Target UDP port")
	numPackets := flag.Int("packets", 100, "Number of packets per type to send")
	rate := flag.Int("rate", 20, "Packets per second (0 = send as fast as possible)")
	apiURL := flag.String("api", "http://127.0.0.1:8081", "API base URL for verification")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Printf("Sending %d packets per type to %s (rate=%d Hz)\n", *numPackets, addr, *rate)

	var interval time.Duration
	if *rate > 0 {
		interval = time.Second / time.Duration(*rate)
	}

	sent := make(map[string]int)
	start := time.Now()

	for i := 0; i < *numPackets; i++ {
		idx := uint8(0)

		write(conn, buildMotionPacket(idx, float32(100+i), float32(200+i), float32(300+i)))
		sent["motion"]++

		write(conn, buildSessionPacket(0, 30, 50, 0))
		sent["session"]++

		write(conn, buildLapDataPacket(idx, uint8(i%50+1), 3, 1))
		sent["lap_data"]++

		write(conn, buildEventPacket("FTLP"))
		sent["event"]++

		write(conn, buildCarTelemetryPacket(idx, uint16(250+i%50), 0.8, 0.1))
		sent["telemetry"]++

		write(conn, buildCarStatusPacket(idx, 50.0, 2000000.0))
		sent["car_status"]++

		write(conn, buildCarDamagePacket(idx, [4]float32{10.0, 11.0, 12.0, 13.0}))
		sent["car_damage"]++

		write(conn, buildSessionHistoryPacket(0, 3, []uint32{90000, 91000, 92000}))
		sent["session_history"]++

		if interval > 0 {
			time.Sleep(interval)
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("\nSending complete in %s\n", elapsed.Round(time.Millisecond))
	fmt.Println("\nPackets sent per type:")
	for name, n := range sent {
		fmt.Printf("  %-20s %d\n", name, n)
	}

	// Wait for the server to flush.
	fmt.Println("\nWaiting 3s for server flush...")
	time.Sleep(3 * time.Second)

	// Verify via API.
	fmt.Println("\n--- API Verification ---")
	verifyLatest(*apiURL)
	verifyTableCounts(*apiURL)
	verifyHealth(*apiURL)
}

func verifyLatest(apiURL string) {
	resp, err := http.Get(apiURL + "/api/telemetry/latest")
	if err != nil {
		fmt.Printf("GET /api/telemetry/latest: error %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		var data map[string]interface{}
		json.Unmarshal(body, &data)
		fmt.Printf("Latest telemetry: speed=%.0f, gear=%.0f\n",
			data["speed"], data["gear"])
	} else {
		fmt.Printf("GET /api/telemetry/latest: %d %s\n", resp.StatusCode, string(body))
	}
}

func verifyTableCounts(apiURL string) {
	tables := []string{
		"telemetry", "car_telemetry_ext", "session_data", "lap_data",
		"motion_data", "car_status", "car_damage", "race_events", "session_history",
	}

	fmt.Println("\nDuckDB row counts:")
	for _, tbl := range tables {
		sql := fmt.Sprintf(`{"sql":"SELECT count(*) as n FROM %s"}`, tbl)
		resp, err := http.Post(apiURL+"/api/query", "application/json", strings.NewReader(sql))
		if err != nil {
			fmt.Printf("  %-20s error: %v\n", tbl, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 200 {
			var results []map[string]interface{}
			json.Unmarshal(body, &results)
			if len(results) > 0 {
				fmt.Printf("  %-20s %v rows\n", tbl, results[0]["n"])
			}
		} else {
			fmt.Printf("  %-20s error %d\n", tbl, resp.StatusCode)
		}
	}
}

func verifyHealth(apiURL string) {
	resp, err := http.Get(apiURL + "/health")
	if err != nil {
		fmt.Printf("\nGET /health: error %v\n", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var health map[string]interface{}
	json.Unmarshal(body, &health)
	fmt.Printf("\nHealth: packets_rx=%.0f, duckdb_ok=%v, mock_mode=%v\n",
		health["packets_rx"], health["duckdb_ok"], health["mock_mode"])
}

// ---------------------------------------------------------------------------
// Packet builders — inline copies using encoding/binary.Write with struct types
// ---------------------------------------------------------------------------

func makeHeader(packetID, playerCarIndex uint8) packets.PacketHeader {
	return packets.PacketHeader{
		PacketFormat:    2025,
		GameYear:        25,
		PacketID:        packetID,
		SessionUID:      999,
		SessionTime:     42.0,
		FrameIdentifier: 1,
		OverallFrameID:  1,
		PlayerCarIndex:  playerCarIndex,
	}
}

func ser(v interface{}) []byte {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
		panic("binary.Write: " + err.Error())
	}
	return buf.Bytes()
}

func write(conn net.Conn, data []byte) {
	if _, err := conn.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "UDP write: %v\n", err)
	}
}

func buildMotionPacket(playerIdx uint8, worldX, worldY, worldZ float32) []byte {
	pkt := packets.PacketMotionData{}
	pkt.Header = makeHeader(0, playerIdx)
	pkt.CarMotionData[playerIdx] = packets.CarMotionData{
		WorldPositionX: worldX, WorldPositionY: worldY, WorldPositionZ: worldZ,
		GForceLateral: 1.5, GForceLongitudinal: 0.3, GForceVertical: 1.0,
		Yaw: 0.1, Pitch: 0.02, Roll: 0.03,
	}
	return ser(&pkt)
}

func buildSessionPacket(weather uint8, trackTemp int8, totalLaps uint8, safetyCarStatus uint8) []byte {
	pkt := packets.PacketSessionData{}
	pkt.Header = makeHeader(1, 0)
	pkt.Weather = weather
	pkt.TrackTemperature = trackTemp
	pkt.AirTemperature = trackTemp - 5
	pkt.TotalLaps = totalLaps
	pkt.TrackLength = 5303
	pkt.SessionType = 10
	pkt.SafetyCarStatus = safetyCarStatus
	pkt.PitStopWindowIdealLap = 15
	pkt.PitStopWindowLatestLap = 25
	return ser(&pkt)
}

func buildLapDataPacket(playerIdx uint8, lapNum uint8, position uint8, sector uint8) []byte {
	pkt := packets.PacketLapData{}
	pkt.Header = makeHeader(2, playerIdx)
	pkt.LapData[playerIdx] = packets.LapData{
		LastLapTimeInMs: 90000, CurrentLapTimeInMs: 45000,
		Sector1TimeInMs: 30000, Sector2TimeInMs: 30000,
		CarPosition: position, CurrentLapNum: lapNum,
		Sector: sector, GridPosition: position,
		DriverStatus: 1, ResultStatus: 2,
	}
	return ser(&pkt)
}

func buildEventPacket(code string) []byte {
	pkt := packets.PacketEventData{}
	pkt.Header = makeHeader(3, 0)
	copy(pkt.EventStringCode[:], code)
	pkt.EventDetails[0] = 5
	return ser(&pkt)
}

func buildCarTelemetryPacket(playerIdx uint8, speed uint16, throttle float32, brake float32) []byte {
	pkt := packets.PacketCarTelemetryData{}
	pkt.Header = makeHeader(6, playerIdx)
	pkt.CarTelemetryData[playerIdx] = packets.CarTelemetryData{
		Speed: speed, Throttle: throttle, Steer: 0.1, Brake: brake,
		Clutch: 50, Gear: 5, EngineRPM: 10000,
		BrakesTemperature:       [4]uint16{500, 510, 520, 530},
		TyresSurfaceTemperature: [4]uint8{95, 96, 97, 98},
		TyresInnerTemperature:   [4]uint8{100, 101, 102, 103},
		EngineTemperature:       110,
		TyresPressure:           [4]float32{23.0, 23.1, 23.2, 23.3},
	}
	pkt.SuggestedGear = 6
	return ser(&pkt)
}

func buildCarStatusPacket(playerIdx uint8, fuelInTank float32, ersStore float32) []byte {
	pkt := packets.PacketCarStatusData{}
	pkt.Header = makeHeader(7, playerIdx)
	pkt.CarStatusData[playerIdx] = packets.CarStatusData{
		FuelMix: 2, FuelInTank: fuelInTank, FuelCapacity: 110.0,
		FuelRemainingLaps: 15.5, ERSStoreEnergy: ersStore, ERSDeployMode: 1,
		ERSHarvestedThisLapMGUK: 100000.0, ERSHarvestedThisLapMGUH: 200000.0,
		ERSDeployedThisLap: 150000.0,
		ActualTyreCompound: 16, VisualTyreCompound: 16, TyresAgeLaps: 5,
		DRSAllowed: 1, EnginePowerICE: 750000.0, EnginePowerMGUK: 120000.0,
	}
	return ser(&pkt)
}

func buildCarDamagePacket(playerIdx uint8, tyresWear [4]float32) []byte {
	pkt := packets.PacketCarDamageData{}
	pkt.Header = makeHeader(10, playerIdx)
	pkt.CarDamageData[playerIdx] = packets.CarDamageData{
		TyresWear: tyresWear, TyresDamage: [4]uint8{5, 6, 7, 8},
		BrakesDamage: [4]uint8{2, 2, 2, 2},
		FrontLeftWingDamage: 10, FrontRightWingDamage: 12,
		RearWingDamage: 3, FloorDamage: 1, EngineDamage: 5,
	}
	return ser(&pkt)
}

func buildSessionHistoryPacket(carIdx uint8, numLaps uint8, lapTimes []uint32) []byte {
	pkt := packets.PacketSessionHistoryData{}
	pkt.Header = makeHeader(11, 0)
	pkt.CarIdx = carIdx
	pkt.NumLaps = numLaps
	pkt.BestLapTimeLapNum = 1
	for i := 0; i < int(numLaps) && i < 100 && i < len(lapTimes); i++ {
		pkt.LapHistoryData[i] = packets.LapHistoryData{
			LapTimeInMs:      lapTimes[i],
			Sector1TimeInMs:  uint16(lapTimes[i] / 3),
			Sector2TimeInMs:  uint16(lapTimes[i] / 3),
			Sector3TimeInMs:  uint16(lapTimes[i] / 3),
			LapValidBitFlags: 0x01,
		}
	}
	return ser(&pkt)
}
