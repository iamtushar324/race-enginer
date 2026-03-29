# Race Engineer

AI-powered real-time race engineer for F1 25. Ingests live UDP telemetry, stores it in DuckDB, serves a React dashboard, and provides voice-driven strategic advice powered by LLMs.

```
F1 25 Game (UDP :20777)
        |
        v
+---------------------+     +-----------------+     +-----------------+
|  Go Telemetry Core  |---->|  React Dashboard |     |  Python Voice   |
|  (port 8081)        | ws  |  (port 5173)     |     |  (port 8000)    |
|                     |     +-----------------+     +-----------------+
|  - UDP ingestion    |              |                       |
|  - DuckDB storage   |<------- REST API -------------------|
|  - LLM advisor      |
|  - Insight engine   |
+---------------------+
        |
        v
  workspace/telemetry.duckdb
```

Three processes run concurrently:

1. **Go Telemetry Core** — UDP listener, DuckDB writer, REST API, LLM-powered strategy analyst and comms gate
2. **React Dashboard** — Live telemetry visualization (Vite + React 19 + Tailwind v4)
3. **Python Voice Service** — edge-tts synthesis + Whisper STT transcription for radio comms

## Features

- **Live UDP Telemetry** — Parses all F1 25 packet types at 20Hz, downsamples to 1Hz for storage
- **DuckDB Storage** — 10 tables covering telemetry, lap data, tire wear, damage, fuel, ERS, weather, and more
- **LLM Strategy Advisor** — Gemini, Claude, or OpenAI-powered analysis with full session context
- **Rule-Based Insight Engine** — Threshold-driven alerts for tire wear, fuel, damage, weather, safety car
- **CommsGate** — Deduplicates insights, translates to F1 radio-style speech via LLM
- **Voice Radio** — Push-to-talk via steering wheel button, Whisper STT, LLM response, TTS playback
- **React Dashboard** — Real-time telemetry gauges, tire/damage visualization, insight log, push-to-talk
- **Mock Mode** — Simulated telemetry for development without the game running

## Prerequisites

- **Go 1.25+**
- **Node.js 18+** (with npm)
- **Python 3.10+**
- **F1 25** (or use mock mode)
- An LLM API key (one of: Gemini, Anthropic, OpenAI)

## Quick Start

```bash
# 1. Clone
git clone https://github.com/iamtushar324/race-enginer.git
cd race-enginer

# 2. Configure
cp .env.example .env
# Edit .env — at minimum set your LLM API key:
#   GEMINI_API_KEY="your-key"    (default provider)
#   or ANTHROPIC_API_KEY / OPENAI_API_KEY with matching LLM_PROVIDER

# 3. Start everything
make start
```

`make start` will:
- Build the Go telemetry-core binary and racedb CLI tool
- Create a Python venv and install voice service dependencies
- Install dashboard npm dependencies
- Launch all 3 processes concurrently

The dashboard opens at **http://localhost:5173** and the API is at **http://localhost:8081**.

### F1 25 Game Setup

In F1 25, go to **Settings > Telemetry** and set:
- **UDP Telemetry**: On
- **UDP Broadcast Mode**: On (or Unicast to your machine's IP)
- **UDP Port**: 20777
- **UDP Send Rate**: 20Hz
- **UDP Format**: 2025

### Mock Mode (No Game Required)

To run with simulated telemetry data:

```bash
# Either set in .env:
TELEMETRY_MODE="mock"

# Or switch at runtime via API:
curl -X POST http://localhost:8081/api/settings/mode \
  -H "Content-Type: application/json" -d '{"mode": "mock"}'
```

## Commands

```bash
make start    # Build and start all 3 processes
make stop     # Stop all processes
make build    # Build Go binaries only
```

## Configuration

All settings are via environment variables in `.env`. Key options:

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_PROVIDER` | `gemini` | `gemini`, `anthropic`, or `openai` |
| `GEMINI_API_KEY` | — | Required when provider is gemini |
| `ANTHROPIC_API_KEY` | — | Required when provider is anthropic |
| `OPENAI_API_KEY` | — | Required when provider is openai |
| `TELEMETRY_MODE` | `real` | `real` = UDP from game, `mock` = simulated data |
| `TELEMETRY_PORT` | `20777` | UDP port (must match F1 25 settings) |
| `API_PORT` | `8081` | REST API / WebSocket port |
| `TALK_LEVEL` | `5` | Insight verbosity 1-10 (higher = more chatter) |
| `TTS_ENGINE` | `edge-tts` | `edge-tts` or `qwen-tts` (MLX voice cloning) |
| `TTS_VOICE` | `en-GB-RyanNeural` | edge-tts voice name |
| `STT_ENGINE` | `whisper` | `whisper`, `parakeet` (NeMo), or `resemble` (cloud) |
| `WHISPER_MODEL` | `medium` | faster-whisper model size |
| `PTT_BUTTON` | `0x00100000` | Steering wheel button bitmask for push-to-talk |

See `.env.example` for the full list with descriptions.

## Push-to-Talk

Map a steering wheel button to **UDP Action 1** in F1 25's UDP settings. When pressed during gameplay, the dashboard records audio from your microphone, transcribes it with Whisper, sends the query to the LLM with full telemetry context, and plays back the response as synthesized speech.

The system sends an immediate acknowledgement ("Copy, checking the data") so you hear a response within ~200ms, then the full LLM answer follows.

## Project Structure

```
telemetry-core/            Go service (UDP + API + DuckDB + LLM)
  cmd/server/              Main server entry point
  cmd/query/               racedb CLI tool
  cmd/insightlog/          Insight history CLI tool
  internal/
    api/                   Fiber REST handlers + WebSocket hub
    config/                Environment config
    ingestion/             UDP packet parsing pipeline
    intelligence/          LLM advisor, analyst, CommsGate, voice client
    insights/              Rule-based insight engine + insight history log
    models/                Data structures
    packets/               F1 25 telemetry packet definitions
    storage/               DuckDB schema and operations
    workspace/             Markdown context file writer
dashboard/                 React frontend (Vite + React 19 + Tailwind v4)
voice_service.py           Python TTS/STT service
workspace/                 Runtime context files + DuckDB database
  driver_profile.md        Driver style and communication preferences
  track_setup.md           Track characteristics and setup notes
  past_learnings.md        Historical session debriefs
  soul.md                  Race engineer personality and radio style
  user.md                  Driver communication preferences
  insights.md              Live accumulated findings (written by insight engine)
```

## REST API

| Method | Route | Description |
|--------|-------|-------------|
| GET | `/health` | Server health check |
| GET | `/ws` | WebSocket for live telemetry, insights, and audio |
| GET | `/api/telemetry/latest` | Latest race state snapshot |
| GET | `/api/settings` | Current settings |
| GET | `/api/insights/history` | Insight log (`?limit=N`) |
| POST | `/api/query` | Execute read-only SQL against DuckDB |
| POST | `/api/strategy` | Push a strategy insight |
| POST | `/api/driver_query` | Submit a driver question to LLM |
| POST | `/api/voice` | Push-to-talk audio upload |
| POST | `/api/settings/mode` | Switch mock/real mode |
| POST | `/api/settings/talk_level` | Set verbosity level |

## CLI Tools

### racedb

Query the DuckDB database directly (read-only, safe while server is running):

```bash
# Build
make build

# Query
./workspace/bin/racedb "SELECT * FROM car_damage ORDER BY timestamp DESC LIMIT 5"
./workspace/bin/racedb --tables    # List tables with row counts
./workspace/bin/racedb --schema    # Show full schema
```

### insightlog

View the insight history from the running server:

```bash
./workspace/bin/insightlog           # Last 50 insights
./workspace/bin/insightlog -limit 20 # Last 20
./workspace/bin/insightlog -json     # Raw JSON output
```

## How Insights Work

```
Rule Engine (threshold-based)
        |
        v
    CommsGate (LLM translator + dedup + talk level filter)
        |
        +---> WebSocket --> Dashboard
        +---> Voice TTS --> Audio playback
        +---> workspace/insights.md
```

1. **Rule Engine** fires insights based on telemetry thresholds (tire wear > 45%, fuel < 3 laps, damage spikes, weather changes, safety car)
2. **Strategy Analyst** runs on a background ticker, queries live DuckDB, and produces strategic insights via LLM (undercut threats, pit windows, pace analysis)
3. **CommsGate** receives all insights, deduplicates them, filters by talk level, and translates to F1 radio-style speech via LLM
4. Translated insights fan out to the dashboard, voice TTS, and the workspace insights file

## Contributing

Contributions are welcome. Please open an issue or submit a pull request.

## License

MIT
