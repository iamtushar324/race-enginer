# Race Engineer: System Architecture

## Overview
The Race Engineer system is an intelligent assistant designed to act as an automated race engineer for sim racing (e.g., F1 games). It parses real-time telemetry, analyzes driver performance, and uses a team of AI analysts working in the background to provide strategic advice to the main AI Race Engineer, who then communicates with the driver.

## Core Components

### 1. Telemetry Parser (`telemetry_engine`)
- **Responsibility:** Ingest raw telemetry data from the racing simulator (via UDP stream similar to `Fredrik2002/f1-25-telemetry-application`), decode packets, and publish normalized data.
- **Outputs:** Normalized `TelemetryTick` events.

### 2. Historical Data Store (`DataStore` via DuckDB)
- **Responsibility:** Subscribes to telemetry events and stores them in a fast, in-process analytical database (DuckDB) for time-series and aggregate analysis.
- **Inputs:** `TelemetryTick`
- **Outputs:** Persists data to a local `.duckdb` file and allows SQL querying.

### 3. Backend Analyst Team (`StrategyTeamWorker`)
- **Responsibility:** Runs continuously in the background (like an `opencode` agent). It periodically queries DuckDB to identify trends (e.g., tire degradation over the last 5 laps, pace drop-off, fuel usage vs target). It uses an LLM to generate strategic advice.
- **Inputs:** SQL aggregates from DuckDB.
- **Outputs:** `StrategyInsight` events published to the Event Bus.

### 4. Main Race Engineer (`LLMAdvisor`)
- **Responsibility:** The "voice in your ear". It maintains the latest live telemetry and the latest `StrategyInsight` from the Analyst Team in its context window. When the driver asks a question ("What's the strategy?"), it responds dynamically based on both the live data and the team's analysis.
- **Inputs:** `TelemetryTick`, `StrategyInsight`, `DriverQuery` (from Voice Engine).
- **Outputs:** `DrivingInsight` events (to be spoken).

### 5. Voice Engine (`voice_engine`)
- **Responsibility:** Handle bidirectional verbal communication with the driver via STT (Speech-to-Text) and TTS (Text-to-Speech).

### 6. Web Dashboard (`ui/app.py`)
- **Responsibility:** Real-time visual representation of telemetry and the Engineer's comms via FastAPI and WebSockets.

## Data Flow Diagram

```text
+----------------+       UDP       +------------------+
| F1 Simulator   | --------------> | Telemetry Parser |
+----------------+                 +------------------+
                                            |
                                            v (TelemetryTick)
                                   +------------------+
                                   |  Event Bus       | 
                                   +------------------+
                                            |
      +-------------------------------------+-------------------------------------+
      |                                     |                                     |
      v                                     v                                     v
+-----------+ (SQL) +----------------+  +----------------+               +-----------------+
| DataStore | <---> | Strategy Team  |  | Race Engineer  | <-----------> | Voice Engine    |
| (DuckDB)  |       | (Background AI)|  | (LLM Advisor)  |               | (STT / TTS)     |
+-----------+       +----------------+  +----------------+               +-----------------+
                            |                     ^
                            v (StrategyInsight)   |
                            +---------------------+
```

## Tech Stack
- **Language:** Python 3.10+
- **Database:** `duckdb` (In-process OLAP database for fast telemetry analytics)
- **Intelligence:** `google-genai` (Gemini 2.5 Flash for both Analyst and Engineer roles)
- **UI:** `FastAPI`, `uvicorn`, `websockets`
- **Concurrency:** `asyncio` Event Bus
