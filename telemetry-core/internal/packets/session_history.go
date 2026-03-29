package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// LapHistoryDataSize is the exact byte size of a single LapHistoryData struct.
// 4+2+1+2+1+2+1+1 = 14
const LapHistoryDataSize = 14

// TyreStintHistoryDataSize is the exact byte size of a single TyreStintHistoryData struct.
const TyreStintHistoryDataSize = 3

// PacketSessionHistoryDataSize is the exact byte size of the full PacketSessionHistoryData.
// 29 (header) + 1+1+1+1+1+1+1 + 100*14 + 8*3 = 29 + 7 + 1400 + 24 = 1460
const PacketSessionHistoryDataSize = HeaderSize + 7 + 100*LapHistoryDataSize + 8*TyreStintHistoryDataSize

// LapHistoryData contains timing data for a single lap.
// Size: 14 bytes
type LapHistoryData struct {
	LapTimeInMs            uint32
	Sector1TimeInMs        uint16
	Sector1TimeMinutesPart uint8
	Sector2TimeInMs        uint16
	Sector2TimeMinutesPart uint8
	Sector3TimeInMs        uint16
	Sector3TimeMinutesPart uint8
	LapValidBitFlags       uint8
}

// TyreStintHistoryData contains data for a single tyre stint.
// Size: 3 bytes
type TyreStintHistoryData struct {
	EndLap              uint8
	TyreActualCompound  uint8
	TyreVisualCompound  uint8
}

// PacketSessionHistoryData contains lap and tyre stint history for a single car.
// Packet ID: 11
// Size: 1460 bytes
type PacketSessionHistoryData struct {
	Header              PacketHeader
	CarIdx              uint8
	NumLaps             uint8
	NumTyreStints       uint8
	BestLapTimeLapNum   uint8
	BestSector1LapNum   uint8
	BestSector2LapNum   uint8
	BestSector3LapNum   uint8
	LapHistoryData      [100]LapHistoryData
	TyreStintHistoryData [8]TyreStintHistoryData
}

// ParseSessionHistoryData parses a PacketSessionHistoryData from raw bytes.
func ParseSessionHistoryData(data []byte) (*PacketSessionHistoryData, error) {
	if len(data) < PacketSessionHistoryDataSize {
		return nil, fmt.Errorf("packet too small for session history data: got %d bytes, need %d", len(data), PacketSessionHistoryDataSize)
	}
	var p PacketSessionHistoryData
	r := bytes.NewReader(data[:PacketSessionHistoryDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse session history data: %w", err)
	}
	return &p, nil
}
