package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// CarMotionDataSize is the exact byte size of a single CarMotionData struct.
const CarMotionDataSize = 60

// PacketMotionDataSize is the exact byte size of the full PacketMotionData.
// 29 (header) + 22 * 60 (car motion data) = 29 + 1320 = 1349
const PacketMotionDataSize = HeaderSize + 22*CarMotionDataSize

// CarMotionData contains motion data for a single car.
// Size: 60 bytes
type CarMotionData struct {
	WorldPositionX   float32
	WorldPositionY   float32
	WorldPositionZ   float32
	WorldVelocityX   float32
	WorldVelocityY   float32
	WorldVelocityZ   float32
	WorldForwardDirX int16
	WorldForwardDirY int16
	WorldForwardDirZ int16
	WorldRightDirX   int16
	WorldRightDirY   int16
	WorldRightDirZ   int16
	GForceLateral      float32
	GForceLongitudinal float32
	GForceVertical     float32
	Yaw   float32
	Pitch float32
	Roll  float32
}

// PacketMotionData contains motion data for all cars on track.
// Packet ID: 0
// Size: 1349 bytes
type PacketMotionData struct {
	Header        PacketHeader
	CarMotionData [22]CarMotionData
}

// ParseMotionData parses a PacketMotionData from raw bytes.
func ParseMotionData(data []byte) (*PacketMotionData, error) {
	if len(data) < PacketMotionDataSize {
		return nil, fmt.Errorf("packet too small for motion data: got %d bytes, need %d", len(data), PacketMotionDataSize)
	}
	var p PacketMotionData
	r := bytes.NewReader(data[:PacketMotionDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse motion data: %w", err)
	}
	return &p, nil
}
