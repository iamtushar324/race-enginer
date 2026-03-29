package models

// RaceState is the complete live snapshot of the current telemetry state.
// Updated atomically on every parsed packet. The insight engine reads this
// (never DuckDB) for sub-microsecond evaluations.
type RaceState struct {
	// Header info
	SessionUID     uint64  `json:"session_uid"`
	FrameID        uint32  `json:"frame_id"`
	PlayerCarIndex uint8   `json:"player_car_index"`
	SessionTime    float32 `json:"session_time"`

	// Car telemetry (player car)
	Speed      uint16  `json:"speed"`       // km/h
	Throttle   float32 `json:"throttle"`    // 0.0–1.0
	Brake      float32 `json:"brake"`       // 0.0–1.0
	Steering   float32 `json:"steering"`    // -1.0 to 1.0
	Gear       int8    `json:"gear"`        // -1=R, 0=N, 1-8
	EngineRPM  uint16  `json:"engine_rpm"`
	DRS        uint8   `json:"drs"`
	EngineTemp uint16  `json:"engine_temperature"`

	// Tire temperatures (RL, RR, FL, FR)
	BrakesTemp      [4]uint16  `json:"brakes_temp"`
	TyresSurfTemp   [4]uint8   `json:"tyres_surface_temp"`
	TyresInnerTemp  [4]uint8   `json:"tyres_inner_temp"`
	TyresPressure   [4]float32 `json:"tyres_pressure"`
	SuggestedGear   int8       `json:"suggested_gear"`

	// Tire wear & damage (RL, RR, FL, FR)
	TyresWear   [4]float32 `json:"tyres_wear"`   // percentage
	TyresDamage [4]uint8   `json:"tyres_damage"` // percentage
	BrakesDmg   [4]uint8   `json:"brakes_damage"`

	// Aero/body damage
	FrontLeftWingDmg  uint8 `json:"front_left_wing_damage"`
	FrontRightWingDmg uint8 `json:"front_right_wing_damage"`
	RearWingDmg       uint8 `json:"rear_wing_damage"`
	FloorDmg          uint8 `json:"floor_damage"`
	DiffuserDmg       uint8 `json:"diffuser_damage"`
	SidepodDmg        uint8 `json:"sidepod_damage"`
	GearBoxDmg        uint8 `json:"gear_box_damage"`
	EngineDmg         uint8 `json:"engine_damage"`
	DRSFault          uint8 `json:"drs_fault"`
	ERSFault          uint8 `json:"ers_fault"`

	// Lap data (player car)
	CurrentLap        uint8   `json:"current_lap"`
	Position          uint8   `json:"position"`
	Sector            uint8   `json:"sector"`           // 0,1,2
	LapDistance       float32 `json:"lap_distance"`     // metres
	TotalDistance     float32 `json:"total_distance"`
	LastLapTimeMs     uint32  `json:"last_lap_time_ms"`
	CurrentLapTimeMs  uint32  `json:"current_lap_time_ms"`
	PitStatus         uint8   `json:"pit_status"`       // 0=none, 1=pitting, 2=in pit
	NumPitStops       uint8   `json:"num_pit_stops"`
	GridPosition      uint8   `json:"grid_position"`
	DriverStatus      uint8   `json:"driver_status"`
	DeltaToFrontMs    int32   `json:"delta_to_front_ms"`
	DeltaToLeaderMs   int32   `json:"delta_to_leader_ms"`

	// Fuel & ERS
	FuelMix           uint8   `json:"fuel_mix"`
	FuelInTank        float32 `json:"fuel_in_tank"`
	FuelRemainingLaps float32 `json:"fuel_remaining_laps"`
	ERSStoreEnergy    float32 `json:"ers_store_energy"`    // Joules (4MJ max)
	ERSDeployMode     uint8   `json:"ers_deploy_mode"`
	ERSHarvestedMGUK  float32 `json:"ers_harvested_mguk"`
	ERSHarvestedMGUH  float32 `json:"ers_harvested_mguh"`
	ERSDeployedLap    float32 `json:"ers_deployed_this_lap"`
	ActualCompound    uint8   `json:"actual_tyre_compound"`
	VisualCompound    uint8   `json:"visual_tyre_compound"`
	TyresAgeLaps      uint8   `json:"tyres_age_laps"`
	DRSAllowed        uint8   `json:"drs_allowed"`
	VehicleFIAFlags   int8    `json:"vehicle_fia_flags"`

	// Session
	Weather           uint8   `json:"weather"`
	TrackTemp         int8    `json:"track_temperature"`
	AirTemp           int8    `json:"air_temperature"`
	TotalLaps         uint8   `json:"total_laps"`
	TrackLength       uint16  `json:"track_length"`
	SessionType       uint8   `json:"session_type"`
	TrackID           int8    `json:"track_id"`
	SessionTimeLeft   uint16  `json:"session_time_left"`
	SafetyCarStatus   uint8   `json:"safety_car_status"`
	RainPercentage    uint8   `json:"rain_percentage"`      // nearest forecast
	PitWindowIdeal    uint8   `json:"pit_window_ideal_lap"`
	PitWindowLatest   uint8   `json:"pit_window_latest_lap"`

	// Weather forecast (first 5 samples for quick insight checks)
	WeatherForecasts [5]WeatherSample `json:"weather_forecasts"`

	// Motion (player car)
	WorldPosX       float32 `json:"world_pos_x"`
	WorldPosY       float32 `json:"world_pos_y"`
	WorldPosZ       float32 `json:"world_pos_z"`
	GForceLateral   float32 `json:"g_force_lateral"`
	GForceLongitude float32 `json:"g_force_longitudinal"`
	GForceVertical  float32 `json:"g_force_vertical"`
	Yaw             float32 `json:"yaw"`
	Pitch           float32 `json:"pitch"`
	Roll            float32 `json:"roll"`

	// Latest event
	LastEventCode    string `json:"last_event_code"`
	LastEventIdx     uint8  `json:"last_event_vehicle_idx"`
	LastButtonStatus uint32 `json:"last_button_status"` // bitmask from BUTN events
}

// WeatherSample is a compact forecast entry for the insight engine.
type WeatherSample struct {
	TimeOffset     uint8 `json:"time_offset"`     // minutes ahead
	Weather        uint8 `json:"weather"`          // 0=clear..5=storm
	RainPercentage uint8 `json:"rain_percentage"`
}
