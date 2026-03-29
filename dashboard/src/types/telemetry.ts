/** Mirrors Go telemetry-core/internal/models/state.go — RaceState struct */
export interface WeatherSample {
  time_offset: number;
  weather: number;
  rain_percentage: number;
}

export interface RaceState {
  // Header
  session_uid: number;
  frame_id: number;
  player_car_index: number;
  session_time: number;

  // Car telemetry
  speed: number;
  throttle: number;
  brake: number;
  steering: number;
  gear: number;
  engine_rpm: number;
  drs: number;
  engine_temperature: number;

  // Tire temperatures (RL, RR, FL, FR)
  brakes_temp: number[];
  tyres_surface_temp: number[];
  tyres_inner_temp: number[];
  tyres_pressure: number[];
  suggested_gear: number;

  // Tire wear & damage (RL, RR, FL, FR)
  tyres_wear: number[];
  tyres_damage: number[];
  brakes_damage: number[];

  // Aero/body damage
  front_left_wing_damage: number;
  front_right_wing_damage: number;
  rear_wing_damage: number;
  floor_damage: number;
  diffuser_damage: number;
  sidepod_damage: number;
  gear_box_damage: number;
  engine_damage: number;
  drs_fault: number;
  ers_fault: number;

  // Lap data
  current_lap: number;
  position: number;
  sector: number;
  lap_distance: number;
  total_distance: number;
  last_lap_time_ms: number;
  current_lap_time_ms: number;
  pit_status: number;
  num_pit_stops: number;
  grid_position: number;
  driver_status: number;
  delta_to_front_ms: number;
  delta_to_leader_ms: number;

  // Fuel & ERS
  fuel_mix: number;
  fuel_in_tank: number;
  fuel_remaining_laps: number;
  ers_store_energy: number;
  ers_deploy_mode: number;
  ers_harvested_mguk: number;
  ers_harvested_mguh: number;
  ers_deployed_this_lap: number;
  actual_tyre_compound: number;
  visual_tyre_compound: number;
  tyres_age_laps: number;
  drs_allowed: number;
  vehicle_fia_flags: number;

  // Session
  weather: number;
  track_temperature: number;
  air_temperature: number;
  total_laps: number;
  track_length: number;
  session_type: number;
  track_id: number;
  session_time_left: number;
  safety_car_status: number;
  rain_percentage: number;
  pit_window_ideal_lap: number;
  pit_window_latest_lap: number;

  // Weather forecasts
  weather_forecasts: WeatherSample[];

  // Motion
  world_pos_x: number;
  world_pos_y: number;
  world_pos_z: number;
  g_force_lateral: number;
  g_force_longitudinal: number;
  g_force_vertical: number;
  yaw: number;
  pitch: number;
  roll: number;

  // Events
  last_event_code: string;
  last_event_vehicle_idx: number;
  last_button_status: number;
}
