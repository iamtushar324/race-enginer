package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// CarDamageDataSize is the exact byte size of a single CarDamageData struct.
// 4*4 + 4*1 + 4*1 + 4*1 + 18*1 = 16+4+4+4+18 = 46
const CarDamageDataSize = 46

// PacketCarDamageDataSize is the exact byte size of the full PacketCarDamageData.
// 29 (header) + 22 * 46 (car damage data) = 29 + 1012 = 1041
const PacketCarDamageDataSize = HeaderSize + 22*CarDamageDataSize

// CarDamageData contains damage data for a single car.
// Size: 46 bytes
type CarDamageData struct {
	TyresWear            [4]float32
	TyresDamage          [4]uint8
	BrakesDamage         [4]uint8
	TyreBlisters         [4]uint8
	FrontLeftWingDamage  uint8
	FrontRightWingDamage uint8
	RearWingDamage       uint8
	FloorDamage          uint8
	DiffuserDamage       uint8
	SidepodDamage        uint8
	DRSFault             uint8
	ERSFault             uint8
	GearBoxDamage        uint8
	EngineDamage         uint8
	EngineMGUHWear       uint8
	EngineESWear         uint8
	EngineCEWear         uint8
	EngineICEWear        uint8
	EngineMGUKWear       uint8
	EngineTCWear         uint8
	EngineBlown          uint8
	EngineSeized         uint8
}

// PacketCarDamageData contains damage data for all cars on track.
// Packet ID: 10
// Size: 1041 bytes
type PacketCarDamageData struct {
	Header        PacketHeader
	CarDamageData [22]CarDamageData
}

// ParseCarDamageData parses a PacketCarDamageData from raw bytes.
func ParseCarDamageData(data []byte) (*PacketCarDamageData, error) {
	if len(data) < PacketCarDamageDataSize {
		return nil, fmt.Errorf("packet too small for car damage data: got %d bytes, need %d", len(data), PacketCarDamageDataSize)
	}
	var p PacketCarDamageData
	r := bytes.NewReader(data[:PacketCarDamageDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse car damage data: %w", err)
	}
	return &p, nil
}
