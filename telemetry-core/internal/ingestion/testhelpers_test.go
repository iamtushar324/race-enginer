package ingestion

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/packets"
)

// ---------------------------------------------------------------------------
// Binary packet builders — construct valid F1 25 packets using the exact
// struct types from internal/packets, serialised via encoding/binary.Write
// with binary.LittleEndian. This guarantees byte-for-byte round-trip
// correctness against the parsers.
// ---------------------------------------------------------------------------

// buildMotionPacket constructs a valid Motion packet (ID 0, 1349 bytes).
func buildMotionPacket(playerIdx uint8, worldX, worldY, worldZ float32) []byte {
	pkt := packets.PacketMotionData{}
	pkt.Header = makeHeader(0, playerIdx)
	pkt.CarMotionData[playerIdx] = packets.CarMotionData{
		WorldPositionX:     worldX,
		WorldPositionY:     worldY,
		WorldPositionZ:     worldZ,
		GForceLateral:      1.5,
		GForceLongitudinal: 0.3,
		GForceVertical:     1.0,
		Yaw:                0.1,
		Pitch:              0.02,
		Roll:               0.03,
	}
	return serialize(&pkt)
}

// buildSessionPacket constructs a valid Session packet (ID 1, 753 bytes).
func buildSessionPacket(weather uint8, trackTemp int8, totalLaps uint8, safetyCarStatus uint8) []byte {
	pkt := packets.PacketSessionData{}
	pkt.Header = makeHeader(1, 0)
	pkt.Weather = weather
	pkt.TrackTemperature = trackTemp
	pkt.AirTemperature = trackTemp - 5
	pkt.TotalLaps = totalLaps
	pkt.TrackLength = 5303
	pkt.SessionType = 10 // Race
	pkt.TrackID = 0
	pkt.SafetyCarStatus = safetyCarStatus
	pkt.PitStopWindowIdealLap = 15
	pkt.PitStopWindowLatestLap = 25
	return serialize(&pkt)
}

// buildLapDataPacket constructs a valid Lap Data packet (ID 2, 1285 bytes).
func buildLapDataPacket(playerIdx uint8, lapNum uint8, position uint8, sector uint8) []byte {
	pkt := packets.PacketLapData{}
	pkt.Header = makeHeader(2, playerIdx)
	pkt.LapData[playerIdx] = packets.LapData{
		LastLapTimeInMs:    90000,
		CurrentLapTimeInMs: 45000,
		Sector1TimeInMs:    30000,
		Sector2TimeInMs:    30000,
		CarPosition:        position,
		CurrentLapNum:      lapNum,
		Sector:             sector,
		GridPosition:       position,
		DriverStatus:       1,
		ResultStatus:       2,
	}
	return serialize(&pkt)
}

// buildEventPacket constructs a valid Event packet (ID 3, 45 bytes).
func buildEventPacket(code string) []byte {
	pkt := packets.PacketEventData{}
	pkt.Header = makeHeader(3, 0)
	copy(pkt.EventStringCode[:], code)
	pkt.EventDetails[0] = 5 // vehicle index
	return serialize(&pkt)
}

// buildCarTelemetryPacket constructs a valid CarTelemetry packet (ID 6, 1352 bytes).
func buildCarTelemetryPacket(playerIdx uint8, speed uint16, throttle float32, brake float32) []byte {
	pkt := packets.PacketCarTelemetryData{}
	pkt.Header = makeHeader(6, playerIdx)
	pkt.CarTelemetryData[playerIdx] = packets.CarTelemetryData{
		Speed:                   speed,
		Throttle:                throttle,
		Steer:                   0.1,
		Brake:                   brake,
		Clutch:                  50,
		Gear:                    5,
		EngineRPM:               10000,
		DRS:                     0,
		BrakesTemperature:       [4]uint16{500, 510, 520, 530},
		TyresSurfaceTemperature: [4]uint8{95, 96, 97, 98},
		TyresInnerTemperature:   [4]uint8{100, 101, 102, 103},
		EngineTemperature:       110,
		TyresPressure:           [4]float32{23.0, 23.1, 23.2, 23.3},
	}
	pkt.SuggestedGear = 6
	return serialize(&pkt)
}

// buildCarStatusPacket constructs a valid CarStatus packet (ID 7, 1239 bytes).
func buildCarStatusPacket(playerIdx uint8, fuelInTank float32, ersStore float32) []byte {
	pkt := packets.PacketCarStatusData{}
	pkt.Header = makeHeader(7, playerIdx)
	pkt.CarStatusData[playerIdx] = packets.CarStatusData{
		FuelMix:                 2,
		FuelInTank:              fuelInTank,
		FuelCapacity:            110.0,
		FuelRemainingLaps:       15.5,
		ERSStoreEnergy:          ersStore,
		ERSDeployMode:           1,
		ERSHarvestedThisLapMGUK: 100000.0,
		ERSHarvestedThisLapMGUH: 200000.0,
		ERSDeployedThisLap:      150000.0,
		ActualTyreCompound:      16,
		VisualTyreCompound:      16,
		TyresAgeLaps:            5,
		DRSAllowed:              1,
		VehicleFIAFlags:         0,
		EnginePowerICE:          750000.0,
		EnginePowerMGUK:         120000.0,
	}
	return serialize(&pkt)
}

// buildCarDamagePacket constructs a valid CarDamage packet (ID 10, 1041 bytes).
func buildCarDamagePacket(playerIdx uint8, tyresWear [4]float32) []byte {
	pkt := packets.PacketCarDamageData{}
	pkt.Header = makeHeader(10, playerIdx)
	pkt.CarDamageData[playerIdx] = packets.CarDamageData{
		TyresWear:            tyresWear,
		TyresDamage:          [4]uint8{5, 6, 7, 8},
		BrakesDamage:         [4]uint8{2, 2, 2, 2},
		FrontLeftWingDamage:  10,
		FrontRightWingDamage: 12,
		RearWingDamage:       3,
		FloorDamage:          1,
		GearBoxDamage:        0,
		EngineDamage:         5,
	}
	return serialize(&pkt)
}

// buildSessionHistoryPacket constructs a valid SessionHistory packet (ID 11, 1460 bytes).
func buildSessionHistoryPacket(carIdx uint8, numLaps uint8, lapTimes []uint32) []byte {
	pkt := packets.PacketSessionHistoryData{}
	pkt.Header = makeHeader(11, 0)
	pkt.CarIdx = carIdx
	pkt.NumLaps = numLaps
	pkt.BestLapTimeLapNum = 1
	for i := 0; i < int(numLaps) && i < 100 && i < len(lapTimes); i++ {
		pkt.LapHistoryData[i] = packets.LapHistoryData{
			LapTimeInMs:     lapTimes[i],
			Sector1TimeInMs: uint16(lapTimes[i] / 3),
			Sector2TimeInMs: uint16(lapTimes[i] / 3),
			Sector3TimeInMs: uint16(lapTimes[i] / 3),
			LapValidBitFlags: 0x01,
		}
	}
	return serialize(&pkt)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeHeader builds a PacketHeader with standard test values.
func makeHeader(packetID, playerCarIndex uint8) packets.PacketHeader {
	return packets.PacketHeader{
		PacketFormat:   2025,
		GameYear:       25,
		PacketID:       packetID,
		SessionUID:     999,
		SessionTime:    42.0,
		FrameIdentifier: 1,
		OverallFrameID: 1,
		PlayerCarIndex: playerCarIndex,
	}
}

// serialize writes a struct to bytes using binary.LittleEndian.
func serialize(v interface{}) []byte {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, v); err != nil {
		panic("binary.Write failed: " + err.Error())
	}
	return buf.Bytes()
}

// freeUDPPort finds an available ephemeral UDP port by briefly binding to :0.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	return port
}
