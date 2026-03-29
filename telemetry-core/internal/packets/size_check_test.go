package packets

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// binarySize returns the number of bytes binary.Read will consume for a given struct.
// This reflects the packed wire format, not Go's in-memory layout with alignment padding.
func binarySize(v interface{}) int {
	return binary.Size(v)
}

func TestStructWireSizes(t *testing.T) {
	checks := []struct {
		name     string
		value    interface{}
		expected int
	}{
		{"PacketHeader", PacketHeader{}, HeaderSize},
		{"CarMotionData", CarMotionData{}, CarMotionDataSize},
		{"PacketMotionData", PacketMotionData{}, PacketMotionDataSize},
		{"MarshalZone", MarshalZone{}, MarshalZoneSize},
		{"WeatherForecastSample", WeatherForecastSample{}, WeatherForecastSampleSize},
		{"PacketSessionData", PacketSessionData{}, PacketSessionDataSize},
		{"LapData", LapData{}, LapDataSize},
		{"PacketLapData", PacketLapData{}, PacketLapDataSize},
		{"PacketEventData", PacketEventData{}, PacketEventDataSize},
		{"ParticipantData", ParticipantData{}, ParticipantDataSize},
		{"PacketParticipantsData", PacketParticipantsData{}, PacketParticipantsDataSize},
		{"CarSetupData", CarSetupData{}, CarSetupDataSize},
		{"PacketCarSetupData", PacketCarSetupData{}, PacketCarSetupDataSize},
		{"CarTelemetryData", CarTelemetryData{}, CarTelemetryDataSize},
		{"PacketCarTelemetryData", PacketCarTelemetryData{}, PacketCarTelemetryDataSize},
		{"CarStatusData", CarStatusData{}, CarStatusDataSize},
		{"PacketCarStatusData", PacketCarStatusData{}, PacketCarStatusDataSize},
		{"FinalClassificationData", FinalClassificationData{}, FinalClassificationDataSize},
		{"PacketFinalClassificationData", PacketFinalClassificationData{}, PacketFinalClassificationDataSize},
		{"LobbyInfoData", LobbyInfoData{}, LobbyInfoDataSize},
		{"PacketLobbyInfoData", PacketLobbyInfoData{}, PacketLobbyInfoDataSize},
		{"CarDamageData", CarDamageData{}, CarDamageDataSize},
		{"PacketCarDamageData", PacketCarDamageData{}, PacketCarDamageDataSize},
		{"LapHistoryData", LapHistoryData{}, LapHistoryDataSize},
		{"TyreStintHistoryData", TyreStintHistoryData{}, TyreStintHistoryDataSize},
		{"PacketSessionHistoryData", PacketSessionHistoryData{}, PacketSessionHistoryDataSize},
		{"TyreSetData", TyreSetData{}, TyreSetDataSize},
		{"PacketTyreSetsData", PacketTyreSetsData{}, PacketTyreSetsDataSize},
		{"PacketMotionExData", PacketMotionExData{}, PacketMotionExDataSize},
		{"TimeTrialDataSet", TimeTrialDataSet{}, TimeTrialDataSetSize},
		{"PacketTimeTrialData", PacketTimeTrialData{}, PacketTimeTrialDataSize},
		{"PacketLapPositions", PacketLapPositions{}, PacketLapPositionsSize},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			wireSize := binarySize(c.value)
			if wireSize != c.expected {
				t.Errorf("%s: binary.Size=%d, expected=%d", c.name, wireSize, c.expected)
			}
		})
	}
}

// TestRoundTripHeader verifies that writing and reading a header produces identical results.
func TestRoundTripHeader(t *testing.T) {
	original := PacketHeader{
		PacketFormat:            2025,
		GameYear:                25,
		GameMajorVersion:        1,
		GameMinorVersion:        3,
		PacketVersion:           1,
		PacketID:                6,
		SessionUID:              0xDEADBEEFCAFE1234,
		SessionTime:             123.456,
		FrameIdentifier:         999,
		OverallFrameID:          10000,
		PlayerCarIndex:          0,
		SecondaryPlayerCarIndex: 255,
	}

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, &original); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	if buf.Len() != HeaderSize {
		t.Fatalf("written size=%d, expected=%d", buf.Len(), HeaderSize)
	}

	parsed, err := ParseHeader(buf.Bytes())
	if err != nil {
		t.Fatalf("failed to parse header: %v", err)
	}

	if *parsed != original {
		t.Errorf("parsed header does not match original:\n  got:  %+v\n  want: %+v", *parsed, original)
	}
}
