import asyncio
import json
import logging
from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse
from race_engineer.core.event_bus import bus

logger = logging.getLogger(__name__)

app = FastAPI(title="Race Engineer Dashboard")

html = """
<!DOCTYPE html>
<html>
    <head>
        <title>Race Engineer Dashboard</title>
        <style>
            body { background-color: #121212; color: #ffffff; font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 20px; }
            h1 { color: #ff3333; text-transform: uppercase; letter-spacing: 2px; }
            .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
            .card { background: #1e1e1e; padding: 20px; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.3); }
            .data-row { display: flex; justify-content: space-between; margin-bottom: 10px; border-bottom: 1px solid #333; padding-bottom: 5px; }
            .label { color: #888; text-transform: uppercase; font-size: 0.85em; }
            .value { font-weight: bold; font-size: 1.2em; color: #00ffcc; }
            ul { list-style: none; padding: 0; margin: 0; max-height: 400px; overflow-y: auto; }
            li { padding: 10px; margin-bottom: 8px; border-radius: 4px; background: #2a2a2a; border-left: 4px solid #555; font-size: 14px; }
            li.encouragement { border-left-color: #00cc66; }
            li.warning { border-left-color: #ff9900; }
            li.strategy { border-left-color: #cc00ff; }
            li.info { border-left-color: #0099ff; }
            .time { font-size: 0.85em; color: #666; margin-right: 10px; }
        </style>
    </head>
    <body>
        <h1>Race Engineer Dashboard</h1>
        <div class="grid">
            <div class="card">
                <h2>Live Telemetry</h2>
                <div class="data-row"><span class="label">Speed</span><span class="value" id="val-speed">0 km/h</span></div>
                <div class="data-row"><span class="label">Gear</span><span class="value" id="val-gear">N</span></div>
                <div class="data-row"><span class="label">RPM</span><span class="value" id="val-rpm">0</span></div>
                <div class="data-row"><span class="label">Lap</span><span class="value" id="val-lap">0</span></div>
                <div class="data-row"><span class="label">Sector</span><span class="value" id="val-sector">0</span></div>
                <h3 style="margin-top:20px; color:#888;">Tire Wear</h3>
                <div class="data-row"><span class="label">Front Left</span><span class="value" id="val-wear-fl">0%</span></div>
                <div class="data-row"><span class="label">Front Right</span><span class="value" id="val-wear-fr">0%</span></div>
                <div class="data-row"><span class="label">Rear Left</span><span class="value" id="val-wear-rl">0%</span></div>
                <div class="data-row"><span class="label">Rear Right</span><span class="value" id="val-wear-rr">0%</span></div>
            </div>
            <div class="card">
                <h2>Engineer Comms</h2>
                <ul id="insights">
                    <li><span class="time">System</span> Waiting for data...</li>
                </ul>
            </div>
        </div>
        <script>
            var ws = new WebSocket(`ws://${location.host}/ws`);
            var insightsList = document.getElementById('insights');
            
            ws.onmessage = function(event) {
                var msg = JSON.parse(event.data);
                if (msg.topic === "telemetry_tick") {
                    var data = msg.payload;
                    document.getElementById('val-speed').innerText = Math.round(data.speed) + " km/h";
                    document.getElementById('val-gear').innerText = data.gear === 0 ? 'N' : (data.gear === -1 ? 'R' : data.gear);
                    document.getElementById('val-rpm').innerText = data.engine_rpm;
                    document.getElementById('val-lap').innerText = data.lap;
                    document.getElementById('val-sector').innerText = data.sector;
                    document.getElementById('val-wear-fl').innerText = data.tire_wear_fl.toFixed(1) + "%";
                    document.getElementById('val-wear-fr').innerText = data.tire_wear_fr.toFixed(1) + "%";
                    document.getElementById('val-wear-rl').innerText = data.tire_wear_rl.toFixed(1) + "%";
                    document.getElementById('val-wear-rr').innerText = data.tire_wear_rr.toFixed(1) + "%";
                } else if (msg.topic === "driving_insight") {
                    var data = msg.payload;
                    var li = document.createElement("li");
                    li.className = data.type;
                    
                    var timeSpan = document.createElement("span");
                    timeSpan.className = "time";
                    var now = new Date();
                    timeSpan.innerText = now.getHours().toString().padStart(2, '0') + ":" + 
                                       now.getMinutes().toString().padStart(2, '0') + ":" + 
                                       now.getSeconds().toString().padStart(2, '0');
                    
                    li.appendChild(timeSpan);
                    li.appendChild(document.createTextNode("[" + data.type.toUpperCase() + "] " + data.message));
                    
                    insightsList.prepend(li);
                    
                    // Keep list manageable
                    if (insightsList.children.length > 20) {
                        insightsList.removeChild(insightsList.lastChild);
                    }
                }
            };
            ws.onclose = function() {
                var li = document.createElement("li");
                li.innerText = "Connection lost. Please refresh.";
                insightsList.prepend(li);
            };
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

    async def telemetry_handler(data):
        await queue.put({"topic": "telemetry_tick", "payload": data.model_dump()})

    async def insight_handler(data):
        await queue.put({"topic": "driving_insight", "payload": data.model_dump()})

    bus.subscribe("telemetry_tick", telemetry_handler)
    bus.subscribe("driving_insight", insight_handler)

    try:
        while True:
            # We use an asyncio task to wait for queue items and send them to the websocket
            msg = await queue.get()
            await websocket.send_json(msg)
    except WebSocketDisconnect:
        logger.info("Websocket client disconnected")
    except Exception as e:
        logger.error(f"Websocket error: {e}")
    finally:
        # Important: clean up subscriptions to avoid memory leaks
        bus.unsubscribe("telemetry_tick", telemetry_handler)
        bus.unsubscribe("driving_insight", insight_handler)
