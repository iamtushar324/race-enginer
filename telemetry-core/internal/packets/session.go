package packets

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// MarshalZoneSize is the exact byte size of a MarshalZone struct.
const MarshalZoneSize = 5

// WeatherForecastSampleSize is the exact byte size of a WeatherForecastSample struct.
const WeatherForecastSampleSize = 8

// PacketSessionDataSize is the exact byte size of the full PacketSessionData.
// 29 (header) + 4 + 2 + 3 + 4 + 6 + 21*5 + 3 + 64*8 + 2 + 12 + 14 + 2 + 4
// + 5 + 3 + 25 + 12 + 8 = 753
const PacketSessionDataSize = 753

// MarshalZone contains data for a single marshal zone.
// Size: 5 bytes
type MarshalZone struct {
	ZoneStart float32
	ZoneFlag  int8
}

// WeatherForecastSample contains a single weather forecast sample.
// Size: 8 bytes
type WeatherForecastSample struct {
	SessionType     uint8
	TimeOffset      uint8
	Weather         uint8
	TrackTemp       int8
	TrackTempChange int8
	AirTemp         int8
	AirTempChange   int8
	RainPercentage  uint8
}

// PacketSessionData contains data about the current session.
// Packet ID: 1
// Size: 753 bytes
type PacketSessionData struct {
	Header                       PacketHeader
	Weather                      uint8
	TrackTemperature             int8
	AirTemperature               int8
	TotalLaps                    uint8
	TrackLength                  uint16
	SessionType                  uint8
	TrackID                      int8
	Formula                      uint8
	SessionTimeLeft              uint16
	SessionDuration              uint16
	PitSpeedLimit                uint8
	GamePaused                   uint8
	IsSpectating                 uint8
	SpectatorCarIndex            uint8
	SLIProNativeSupport          uint8
	NumMarshalZones              uint8
	MarshalZones                 [21]MarshalZone
	SafetyCarStatus              uint8
	NetworkGame                  uint8
	NumWeatherForecastSamples    uint8
	WeatherForecastSamples       [64]WeatherForecastSample
	ForecastAccuracy             uint8
	AIDifficulty                 uint8
	SeasonLinkID                 uint32
	WeekendLinkID                uint32
	SessionLinkID                uint32
	PitStopWindowIdealLap        uint8
	PitStopWindowLatestLap       uint8
	PitStopRejoinPosition        uint8
	SteeringAssist               uint8
	BrakingAssist                uint8
	GearboxAssist                uint8
	PitAssist                    uint8
	PitReleaseAssist             uint8
	ERSAssist                    uint8
	DRSAssist                    uint8
	DynamicRacingLine            uint8
	DynamicRacingLineType        uint8
	GameMode                     uint8
	RuleSet                      uint8
	TimeOfDay                    uint32
	SessionLength                uint8
	SpeedUnitsLeadPlayer         uint8
	TemperatureUnitsLeadPlayer   uint8
	SpeedUnitsSecondaryPlayer    uint8
	TemperatureUnitsSecondaryPlayer uint8
	NumSafetyCarPeriods          uint8
	NumVirtualSafetyCarPeriods   uint8
	NumRedFlagPeriods            uint8
	EqualCarPerformance          uint8
	RecoveryMode                 uint8
	FlashbackLimit               uint8
	SurfaceType                  uint8
	LowFuelMode                  uint8
	RaceStarts                   uint8
	TyreTemperature              uint8
	PitLaneTyreSim               uint8
	CarDamage                    uint8
	CarDamageRate                uint8
	Collisions                   uint8
	CollisionsOffFirstLapOnly    uint8
	MPUnsafePitRelease           uint8
	MPOffForGriefing             uint8
	CornerCuttingStringency      uint8
	ParcFermeRules               uint8
	PitStopExperience            uint8
	SafetyCar                    uint8
	SafetyCarExperience          uint8
	FormationLap                 uint8
	FormationLapExperience       uint8
	RedFlags                     uint8
	AffectsLicenceLevelSolo      uint8
	AffectsLicenceLevelMP        uint8
	NumSessionsInWeekend         uint8
	WeekendStructure             [12]uint8
	Sector2LapDistanceStart      float32
	Sector3LapDistanceStart      float32
}

// ParseSessionData parses a PacketSessionData from raw bytes.
func ParseSessionData(data []byte) (*PacketSessionData, error) {
	if len(data) < PacketSessionDataSize {
		return nil, fmt.Errorf("packet too small for session data: got %d bytes, need %d", len(data), PacketSessionDataSize)
	}
	var p PacketSessionData
	r := bytes.NewReader(data[:PacketSessionDataSize])
	if err := binary.Read(r, binary.LittleEndian, &p); err != nil {
		return nil, fmt.Errorf("failed to parse session data: %w", err)
	}
	return &p, nil
}
