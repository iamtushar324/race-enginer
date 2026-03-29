package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// HeaderSize is the exact byte size of the PacketHeader struct.
const HeaderSize = 29

// PacketHeader is the common header for all F1 25 telemetry packets.
// Size: 29 bytes
type PacketHeader struct {
	PacketFormat            uint16  // 2025 for F1 25
	GameYear                uint8   // last two digits, e.g. 25
	GameMajorVersion        uint8
	GameMinorVersion        uint8
	PacketVersion           uint8
	PacketID                uint8
	SessionUID              uint64
	SessionTime             float32
	FrameIdentifier         uint32
	OverallFrameID          uint32
	PlayerCarIndex          uint8
	SecondaryPlayerCarIndex uint8
}

// ParseHeader parses a PacketHeader from raw bytes.
func ParseHeader(data []byte) (*PacketHeader, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("packet too small for header: got %d bytes, need %d", len(data), HeaderSize)
	}
	var h PacketHeader
	r := bytes.NewReader(data[:HeaderSize])
	if err := binary.Read(r, binary.LittleEndian, &h); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}
	return &h, nil
}
