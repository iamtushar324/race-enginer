# Race Engineer — Agent Guide

AI-powered F1 race engineer for F1 25. Ingests real-time UDP telemetry, stores it in DuckDB, serves a React dashboard, and provides voice-driven strategic advice via Gemini.

## Architecture

```
F1 25 Game (UDP :20777)
        │
        ▼
┌─────────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Go Telemetry Core  │────▶│  React Dashboard │     │  Python Voice   │
│  (port 8081)        │ ws  │  (port 5173)     │     │  (port 8000)    │
│                     │     └─────────────────┘     └─────────────────┘
│  - UDP ingestion    │              │                       │
│  - DuckDB storage   │◀─────── REST API ──────────────────┘
│  - Gemini advisor   │
│  - Insight engine   │
└─────────────────────┘
        │
        ▼
  workspace/telemetry.duckdb
```

**3 processes** launched via `make start`:
1. **Go Telemetry Core** — UDP listener, DuckDB writer, REST API, Gemini advisor
2. **React Dashboard** — Live telemetry visualization (Vite + React 19 + Tailwind v4)
3. **Python Voice Service** — edge-tts synthesis + Whisper STT transcription for radio comms

## Querying Telemetry Data

### CLI tool: `./workspace/bin/racedb`

A read-only CLI that queries DuckDB directly. Safe to run while the server is live.

```bash
# Run a SQL query (returns JSON array)
./workspace/bin/racedb "SELECT * FROM car_damage ORDER BY timestamp DESC LIMIT 5"

# List all tables with row counts
./workspace/bin/racedb --tables

# Show full schema (all columns and types)
./workspace/bin/racedb --schema
```

Build it: `cd telemetry-core && go build -o ../workspace/bin/racedb cmd/query/main.go`

The database defaults to `workspace/telemetry.duckdb`. Override with `DB_PATH` env var.

### CLI tool: `./workspace/bin/insightlog`

Queries the running server's insight history log. Use this before publishing insights to check what's already been said.

```bash
# Show last 50 insights (default)
./workspace/bin/insightlog

# Show last 20 insights
./workspace/bin/insightlog -limit 20

# Output raw JSON (for piping to other tools)
./workspace/bin/insightlog -json

# Use a custom API URL
./workspace/bin/insightlog -api http://localhost:8081
```

Build it: `cd telemetry-core && go build -o ../workspace/bin/insightlog cmd/insightlog/main.go`

The API URL defaults to `http://localhost:8081`. Override with `-api` flag or `API_URL` env var.

### REST API query endpoint

```bash
curl -X POST http://localhost:8081/api/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT COUNT(*) as n FROM lap_data"}'
```

## DuckDB Schema

The database has 10 tables. All include `timestamp DEFAULT CURRENT_TIMESTAMP`.

### telemetry
Core driving data sampled at 1Hz.
| Column | Type |
|--------|------|
| speed | DOUBLE |
| gear | INTEGER |
| throttle | DOUBLE |
| brake | DOUBLE |
| steering | DOUBLE |
| engine_rpm | INTEGER |
| tire_wear_fl/fr/rl/rr | DOUBLE |
| lap | INTEGER |
| track_position | DOUBLE |
| sector | INTEGER |

### session_data
Track conditions and session metadata.
| Column | Type |
|--------|------|
| weather | INTEGER |
| track_temperature | INTEGER |
| air_temperature | INTEGER |
| total_laps | INTEGER |
| track_length | INTEGER |
| session_type | INTEGER |
| track_id | INTEGER |
| session_time_left | INTEGER |
| safety_car_status | INTEGER |
| rain_percentage | INTEGER |
| pit_stop_window_ideal_lap | INTEGER |
| pit_stop_window_latest_lap | INTEGER |

### lap_data
Per-car lap timing and position.
| Column | Type |
|--------|------|
| car_index | INTEGER |
| last_lap_time_in_ms | INTEGER |
| current_lap_time_in_ms | INTEGER |
| sector1_time_in_ms | INTEGER |
| sector2_time_in_ms | INTEGER |
| car_position | INTEGER |
| current_lap_num | INTEGER |
| pit_status | INTEGER |
| num_pit_stops | INTEGER |
| sector | INTEGER |
| penalties | INTEGER |
| driver_status | INTEGER |
| result_status | INTEGER |
| delta_to_car_in_front_in_ms | INTEGER |
| delta_to_race_leader_in_ms | INTEGER |
| speed_trap_fastest_speed | DOUBLE |
| grid_position | INTEGER |

### car_status
Fuel, ERS, tire compound, DRS.
| Column | Type |
|--------|------|
| car_index | INTEGER |
| fuel_mix | INTEGER |
| fuel_in_tank | DOUBLE |
| fuel_remaining_laps | DOUBLE |
| ers_store_energy | DOUBLE |
| ers_deploy_mode | INTEGER |
| ers_harvested_this_lap_mguk | DOUBLE |
| ers_harvested_this_lap_mguh | DOUBLE |
| ers_deployed_this_lap | DOUBLE |
| actual_tyre_compound | INTEGER |
| visual_tyre_compound | INTEGER |
| tyres_age_laps | INTEGER |
| drs_allowed | INTEGER |
| vehicle_fia_flags | INTEGER |
| engine_power_ice | DOUBLE |
| engine_power_mguk | DOUBLE |

### car_damage
Tire wear/damage, wing damage, engine component wear.
| Column | Type |
|--------|------|
| car_index | INTEGER |
| tyres_wear_fl/fr/rl/rr | DOUBLE |
| tyres_damage_fl/fr/rl/rr | INTEGER |
| front_left_wing_damage | INTEGER |
| front_right_wing_damage | INTEGER |
| rear_wing_damage | INTEGER |
| floor_damage | INTEGER |
| diffuser_damage | INTEGER |
| sidepod_damage | INTEGER |
| gear_box_damage | INTEGER |
| engine_damage | INTEGER |
| engine_mguh/es/ce/ice/mguk/tc_wear | INTEGER |
| drs_fault | INTEGER |
| ers_fault | INTEGER |

### car_telemetry_ext
Temperatures, pressures, DRS status.
| Column | Type |
|--------|------|
| car_index | INTEGER |
| brakes_temp_fl/fr/rl/rr | INTEGER |
| tyres_surface_temp_fl/fr/rl/rr | INTEGER |
| tyres_inner_temp_fl/fr/rl/rr | INTEGER |
| engine_temperature | INTEGER |
| tyres_pressure_fl/fr/rl/rr | DOUBLE |
| drs | INTEGER |
| clutch | INTEGER |
| suggested_gear | INTEGER |

### motion_data
3D position and G-forces.
| Column | Type |
|--------|------|
| car_index | INTEGER |
| world_position_x/y/z | DOUBLE |
| g_force_lateral | DOUBLE |
| g_force_longitudinal | DOUBLE |
| g_force_vertical | DOUBLE |
| yaw | DOUBLE |
| pitch | DOUBLE |
| roll | DOUBLE |

### race_events
Discrete events (crashes, penalties, etc.).
| Column | Type |
|--------|------|
| event_code | VARCHAR |
| vehicle_idx | INTEGER |
| detail_text | VARCHAR |

### session_history
Historical lap data per car.
| Column | Type |
|--------|------|
| car_index | INTEGER |
| lap_num | INTEGER |
| lap_time_in_ms | INTEGER |
| sector1/2/3_time_in_ms | INTEGER |
| lap_valid | INTEGER |

### raw_packets
Raw JSON dump of every packet (for debugging).
| Column | Type |
|--------|------|
| packet_id | INTEGER |
| packet_name | VARCHAR |
| session_uid | UBIGINT |
| frame_id | INTEGER |
| data | JSON |

## Detecting the Player Car Index

**IMPORTANT:** The player's `car_index` changes every session. NEVER hardcode it. Always detect it dynamically:

```bash
# From the REST API (returns the full race state including player_car_index)
curl -s http://localhost:8081/api/telemetry/latest | jq '.player_car_index'

# Or via racedb — the player_car_index is in the packet headers, stored in motion_data
# The /api/telemetry/latest endpoint is the canonical source.
```

Use the returned value in all `WHERE car_index = ...` clauses below.

## Common Analysis Queries

Replace `$IDX` with the player car index from `/api/telemetry/latest`.

```sql
-- Tire wear trend (last 20 samples)
SELECT timestamp, tyres_wear_fl, tyres_wear_fr, tyres_wear_rl, tyres_wear_rr
FROM car_damage WHERE car_index = $IDX
ORDER BY timestamp DESC LIMIT 20;

-- Lap time progression
SELECT current_lap_num, last_lap_time_in_ms / 1000.0 AS lap_time_sec,
       sector1_time_in_ms, sector2_time_in_ms
FROM lap_data WHERE car_index = $IDX AND last_lap_time_in_ms > 0
ORDER BY current_lap_num;

-- Fuel pace (fuel consumed per lap)
SELECT current_lap_num, fuel_in_tank, fuel_remaining_laps
FROM (SELECT *, LAG(fuel_in_tank) OVER (ORDER BY timestamp) AS prev_fuel FROM car_status WHERE car_index = $IDX)
WHERE prev_fuel IS NOT NULL AND fuel_in_tank < prev_fuel;

-- Weather changes
SELECT timestamp, weather, track_temperature, air_temperature, rain_percentage
FROM session_data ORDER BY timestamp DESC LIMIT 10;

-- Position changes
SELECT timestamp, car_position, delta_to_car_in_front_in_ms / 1000.0 AS gap_ahead_sec
FROM lap_data WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 20;

-- G-force extremes
SELECT MAX(ABS(g_force_lateral)) AS max_lateral_g,
       MIN(g_force_longitudinal) AS max_braking_g,
       MAX(g_force_longitudinal) AS max_accel_g
FROM motion_data WHERE car_index = $IDX;
```

## REST API Endpoints

| Method | Route | Description |
|--------|-------|-------------|
| GET | `/health` | Server health (uptime, packet count, DB status) |
| GET | `/ws` | WebSocket — live push of telemetry, insights, and health |
| GET | `/api/telemetry/latest` | Latest race state from atomic cache |
| GET | `/api/settings` | Current settings (mock mode, talk level, UDP config) |
| GET | `/api/insights/next` | Consume next pending insight |
| GET | `/api/insights/history` | Full insight log as JSON (query: `?limit=N`, default 50, max 500) |
| GET | `/api/workspace` | Returns `workspace/insights.md` as plain text |
| POST | `/api/query` | Execute read-only SQL: `{"sql": "SELECT ..."}` |
| POST | `/api/strategy` | Push strategy insight: `{"source": "...", "insight": "...", "priority": 3}` |
| POST | `/api/driver_query` | Submit driver question for Gemini: `{"query": "..."}` |
| POST | `/api/voice` | Push-to-talk: multipart audio → Whisper STT → Gemini response |
| POST | `/api/settings/mode` | Switch mock/real: `{"mode": "mock"}` |
| POST | `/api/settings/talk_level` | Set verbosity: `{"level": 5}` |

### Pushing insights

```bash
curl -X POST http://localhost:8081/api/strategy \
  -H "Content-Type: application/json" \
  -d '{
    "source": "strategy-agent",
    "insight": "Rear tire degradation accelerating. Consider boxing in 3 laps.",
    "priority": 4
  }'
```

Priority: 1 (info) to 5 (critical). Priority 4+ triggers voice alerts.

## Workspace Files

All context files live in `workspace/`:

| File | Purpose |
|------|---------|
| `driver_profile.md` | Driver name, style, communication preferences, strengths/weaknesses |
| `track_setup.md` | Current track characteristics, setup recommendations, danger zones |
| `past_learnings.md` | Historical session debriefs, tire data, incident analysis |
| `insights.md` | Live state + accumulated findings (written by the insight engine) |
| `soul.md` | Race engineer personality, radio style, communication rules |
| `user.md` | Driver communication preferences, what they want/don't want to hear |
| `telemetry.duckdb` | DuckDB database with all telemetry tables |

Note: `SOUL.md` and `USER.md` in the project root are kept for backwards compatibility. The Go code loads from `workspace/` first, then falls back to the root.

## Configuration

All via environment variables (`.env` file loaded automatically):

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_PATH` | `workspace/telemetry.duckdb` | DuckDB file path |
| `TELEMETRY_MODE` | `real` | `real` = UDP listener, `mock` = fake data |
| `TELEMETRY_HOST` | `0.0.0.0` | UDP bind host |
| `TELEMETRY_PORT` | `20777` | UDP port (F1 25 default) |
| `UDP_MODE` | `broadcast` | `broadcast` or `unicast` |
| `API_PORT` | `8081` | REST API port |
| `LLM_PROVIDER` | `gemini` | LLM backend: `gemini`, `anthropic`, `openai` |
| `LLM_MODEL` | — | Override model name (empty = provider default) |
| `GEMINI_API_KEY` | — | Google Gemini API key |
| `ANTHROPIC_API_KEY` | — | Anthropic API key (for Claude) |
| `OPENAI_API_KEY` | — | OpenAI API key |
| `VOICE_URL` | `http://localhost:8000` | Python voice service URL |
| `TTS_ENGINE` | `edge-tts` | TTS backend: `edge-tts`, `qwen-tts` (MLX voice cloning) |
| `TTS_VOICE` | `en-GB-RyanNeural` | edge-tts voice name |
| `TTS_VOICE_REF` | `workspace/voices/reference.wav` | Voice reference WAV for Qwen TTS cloning |
| `TTS_SPEED` | `1.0` | Qwen TTS speech speed multiplier |
| `STT_ENGINE` | `whisper` | STT backend: `whisper`, `parakeet` (NeMo), `resemble` (cloud) |
| `WHISPER_MODEL` | `medium` | faster-whisper model size |
| `WORKSPACE_DIR` | `workspace` | Workspace directory path |
| `TALK_LEVEL` | `5` | Insight verbosity 1-10 |
| `LOG_LEVEL` | `info` | Log level: trace, debug, info, warn, error, fatal, panic |
| `BATCH_SIZE` | `20` | DuckDB flush threshold |
| `SAMPLE_RATE` | `20` | 20Hz to 1Hz sampling |
| `WS_PUSH_RATE` | `10` | WebSocket push rate in Hz (5=200ms, 10=100ms, 20=50ms) |
| `PTT_BUTTON` | `0x00100000` | F1 BUTN bitmask for push-to-talk (UDP Action 1). Set `0` to disable |

## Intelligence Pipeline

### How Insights Flow

```
                 ┌──────────────┐
                 │  Rule Engine │ (Go, threshold-based)
                 └──────┬───────┘
                        │ raw insight
                        ▼
              ┌──────────────────┐
              │    CommsGate     │ (LLM translator)
              │  - dedup check   │
              │  - talk level    │
              │  - translate to  │
              │    radio comms   │
              └──────┬───────────┘
                     │ translated insight
         ┌───────────┼───────────┐
         ▼           ▼           ▼
   ┌──────────┐ ┌────────┐ ┌───────────┐
   │ WebSocket│ │ Voice  │ │ Workspace │
   │ → Dash   │ │ TTS    │ │ insights  │
   └──────────┘ └────────┘ └───────────┘
```

1. **Rule Engine** (`internal/insights/`) — fires insights based on telemetry thresholds (tire wear, fuel, damage, weather changes, safety car, etc.)
2. **Strategy Analyst** (`internal/intelligence/analyst.go`) — background agent that periodically queries live DuckDB data and produces strategic insights via LLM
3. **CommsGate** (`internal/intelligence/comms_gate.go`) — central gate that receives all insights, deduplicates, checks talk level, and translates to radio-style speech via LLM
4. **Fanout** — translated insights go to: WebSocket (dashboard), Voice TTS (audio), and workspace/insights.md

### Strategy Analyst

The analyst runs on a background ticker and queries the **live DuckDB** for current session data. It builds a context snapshot with:

- **Session identity**: track name, session type (Practice/Quali/Race), current lap/total laps
- **Weather**: conditions, track temp, air temp, rain percentage, safety car status
- **Pit windows**: ideal and latest pit laps from the game's strategy data
- **Our car** (dynamic `player_car_index` from live cache): position, grid position, last lap time, gaps to car ahead and leader, pit stops, pit status
- **Nearby competitors** (±3 positions): their compound, tire age, pit stops, pit status — for undercut/overcut analysis
- **Tire wear trend**: last 3 samples showing degradation rate
- **Car damage**: all components (wings, floor, diffuser, sidepod, gearbox, engine)
- **Fuel & ERS**: fuel in tank, remaining laps, compound, ERS store and deploy mode
- **Lap time progression**: last 5 lap times to detect pace changes

The analyst's LLM prompt instructs it to focus on the **current live session** — not historical data. It should identify undercut threats, tire cliff warnings, weather-triggered strategy changes, and fuel targets.

### CommsGate Translation

When CommsGate translates insights to radio speech, it receives the same rich session context via `BuildContext()`:
- Track name, session type, lap X/Y
- Position (started P-X), pit stops, gaps
- Speed, gear, RPM
- Tire compound, age, wear percentages per corner
- Fuel (kg, laps remaining, mix mode), ERS (% and deploy mode), DRS
- Damage breakdown
- Weather, temperatures, rain %, safety car
- Pit window, session time remaining

This ensures the LLM can give contextually appropriate radio messages.

### Avoiding Duplicate Insights

Before pushing an insight via `/api/strategy`, **always check recent insight history** to avoid repeating what's already been said:

```bash
# Check recent insights
./workspace/bin/insightlog -limit 20

# Or via API
curl http://localhost:8081/api/insights/history?limit=20
```

Do NOT push insights that:
- Repeat information from the last 5 minutes
- State obvious facts the driver already knows (e.g., "you are P3" when position hasn't changed)
- Contradict a more recent insight

DO push insights that:
- Alert to new developments (weather change incoming, competitor pitted, damage detected)
- Provide actionable strategy (pit window opening, tire cliff approaching, fuel target change)
- Respond to significant state changes (safety car, position change, DRS availability)

### Voice Pipeline

```
Driver speaks (push-to-talk)
        │
        ▼
   Dashboard records audio (WebM/Opus)
        │
        ▼
   POST /api/voice (multipart)
        │
        ├──→ Immediate: fire ack audio ("Copy, checking the data")
        │    (synthesized by Python voice service, broadcast via WS)
        │
        ├──→ Proxy audio to Python /transcribe (Whisper STT)
        │
        ▼
   Transcription text → CommsGate.HandleQuery()
        │
        ▼
   LLM generates response with full telemetry context
        │
        ▼
   Response → Voice TTS → WebSocket audio broadcast
```

The ack fires immediately on audio receipt (before transcription starts) so the driver hears a response within ~200ms. The LLM is instructed to NOT start with acknowledgement phrases ("Copy that", "Roger", etc.) since the system already sent one.

## Enum Value Mappings

These integer values appear in the DuckDB tables. Use them to interpret telemetry data.

### Track IDs (`session_data.track_id`)

| ID | Track | ID | Track | ID | Track |
|----|-------|----|-------|----|-------|
| 0 | Melbourne | 11 | Monza | 22 | Silverstone Short |
| 1 | Paul Ricard | 12 | Singapore | 23 | Austin Short |
| 2 | Shanghai | 13 | Suzuka | 24 | Suzuka Short |
| 3 | Bahrain | 14 | Abu Dhabi | 25 | Hanoi |
| 4 | Catalunya | 15 | Austin | 26 | Zandvoort |
| 5 | Monaco | 16 | Interlagos | 27 | Imola |
| 6 | Montreal | 17 | Red Bull Ring | 28 | Portimao |
| 7 | Silverstone | 18 | Sochi | 29 | Jeddah |
| 8 | Hockenheim | 19 | Mexico City | 30 | Miami |
| 9 | Hungaroring | 20 | Baku | 31 | Las Vegas |
| 10 | Spa | 21 | Sakhir Short | 32 | Losail |

### Session Types (`session_data.session_type`)

| Value | Type | Value | Type |
|-------|------|-------|------|
| 0 | Unknown | 7 | Q3 |
| 1 | P1 | 8 | Short Q |
| 2 | P2 | 9 | OSQ |
| 3 | P3 | 10 | Race |
| 4 | Short P | 11 | Race 2 |
| 5 | Q1 | 12 | Race 3 |
| 6 | Q2 | 13 | Time Trial |

### Tire Compounds (`car_status.actual_tyre_compound`)

| Value | Compound |
|-------|----------|
| 16 | Soft |
| 17 | Medium |
| 18 | Hard |
| 7 | Inter |
| 8 | Wet |

### Weather (`session_data.weather`)

| Value | Condition |
|-------|-----------|
| 0 | Clear |
| 1 | Light Cloud |
| 2 | Overcast |
| 3 | Light Rain |
| 4 | Heavy Rain |
| 5 | Storm |

### Fuel Mix (`car_status.fuel_mix`)

| Value | Mode |
|-------|------|
| 0 | Lean |
| 1 | Standard |
| 2 | Rich |
| 3 | Max |

### ERS Deploy Mode (`car_status.ers_deploy_mode`)

| Value | Mode |
|-------|------|
| 0 | None |
| 1 | Medium |
| 2 | Hotlap |
| 3 | Overtake |

### Safety Car (`session_data.safety_car_status`)

| Value | Status |
|-------|--------|
| 0 | None |
| 1 | Full Safety Car |
| 2 | Virtual Safety Car |
| 3 | Formation Lap |

### Pit Status (`lap_data.pit_status`)

| Value | Status |
|-------|--------|
| 0 | On Track |
| 1 | Pit Lane |
| 2 | In Pit Area |

### Driver Status (`lap_data.driver_status`)

| Value | Status |
|-------|--------|
| 0 | In Garage |
| 1 | Flying Lap |
| 2 | In Lap |
| 3 | Out Lap |
| 4 | On Track |

## Advanced Analysis Queries

Replace `$IDX` with the player car index from `/api/telemetry/latest`.

```sql
-- Session identity: what track and session are we in?
SELECT track_id, session_type, total_laps, weather, track_temperature,
       air_temperature, rain_percentage, safety_car_status,
       pit_stop_window_ideal_lap, pit_stop_window_latest_lap,
       session_time_left
FROM session_data ORDER BY timestamp DESC LIMIT 1;

-- Our car's current state
SELECT car_position, current_lap_num, last_lap_time_in_ms,
       delta_to_car_in_front_in_ms, delta_to_race_leader_in_ms,
       num_pit_stops, pit_status, grid_position
FROM lap_data WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1;

-- Nearby competitors (within ±3 positions of us)
SELECT ld.car_index, ld.car_position, ld.current_lap_num,
       ld.last_lap_time_in_ms, ld.num_pit_stops, ld.pit_status,
       cs.actual_tyre_compound, cs.tyres_age_laps
FROM lap_data ld
JOIN car_status cs ON ld.car_index = cs.car_index
WHERE ld.car_position BETWEEN
  (SELECT car_position - 3 FROM lap_data WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1) AND
  (SELECT car_position + 3 FROM lap_data WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1)
AND ld.timestamp = (SELECT MAX(timestamp) FROM lap_data)
ORDER BY ld.car_position;

-- Tire degradation rate (wear change over last 5 samples)
SELECT timestamp, tyres_wear_fl, tyres_wear_fr, tyres_wear_rl, tyres_wear_rr
FROM car_damage WHERE car_index = $IDX
ORDER BY timestamp DESC LIMIT 5;

-- Fuel burn rate per lap
SELECT current_lap_num, fuel_in_tank, fuel_remaining_laps,
       LAG(fuel_in_tank) OVER (ORDER BY timestamp) - fuel_in_tank AS fuel_per_lap
FROM car_status WHERE car_index = $IDX
ORDER BY timestamp DESC LIMIT 10;

-- Brake and tire temperatures (overheating detection)
SELECT brakes_temp_fl, brakes_temp_fr, brakes_temp_rl, brakes_temp_rr,
       tyres_surface_temp_fl, tyres_surface_temp_fr, tyres_surface_temp_rl, tyres_surface_temp_rr,
       tyres_inner_temp_fl, tyres_inner_temp_fr, tyres_inner_temp_rl, tyres_inner_temp_rr
FROM car_telemetry_ext WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 1;

-- ERS energy management
SELECT ers_store_energy, ers_deploy_mode,
       ers_harvested_this_lap_mguk, ers_harvested_this_lap_mguh,
       ers_deployed_this_lap
FROM car_status WHERE car_index = $IDX ORDER BY timestamp DESC LIMIT 5;

-- Detect if a competitor just pitted (pit_status changed)
SELECT car_index, car_position, pit_status, actual_tyre_compound, tyres_age_laps
FROM (
  SELECT ld.car_index, ld.car_position, ld.pit_status, cs.actual_tyre_compound, cs.tyres_age_laps,
         LAG(ld.pit_status) OVER (PARTITION BY ld.car_index ORDER BY ld.timestamp) AS prev_pit
  FROM lap_data ld JOIN car_status cs ON ld.car_index = cs.car_index
)
WHERE pit_status != prev_pit AND pit_status IN (1, 2);
```

## Writing Effective Insights

When crafting strategy insights to push via `/api/strategy`, follow these guidelines:

1. **Be specific and actionable** — "Box this lap, switch to Mediums" not "Consider pitting soon"
2. **Include numbers** — "Gap to Hamilton is 1.2s and closing at 0.3s/lap" not "The gap is closing"
3. **Reference the current session** — Use track name, lap number, competitor positions
4. **Set appropriate priority**:
   - **1-2**: Informational (pace comparisons, tire wear updates)
   - **3**: Tactical (pit window approaching, undercut opportunity)
   - **4**: Urgent (rain incoming, safety car, tire cliff imminent)
   - **5**: Critical (mechanical failure, immediate box required)
5. **Sound like a real F1 engineer** — Concise, direct, use F1 terminology
6. **One insight per message** — Don't bundle multiple topics

## Development

```bash
make start    # Start all 3 processes (builds Go binaries first)
make stop     # Stop all processes
make build    # Build Go binaries only (telemetry-core + racedb)
```

### Project structure

```
telemetry-core/          Go service (UDP + API + DuckDB + Gemini)
  cmd/server/            Main server entry point
  cmd/query/             racedb CLI tool
  cmd/insightlog/        Insight history CLI tool
  internal/
    api/                 Fiber REST handlers + WebSocket hub
    config/              Environment config
    ingestion/           UDP packet parsing pipeline
    intelligence/        Gemini advisor, analyst, CommsGate, voice client
    insights/            Rule-based insight engine + insight history log
    models/              Data structures
    packets/             F1 telemetry packet definitions
    storage/             DuckDB schema and operations
    workspace/           Markdown context file writer
dashboard/               React frontend (Vite + React 19 + Tailwind v4)
workspace/               Agent context files + DuckDB
workspace/bin/           Compiled binaries (telemetry-core, racedb, insightlog) + publish scripts
voice_service.py         Python TTS/STT service
```
