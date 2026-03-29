package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// FinalClassificationDataSize is the exact byte size of a single FinalClassificationData struct.
// 1+1+1+1+1+1+1+4+8+1+1+1+8+8+8 = 46
const FinalClassificationDataSize = 46

// PacketFinalClassificationDataSize is the exact byte size of the full PacketFinalClassificationData.
// 29 (header) + 1 (numCars) + 22 * 46 (classification data) = 29 + 1 + 1012 = 1042
const PacketFinalClassificationDataSize = HeaderSize + 1 + 22*FinalClassificationDataSize

// FinalClassificationData contains classification data for a single car.
// Size: 46 bytes
type FinalClassificationData struct {
	Position          uint8
	NumLaps           uint8
	GridPosition      uint8
	Points            uint8
	NumPitStops       uint8
	ResultStatus      uint8
	ResultReason      uint8
	BestLapTimeInMs   uint32
	TotalRaceTime     float64
	PenaltiesTime     uint8
	NumPenalties      uint8
	NumTyreStints     uint8
	TyreStintsActual  [8]uint8
	TyreStintsVisual  [8]uint8
	TyreStintsEndLaps [8]uint8
}

// PacketFinalClassificationData contains final classification data for all cars.
// Packet ID: 8
// Size: 1042 bytes
type PacketFinalClassificationData struct {
	Header                  PacketHeader
	NumCars                 uint8
	FinalClassificationData [22]FinalClassificationData
}

// ParseFinalClassificationData parses a PacketFinalClassificationData from raw bytes.
func ParseFinalClassificationData(data []byte) (*PacketFinalClassificationData, error) {
	if len(data) < PacketFinalClassificationDataSize {
		return nil, fmt.Errorf("packet too small for final classification data: got %d bytes, need %d", len(data), PacketFinalClassificationDataSize)
	}
	var p PacketFinalClassificationData
	r := bytes.NewReader(data[:PacketFinalClassificationDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse final classification data: %w", err)
	}
	return &p, nil
}
