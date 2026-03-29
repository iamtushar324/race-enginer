Analyze the competitive picture around our car.

First, detect the player car index:
```bash
curl -s http://localhost:8081/api/telemetry/latest | jq '.player_car_index'
```

Then run these queries via `./bin/racedb` (substitute `$IDX` with the player car index):

1. Our position: `SELECT car_position, current_lap_num, last_lap_time_in_ms, delta_to_car_in_front_in_ms, delta_to_race_leader_in_ms FROM lap_data WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1`

2. All cars sorted by position: `SELECT ld.car_index, ld.car_position, ld.current_lap_num, ld.last_lap_time_in_ms, ld.delta_to_car_in_front_in_ms, ld.num_pit_stops, ld.pit_status, cs.actual_tyre_compound, cs.tyres_age_laps FROM lap_data ld JOIN car_status cs ON ld.car_index = cs.car_index WHERE ld.timestamp = (SELECT MAX(timestamp) FROM lap_data) ORDER BY ld.car_position LIMIT 20`

3. Recent lap times for cars near us (±3 positions): run a query for each nearby car_index to get their last 5 lap times.

Decode compounds (16=Soft, 17=Medium, 18=Hard, 7=Inter, 8=Wet) and pit_status (0=Track, 1=Pit Lane, 2=In Pit).

Present:
- Who is ahead and behind, their gaps, compounds, tire age
- Who recently pitted and what they switched to
- Undercut/overcut threat assessment
- Pace comparison (are they faster or slower on recent laps?)
