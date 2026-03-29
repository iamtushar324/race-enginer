package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// CarStatusDataSize is the exact byte size of a single CarStatusData struct.
// 1+1+1+1+1+4+4+4+2+2+1+1+2+1+1+1+1+4+4+4+1+4+4+4+1 = 55
const CarStatusDataSize = 55

// PacketCarStatusDataSize is the exact byte size of the full PacketCarStatusData.
// 29 (header) + 22 * 55 (car status data) = 29 + 1210 = 1239
const PacketCarStatusDataSize = HeaderSize + 22*CarStatusDataSize

// CarStatusData contains status data for a single car.
// Size: 55 bytes
type CarStatusData struct {
	TractionControl       uint8
	AntiLockBrakes        uint8
	FuelMix               uint8
	FrontBrakeBias        uint8
	PitLimiterStatus      uint8
	FuelInTank            float32
	FuelCapacity          float32
	FuelRemainingLaps     float32
	MaxRPM                uint16
	IdleRPM               uint16
	MaxGears              uint8
	DRSAllowed            uint8
	DRSActivationDistance uint16
	ActualTyreCompound    uint8
	VisualTyreCompound    uint8
	TyresAgeLaps          uint8
	VehicleFIAFlags       int8
	EnginePowerICE        float32
	EnginePowerMGUK       float32
	ERSStoreEnergy        float32
	ERSDeployMode         uint8
	ERSHarvestedThisLapMGUK float32
	ERSHarvestedThisLapMGUH float32
	ERSDeployedThisLap    float32
	NetworkPaused         uint8
}

// PacketCarStatusData contains status data for all cars on track.
// Packet ID: 7
// Size: 1239 bytes
type PacketCarStatusData struct {
	Header        PacketHeader
	CarStatusData [22]CarStatusData
}

// ParseCarStatusData parses a PacketCarStatusData from raw bytes.
func ParseCarStatusData(data []byte) (*PacketCarStatusData, error) {
	if len(data) < PacketCarStatusDataSize {
		return nil, fmt.Errorf("packet too small for car status data: got %d bytes, need %d", len(data), PacketCarStatusDataSize)
	}
	var p PacketCarStatusData
	r := bytes.NewReader(data[:PacketCarStatusDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse car status data: %w", err)
	}
	return &p, nil
}
