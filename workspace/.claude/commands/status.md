Query the current live session state and present a concise race engineer briefing.

First, detect the player car index:
```bash
curl -s http://localhost:8081/api/telemetry/latest | jq '.player_car_index'
```

Then run these queries via `./bin/racedb` (substitute `$IDX` with the player car index):

1. `SELECT track_id, session_type, total_laps, weather, track_temperature, air_temperature, rain_percentage, safety_car_status, session_time_left FROM session_data ORDER BY timestamp DESC LIMIT 1`
2. `SELECT car_position, current_lap_num, last_lap_time_in_ms, delta_to_car_in_front_in_ms, delta_to_race_leader_in_ms, num_pit_stops, grid_position FROM lap_data WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1`
3. `SELECT fuel_in_tank, fuel_remaining_laps, actual_tyre_compound, tyres_age_laps, ers_store_energy, ers_deploy_mode, fuel_mix FROM car_status WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1`
4. `SELECT tyres_wear_fl, tyres_wear_fr, tyres_wear_rl, tyres_wear_rr FROM car_damage WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1`

Decode all integer values using the enum mappings in CLAUDE.md (track names, session types, compounds, weather, etc.).

Present as a brief radio-style status update, e.g.:
- Track & session
- Position, gap to car ahead, gap to leader
- Lap X of Y
- Tires: compound, age, wear per corner
- Fuel: kg remaining, laps of fuel
- Weather conditions
- Any flags or safety car
