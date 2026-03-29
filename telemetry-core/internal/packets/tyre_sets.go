package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// TyreSetDataSize is the exact byte size of a single TyreSetData struct.
// 1+1+1+1+1+1+1+2+1 = 10
const TyreSetDataSize = 10

// PacketTyreSetsDataSize is the exact byte size of the full PacketTyreSetsData.
// 29 (header) + 1 (carIdx) + 20 * 10 (tyre set data) + 1 (fittedIdx) = 29 + 1 + 200 + 1 = 231
const PacketTyreSetsDataSize = HeaderSize + 1 + 20*TyreSetDataSize + 1

// TyreSetData contains data for a single tyre set.
// Size: 10 bytes
type TyreSetData struct {
	ActualTyreCompound uint8
	VisualTyreCompound uint8
	Wear               uint8
	Available          uint8
	RecommendedSession uint8
	LifeSpan           uint8
	UsableLife         uint8
	LapDeltaTime       uint16
	Fitted             uint8
}

// PacketTyreSetsData contains tyre set data for a single car.
// Packet ID: 12
// Size: 231 bytes
type PacketTyreSetsData struct {
	Header    PacketHeader
	CarIdx    uint8
	TyreSets  [20]TyreSetData
	FittedIdx uint8
}

// ParseTyreSetsData parses a PacketTyreSetsData from raw bytes.
func ParseTyreSetsData(data []byte) (*PacketTyreSetsData, error) {
	if len(data) < PacketTyreSetsDataSize {
		return nil, fmt.Errorf("packet too small for tyre sets data: got %d bytes, need %d", len(data), PacketTyreSetsDataSize)
	}
	var p PacketTyreSetsData
	r := bytes.NewReader(data[:PacketTyreSetsDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse tyre sets data: %w", err)
	}
	return &p, nil
}
