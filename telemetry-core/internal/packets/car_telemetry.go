package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// CarTelemetryDataSize is the exact byte size of a single CarTelemetryData struct.
// 2+4+4+4+1+1+2+1+1+2+4*2+4*1+4*1+2+4*4+4*1 = 60
const CarTelemetryDataSize = 60

// PacketCarTelemetryDataSize is the exact byte size of the full PacketCarTelemetryData.
// 29 (header) + 22 * 60 (car telemetry data) + 1 + 1 + 1 = 1352
const PacketCarTelemetryDataSize = HeaderSize + 22*CarTelemetryDataSize + 3

// CarTelemetryData contains telemetry data for a single car.
// Size: 60 bytes
type CarTelemetryData struct {
	Speed                   uint16
	Throttle                float32
	Steer                   float32
	Brake                   float32
	Clutch                  uint8
	Gear                    int8
	EngineRPM               uint16
	DRS                     uint8
	RevLightsPercent        uint8
	RevLightsBitValue       uint16
	BrakesTemperature       [4]uint16
	TyresSurfaceTemperature [4]uint8
	TyresInnerTemperature   [4]uint8
	EngineTemperature       uint16
	TyresPressure           [4]float32
	SurfaceType             [4]uint8
}

// PacketCarTelemetryData contains telemetry data for all cars on track.
// Packet ID: 6
// Size: 1352 bytes
type PacketCarTelemetryData struct {
	Header                  PacketHeader
	CarTelemetryData        [22]CarTelemetryData
	MFDPanelIndex           uint8
	MFDPanelIndexSecondary  uint8
	SuggestedGear           int8
}

// ParseCarTelemetryData parses a PacketCarTelemetryData from raw bytes.
func ParseCarTelemetryData(data []byte) (*PacketCarTelemetryData, error) {
	if len(data) < PacketCarTelemetryDataSize {
		return nil, fmt.Errorf("packet too small for car telemetry data: got %d bytes, need %d", len(data), PacketCarTelemetryDataSize)
	}
	var p PacketCarTelemetryData
	r := bytes.NewReader(data[:PacketCarTelemetryDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse car telemetry data: %w", err)
	}
	return &p, nil
}
