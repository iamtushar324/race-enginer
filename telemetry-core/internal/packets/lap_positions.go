package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// PacketLapPositionsSize is the exact byte size of the full PacketLapPositions.
// 29 (header) + 1 (numLaps) + 1 (lapStart) + 50*22 (positionForVehicleIdx) = 29 + 2 + 1100 = 1131
const PacketLapPositionsSize = HeaderSize + 1 + 1 + 50*22

// PacketLapPositions contains lap position data.
// Packet ID: 15
// Size: 1131 bytes
type PacketLapPositions struct {
	Header                  PacketHeader
	NumLaps                 uint8
	LapStart                uint8
	PositionForVehicleIdx   [50][22]uint8
}

// ParseLapPositions parses a PacketLapPositions from raw bytes.
func ParseLapPositions(data []byte) (*PacketLapPositions, error) {
	if len(data) < PacketLapPositionsSize {
		return nil, fmt.Errorf("packet too small for lap positions: got %d bytes, need %d", len(data), PacketLapPositionsSize)
	}
	var p PacketLapPositions
	r := bytes.NewReader(data[:PacketLapPositionsSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse lap positions: %w", err)
	}
	return &p, nil
}
