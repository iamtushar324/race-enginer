package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// PacketMotionExDataSize is the exact byte size of the full PacketMotionExData.
// 29 (header)
// + 4*4 (SuspensionPosition) = 16
// + 4*4 (SuspensionVelocity) = 16
// + 4*4 (SuspensionAcceleration) = 16
// + 4*4 (WheelSpeed) = 16
// + 4*4 (WheelSlipRatio) = 16
// + 4*4 (WheelSlipAngle) = 16
// + 4*4 (WheelLatForce) = 16
// + 4*4 (WheelLongForce) = 16
// + 4 (HeightOfCOGAboveGround)
// + 4+4+4 (LocalVelocity XYZ)
// + 4+4+4 (AngularVelocity XYZ)
// + 4+4+4 (AngularAcceleration XYZ)
// + 4 (FrontWheelsAngle)
// + 4*4 (WheelVertForce) = 16
// + 4+4 (FrontAeroHeight, RearAeroHeight)
// + 4+4 (FrontRollAngle, RearRollAngle)
// + 4+4 (ChassisYaw, ChassisPitch)
// + 4*4 (WheelCamber) = 16
// + 4*4 (WheelCamberGain) = 16
// = 29 + 128 + 4 + 12 + 12 + 12 + 4 + 16 + 8 + 8 + 8 + 16 + 16 = 29 + 244 = 273
const PacketMotionExDataSize = 273

// PacketMotionExData contains extended motion data for the player's car.
// Packet ID: 13
// Size: 273 bytes
type PacketMotionExData struct {
	Header                   PacketHeader
	SuspensionPosition       [4]float32
	SuspensionVelocity       [4]float32
	SuspensionAcceleration   [4]float32
	WheelSpeed               [4]float32
	WheelSlipRatio           [4]float32
	WheelSlipAngle           [4]float32
	WheelLatForce            [4]float32
	WheelLongForce           [4]float32
	HeightOfCOGAboveGround   float32
	LocalVelocityX           float32
	LocalVelocityY           float32
	LocalVelocityZ           float32
	AngularVelocityX         float32
	AngularVelocityY         float32
	AngularVelocityZ         float32
	AngularAccelerationX     float32
	AngularAccelerationY     float32
	AngularAccelerationZ     float32
	FrontWheelsAngle         float32
	WheelVertForce           [4]float32
	FrontAeroHeight          float32
	RearAeroHeight           float32
	FrontRollAngle           float32
	RearRollAngle            float32
	ChassisYaw               float32
	ChassisPitch             float32
	WheelCamber              [4]float32
	WheelCamberGain          [4]float32
}

// ParseMotionExData parses a PacketMotionExData from raw bytes.
func ParseMotionExData(data []byte) (*PacketMotionExData, error) {
	if len(data) < PacketMotionExDataSize {
		return nil, fmt.Errorf("packet too small for motion ex data: got %d bytes, need %d", len(data), PacketMotionExDataSize)
	}
	var p PacketMotionExData
	r := bytes.NewReader(data[:PacketMotionExDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse motion ex data: %w", err)
	}
	return &p, nil
}
