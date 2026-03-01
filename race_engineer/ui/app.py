import asyncio
import logging
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from pydantic import BaseModel
from race_engineer.core.event_bus import bus
from race_engineer.intelligence.models import StrategyInsight
from race_engineer.telemetry.models import DriverQuery, TalkLevelPayload

logger = logging.getLogger(__name__)

app = FastAPI(title="Race Engineer Dashboard")


class StrategyPayload(BaseModel):
    summary: str
    recommendation: str
    criticality: int


class AgentStatusPayload(BaseModel):
    message: str


class DriverQueryPayload(BaseModel):
    query: str


@app.post("/api/strategy")
async def receive_strategy_from_agent(payload: StrategyPayload):
    """Webhook endpoint for external Analyst agents to push strategic insights."""
    insight = StrategyInsight(
        summary=payload.summary,
        recommendation=payload.recommendation,
        criticality=payload.criticality,
    )
    logger.info(f"Received external strategy insight via webhook: {payload.summary}")
    await bus.publish("strategy_insight", insight)
    return {"status": "success"}


@app.post("/api/agent_status")
async def receive_agent_status(payload: AgentStatusPayload):
    """Webhook for external Analyst agents to report what they are doing."""
    await bus.publish("agent_status", {"message": payload.message})
    return {"status": "success"}


@app.post("/api/driver_query")
async def receive_manual_driver_query(payload: DriverQueryPayload):
    """Endpoint to simulate the driver speaking via a UI button."""
    query = DriverQuery(query=payload.query, confidence=1.0)
    await bus.publish("driver_query", query)
    return {"status": "success"}


@app.post("/api/talk_level")
async def receive_talk_level(payload: TalkLevelPayload):
    """Endpoint to set the verbosity level of the Race Engineer."""
    await bus.publish("talk_level_changed", {"talk_level": payload.talk_level})
    return {"status": "success", "talk_level": payload.talk_level}


class SQLQueryPayload(BaseModel):
    sql: str


# Reference to the DataStore, set by main.py at startup
_datastore = None


def set_datastore(ds):
    global _datastore
    _datastore = ds


# Reference to the ParserManager, set by main.py at startup
_parser_manager = None


def set_parser_manager(pm):
    global _parser_manager
    _parser_manager = pm


class TelemetryModePayload(BaseModel):
    mode: str  # "mock" or "real"
    host: str = "0.0.0.0"
    port: int = 20777


@app.get("/api/telemetry_status")
async def get_telemetry_status():
    """Get current telemetry mode and connection status."""
    if _parser_manager is None:
        return {"mode": "mock", "status": "not_started"}
    return _parser_manager.get_status()


@app.post("/api/telemetry_mode")
async def set_telemetry_mode(payload: TelemetryModePayload):
    """Switch between mock and real telemetry modes."""
    if _parser_manager is None:
        return {"status": "error", "error": "Parser manager not initialized"}
    if payload.mode not in ("mock", "real"):
        return {"status": "error", "error": "Mode must be 'mock' or 'real'"}
    await _parser_manager.switch_mode(payload.mode, payload.host, payload.port)
    return {"status": "success", "mode": payload.mode}


@app.post("/api/query")
async def execute_sql_query(payload: SQLQueryPayload):
    """Endpoint for external agents to query the DuckDB database via HTTP.
    Avoids DuckDB's single-writer lock issue with direct file access."""
    if _datastore is None:
        return {"status": "error", "error": "DataStore not initialized yet"}
    try:
        import asyncio

        loop = asyncio.get_running_loop()
        rows = await loop.run_in_executor(None, _datastore.query, payload.sql)
        return {"status": "success", "rows": rows}
    except Exception as e:
        return {"status": "error", "error": str(e)}


html = """
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Race Engineer | Pit Wall</title>
    <style>
        :root {
            --bg-color: #0d1117;
            --panel-bg: #161b22;
            --border: #30363d;
            --text: #c9d1d9;
            --accent: #58a6ff;
            --success: #2ea043;
            --warning: #d29922;
            --danger: #f85149;
            --purple: #bc8cff;
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            background-color: var(--bg-color); color: var(--text);
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
            padding: 12px; height: 100vh; display: flex; flex-direction: column;
        }
        h1 { color: #fff; font-size: 20px; border-bottom: 1px solid var(--border); padding-bottom: 8px; margin-bottom: 12px; }
        h2 { font-size: 13px; color: #8b949e; text-transform: uppercase; letter-spacing: 1px; margin-bottom: 10px; }
        h3 { font-size: 12px; color: #8b949e; text-transform: uppercase; letter-spacing: 0.5px; margin: 12px 0 6px; }

        .dashboard {
            display: grid; grid-template-columns: 1.2fr 1fr 1fr 1fr; gap: 12px; flex-grow: 1; overflow: hidden;
        }
        .panel {
            background: var(--panel-bg); border: 1px solid var(--border); border-radius: 8px;
            padding: 14px; display: flex; flex-direction: column; overflow-y: auto;
        }

        /* Data boxes */
        .data-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
        .data-grid-3 { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 8px; }
        .data-box { background: #000; padding: 10px; border-radius: 6px; text-align: center; border: 1px solid #222; }
        .data-label { color: #8b949e; font-size: 10px; text-transform: uppercase; }
        .data-value { font-size: 26px; font-weight: bold; color: #fff; margin-top: 3px; font-variant-numeric: tabular-nums; }
        .data-unit { color: #555; font-size: 10px; }
        .data-sm { font-size: 18px; }
        .data-xs { font-size: 14px; }

        /* Bars */
        .bar-container { background: #222; height: 8px; border-radius: 4px; overflow: hidden; margin-top: 4px; }
        .bar-fill { height: 100%; width: 0%; transition: width 0.1s linear; }
        .bg-throttle { background: var(--success); }
        .bg-brake { background: var(--danger); }
        .bg-rpm { background: var(--accent); }
        .bg-ers { background: #FFD700; }
        .bg-fuel { background: #FF9300; }

        /* Indicators */
        .indicator { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: bold; }
        .ind-green { background: #1a3a1a; color: var(--success); border: 1px solid var(--success); }
        .ind-red { background: #3a1a1a; color: var(--danger); border: 1px solid var(--danger); }
        .ind-yellow { background: #3a3a1a; color: var(--warning); border: 1px solid var(--warning); }
        .ind-blue { background: #1a1a3a; color: var(--accent); border: 1px solid var(--accent); }
        .ind-purple { background: #2a1a3a; color: var(--purple); border: 1px solid var(--purple); }

        /* Tire grid */
        .tire-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 6px; }
        .tire-box { background: #000; padding: 8px; border-radius: 6px; text-align: center; border: 1px solid #222; }
        .tire-val { font-size: 16px; font-weight: bold; margin-top: 2px; }
        .tire-temp { font-size: 11px; color: #888; margin-top: 2px; }
        .good { color: var(--success); }
        .warn { color: var(--warning); }
        .critical { color: var(--danger); }

        /* Damage bars */
        .dmg-row { display: flex; align-items: center; gap: 8px; margin: 3px 0; }
        .dmg-label { font-size: 11px; color: #8b949e; min-width: 70px; }
        .dmg-bar { flex: 1; background: #222; height: 6px; border-radius: 3px; overflow: hidden; }
        .dmg-fill { height: 100%; background: var(--success); transition: width 0.3s; }
        .dmg-val { font-size: 11px; min-width: 30px; text-align: right; }

        /* Weather */
        .weather-icon { font-size: 28px; margin-right: 8px; }
        .weather-row { display: flex; align-items: center; margin: 4px 0; }
        .weather-label { color: #8b949e; font-size: 12px; min-width: 80px; }
        .weather-value { font-size: 14px; font-weight: bold; }

        /* Session info */
        .info-row { display: flex; justify-content: space-between; padding: 4px 0; border-bottom: 1px solid #1a1a1a; }
        .info-label { color: #8b949e; font-size: 12px; }
        .info-value { font-size: 13px; font-weight: bold; }

        /* Logs */
        .log-list { list-style: none; flex-grow: 1; overflow-y: auto; font-family: monospace; font-size: 12px; }
        .log-item { padding: 6px; border-bottom: 1px solid #1a1a1a; }
        .time { color: #555; margin-right: 6px; }
        .encourage { color: var(--success); }
        .warn-log { color: var(--warning); }
        .strat-log { color: var(--purple); }
        .info-log { color: var(--accent); }
        .agent-log { color: #a5d6ff; }
        .event-log { color: #FF9300; }

        /* Controls */
        .controls { margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border); }
        input[type="text"] {
            width: 100%; padding: 8px; background: #000; color: #fff; border: 1px solid var(--border);
            border-radius: 6px; margin-bottom: 8px; font-size: 13px;
        }
        button {
            width: 100%; padding: 8px; background: var(--border); color: #fff; border: none;
            border-radius: 6px; cursor: pointer; font-weight: bold; font-size: 13px;
        }
        button:hover { background: #4b535d; }

        /* Lap time display */
        .lap-time { font-family: monospace; font-size: 14px; color: #fff; }

        /* Toggle switch */
        .toggle-switch { position: relative; display: inline-block; width: 36px; height: 20px; }
        .toggle-switch input { opacity: 0; width: 0; height: 0; }
        .toggle-slider {
            position: absolute; cursor: pointer; top: 0; left: 0; right: 0; bottom: 0;
            background-color: #30363d; border-radius: 10px; transition: 0.3s;
        }
        .toggle-slider:before {
            position: absolute; content: ""; height: 14px; width: 14px; left: 3px; bottom: 3px;
            background-color: #fff; border-radius: 50%; transition: 0.3s;
        }
        .toggle-switch input:checked + .toggle-slider { background-color: var(--accent); }
        .toggle-switch input:checked + .toggle-slider:before { transform: translateX(16px); }

        /* Header bar */
        .header-bar {
            display: flex; justify-content: space-between; align-items: center;
            border-bottom: 1px solid var(--border); padding-bottom: 8px; margin-bottom: 12px;
        }
        .header-bar h1 { border-bottom: none; padding-bottom: 0; margin-bottom: 0; }
        .header-controls { display: flex; align-items: center; gap: 14px; }
        .conn-status { display: flex; align-items: center; gap: 6px; }
        .conn-dot { width: 8px; height: 8px; border-radius: 50%; }
        .conn-text { font-size: 11px; color: #8b949e; }
        .mode-toggle { display: flex; align-items: center; gap: 6px; }
        .mode-label { font-size: 11px; color: #8b949e; }
        .udp-config {
            display: none; align-items: center; gap: 6px;
        }
        .udp-config input {
            padding: 3px 6px; font-size: 11px; background: #000; color: #fff;
            border: 1px solid #30363d; border-radius: 4px; font-family: monospace;
        }
        .udp-config .udp-host { width: 100px; }
        .udp-config .udp-port { width: 60px; }
        .udp-config .colon { color: #555; font-size: 12px; }
    </style>
</head>
<body>
    <div class="header-bar">
        <h1>Race Engineer | Team Pit Wall</h1>
        <div class="header-controls">
            <div class="conn-status">
                <span class="conn-dot" id="conn-dot" style="background: #2ea043;"></span>
                <span class="conn-text" id="conn-text">Mock Running</span>
            </div>
            <div class="mode-toggle">
                <span class="mode-label">Mock</span>
                <label class="toggle-switch">
                    <input type="checkbox" id="mode-toggle" onchange="toggleMode()">
                    <span class="toggle-slider"></span>
                </label>
                <span class="mode-label">Real</span>
            </div>
            <div class="udp-config" id="udp-config">
                <input type="text" class="udp-host" id="udp-host" value="0.0.0.0" placeholder="Host">
                <span class="colon">:</span>
                <input type="number" class="udp-port" id="udp-port" value="20777" placeholder="Port">
            </div>
        </div>
    </div>

    <div class="dashboard">
        <!-- Panel 1: Live Telemetry -->
        <div class="panel">
            <h2>Live Telemetry</h2>
            <div class="data-grid">
                <div class="data-box">
                    <div class="data-label">Speed</div>
                    <div class="data-value" id="val-speed">0</div>
                    <div class="data-unit">km/h</div>
                </div>
                <div class="data-box">
                    <div class="data-label">Gear</div>
                    <div class="data-value" id="val-gear">N</div>
                    <div style="margin-top:4px;">
                        <span class="indicator ind-red" id="ind-drs" style="display:none;">DRS</span>
                    </div>
                </div>
                <div class="data-box">
                    <div class="data-label">Lap</div>
                    <div class="data-value data-sm" id="val-lap">1</div>
                    <div class="data-unit" id="val-sector">Sector 1</div>
                </div>
                <div class="data-box">
                    <div class="data-label">RPM</div>
                    <div class="data-value data-sm" id="val-rpm">0</div>
                    <div class="bar-container"><div class="bar-fill bg-rpm" id="bar-rpm"></div></div>
                </div>
            </div>

            <div style="margin-top: 10px;">
                <div class="data-label">Throttle</div>
                <div class="bar-container"><div class="bar-fill bg-throttle" id="bar-throttle"></div></div>
            </div>
            <div style="margin-top: 6px;">
                <div class="data-label">Brake</div>
                <div class="bar-container"><div class="bar-fill bg-brake" id="bar-brake"></div></div>
            </div>

            <!-- ERS & Fuel -->
            <h3>Energy & Fuel</h3>
            <div class="data-grid">
                <div class="data-box">
                    <div class="data-label">ERS</div>
                    <div class="data-value data-sm" id="val-ers">100%</div>
                    <div class="bar-container"><div class="bar-fill bg-ers" id="bar-ers"></div></div>
                    <div class="data-unit" id="val-ers-mode">Medium</div>
                </div>
                <div class="data-box">
                    <div class="data-label">Fuel</div>
                    <div class="data-value data-sm" id="val-fuel">57.0</div>
                    <div class="bar-container"><div class="bar-fill bg-fuel" id="bar-fuel"></div></div>
                    <div class="data-unit" id="val-fuel-mix">Standard</div>
                </div>
            </div>

            <!-- Tire compound -->
            <div style="margin-top: 8px; text-align: center;">
                <span class="indicator ind-red" id="ind-compound">H</span>
                <span style="font-size: 12px; color: #888; margin-left: 6px;">Age: <span id="val-tyre-age">0</span> laps</span>
            </div>

            <!-- Tire Wear -->
            <h3>Tire Wear</h3>
            <div class="tire-grid">
                <div class="tire-box"><div class="data-label">FL</div><div class="tire-val" id="wear-fl">0%</div></div>
                <div class="tire-box"><div class="data-label">FR</div><div class="tire-val" id="wear-fr">0%</div></div>
                <div class="tire-box"><div class="data-label">RL</div><div class="tire-val" id="wear-rl">0%</div></div>
                <div class="tire-box"><div class="data-label">RR</div><div class="tire-val" id="wear-rr">0%</div></div>
            </div>
        </div>

        <!-- Panel 2: Tire & Damage -->
        <div class="panel">
            <h2>Tire & Damage</h2>

            <h3>Surface Temp (C)</h3>
            <div class="tire-grid">
                <div class="tire-box"><div class="data-label">FL</div><div class="tire-temp" id="stemp-fl">--</div></div>
                <div class="tire-box"><div class="data-label">FR</div><div class="tire-temp" id="stemp-fr">--</div></div>
                <div class="tire-box"><div class="data-label">RL</div><div class="tire-temp" id="stemp-rl">--</div></div>
                <div class="tire-box"><div class="data-label">RR</div><div class="tire-temp" id="stemp-rr">--</div></div>
            </div>

            <h3>Inner Temp (C)</h3>
            <div class="tire-grid">
                <div class="tire-box"><div class="data-label">FL</div><div class="tire-temp" id="itemp-fl">--</div></div>
                <div class="tire-box"><div class="data-label">FR</div><div class="tire-temp" id="itemp-fr">--</div></div>
                <div class="tire-box"><div class="data-label">RL</div><div class="tire-temp" id="itemp-rl">--</div></div>
                <div class="tire-box"><div class="data-label">RR</div><div class="tire-temp" id="itemp-rr">--</div></div>
            </div>

            <h3>Brake Temp (C)</h3>
            <div class="tire-grid">
                <div class="tire-box"><div class="data-label">FL</div><div class="tire-temp" id="btemp-fl">--</div></div>
                <div class="tire-box"><div class="data-label">FR</div><div class="tire-temp" id="btemp-fr">--</div></div>
                <div class="tire-box"><div class="data-label">RL</div><div class="tire-temp" id="btemp-rl">--</div></div>
                <div class="tire-box"><div class="data-label">RR</div><div class="tire-temp" id="btemp-rr">--</div></div>
            </div>

            <h3>Tire Pressure (PSI)</h3>
            <div class="tire-grid">
                <div class="tire-box"><div class="data-label">FL</div><div class="tire-temp" id="press-fl">--</div></div>
                <div class="tire-box"><div class="data-label">FR</div><div class="tire-temp" id="press-fr">--</div></div>
                <div class="tire-box"><div class="data-label">RL</div><div class="tire-temp" id="press-rl">--</div></div>
                <div class="tire-box"><div class="data-label">RR</div><div class="tire-temp" id="press-rr">--</div></div>
            </div>

            <h3>Damage</h3>
            <div id="damage-bars">
                <div class="dmg-row"><span class="dmg-label">FL Wing</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-flw"></div></div><span class="dmg-val" id="dmg-flw-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">FR Wing</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-frw"></div></div><span class="dmg-val" id="dmg-frw-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">Rear Wing</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-rw"></div></div><span class="dmg-val" id="dmg-rw-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">Floor</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-floor"></div></div><span class="dmg-val" id="dmg-floor-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">Diffuser</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-diff"></div></div><span class="dmg-val" id="dmg-diff-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">Sidepod</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-side"></div></div><span class="dmg-val" id="dmg-side-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">Gearbox</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-gb"></div></div><span class="dmg-val" id="dmg-gb-val">0%</span></div>
                <div class="dmg-row"><span class="dmg-label">Engine</span><div class="dmg-bar"><div class="dmg-fill" id="dmg-eng"></div></div><span class="dmg-val" id="dmg-eng-val">0%</span></div>
            </div>
        </div>

        <!-- Panel 3: Weather & Session -->
        <div class="panel">
            <h2>Weather & Session</h2>

            <div class="data-box" style="margin-bottom: 10px;">
                <div style="display: flex; align-items: center; justify-content: center;">
                    <span class="weather-icon" id="weather-icon">--</span>
                    <span style="font-size: 18px; font-weight: bold;" id="weather-text">--</span>
                </div>
                <div style="margin-top: 6px; display: flex; justify-content: center; gap: 16px;">
                    <span><span class="data-label">Track</span> <span id="val-track-temp" style="font-weight:bold;">--</span>C</span>
                    <span><span class="data-label">Air</span> <span id="val-air-temp" style="font-weight:bold;">--</span>C</span>
                </div>
            </div>

            <div style="margin-bottom: 8px; text-align: center;">
                <span class="indicator" id="ind-safety-car" style="display:none;">SC</span>
            </div>

            <h3>Session Info</h3>
            <div class="info-row"><span class="info-label">Time Left</span><span class="info-value" id="val-time-left">--</span></div>
            <div class="info-row"><span class="info-label">Position</span><span class="info-value" id="val-position">--</span></div>
            <div class="info-row"><span class="info-label">Gap to Front</span><span class="info-value" id="val-gap-front">--</span></div>
            <div class="info-row"><span class="info-label">Gap to Leader</span><span class="info-value" id="val-gap-leader">--</span></div>
            <div class="info-row"><span class="info-label">Pit Window</span><span class="info-value" id="val-pit-window">--</span></div>
            <div class="info-row"><span class="info-label">Pit Stops</span><span class="info-value" id="val-pit-stops">0</span></div>

            <h3>Lap Times</h3>
            <div class="info-row"><span class="info-label">Last Lap</span><span class="info-value lap-time" id="val-last-lap">--</span></div>
            <div class="info-row"><span class="info-label">Best Lap</span><span class="info-value lap-time" id="val-best-lap" style="color: var(--purple);">--</span></div>

            <h3>Race Events</h3>
            <ul class="log-list" id="event-log" style="max-height: 120px;">
            </ul>
        </div>

        <!-- Panel 4: Comms & Control -->
        <div class="panel">
            <h2>Race Engineer Comms</h2>
            <ul class="log-list" id="insights">
                <li class="log-item"><span class="time">00:00:00</span> <span class="info-log">System Ready.</span></li>
            </ul>

            <h3 style="margin-top: 8px;">Agent Activity</h3>
            <ul class="log-list" id="agent-log" style="max-height: 100px;">
                <li class="log-item"><span class="time">00:00:00</span> <span class="agent-log">Waiting for agent...</span></li>
            </ul>

            <div class="controls">
                <div style="margin-bottom: 12px;">
                    <label style="font-size: 12px; font-weight: bold;">Talk Level:</label>
                    <div style="display: flex; align-items: center; gap: 6px; margin-top: 4px;">
                        <span style="font-size: 10px; color: #555;">Quiet</span>
                        <input type="range" id="talk-level" min="1" max="10" value="5" style="flex-grow: 1; accent-color: var(--accent); cursor: pointer;" onchange="sendTalkLevel()">
                        <span style="font-size: 10px; color: #555;">Chatty</span>
                        <span id="talk-level-val" style="width: 20px; text-align: center; font-weight: bold; background: #222; padding: 1px 4px; border-radius: 3px; font-size: 12px;">5</span>
                    </div>
                </div>
                <input type="text" id="query-input" placeholder="Driver radio message...">
                <button onclick="sendQuery()">Simulate Driver Radio</button>
            </div>
        </div>
    </div>

    <script>
        document.getElementById("query-input").addEventListener("keypress", function(e) {
            if (e.key === "Enter") { e.preventDefault(); sendQuery(); }
        });

        function ts() {
            var n = new Date();
            return n.getHours().toString().padStart(2,'0') + ":" +
                   n.getMinutes().toString().padStart(2,'0') + ":" +
                   n.getSeconds().toString().padStart(2,'0');
        }

        function addLog(id, msg, cls) {
            var ul = document.getElementById(id);
            var li = document.createElement("li");
            li.className = "log-item";
            li.innerHTML = '<span class="time">' + ts() + '</span> <span class="' + cls + '">' + msg + '</span>';
            ul.prepend(li);
            if (ul.children.length > 50) ul.removeChild(ul.lastChild);
        }

        function setTireClass(el, val) {
            el.innerText = val.toFixed(1) + "%";
            el.className = "tire-val " + (val < 25 ? "good" : (val < 45 ? "warn" : "critical"));
        }

        function setTempColor(el, val, low, high) {
            el.innerText = val;
            el.style.color = val < low ? '#58a6ff' : (val > high ? '#f85149' : '#2ea043');
        }

        function setDmg(barId, valId, pct) {
            var bar = document.getElementById(barId);
            var valEl = document.getElementById(valId);
            bar.style.width = pct + "%";
            bar.style.background = pct < 25 ? '#2ea043' : (pct < 50 ? '#d29922' : '#f85149');
            valEl.innerText = pct + "%";
            valEl.style.color = pct < 25 ? '#2ea043' : (pct < 50 ? '#d29922' : '#f85149');
        }

        function fmtLapTime(ms) {
            if (!ms || ms <= 0) return "--";
            var mins = Math.floor(ms / 60000);
            var secs = ((ms % 60000) / 1000).toFixed(3);
            return (mins > 0 ? mins + ":" : "") + (mins > 0 ? secs.padStart(6, '0') : secs);
        }

        function fmtTimeLeft(secs) {
            if (secs <= 0) return "0:00";
            var m = Math.floor(secs / 60);
            var s = secs % 60;
            return m + ":" + s.toString().padStart(2, '0');
        }

        var weatherIcons = {0:"\\u2600\\uFE0F",1:"\\u26C5",2:"\\u2601\\uFE0F",3:"\\uD83C\\uDF26\\uFE0F",4:"\\uD83C\\uDF27\\uFE0F",5:"\\u26C8\\uFE0F"};
        var weatherNames = {0:"Clear",1:"Light Cloud",2:"Overcast",3:"Light Rain",4:"Heavy Rain",5:"Storm"};
        var fuelMixNames = {0:"Lean",1:"Standard",2:"Rich",3:"Max"};
        var ersNames = {0:"None",1:"Medium",2:"Hotlap",3:"Overtake"};
        var compoundNames = {16:"S",17:"M",18:"H",7:"I",8:"W"};
        var compoundColors = {16:"#f85149",17:"#d29922",18:"#ffffff",7:"#2ea043",8:"#58a6ff"};
        var scNames = {0:"",1:"SAFETY CAR",2:"VIRTUAL SC",3:"FORMATION LAP"};

        var bestLapTime = 0;

        function sendQuery() {
            var inp = document.getElementById("query-input");
            if(!inp.value) return;
            addLog('insights', '[DRIVER] ' + inp.value, 'info-log');
            fetch('/api/driver_query', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({query:inp.value})});
            inp.value = '';
        }

        function sendTalkLevel() {
            var v = parseInt(document.getElementById("talk-level").value);
            document.getElementById("talk-level-val").innerText = v;
            fetch('/api/talk_level', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({talk_level:v})});
        }

        var currentMode = 'mock';

        function toggleMode() {
            var toggle = document.getElementById('mode-toggle');
            var newMode = toggle.checked ? 'real' : 'mock';
            var host = document.getElementById('udp-host').value || '0.0.0.0';
            var port = parseInt(document.getElementById('udp-port').value) || 20777;
            fetch('/api/telemetry_mode', {
                method: 'POST', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({mode: newMode, host: host, port: port})
            }).then(function(r) { return r.json(); })
            .then(function(data) {
                if (data.status === 'success') {
                    currentMode = newMode;
                    updateConnUI(newMode, newMode === 'mock' ? 'running' : 'listening');
                }
            });
        }

        function updateConnUI(mode, status, host, port) {
            var dot = document.getElementById('conn-dot');
            var text = document.getElementById('conn-text');
            var config = document.getElementById('udp-config');
            currentMode = mode;
            document.getElementById('mode-toggle').checked = (mode === 'real');

            if (mode === 'mock') {
                dot.style.background = '#2ea043';
                text.innerText = 'Mock Running';
                config.style.display = 'none';
            } else {
                config.style.display = 'flex';
                if (status === 'connected') {
                    dot.style.background = '#2ea043';
                    text.innerText = 'Connected';
                } else if (status === 'listening') {
                    dot.style.background = '#d29922';
                    text.innerText = 'Waiting for game...';
                } else if (status === 'error') {
                    dot.style.background = '#f85149';
                    text.innerText = 'Error';
                } else {
                    dot.style.background = '#f85149';
                    text.innerText = 'Disconnected';
                }
                if (host) document.getElementById('udp-host').value = host;
                if (port) document.getElementById('udp-port').value = port;
            }
        }

        // Fetch initial status on load
        fetch('/api/telemetry_status').then(function(r) { return r.json(); })
        .then(function(d) { updateConnUI(d.mode, d.status, d.host, d.port); });

        var ws = new WebSocket('ws://' + location.host + '/ws');

        ws.onmessage = function(event) {
            var msg = JSON.parse(event.data);

            if (msg.topic === "telemetry_tick") {
                var d = msg.payload;
                document.getElementById('val-speed').innerText = Math.round(d.speed);
                document.getElementById('val-gear').innerText = d.gear === 0 ? 'N' : (d.gear === -1 ? 'R' : d.gear);
                document.getElementById('val-lap').innerText = d.lap;
                document.getElementById('val-sector').innerText = "Sector " + d.sector;
                document.getElementById('val-rpm').innerText = Math.round(d.engine_rpm);
                document.getElementById('bar-throttle').style.width = (d.throttle * 100) + "%";
                document.getElementById('bar-brake').style.width = (d.brake * 100) + "%";
                document.getElementById('bar-rpm').style.width = Math.min((d.engine_rpm / 13000) * 100, 100) + "%";
                setTireClass(document.getElementById('wear-fl'), d.tire_wear_fl);
                setTireClass(document.getElementById('wear-fr'), d.tire_wear_fr);
                setTireClass(document.getElementById('wear-rl'), d.tire_wear_rl);
                setTireClass(document.getElementById('wear-rr'), d.tire_wear_rr);

            } else if (msg.topic === "car_telemetry_ext") {
                var d = msg.payload;
                // Surface temps [RL, RR, FL, FR]
                setTempColor(document.getElementById('stemp-fl'), d.surface_temps[2], 80, 110);
                setTempColor(document.getElementById('stemp-fr'), d.surface_temps[3], 80, 110);
                setTempColor(document.getElementById('stemp-rl'), d.surface_temps[0], 80, 110);
                setTempColor(document.getElementById('stemp-rr'), d.surface_temps[1], 80, 110);
                // Inner temps
                setTempColor(document.getElementById('itemp-fl'), d.inner_temps[2], 85, 105);
                setTempColor(document.getElementById('itemp-fr'), d.inner_temps[3], 85, 105);
                setTempColor(document.getElementById('itemp-rl'), d.inner_temps[0], 85, 105);
                setTempColor(document.getElementById('itemp-rr'), d.inner_temps[1], 85, 105);
                // Brake temps
                setTempColor(document.getElementById('btemp-fl'), d.brake_temps[2], 200, 900);
                setTempColor(document.getElementById('btemp-fr'), d.brake_temps[3], 200, 900);
                setTempColor(document.getElementById('btemp-rl'), d.brake_temps[0], 200, 900);
                setTempColor(document.getElementById('btemp-rr'), d.brake_temps[1], 200, 900);
                // Pressures
                document.getElementById('press-fl').innerText = d.pressures[2].toFixed(1);
                document.getElementById('press-fr').innerText = d.pressures[3].toFixed(1);
                document.getElementById('press-rl').innerText = d.pressures[0].toFixed(1);
                document.getElementById('press-rr').innerText = d.pressures[1].toFixed(1);
                // DRS
                var drsEl = document.getElementById('ind-drs');
                drsEl.style.display = d.drs ? 'inline-block' : 'none';
                drsEl.className = 'indicator ' + (d.drs ? 'ind-green' : 'ind-red');

            } else if (msg.topic === "car_status_ext") {
                var d = msg.payload;
                // ERS
                var ersPct = (d.ers_store / 4000000 * 100);
                document.getElementById('val-ers').innerText = ersPct.toFixed(0) + "%";
                document.getElementById('bar-ers').style.width = ersPct + "%";
                document.getElementById('val-ers-mode').innerText = ersNames[d.ers_mode] || "?";
                // Fuel
                document.getElementById('val-fuel').innerText = d.fuel_remaining_laps.toFixed(1);
                document.getElementById('bar-fuel').style.width = Math.min(100, (d.fuel_in_tank / 110 * 100)) + "%";
                document.getElementById('val-fuel-mix').innerText = fuelMixNames[d.fuel_mix] || "?";
                // Compound
                var cmp = d.visual_compound;
                var cmpEl = document.getElementById('ind-compound');
                cmpEl.innerText = compoundNames[cmp] || "?";
                cmpEl.style.color = compoundColors[cmp] || "#fff";
                cmpEl.style.borderColor = compoundColors[cmp] || "#fff";
                document.getElementById('val-tyre-age').innerText = d.tyre_age;
                // DRS allowed
                if (d.drs_allowed) {
                    document.getElementById('ind-drs').className = 'indicator ind-green';
                }

            } else if (msg.topic === "car_damage_ext") {
                var d = msg.payload;
                setDmg('dmg-flw', 'dmg-flw-val', d.fl_wing);
                setDmg('dmg-frw', 'dmg-frw-val', d.fr_wing);
                setDmg('dmg-rw', 'dmg-rw-val', d.rear_wing);
                setDmg('dmg-floor', 'dmg-floor-val', d.floor);
                setDmg('dmg-diff', 'dmg-diff-val', d.diffuser);
                setDmg('dmg-side', 'dmg-side-val', d.sidepod);
                setDmg('dmg-gb', 'dmg-gb-val', d.gearbox);
                setDmg('dmg-eng', 'dmg-eng-val', d.engine);

            } else if (msg.topic === "session_info") {
                var d = msg.payload;
                document.getElementById('weather-icon').innerText = weatherIcons[d.weather] || "?";
                document.getElementById('weather-text').innerText = weatherNames[d.weather] || "?";
                document.getElementById('val-track-temp').innerText = d.track_temp;
                document.getElementById('val-air-temp').innerText = d.air_temp;
                document.getElementById('val-time-left').innerText = fmtTimeLeft(d.time_left);
                document.getElementById('val-pit-window').innerText = "Lap " + d.pit_ideal + "-" + d.pit_latest;
                // Safety car
                var scEl = document.getElementById('ind-safety-car');
                if (d.safety_car > 0) {
                    scEl.style.display = 'inline-block';
                    scEl.innerText = scNames[d.safety_car] || "SC";
                    scEl.className = 'indicator ' + (d.safety_car === 1 ? 'ind-yellow' : 'ind-blue');
                } else {
                    scEl.style.display = 'none';
                }

            } else if (msg.topic === "lap_info") {
                var d = msg.payload;
                document.getElementById('val-position').innerText = "P" + d.position + "/" + (d.total_cars || 20);
                document.getElementById('val-gap-front').innerText = d.gap_front > 0 ? "+" + (d.gap_front/1000).toFixed(3) + "s" : "--";
                document.getElementById('val-gap-leader').innerText = d.gap_leader > 0 ? "+" + (d.gap_leader/1000).toFixed(3) + "s" : "Leader";
                document.getElementById('val-last-lap').innerText = fmtLapTime(d.last_lap);
                document.getElementById('val-pit-stops').innerText = d.pit_stops;
                if (d.last_lap > 0 && (bestLapTime === 0 || d.last_lap < bestLapTime)) {
                    bestLapTime = d.last_lap;
                    document.getElementById('val-best-lap').innerText = fmtLapTime(bestLapTime);
                }

            } else if (msg.topic === "race_event") {
                addLog('event-log', msg.payload.text, 'event-log');

            } else if (msg.topic === "driving_insight") {
                var d = msg.payload;
                var cls = "info-log";
                if (d.type === "warning") cls = "warn-log";
                if (d.type === "encouragement") cls = "encourage";
                if (d.type === "strategy") cls = "strat-log";
                addLog('insights', '[ENGINEER] ' + d.message, cls);

            } else if (msg.topic === "agent_status") {
                addLog('agent-log', '> ' + msg.payload.message, 'agent-log');

            } else if (msg.topic === "telemetry_status") {
                var d = msg.payload;
                updateConnUI(d.mode, d.status, d.host, d.port);
                if (d.status === 'connected') {
                    addLog('insights', '[SYSTEM] F1 25 game connected - receiving live telemetry', 'info-log');
                } else if (d.status === 'disconnected') {
                    addLog('insights', '[SYSTEM] F1 25 game disconnected', 'warn-log');
                } else if (d.status === 'error') {
                    addLog('insights', '[SYSTEM] UDP error: ' + (d.error || 'unknown'), 'warn-log');
                }
            }
        };

        ws.onclose = function() { addLog('insights', 'Connection lost.', 'warn-log'); };
    </script>
</body>
</html>
"""


@app.get("/")
async def get_dashboard():
    return HTMLResponse(html)


@app.websocket("/ws")
async def websocket_endpoint(websocket: WebSocket):
    await websocket.accept()
    queue: asyncio.Queue = asyncio.Queue()

    # Legacy telemetry tick
    async def telemetry_handler(data):
        await queue.put({"topic": "telemetry_tick", "payload": data.model_dump()})

    async def insight_handler(data):
        await queue.put({"topic": "driving_insight", "payload": data.model_dump()})

    async def agent_status_handler(data):
        await queue.put({"topic": "agent_status", "payload": data})

    # New packet handlers for UI
    async def car_telemetry_handler(data):
        idx = data.header.player_car_index
        if idx >= len(data.car_telemetry_data):
            return
        ct = data.car_telemetry_data[idx]
        await queue.put(
            {
                "topic": "car_telemetry_ext",
                "payload": {
                    "surface_temps": ct.tyres_surface_temperature,
                    "inner_temps": ct.tyres_inner_temperature,
                    "brake_temps": ct.brakes_temperature,
                    "pressures": ct.tyres_pressure,
                    "drs": ct.drs,
                    "engine_temp": ct.engine_temperature,
                },
            }
        )

    async def car_status_handler(data):
        idx = data.header.player_car_index
        if idx >= len(data.car_status_data):
            return
        s = data.car_status_data[idx]
        await queue.put(
            {
                "topic": "car_status_ext",
                "payload": {
                    "ers_store": s.ers_store_energy,
                    "ers_mode": s.ers_deploy_mode,
                    "fuel_in_tank": s.fuel_in_tank,
                    "fuel_remaining_laps": s.fuel_remaining_laps,
                    "fuel_mix": s.fuel_mix,
                    "visual_compound": s.visual_tyre_compound,
                    "tyre_age": s.tyres_age_laps,
                    "drs_allowed": s.drs_allowed,
                },
            }
        )

    async def car_damage_handler(data):
        idx = data.header.player_car_index
        if idx >= len(data.car_damage_data):
            return
        d = data.car_damage_data[idx]
        await queue.put(
            {
                "topic": "car_damage_ext",
                "payload": {
                    "fl_wing": d.front_left_wing_damage,
                    "fr_wing": d.front_right_wing_damage,
                    "rear_wing": d.rear_wing_damage,
                    "floor": d.floor_damage,
                    "diffuser": d.diffuser_damage,
                    "sidepod": d.sidepod_damage,
                    "gearbox": d.gear_box_damage,
                    "engine": d.engine_damage,
                },
            }
        )

    async def session_handler(data):
        await queue.put(
            {
                "topic": "session_info",
                "payload": {
                    "weather": data.weather,
                    "track_temp": data.track_temperature,
                    "air_temp": data.air_temperature,
                    "time_left": data.session_time_left,
                    "safety_car": data.safety_car_status,
                    "pit_ideal": data.pit_stop_window_ideal_lap,
                    "pit_latest": data.pit_stop_window_latest_lap,
                },
            }
        )

    async def lap_data_handler(data):
        idx = data.header.player_car_index
        if idx >= len(data.car_lap_data):
            return
        lap = data.car_lap_data[idx]
        await queue.put(
            {
                "topic": "lap_info",
                "payload": {
                    "position": lap.car_position,
                    "total_cars": 20,
                    "gap_front": lap.delta_to_car_in_front_in_ms,
                    "gap_leader": lap.delta_to_race_leader_in_ms,
                    "last_lap": lap.last_lap_time_in_ms,
                    "pit_stops": lap.num_pit_stops,
                },
            }
        )

    async def event_handler(data):
        detail = ""
        d = data.event_details
        if d.speed is not None:
            detail = f" ({d.speed:.0f} km/h)"
        elif d.lap_time is not None:
            detail = f" ({d.lap_time:.3f}s)"
        elif d.overtaking_vehicle_idx is not None:
            detail = f" (car {d.overtaking_vehicle_idx} overtakes {d.being_overtaken_vehicle_idx})"
        await queue.put(
            {
                "topic": "race_event",
                "payload": {
                    "code": data.event_string_code,
                    "text": f"[{data.event_string_code}]{detail}",
                },
            }
        )

    async def telemetry_status_handler(data):
        await queue.put({"topic": "telemetry_status", "payload": data})

    bus.subscribe("telemetry_tick", telemetry_handler)
    bus.subscribe("driving_insight", insight_handler)
    bus.subscribe("agent_status", agent_status_handler)
    bus.subscribe("packet_car_telemetry", car_telemetry_handler)
    bus.subscribe("packet_car_status", car_status_handler)
    bus.subscribe("packet_car_damage", car_damage_handler)
    bus.subscribe("packet_session", session_handler)
    bus.subscribe("packet_lap_data", lap_data_handler)
    bus.subscribe("packet_event", event_handler)
    bus.subscribe("telemetry_status", telemetry_status_handler)

    try:
        while True:
            msg = await queue.get()
            await websocket.send_json(msg)
    except WebSocketDisconnect:
        pass
    except Exception as e:
        logger.error(f"Websocket error: {e}")
    finally:
        bus.unsubscribe("telemetry_tick", telemetry_handler)
        bus.unsubscribe("driving_insight", insight_handler)
        bus.unsubscribe("agent_status", agent_status_handler)
        bus.unsubscribe("packet_car_telemetry", car_telemetry_handler)
        bus.unsubscribe("packet_car_status", car_status_handler)
        bus.unsubscribe("packet_car_damage", car_damage_handler)
        bus.unsubscribe("packet_session", session_handler)
        bus.unsubscribe("packet_lap_data", lap_data_handler)
        bus.unsubscribe("packet_event", event_handler)
        bus.unsubscribe("telemetry_status", telemetry_status_handler)
