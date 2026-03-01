import os
import time
import httpx
import logging
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

from google import genai
from google.genai import types

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s [OPENCODE AGENT] %(message)s"
)
logger = logging.getLogger(__name__)

# Config
WORKSPACE_DIR = os.path.join(os.path.dirname(__file__), "workspace")
DUCKDB_PATH = os.path.join(os.path.dirname(__file__), "live_session.duckdb")
API_URL = "http://localhost:8000/api/strategy"


def read_workspace_files():
    """Reads the contextual markdown files from the workspace."""
    context = ""
    for filename in os.listdir(WORKSPACE_DIR):
        if filename.endswith(".md"):
            with open(os.path.join(WORKSPACE_DIR, filename), "r") as f:
                context += f"\n\n--- {filename} ---\n{f.read()}"
    return context


def broadcast_status(message: str):
    """Sends a status update to the UI dashboard so the user can see what the agent is doing."""
    logger.info(message)
    try:
        httpx.post("http://localhost:8000/api/agent_status", json={"message": message})
    except Exception:
        pass


def _query_via_http(sql: str):
    """Queries DuckDB through the main server's HTTP API to avoid lock conflicts."""
    try:
        response = httpx.post(
            "http://localhost:8000/api/query", json={"sql": sql}, timeout=5.0
        )
        response.raise_for_status()
        data = response.json()
        if data.get("status") == "success":
            return data.get("rows", [])
        else:
            logger.warning(f"Query error: {data.get('error')}")
            return None
    except Exception as e:
        logger.warning(f"HTTP query failed: {e}")
        return None


def query_telemetry():
    """Queries the DuckDB instance via the main server's HTTP API."""
    data_sections = []

    # 1. Tire wear from car_damage table (new expanded data)
    rows = _query_via_http("""
        SELECT tyres_wear_fl, tyres_wear_fr, tyres_wear_rl, tyres_wear_rr
        FROM car_damage WHERE car_index = 0
        ORDER BY timestamp DESC LIMIT 3
    """)
    if rows:
        data_sections.append("Recent Tire Wear (FL/FR/RL/RR):")
        for row in rows:
            data_sections.append(
                f"  FL:{row[0]:.1f}% FR:{row[1]:.1f}% RL:{row[2]:.1f}% RR:{row[3]:.1f}%"
            )

    # 2. Fuel and compound
    rows = _query_via_http("""
        SELECT fuel_in_tank, fuel_remaining_laps, actual_tyre_compound, tyres_age_laps
        FROM car_status WHERE car_index = 0
        ORDER BY timestamp DESC LIMIT 1
    """)
    if rows:
        compound_map = {16: "Soft", 17: "Medium", 18: "Hard", 7: "Inter", 8: "Wet"}
        r = rows[0]
        cmp = compound_map.get(int(r[2]), "Unknown")
        data_sections.append(
            f"Fuel: {r[0]:.1f}kg ({r[1]:.1f} laps remaining), Compound: {cmp}, Age: {int(r[3])} laps"
        )

    # 3. Lap times
    rows = _query_via_http("""
        SELECT lap_num, lap_time_in_ms, sector1_time_in_ms, sector2_time_in_ms, sector3_time_in_ms
        FROM session_history WHERE car_index = 0 ORDER BY lap_num DESC LIMIT 3
    """)
    if rows:
        data_sections.append("Lap Times:")
        for r in rows:
            data_sections.append(
                f"  Lap {int(r[0])}: {r[1] / 1000:.3f}s (S1:{r[2] / 1000:.3f} S2:{r[3] / 1000:.3f} S3:{r[4] / 1000:.3f})"
            )

    # 4. Weather & session
    rows = _query_via_http("""
        SELECT weather, track_temperature, air_temperature, rain_percentage
        FROM session_data ORDER BY timestamp DESC LIMIT 1
    """)
    if rows:
        weather_map = {
            0: "Clear",
            1: "Light Cloud",
            2: "Overcast",
            3: "Light Rain",
            4: "Heavy Rain",
            5: "Storm",
        }
        r = rows[0]
        data_sections.append(
            f"Weather: {weather_map.get(int(r[0]), '?')}, Track: {int(r[1])}C, Air: {int(r[2])}C, Rain: {int(r[3])}%"
        )

    # 5. Fallback to legacy table if new tables are empty
    if not data_sections:
        rows = _query_via_http("""
            SELECT lap, MAX(tire_wear_fl) as max_wear_fl, MAX(tire_wear_rr) as max_wear_rr
            FROM telemetry GROUP BY lap ORDER BY lap DESC LIMIT 3
        """)
        if rows:
            data_sections.append("Recent Laps Tire Wear (Lap, FL%, RR%):")
            for row in rows:
                data_sections.append(
                    f"- Lap {int(row[0])}: Front-Left {row[1]:.1f}%, Rear-Right {row[2]:.1f}%"
                )

    if not data_sections:
        return "No telemetry data available yet. Main server may not be running."

    return "\n".join(data_sections)


# Define the Tool for Gemini to call
def send_insight_to_race_engineer(summary: str, recommendation: str, criticality: int):
    """
    Sends a strategic insight to the main Race Engineer server.
    Use this tool when you have formed a concrete recommendation based on telemetry and workspace knowledge.

    Args:
        summary: A short description of the current situation (e.g. "Rear right wear is 46%").
        recommendation: What the driver should do (e.g. "Box this lap for hard tires").
        criticality: Priority level 1 to 5. 5 is extremely critical (immediate pit stop required).
    """
    logger.info(
        f"Tool called! Sending insight to Race Engineer: {recommendation} (Criticality: {criticality})"
    )
    try:
        response = httpx.post(
            API_URL,
            json={
                "summary": summary,
                "recommendation": recommendation,
                "criticality": criticality,
            },
        )
        response.raise_for_status()
        return "Successfully sent to Race Engineer."
    except Exception as e:
        logger.error(f"Failed to push to Race Engineer webhook: {e}")
        return f"Error sending insight: {e}"


def run_agent_loop():
    logger.info("Starting OpenCode Analyst Agent Workspace Server...")
    api_key = os.getenv("GEMINI_API_KEY")
    client = genai.Client(api_key=api_key) if api_key else None

    while True:
        try:
            broadcast_status("Reading workspace markdown files...")
            workspace_context = read_workspace_files()
            system_instruction = (
                "You are an OpenCode Agent acting as a backend Strategy Analyst for an F1 team. "
                "You sit in a remote garage reading historical workspace files and live database queries. "
                "Your job is to cross-reference the live telemetry with your workspace learnings. "
                f"Here is your Workspace Knowledge:\n{workspace_context}\n\n"
                "If you see a situation that warrants action (e.g., tire wear crossing a critical threshold "
                "mentioned in past_learnings.md), you MUST use the `send_insight_to_race_engineer` tool "
                "to alert the Race Engineer. If everything is fine, just say 'No action needed'."
            )

            broadcast_status("Querying historical telemetry database (DuckDB)...")
            telemetry_data = query_telemetry()
            broadcast_status("Analyzing data context using Gemini 2.5 Flash...")

            prompt = f"Live Database Query Results:\n{telemetry_data}\n\nDo we need to send an insight?"

            # Using function calling (tools)
            if client:
                response = client.models.generate_content(
                    model="gemini-2.5-flash",
                    contents=prompt,
                    config=types.GenerateContentConfig(
                        system_instruction=system_instruction,
                        temperature=0.2,
                        tools=[send_insight_to_race_engineer],
                    ),
                )

                # Check for function calls
                if hasattr(response, "function_calls") and response.function_calls:
                    for function_call in response.function_calls:
                        if function_call.name == "send_insight_to_race_engineer":
                            args = function_call.args
                            if args:
                                broadcast_status(
                                    "CRITICAL: Executing 'send_insight_to_race_engineer' tool."
                                )
                                # Execute the local function
                                result = send_insight_to_race_engineer(
                                    summary=args.get("summary", ""),
                                    recommendation=args.get("recommendation", ""),
                                    criticality=int(args.get("criticality", 3)),
                                )
                else:
                    if response.text:
                        broadcast_status(f"Analysis Complete: {response.text.strip()}")
            else:
                broadcast_status("No API key, running MOCK Tool Call...")
                send_insight_to_race_engineer(
                    summary="Rear tire wear is high due to aggressive driving",
                    recommendation="Tell Tushar to chill on exits or box next lap.",
                    criticality=4,
                )

        except Exception as e:
            logger.error(f"Agent loop error: {e}")

        # Poll every 15 seconds
        broadcast_status("Sleeping for 15 seconds before next analysis loop.")
        time.sleep(15)


if __name__ == "__main__":
    run_agent_loop()
