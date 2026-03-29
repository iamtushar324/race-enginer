package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// LapDataSize is the exact byte size of a single LapData struct.
// 4+4+2+1+2+1+2+1+2+1+4+4+4+1*14+1+2+2+1+4+1 = 57
const LapDataSize = 57

// PacketLapDataSize is the exact byte size of the full PacketLapData.
// 29 (header) + 22 * 57 (lap data) + 1 + 1 = 1285
const PacketLapDataSize = HeaderSize + 22*LapDataSize + 2

// LapData contains lap timing data for a single car.
// Size: 57 bytes
type LapData struct {
	LastLapTimeInMs            uint32
	CurrentLapTimeInMs         uint32
	Sector1TimeInMs            uint16
	Sector1TimeMinutes         uint8
	Sector2TimeInMs            uint16
	Sector2TimeMinutes         uint8
	DeltaToCarInFrontMsPart    uint16
	DeltaToCarInFrontMinutesPart uint8
	DeltaToRaceLeaderMsPart    uint16
	DeltaToRaceLeaderMinutesPart uint8
	LapDistance                float32
	TotalDistance              float32
	SafetyCarDelta             float32
	CarPosition                uint8
	CurrentLapNum              uint8
	PitStatus                  uint8
	NumPitStops                uint8
	Sector                     uint8
	CurrentLapInvalid          uint8
	Penalties                  uint8
	TotalWarnings              uint8
	CornerCuttingWarnings      uint8
	NumUnservedDriveThrough    uint8
	NumUnservedStopGo          uint8
	GridPosition               uint8
	DriverStatus               uint8
	ResultStatus               uint8
	PitLaneTimerActive         uint8
	PitLaneTimeInLaneMs        uint16
	PitStopTimerMs             uint16
	PitStopShouldServePen      uint8
	SpeedTrapFastestSpeed      float32
	SpeedTrapFastestLap        uint8
}

// PacketLapData contains lap data for all cars on track.
// Packet ID: 2
// Size: 1285 bytes
type PacketLapData struct {
	Header                PacketHeader
	LapData               [22]LapData
	TimeTrialPBCarIdx     uint8
	TimeTrialRivalCarIdx  int8
}

// ParseLapData parses a PacketLapData from raw bytes.
func ParseLapData(data []byte) (*PacketLapData, error) {
	if len(data) < PacketLapDataSize {
		return nil, fmt.Errorf("packet too small for lap data: got %d bytes, need %d", len(data), PacketLapDataSize)
	}
	var p PacketLapData
	r := bytes.NewReader(data[:PacketLapDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse lap data: %w", err)
	}
	return &p, nil
}
