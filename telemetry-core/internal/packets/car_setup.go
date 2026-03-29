package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// CarSetupDataSize is the exact byte size of a single CarSetupData struct.
// 1+1+1+1+4+4+4+4+1+1+1+1+1+1+1+1+1+4+4+4+4+1+4 = 50
const CarSetupDataSize = 50

// PacketCarSetupDataSize is the exact byte size of the full PacketCarSetupData.
// 29 (header) + 22 * 50 (car setup data) + 4 (nextFrontWingValue) = 29 + 1100 + 4 = 1133
const PacketCarSetupDataSize = HeaderSize + 22*CarSetupDataSize + 4

// CarSetupData contains setup data for a single car.
// Size: 50 bytes
type CarSetupData struct {
	FrontWing              uint8
	RearWing               uint8
	OnThrottle             uint8
	OffThrottle            uint8
	FrontCamber            float32
	RearCamber             float32
	FrontToe               float32
	RearToe                float32
	FrontSuspension        uint8
	RearSuspension         uint8
	FrontAntiRollBar       uint8
	RearAntiRollBar        uint8
	FrontSuspensionHeight  uint8
	RearSuspensionHeight   uint8
	BrakePressure          uint8
	BrakeBias              uint8
	EngineBraking          uint8
	RearLeftTyrePressure   float32
	RearRightTyrePressure  float32
	FrontLeftTyrePressure  float32
	FrontRightTyrePressure float32
	Ballast                uint8
	FuelLoad               float32
}

// PacketCarSetupData contains setup data for all cars on track.
// Packet ID: 5
// Size: 1133 bytes
type PacketCarSetupData struct {
	Header            PacketHeader
	CarSetups         [22]CarSetupData
	NextFrontWingValue float32
}

// ParseCarSetupData parses a PacketCarSetupData from raw bytes.
func ParseCarSetupData(data []byte) (*PacketCarSetupData, error) {
	if len(data) < PacketCarSetupDataSize {
		return nil, fmt.Errorf("packet too small for car setup data: got %d bytes, need %d", len(data), PacketCarSetupDataSize)
	}
	var p PacketCarSetupData
	r := bytes.NewReader(data[:PacketCarSetupDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse car setup data: %w", err)
	}
	return &p, nil
}
