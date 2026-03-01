"""
F1 25 Packet Type Pydantic Models.
All 16 packet types mirroring the F1 25 UDP specification.
"""

from typing import List, Optional
from pydantic import BaseModel, Field


# --- Packet Header (shared across all packets) ---


class PacketHeader(BaseModel):
    packet_format: int = 2025
    game_year: int = 25
    game_major_version: int = 1
    game_minor_version: int = 0
    packet_version: int = 1
    packet_id: int = 0
    session_uid: int = 0
    session_time: float = 0.0
    frame_identifier: int = 0
    overall_frame_identifier: int = 0
    player_car_index: int = 0
    secondary_player_car_index: int = 255


# --- Packet 0: Motion Data ---


class CarMotionData(BaseModel):
    world_position_x: float = 0.0
    world_position_y: float = 0.0
    world_position_z: float = 0.0
    world_velocity_x: float = 0.0
    world_velocity_y: float = 0.0
    world_velocity_z: float = 0.0
    world_forward_dir_x: int = 0
    world_forward_dir_y: int = 0
    world_forward_dir_z: int = 0
    world_right_dir_x: int = 0
    world_right_dir_y: int = 0
    world_right_dir_z: int = 0
    g_force_lateral: float = 0.0
    g_force_longitudinal: float = 0.0
    g_force_vertical: float = 1.0
    yaw: float = 0.0
    pitch: float = 0.0
    roll: float = 0.0


class PacketMotionData(BaseModel):
    header: PacketHeader
    car_motion_data: List[CarMotionData]  # 22 cars


# --- Packet 1: Session Data ---


class MarshalZone(BaseModel):
    zone_start: float = 0.0
    zone_flag: int = 0


class WeatherForecastSample(BaseModel):
    session_type: int = 0
    time_offset: int = 0
    weather: int = 0
    track_temperature: int = 30
    track_temperature_change: int = 0
    air_temperature: int = 25
    air_temperature_change: int = 0
    rain_percentage: int = 0


class PacketSessionData(BaseModel):
    header: PacketHeader
    weather: int = 0
    track_temperature: int = 35
    air_temperature: int = 25
    total_laps: int = 57
    track_length: int = 5412
    session_type: int = 10
    track_id: int = 0
    formula: int = 0
    session_time_left: int = 3600
    session_duration: int = 7200
    pit_speed_limit: int = 80
    game_paused: int = 0
    is_spectating: int = 0
    num_marshal_zones: int = 0
    marshal_zones: List[MarshalZone] = Field(default_factory=list)
    safety_car_status: int = 0
    network_game: int = 0
    num_weather_forecast_samples: int = 0
    weather_forecast_samples: List[WeatherForecastSample] = Field(default_factory=list)
    forecast_accuracy: int = 0
    ai_difficulty: int = 90
    pit_stop_window_ideal_lap: int = 0
    pit_stop_window_latest_lap: int = 0
    pit_stop_rejoin_position: int = 0
    num_safety_car_periods: int = 0
    num_virtual_safety_car_periods: int = 0
    num_red_flag_periods: int = 0
    sector2_lap_distance_start: float = 0.0
    sector3_lap_distance_start: float = 0.0


# --- Packet 2: Lap Data ---


class CarLapData(BaseModel):
    last_lap_time_in_ms: int = 0
    current_lap_time_in_ms: int = 0
    sector1_time_in_ms: int = 0
    sector1_time_minutes: int = 0
    sector2_time_in_ms: int = 0
    sector2_time_minutes: int = 0
    delta_to_car_in_front_in_ms: int = 0
    delta_to_race_leader_in_ms: int = 0
    lap_distance: float = 0.0
    total_distance: float = 0.0
    safety_car_delta: float = 0.0
    car_position: int = 1
    current_lap_num: int = 1
    pit_status: int = 0
    num_pit_stops: int = 0
    sector: int = 0
    current_lap_invalid: int = 0
    penalties: int = 0
    total_warnings: int = 0
    corner_cutting_warnings: int = 0
    num_unserved_drive_through_pens: int = 0
    num_unserved_stop_go_pens: int = 0
    grid_position: int = 1
    driver_status: int = 4
    result_status: int = 2
    pit_lane_timer_active: int = 0
    pit_lane_time_in_lane_in_ms: int = 0
    pit_stop_timer_in_ms: int = 0
    pit_stop_should_serve_pen: int = 0
    speed_trap_fastest_speed: float = 0.0
    speed_trap_fastest_lap: int = 0


class PacketLapData(BaseModel):
    header: PacketHeader
    car_lap_data: List[CarLapData]  # 22 cars


# --- Packet 3: Event Data ---


class EventDataDetails(BaseModel):
    vehicle_idx: Optional[int] = None
    lap_time: Optional[float] = None
    speed: Optional[float] = None
    penalty_type: Optional[int] = None
    infringement_type: Optional[int] = None
    other_vehicle_idx: Optional[int] = None
    time: Optional[int] = None
    lap_num: Optional[int] = None
    places_gained: Optional[int] = None
    num_lights: Optional[int] = None
    overtaking_vehicle_idx: Optional[int] = None
    being_overtaken_vehicle_idx: Optional[int] = None
    safety_car_type: Optional[int] = None
    event_type: Optional[int] = None


class PacketEventData(BaseModel):
    header: PacketHeader
    event_string_code: str = ""
    event_details: EventDataDetails = Field(default_factory=EventDataDetails)


# --- Packet 4: Participants Data ---


class ParticipantData(BaseModel):
    ai_controlled: int = 1
    driver_id: int = 0
    network_id: int = 255
    team_id: int = 0
    my_team: int = 0
    race_number: int = 0
    nationality: int = 0
    name: str = ""
    your_telemetry: int = 1
    platform: int = 0


class PacketParticipantsData(BaseModel):
    header: PacketHeader
    num_active_cars: int = 20
    participants: List[ParticipantData]  # 22 cars


# --- Packet 5: Car Setup Data ---


class CarSetup(BaseModel):
    front_wing: int = 5
    rear_wing: int = 5
    on_throttle: int = 75
    off_throttle: int = 50
    front_camber: float = -3.0
    rear_camber: float = -1.5
    front_toe: float = 0.05
    rear_toe: float = 0.20
    front_suspension: int = 5
    rear_suspension: int = 5
    front_anti_roll_bar: int = 5
    rear_anti_roll_bar: int = 5
    front_suspension_height: int = 3
    rear_suspension_height: int = 5
    brake_pressure: int = 90
    brake_bias: int = 56
    engine_braking: int = 50
    front_left_tyre_pressure: float = 23.5
    front_right_tyre_pressure: float = 23.5
    rear_left_tyre_pressure: float = 22.0
    rear_right_tyre_pressure: float = 22.0
    ballast: int = 0
    fuel_load: float = 100.0


class PacketCarSetupData(BaseModel):
    header: PacketHeader
    car_setups: List[CarSetup]  # 22 cars


# --- Packet 6: Car Telemetry Data ---


class CarTelemetry(BaseModel):
    speed: int = 0
    throttle: float = 0.0
    steer: float = 0.0
    brake: float = 0.0
    clutch: int = 0
    gear: int = 0
    engine_rpm: int = 0
    drs: int = 0
    rev_lights_percent: int = 0
    rev_lights_bit_value: int = 0
    brakes_temperature: List[int] = Field(default_factory=lambda: [200, 200, 200, 200])
    tyres_surface_temperature: List[int] = Field(
        default_factory=lambda: [100, 100, 100, 100]
    )
    tyres_inner_temperature: List[int] = Field(
        default_factory=lambda: [100, 100, 100, 100]
    )
    engine_temperature: int = 100
    tyres_pressure: List[float] = Field(
        default_factory=lambda: [23.5, 23.5, 22.0, 22.0]
    )
    surface_type: List[int] = Field(default_factory=lambda: [0, 0, 0, 0])


class PacketCarTelemetryData(BaseModel):
    header: PacketHeader
    car_telemetry_data: List[CarTelemetry]  # 22 cars
    mfd_panel_index: int = 255
    mfd_panel_index_secondary_player: int = 255
    suggested_gear: int = 0


# --- Packet 7: Car Status Data ---


class CarStatus(BaseModel):
    traction_control: int = 0
    anti_lock_brakes: int = 0
    fuel_mix: int = 1
    front_brake_bias: int = 56
    pit_limiter_status: int = 0
    fuel_in_tank: float = 100.0
    fuel_capacity: float = 110.0
    fuel_remaining_laps: float = 57.0
    max_rpm: int = 13000
    idle_rpm: int = 3500
    max_gears: int = 8
    drs_allowed: int = 0
    drs_activation_distance: int = 0
    actual_tyre_compound: int = 18
    visual_tyre_compound: int = 18
    tyres_age_laps: int = 0
    vehicle_fia_flags: int = 0
    engine_power_ice: float = 750.0
    engine_power_mguk: float = 120.0
    ers_store_energy: float = 4000000.0
    ers_deploy_mode: int = 1
    ers_harvested_this_lap_mguk: float = 0.0
    ers_harvested_this_lap_mguh: float = 0.0
    ers_deployed_this_lap: float = 0.0
    network_paused: int = 0


class PacketCarStatusData(BaseModel):
    header: PacketHeader
    car_status_data: List[CarStatus]  # 22 cars


# --- Packet 8: Final Classification Data ---


class FinalClassification(BaseModel):
    position: int = 0
    num_laps: int = 0
    grid_position: int = 0
    points: int = 0
    num_pit_stops: int = 0
    result_status: int = 0
    result_reason: int = 0
    best_lap_time_in_ms: int = 0
    total_race_time: float = 0.0
    penalties_time: int = 0
    num_penalties: int = 0
    num_tyre_stints: int = 0
    tyre_stints_actual: List[int] = Field(default_factory=list)
    tyre_stints_visual: List[int] = Field(default_factory=list)
    tyre_stints_end_laps: List[int] = Field(default_factory=list)


class PacketFinalClassificationData(BaseModel):
    header: PacketHeader
    num_cars: int = 0
    classification_data: List[FinalClassification] = Field(default_factory=list)


# --- Packet 9: Lobby Info Data ---


class LobbyInfo(BaseModel):
    ai_controlled: int = 1
    team_id: int = 0
    nationality: int = 0
    platform: int = 0
    name: str = ""
    car_number: int = 0
    ready_status: int = 0


class PacketLobbyInfoData(BaseModel):
    header: PacketHeader
    num_players: int = 0
    lobby_players: List[LobbyInfo] = Field(default_factory=list)


# --- Packet 10: Car Damage Data ---


class CarDamage(BaseModel):
    tyres_wear: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    tyres_damage: List[int] = Field(default_factory=lambda: [0, 0, 0, 0])
    tyre_blisters: List[int] = Field(default_factory=lambda: [0, 0, 0, 0])
    brakes_damage: List[int] = Field(default_factory=lambda: [0, 0, 0, 0])
    front_left_wing_damage: int = 0
    front_right_wing_damage: int = 0
    rear_wing_damage: int = 0
    floor_damage: int = 0
    diffuser_damage: int = 0
    sidepod_damage: int = 0
    drs_fault: int = 0
    ers_fault: int = 0
    gear_box_damage: int = 0
    engine_damage: int = 0
    engine_mguh_wear: int = 0
    engine_es_wear: int = 0
    engine_ce_wear: int = 0
    engine_ice_wear: int = 0
    engine_mguk_wear: int = 0
    engine_tc_wear: int = 0
    engine_blown: int = 0
    engine_seized: int = 0


class PacketCarDamageData(BaseModel):
    header: PacketHeader
    car_damage_data: List[CarDamage]  # 22 cars


# --- Packet 11: Session History Data ---


class LapHistoryData(BaseModel):
    lap_time_in_ms: int = 0
    sector1_time_in_ms: int = 0
    sector1_time_minutes: int = 0
    sector2_time_in_ms: int = 0
    sector2_time_minutes: int = 0
    sector3_time_in_ms: int = 0
    sector3_time_minutes: int = 0
    lap_valid_bit_flags: int = 0


class TyreStintHistoryData(BaseModel):
    end_lap: int = 0
    tyre_actual_compound: int = 18
    tyre_visual_compound: int = 18


class PacketSessionHistoryData(BaseModel):
    header: PacketHeader
    car_idx: int = 0
    num_laps: int = 0
    num_tyre_stints: int = 0
    best_lap_time_lap_num: int = 0
    best_sector1_lap_num: int = 0
    best_sector2_lap_num: int = 0
    best_sector3_lap_num: int = 0
    lap_history_data: List[LapHistoryData] = Field(default_factory=list)
    tyre_stint_history_data: List[TyreStintHistoryData] = Field(default_factory=list)


# --- Packet 12: Tyre Sets Data ---


class TyreSetData(BaseModel):
    actual_tyre_compound: int = 18
    visual_tyre_compound: int = 18
    wear: int = 0
    available: int = 1
    recommended_session: int = 0
    life_span: int = 100
    usable_life: int = 100
    lap_delta_time: int = 0
    fitted: int = 0


class PacketTyreSetsData(BaseModel):
    header: PacketHeader
    car_idx: int = 0
    fitted_idx: int = 0
    tyre_set_data: List[TyreSetData] = Field(default_factory=list)  # 20 sets


# --- Packet 13: Motion Ex Data (player car only) ---


class PacketMotionExData(BaseModel):
    header: PacketHeader
    suspension_position: List[float] = Field(
        default_factory=lambda: [0.0, 0.0, 0.0, 0.0]
    )
    suspension_velocity: List[float] = Field(
        default_factory=lambda: [0.0, 0.0, 0.0, 0.0]
    )
    suspension_acceleration: List[float] = Field(
        default_factory=lambda: [0.0, 0.0, 0.0, 0.0]
    )
    wheel_speed: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_slip_ratio: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_slip_angle: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_lat_force: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_long_force: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_vert_force: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_camber: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    wheel_camber_gain: List[float] = Field(default_factory=lambda: [0.0, 0.0, 0.0, 0.0])
    height_of_cog_above_ground: float = 0.3
    front_aero_height: float = 0.05
    rear_aero_height: float = 0.08
    front_roll_angle: float = 0.0
    rear_roll_angle: float = 0.0
    chassis_yaw: float = 0.0
    local_velocity_x: float = 0.0
    local_velocity_y: float = 0.0
    local_velocity_z: float = 0.0
    angular_velocity_x: float = 0.0
    angular_velocity_y: float = 0.0
    angular_velocity_z: float = 0.0
    angular_acceleration_x: float = 0.0
    angular_acceleration_y: float = 0.0
    angular_acceleration_z: float = 0.0
    front_wheels_angle: float = 0.0


# --- Packet 14: Time Trial Data ---


class TimeTrialDataSet(BaseModel):
    car_idx: int = 0
    team_id: int = 0
    lap_time_in_ms: int = 0
    sector1_time_in_ms: int = 0
    sector2_time_in_ms: int = 0
    sector3_time_in_ms: int = 0
    traction_control: int = 0
    gearbox_assist: int = 0
    anti_lock_brakes: int = 0
    equal_car_performance: int = 0
    custom_setup: int = 0
    valid: int = 0


class PacketTimeTrialData(BaseModel):
    header: PacketHeader
    player_session: TimeTrialDataSet = Field(default_factory=TimeTrialDataSet)
    personal_best: TimeTrialDataSet = Field(default_factory=TimeTrialDataSet)
    rival: TimeTrialDataSet = Field(default_factory=TimeTrialDataSet)


# --- Packet 15: Lap Positions ---


class PacketLapPositions(BaseModel):
    header: PacketHeader
    num_laps: int = 0
    lap_start: int = 0
    position_for_vehicle_idx: List[List[int]] = Field(
        default_factory=list
    )  # [50 laps][22 cars]
