# Data Analysis Team (OpenCode Agent) Architecture Plan

## 1. System Understanding & The Flow of Insights
After a deep dive into the `telemetry-core` Go backend, the flow of strategic insights is as follows:

1. **The Webhook (`/api/strategy`):** Accepts a JSON payload (`models.StrategyPayload` with `summary`, `recommendation`, `criticality`).
2. **The Analyst Webhook Handler (`intelligence.Analyst`):** Converts this payload into a `models.DrivingInsight` and pushes it onto the `rawChan`.
3. **The Translator (`intelligence.Translator`):** This is a crucial component. It acts as the actual "Voice" of the Race Engineer. 
    - It listens to `rawChan`. 
    - **Batching:** If the `priority` (criticality) is < 4, it batches insights for 5 seconds.
    - **Bypass:** If `priority` >= 4, it translates and sends immediately.
    - **Translation:** It uses Gemini, combined with `SOUL.md` (the engineer's persona) and `USER.md` (driver preferences), to convert the raw, factual insights into authentic, short F1 radio chatter (e.g., converting "Pace dropping vs Max, Box at LAP 8" into "Box box, target lap 8, securing undercut on Max").
    - It pushes the finalized, translated string to `translatedChan`.
4. **The Voice Client (`intelligence.VoiceClient`):** Drains `translatedChan` and POSTs the final F1 radio string to the Python `voice_service.py` at `/speak`.
5. **The Voice Service (`voice_service.py`):** Uses an `asyncio.PriorityQueue` to speak the text via `pyttsx3`.

## 2. Core Components of the New Architecture

### A. The Data Analyst Agent (`data_analyst_agent.py`)
- **Type:** A Python-based autonomous agent (e.g., a simple custom loop using the `google-genai` SDK with function calling).
- **Execution:** Runs as a continuous daemon alongside the Go `telemetry-core`.
- **Role:** It acts purely as the "Strategy Team on the pit wall." It does *not* need to sound like a Race Engineer. It just needs to find raw, factual insights by exploring data.
- **Workflow:** 
  1. Runs a loop (every 10-15 seconds) or triggers on a local event.
  2. Uses the `query_duckdb` tool to fetch recent telemetry from the `data/telemetry.duckdb` file (which contains 10 synced tables like `session_history`, `car_damage`, `car_telemetry_ext`).
  3. Evaluates trends (e.g., tire degradation over 3 laps, sector times).
  4. If an anomaly or strategic opportunity is found, uses the `send_insight` tool.

### B. Agent Tools

1. **`query_duckdb` (Database Access Tool)**
   - **Description:** Executes read-only SQL queries against the DuckDB instance.
   - **Target Tables:** `session_history` (lap times), `car_status` (fuel/compound), `car_damage` (tire wear), `car_telemetry_ext` (temps/pressures).

2. **`send_insight` (Routine Comms Tool)**
   - **Description:** Sends a factual strategic or performance insight to the Race Engineer.
   - **Action:** POST to `http://localhost:8080/api/strategy` with `criticality` 1-3.
   - **Behavior:** The Go `Translator` will batch this with other recent insights and format it into a smooth radio message.

3. **`send_critical_alert` (Emergency Comms Tool)**
   - **Description:** Sends an urgent safety or failure alert.
   - **Action:** POST to `http://localhost:8080/api/strategy` with `criticality` >= 4.
   - **Behavior:** The Go `Translator` will bypass the batch timer, immediately generate a panicked/urgent radio string, and send it to the TTS engine, interrupting the queue.

## 3. Example Workflows

### Scenario 1: The Undercut Strategy (Routine)
1. Agent runs `query_duckdb`: `SELECT lap_num, lap_time_in_ms FROM session_history WHERE car_index IN (0, 1) ORDER BY lap_num DESC LIMIT 5`
2. Agent runs `query_duckdb`: `SELECT tyres_wear_fl, tyres_wear_rl FROM car_damage WHERE car_index = 0 ORDER BY timestamp DESC LIMIT 1`
3. Agent analyzes: Player pace dropping, wear at 50%.
4. Agent calls `send_insight`:
   - `summary`: "Pace dropping vs Max due to 50% wear."
   - `recommendation`: "Box at LAP 8 for hards to secure undercut."
   - `criticality`: 3
5. The Go `Translator` intercepts it, reads `SOUL.md`, and outputs to TTS: *"Copy that, strategy team thinks we should box this lap for hards. Securing the undercut."*

### Scenario 2: Imminent Failure (Critical)
1. Agent queries `car_damage` and spots `engine_temperature` spiking.
2. Agent calls `send_critical_alert`:
   - `summary`: "Engine Overheating rapidly."
   - `recommendation`: "Lift and coast immediately, shift early."
   - `criticality`: 5
3. The Go `Translator` bypasses the batching queue and immediately outputs: *"Engine temps critical, lift and coast now! Shift early!"*

## 4. Implementation Steps
1. **Disable Built-in Go Analyst Loop:** In `telemetry-core/cmd/server/main.go`, disable the `analyst.Run(ctx)` goroutine so it doesn't conflict with our new Python agent, but leave the `strategyWebhookHandler` active.
2. **Build `data_analyst_agent.py`:** 
   - Initialize `google.genai` client.
   - Define `query_duckdb(sql_query)` tool using Python's `duckdb` library.
   - Define `send_insight(summary, recommendation, criticality)` tool using `requests.post`.
   - Create the ReAct while-loop that provides the agent with the schema and asks it to proactively monitor the race.
3. **Update Start Scripts:** Ensure `data_analyst_agent.py` starts automatically in `start.sh` alongside the API and Voice service.