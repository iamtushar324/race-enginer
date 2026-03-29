package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// LobbyInfoDataSize is the exact byte size of a single LobbyInfoData struct.
// 1+1+1+1+32+1+1+1+2+1 = 42
const LobbyInfoDataSize = 42

// PacketLobbyInfoDataSize is the exact byte size of the full PacketLobbyInfoData.
// 29 (header) + 1 (numPlayers) + 22 * 42 (lobby info data) = 29 + 1 + 924 = 954
const PacketLobbyInfoDataSize = HeaderSize + 1 + 22*LobbyInfoDataSize

// LobbyInfoData contains data for a single lobby participant.
// Size: 42 bytes
type LobbyInfoData struct {
	AIControlled    uint8
	TeamID          uint8
	Nationality     uint8
	Platform        uint8
	Name            [32]byte // null-terminated UTF-8
	CarNumber       uint8
	YourTelemetry   uint8
	ShowOnlineNames uint8
	TechLevel       uint16
	ReadyStatus     uint8
}

// GetName returns the lobby participant name as a Go string, trimming null bytes.
func (l *LobbyInfoData) GetName() string {
	n := 0
	for n < len(l.Name) && l.Name[n] != 0 {
		n++
	}
	return string(l.Name[:n])
}

// PacketLobbyInfoData contains lobby information for all players.
// Packet ID: 9
// Size: 954 bytes
type PacketLobbyInfoData struct {
	Header     PacketHeader
	NumPlayers uint8
	LobbyPlayers [22]LobbyInfoData
}

// ParseLobbyInfoData parses a PacketLobbyInfoData from raw bytes.
func ParseLobbyInfoData(data []byte) (*PacketLobbyInfoData, error) {
	if len(data) < PacketLobbyInfoDataSize {
		return nil, fmt.Errorf("packet too small for lobby info data: got %d bytes, need %d", len(data), PacketLobbyInfoDataSize)
	}
	var p PacketLobbyInfoData
	r := bytes.NewReader(data[:PacketLobbyInfoDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse lobby info data: %w", err)
	}
	return &p, nil
}
