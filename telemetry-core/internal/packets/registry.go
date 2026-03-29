package packets

import "fmt"

// Packet ID constants matching the F1 25 UDP specification.
const (
	PacketIDMotion              uint8 = 0
	PacketIDSession             uint8 = 1
	PacketIDLapData             uint8 = 2
	PacketIDEvent               uint8 = 3
	PacketIDParticipants        uint8 = 4
	PacketIDCarSetup            uint8 = 5
	PacketIDCarTelemetry        uint8 = 6
	PacketIDCarStatus           uint8 = 7
	PacketIDFinalClassification uint8 = 8
	PacketIDLobbyInfo           uint8 = 9
	PacketIDCarDamage           uint8 = 10
	PacketIDSessionHistory      uint8 = 11
	PacketIDTyreSets            uint8 = 12
	PacketIDMotionEx            uint8 = 13
	PacketIDTimeTrial           uint8 = 14
	PacketIDLapPositions        uint8 = 15
)

// PacketSizes maps packet IDs to their expected byte sizes.
var PacketSizes = map[uint8]int{
	PacketIDMotion:              PacketMotionDataSize,
	PacketIDSession:             PacketSessionDataSize,
	PacketIDLapData:             PacketLapDataSize,
	PacketIDEvent:               PacketEventDataSize,
	PacketIDParticipants:        PacketParticipantsDataSize,
	PacketIDCarSetup:            PacketCarSetupDataSize,
	PacketIDCarTelemetry:        PacketCarTelemetryDataSize,
	PacketIDCarStatus:           PacketCarStatusDataSize,
	PacketIDFinalClassification: PacketFinalClassificationDataSize,
	PacketIDLobbyInfo:           PacketLobbyInfoDataSize,
	PacketIDCarDamage:           PacketCarDamageDataSize,
	PacketIDSessionHistory:      PacketSessionHistoryDataSize,
	PacketIDTyreSets:            PacketTyreSetsDataSize,
	PacketIDMotionEx:            PacketMotionExDataSize,
	PacketIDTimeTrial:           PacketTimeTrialDataSize,
	PacketIDLapPositions:        PacketLapPositionsSize,
}

// PacketNames maps packet IDs to human-readable names.
var PacketNames = map[uint8]string{
	PacketIDMotion:              "Motion",
	PacketIDSession:             "Session",
	PacketIDLapData:             "LapData",
	PacketIDEvent:               "Event",
	PacketIDParticipants:        "Participants",
	PacketIDCarSetup:            "CarSetup",
	PacketIDCarTelemetry:        "CarTelemetry",
	PacketIDCarStatus:           "CarStatus",
	PacketIDFinalClassification: "FinalClassification",
	PacketIDLobbyInfo:           "LobbyInfo",
	PacketIDCarDamage:           "CarDamage",
	PacketIDSessionHistory:      "SessionHistory",
	PacketIDTyreSets:            "TyreSets",
	PacketIDMotionEx:            "MotionEx",
	PacketIDTimeTrial:           "TimeTrial",
	PacketIDLapPositions:        "LapPositions",
}

// ParseAuto extracts the packet ID from the header and dispatches to the correct parser.
// It returns the parsed packet struct as an interface{} and any error encountered.
// The caller should type-assert on the returned value based on the packet ID.
func ParseAuto(data []byte) (interface{}, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("data too small to contain a header: got %d bytes, need %d", len(data), HeaderSize)
	}

	header, err := ParseHeader(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse header for dispatch: %w", err)
	}

	return Parse(header.PacketID, data)
}

// Parse dispatches to the correct parser based on the given packet ID.
// It returns the parsed packet struct as an interface{} and any error encountered.
// The caller should type-assert on the returned value based on the packet ID.
func Parse(packetID uint8, data []byte) (interface{}, error) {
	switch packetID {
	case PacketIDMotion:
		return ParseMotionData(data)
	case PacketIDSession:
		return ParseSessionData(data)
	case PacketIDLapData:
		return ParseLapData(data)
	case PacketIDEvent:
		return ParseEventData(data)
	case PacketIDParticipants:
		return ParseParticipantsData(data)
	case PacketIDCarSetup:
		return ParseCarSetupData(data)
	case PacketIDCarTelemetry:
		return ParseCarTelemetryData(data)
	case PacketIDCarStatus:
		return ParseCarStatusData(data)
	case PacketIDFinalClassification:
		return ParseFinalClassificationData(data)
	case PacketIDLobbyInfo:
		return ParseLobbyInfoData(data)
	case PacketIDCarDamage:
		return ParseCarDamageData(data)
	case PacketIDSessionHistory:
		return ParseSessionHistoryData(data)
	case PacketIDTyreSets:
		return ParseTyreSetsData(data)
	case PacketIDMotionEx:
		return ParseMotionExData(data)
	case PacketIDTimeTrial:
		return ParseTimeTrialData(data)
	case PacketIDLapPositions:
		return ParseLapPositions(data)
	default:
		return nil, fmt.Errorf("unknown packet ID: %d", packetID)
	}
}
