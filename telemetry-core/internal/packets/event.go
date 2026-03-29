package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// EventCode represents an F1 25 event string code.
type EventCode string

// Event code constants.
const (
	EventSessionStarted    EventCode = "SSTA"
	EventSessionEnded      EventCode = "SEND"
	EventFastestLap        EventCode = "FTLP"
	EventRetirement        EventCode = "RTMT"
	EventDRSEnabled        EventCode = "DRSE"
	EventDRSDisabled       EventCode = "DRSD"
	EventTeamMateInPits    EventCode = "TMPT"
	EventChequeredFlag     EventCode = "CHQF"
	EventRaceWinner        EventCode = "RCWN"
	EventPenaltyIssued     EventCode = "PENA"
	EventSpeedTrap         EventCode = "SPTP"
	EventStartLights       EventCode = "STLG"
	EventLightsOut         EventCode = "LGOT"
	EventDriveThroughServed EventCode = "DTSV"
	EventStopGoServed      EventCode = "SGSV"
	EventFlashback         EventCode = "FLBK"
	EventButtons           EventCode = "BUTN"
	EventOvertake          EventCode = "OVTK"
	EventSafetyCar         EventCode = "SAFC"
	EventCollision         EventCode = "COLL"
)

// EventDetailSize is the size of the raw event detail union (largest member: SpeedTrap at 12 bytes).
const EventDetailSize = 12

// PacketEventDataSize is the exact byte size of the full PacketEventData.
// 29 (header) + 4 (event string code) + 12 (event detail union) = 45
const PacketEventDataSize = HeaderSize + 4 + EventDetailSize

// FastestLapEvent contains data for the FTLP event.
// Size: 5 bytes
type FastestLapEvent struct {
	VehicleIdx uint8
	LapTime    float32
}

// RetirementEvent contains data for the RTMT event.
// Size: 2 bytes
type RetirementEvent struct {
	VehicleIdx uint8
	Reason     uint8
}

// SpeedTrapEvent contains data for the SPTP event.
// Size: 12 bytes
type SpeedTrapEvent struct {
	VehicleIdx              uint8
	Speed                   float32
	OverallFastestInSession uint8
	DriverFastestInSession  uint8
	FastestVehicleIdx       uint8
	FastestSpeedInSession   float32
}

// PenaltyEvent contains data for the PENA event.
// Size: 7 bytes
type PenaltyEvent struct {
	PenaltyType     uint8
	InfringementType uint8
	VehicleIdx      uint8
	OtherVehicleIdx uint8
	Time            uint8
	LapNum          uint8
	PlacesGained    uint8
}

// StartLightsEvent contains data for the STLG event.
// Size: 1 byte
type StartLightsEvent struct {
	NumLights uint8
}

// OvertakeEvent contains data for the OVTK event.
// Size: 2 bytes
type OvertakeEvent struct {
	OvertakingVehicleIdx    uint8
	BeingOvertakenVehicleIdx uint8
}

// SafetyCarEvent contains data for the SAFC event.
// Size: 2 bytes
type SafetyCarEvent struct {
	SafetyCarType uint8
	EventType     uint8
}

// CollisionEvent contains data for the COLL event.
// Size: 2 bytes
type CollisionEvent struct {
	Vehicle1Idx uint8
	Vehicle2Idx uint8
}

// FlashbackEvent contains data for the FLBK event.
// Size: 8 bytes
type FlashbackEvent struct {
	FlashbackFrameID    uint32
	FlashbackSessionTime float32
}

// ButtonsEvent contains data for the BUTN event.
// Size: 4 bytes
type ButtonsEvent struct {
	ButtonStatus uint32
}

// DRSDisabledEvent contains data for the DRSD event.
// Size: 1 byte
type DRSDisabledEvent struct {
	Reason uint8
}

// StopGoPenaltyServedEvent contains data for the SGSV event.
// Size: 5 bytes
type StopGoPenaltyServedEvent struct {
	VehicleIdx uint8
	StopTime   float32
}

// PacketEventData contains event data.
// Packet ID: 3
// Size: 45 bytes
type PacketEventData struct {
	Header          PacketHeader
	EventStringCode [4]uint8
	EventDetails    [12]uint8
}

// GetEventCode returns the event code as an EventCode string.
func (p *PacketEventData) GetEventCode() EventCode {
	return EventCode(string(p.EventStringCode[:]))
}

// ParseEventData parses a PacketEventData from raw bytes.
func ParseEventData(data []byte) (*PacketEventData, error) {
	if len(data) < PacketEventDataSize {
		return nil, fmt.Errorf("packet too small for event data: got %d bytes, need %d", len(data), PacketEventDataSize)
	}
	var p PacketEventData
	r := bytes.NewReader(data[:PacketEventDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse event data: %w", err)
	}
	return &p, nil
}

// ParseEventDetails parses the raw event detail bytes into the appropriate struct
// based on the event code.
func ParseEventDetails(code EventCode, data [12]uint8) (interface{}, error) {
	r := bytes.NewReader(data[:])
	switch code {
	case EventFastestLap:
		var e FastestLapEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse fastest lap event: %w", err)
		}
		return &e, nil

	case EventRetirement:
		var e RetirementEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse retirement event: %w", err)
		}
		return &e, nil

	case EventSpeedTrap:
		var e SpeedTrapEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse speed trap event: %w", err)
		}
		return &e, nil

	case EventPenaltyIssued:
		var e PenaltyEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse penalty event: %w", err)
		}
		return &e, nil

	case EventStartLights:
		var e StartLightsEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse start lights event: %w", err)
		}
		return &e, nil

	case EventOvertake:
		var e OvertakeEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse overtake event: %w", err)
		}
		return &e, nil

	case EventSafetyCar:
		var e SafetyCarEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse safety car event: %w", err)
		}
		return &e, nil

	case EventCollision:
		var e CollisionEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse collision event: %w", err)
		}
		return &e, nil

	case EventFlashback:
		var e FlashbackEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse flashback event: %w", err)
		}
		return &e, nil

	case EventButtons:
		var e ButtonsEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse buttons event: %w", err)
		}
		return &e, nil

	case EventDRSDisabled:
		var e DRSDisabledEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse DRS disabled event: %w", err)
		}
		return &e, nil

	case EventStopGoServed:
		var e StopGoPenaltyServedEvent
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, fmt.Errorf("failed to parse stop go penalty served event: %w", err)
		}
		return &e, nil

	case EventSessionStarted, EventSessionEnded, EventDRSEnabled,
		EventTeamMateInPits, EventChequeredFlag, EventRaceWinner,
		EventLightsOut, EventDriveThroughServed:
		// These events have no additional detail data or use only vehicleIdx.
		// Return nil detail for events with no payload.
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown event code: %s", string(code))
	}
}
