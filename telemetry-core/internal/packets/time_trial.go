package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// TimeTrialDataSetSize is the exact byte size of a single TimeTrialDataSet struct.
// 1+1+4+4+4+4+1+1+1+1+1+1 = 24
const TimeTrialDataSetSize = 24

// PacketTimeTrialDataSize is the exact byte size of the full PacketTimeTrialData.
// 29 (header) + 3 * 24 (time trial data sets) = 29 + 72 = 101
const PacketTimeTrialDataSize = HeaderSize + 3*TimeTrialDataSetSize

// TimeTrialDataSet contains time trial data for a single data set.
// Size: 24 bytes
type TimeTrialDataSet struct {
	CarIdx              uint8
	TeamID              uint8
	LapTimeInMs         uint32
	Sector1TimeInMs     uint32
	Sector2TimeInMs     uint32
	Sector3TimeInMs     uint32
	TractionControl     uint8
	GearboxAssist       uint8
	AntiLockBrakes      uint8
	EqualCarPerformance uint8
	CustomSetup         uint8
	Valid               uint8
}

// PacketTimeTrialData contains time trial data.
// Packet ID: 14
// Size: 101 bytes
type PacketTimeTrialData struct {
	Header             PacketHeader
	PlayerSessionBest  TimeTrialDataSet
	PersonalBest       TimeTrialDataSet
	Rival              TimeTrialDataSet
}

// ParseTimeTrialData parses a PacketTimeTrialData from raw bytes.
func ParseTimeTrialData(data []byte) (*PacketTimeTrialData, error) {
	if len(data) < PacketTimeTrialDataSize {
		return nil, fmt.Errorf("packet too small for time trial data: got %d bytes, need %d", len(data), PacketTimeTrialDataSize)
	}
	var p PacketTimeTrialData
	r := bytes.NewReader(data[:PacketTimeTrialDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse time trial data: %w", err)
	}
	return &p, nil
}
