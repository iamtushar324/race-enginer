package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// LiveryColourSize is the exact byte size of a LiveryColour struct.
const LiveryColourSize = 3

// ParticipantDataSize is the exact byte size of a single ParticipantData struct.
// 1+1+1+1+1+1+1+32+1+1+2+1+1+4*3 = 57
const ParticipantDataSize = 57

// PacketParticipantsDataSize is the exact byte size of the full PacketParticipantsData.
// 29 (header) + 1 (numActiveCars) + 22 * 57 (participant data) = 29 + 1 + 1254 = 1284
const PacketParticipantsDataSize = HeaderSize + 1 + 22*ParticipantDataSize

// LiveryColour contains RGB colour data for a livery.
// Size: 3 bytes
type LiveryColour struct {
	Red   uint8
	Green uint8
	Blue  uint8
}

// ParticipantData contains data for a single participant.
// Size: 57 bytes
type ParticipantData struct {
	AIControlled    uint8
	DriverID        uint8
	NetworkID       uint8
	TeamID          uint8
	MyTeam          uint8
	RaceNumber      uint8
	Nationality     uint8
	Name            [32]byte // null-terminated UTF-8
	YourTelemetry   uint8
	ShowOnlineNames uint8
	TechLevel       uint16
	Platform        uint8
	NumColours      uint8
	LiveryColours   [4]LiveryColour
}

// GetName returns the participant name as a Go string, trimming null bytes.
func (p *ParticipantData) GetName() string {
	n := 0
	for n < len(p.Name) && p.Name[n] != 0 {
		n++
	}
	return string(p.Name[:n])
}

// PacketParticipantsData contains data for all participants.
// Packet ID: 4
// Size: 1284 bytes
type PacketParticipantsData struct {
	Header        PacketHeader
	NumActiveCars uint8
	Participants  [22]ParticipantData
}

// ParseParticipantsData parses a PacketParticipantsData from raw bytes.
func ParseParticipantsData(data []byte) (*PacketParticipantsData, error) {
	if len(data) < PacketParticipantsDataSize {
		return nil, fmt.Errorf("packet too small for participants data: got %d bytes, need %d", len(data), PacketParticipantsDataSize)
	}
	var p PacketParticipantsData
	r := bytes.NewReader(data[:PacketParticipantsDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse participants data: %w", err)
	}
	return &p, nil
}
