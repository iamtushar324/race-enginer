# Gemini Context: Race Engineer

AI-powered F1 race engineer for F1 25. Real-time telemetry ingestion, DuckDB analytics, voice-driven strategy.

## Architecture

3 processes launched via `make start`:

1. **Go Telemetry Core** (`telemetry-core/`) — UDP listener on port 20777, DuckDB storage, Fiber REST API on port 8081, Gemini advisor for driver queries and strategic insights.
2. **React Dashboard** (`dashboard/`) — Vite + React 19 + Tailwind v4 on port 5173. Visualizes live telemetry, communications, and settings.
3. **Python Voice Service** (`voice_service.py`) — edge-tts synthesis + Whisper STT transcription on port 8000.

## Tech Stack

- **Languages:** Go 1.25, TypeScript (React), Python
- **Database:** DuckDB (in-process OLAP, 10 telemetry tables)
- **AI/LLM:** Google Gemini 2.5 Flash (Go SDK `google/generative-ai-go`)
- **Networking:** UDP (F1 25 telemetry), WebSockets (real-time UI), HTTP (inter-service)

## Querying Telemetry

### CLI tool (preferred)
```bash
./bin/racedb "SELECT * FROM car_damage ORDER BY timestamp DESC LIMIT 5"
./bin/racedb --tables    # List tables with row counts
./bin/racedb --schema    # Full schema
```

Opens DuckDB in read-only mode. Safe while server is running. Database at `workspace/telemetry.duckdb`.

### REST API
```bash
curl -X POST http://localhost:8081/api/query \
  -H "Content-Type: application/json" \
  -d '{"sql": "SELECT COUNT(*) as n FROM lap_data"}'
```

## DuckDB Tables

10 tables (all with `timestamp` column):
- **telemetry** — speed, gear, throttle, brake, steering, tire wear, lap, sector
- **session_data** — weather, temps, track_id, session_type, safety_car, rain%
- **lap_data** — lap times, sectors, position, pit status, deltas, penalties
- **car_status** — fuel, ERS, tire compound, DRS, engine power
- **car_damage** — tire wear/damage, wing damage, engine component wear
- **car_telemetry_ext** — brake/tire/engine temps, pressures, DRS, clutch
- **motion_data** — 3D position (x,y,z), G-forces, rotation (yaw, pitch, roll)
- **race_events** — event codes, vehicle index, detail text
- **session_history** — per-car lap times, sector times, validity
- **raw_packets** — JSON dump of raw packets (debugging)

## API Endpoints

| Method | Route | Description |
|--------|-------|-------------|
| GET | `/health` | Server health |
| GET | `/ws` | WebSocket — live push of telemetry, insights, health |
| GET | `/api/telemetry/latest` | Latest race state |
| GET | `/api/settings` | Current settings |
| GET | `/api/insights/next` | Next pending insight |
| GET | `/api/workspace` | `insights.md` as plain text |
| POST | `/api/query` | Execute read-only SQL |
| POST | `/api/strategy` | Push strategy insight |
| POST | `/api/driver_query` | Submit driver question |
| POST | `/api/voice` | Push-to-talk: multipart audio → Whisper → Gemini |
| POST | `/api/settings/mode` | Switch mock/real mode |
| POST | `/api/settings/talk_level` | Set verbosity (1-10) |

## Workspace Files

All in `workspace/`:
- `driver_profile.md` — Driver style, preferences, incident history
- `track_setup.md` — Track characteristics, setup recommendations, danger zones
- `past_learnings.md` — Historical session debriefs and data
- `insights.md` — Live state + accumulated findings
- `telemetry.duckdb` — The DuckDB database

## Configuration

Key env vars (in `.env`):
- `DB_PATH` — DuckDB path (default: `workspace/telemetry.duckdb`)
- `TELEMETRY_MODE` — `real` or `mock`
- `GEMINI_API_KEY` — Required for LLM features
- `API_PORT` — REST API (default: 8081)
- `LOG_LEVEL` — Log level: trace, debug, info, warn, error, fatal, panic (default: info)
- `WS_PUSH_RATE` — WebSocket push rate in Hz (default: 10)
- `TTS_ENGINE` — TTS backend: edge-tts (default), future: coqui, bark
- `TTS_VOICE` — edge-tts voice (default: `en-GB-RyanNeural`)
- `WHISPER_MODEL` — faster-whisper model size (default: `medium`)

## Commands

```bash
make start    # Start all 3 processes
make stop     # Stop all processes
make build    # Build Go binaries (telemetry-core + racedb)
```

## Project Structure

```
telemetry-core/          Go service
  cmd/server/            Server entry point
  cmd/query/             racedb CLI tool
  internal/              API, config, ingestion, intelligence, storage, etc.
dashboard/               React frontend
workspace/               Agent context files + DuckDB
voice_service.py         Python voice service
```

See `AGENTS.md` for the complete reference including full schema, example queries, and API payload formats.
