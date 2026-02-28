# Race Engineer: System Architecture

## Overview
The Race Engineer system is an intelligent assistant designed to act as an automated race engineer for sim racing (e.g., F1 games). It parses real-time telemetry, analyzes driver performance, and communicates actionable feedback via voice.

## Core Components

### 1. Telemetry Parser (`telemetry_engine`)
- **Responsibility:** Ingest raw telemetry data from the racing simulator (usually via a UDP stream), decode the packets, and normalize them into a standardized format.
- **Inputs:** Raw UDP packets from the game.
- **Outputs:** Normalized telemetry data (Speed, Gear, Throttle, Brake, Steering, Car Position, Lap Times, Tire Wear, etc.).

### 2. Feedback Engine (`feedback_engine`)
- **Responsibility:** Analyze the normalized telemetry data to generate driving insights, identify areas for improvement (e.g., suboptimal braking points, early acceleration, high tire wear), and track race strategy.
- **Inputs:** Normalized telemetry data, historical lap data.
- **Outputs:** Driving insights, performance alerts, and strategic recommendations (e.g., "Brake 10 meters later into Turn 1", "Tire temperatures are dropping").

### 3. Voice Engine (`voice_engine`)
- **Responsibility:** Handle all bidirectional verbal communication with the driver. It uses Speech-to-Text (STT) to understand driver queries and Text-to-Speech (TTS) to deliver feedback naturally.
- **Inputs:** Driving insights from the Feedback Engine, Audio input from the driver's microphone.
- **Outputs:** Audio output (spoken feedback), text transcripts of driver queries.

### 4. Core Orchestrator / Event Bus (`core`)
- **Responsibility:** Manage the lifecycle of the application and facilitate communication between components using an event-driven architecture.
- **Data Flow:**
  - `Telemetry Parser` publishes `TelemetryTick` and `LapCompleted` events.
  - `Feedback Engine` subscribes to telemetry events, processes them, and publishes `InsightGenerated` events.
  - `Voice Engine` subscribes to `InsightGenerated` events to announce them to the driver, and publishes `DriverQuery` events when the driver speaks.

## Data Flow Diagram

```text
+----------------+       UDP       +------------------+
| F1 Simulator   | --------------> | Telemetry Parser |
+----------------+                 +------------------+
                                            |
                                            v (Normalized Telemetry)
                                   +------------------+
                                   |  Event Bus       | <---- State Management / Orchestration
                                   +------------------+
                                            |
                    +-----------------------+-----------------------+
                    |                                               |
                    v (Telemetry Streams)                           v (Feedback / Alerts)
           +-----------------+                             +-----------------+
           | Feedback Engine |                             | Voice Engine    |
           +-----------------+                             +-----------------+
                    |                                         ^           |
                    +---(Generated Insights)------------------+           v
                                                                    Audio I/O (Driver)
```

## Tech Stack (Proposed)
- **Language:** Python 3.10+
- **Telemetry:** `fastf1` (if offline data) or custom UDP sockets / `f1-2021-udp` parsers for real-time.
- **Voice:** `SpeechRecognition` / `whisper` for STT, `pyttsx3` or ElevenLabs API for TTS.
- **Concurrency:** `asyncio` for non-blocking event handling and I/O.
- **Architecture Pattern:** Pub/Sub Event Bus.
