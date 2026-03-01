import os
import asyncio
import logging
from google import genai
from google.genai import types

from race_engineer.core.event_bus import bus
from race_engineer.data.store import DataStore
from race_engineer.intelligence.models import StrategyInsight

logger = logging.getLogger(__name__)


class StrategyTeamWorker:
    """
    A background AI agent representing the "Analyst Team".
    Queries expanded DuckDB tables for tire wear, fuel, lap times, weather,
    and publishes StrategyInsights for the Race Engineer.
    """

    def __init__(self, datastore: DataStore, poll_interval: int = 15):
        self.datastore = datastore
        self.poll_interval = poll_interval
        self.api_key = os.getenv("GEMINI_API_KEY")
        self.client = genai.Client(api_key=self.api_key) if self.api_key else None
        self._is_running = False

    async def start(self):
        self._is_running = True
        logger.info("Strategy Team Analyst background agent started.")
        await self._worker_loop()

    def stop(self):
        self._is_running = False

    async def _worker_loop(self):
        while self._is_running:
            await asyncio.sleep(self.poll_interval)
            await self._analyze_telemetry_trends()

    async def _analyze_telemetry_trends(self):
        """Run analytical queries against expanded tables and feed to Gemini."""
        try:
            loop = asyncio.get_running_loop()
            data_sections = []

            # 1. Tire wear trend (from car_damage table)
            try:
                wear_data = await loop.run_in_executor(
                    None,
                    self.datastore.query,
                    """
                    SELECT cd.timestamp, cd.tyres_wear_fl, cd.tyres_wear_fr,
                           cd.tyres_wear_rl, cd.tyres_wear_rr
                    FROM car_damage cd
                    WHERE cd.car_index = 0
                    ORDER BY cd.timestamp DESC LIMIT 5
                """,
                )
                if wear_data:
                    data_sections.append("Tire Wear (latest 5 samples, FL/FR/RL/RR):")
                    for row in wear_data:
                        data_sections.append(
                            f"  FL:{row[1]:.1f}% FR:{row[2]:.1f}% RL:{row[3]:.1f}% RR:{row[4]:.1f}%"
                        )
            except Exception:
                pass

            # 2. Fuel consumption trend
            try:
                fuel_data = await loop.run_in_executor(
                    None,
                    self.datastore.query,
                    """
                    SELECT cs.fuel_in_tank, cs.fuel_remaining_laps,
                           cs.actual_tyre_compound, cs.tyres_age_laps
                    FROM car_status cs
                    WHERE cs.car_index = 0
                    ORDER BY cs.timestamp DESC LIMIT 3
                """,
                )
                if fuel_data:
                    data_sections.append("Fuel & Compound (latest 3 samples):")
                    for row in fuel_data:
                        compound_map = {
                            16: "Soft",
                            17: "Medium",
                            18: "Hard",
                            7: "Inter",
                            8: "Wet",
                        }
                        cmp = compound_map.get(row[2], "Unknown")
                        data_sections.append(
                            f"  Tank:{row[0]:.1f}kg Remaining:{row[1]:.1f} laps Compound:{cmp} Age:{row[3]} laps"
                        )
            except Exception:
                pass

            # 3. Lap time trend from session history
            try:
                lap_times = await loop.run_in_executor(
                    None,
                    self.datastore.query,
                    """
                    SELECT lap_num, lap_time_in_ms,
                           sector1_time_in_ms, sector2_time_in_ms, sector3_time_in_ms
                    FROM session_history
                    WHERE car_index = 0
                    ORDER BY lap_num DESC LIMIT 5
                """,
                )
                if lap_times:
                    data_sections.append("Lap Times (latest 5 laps):")
                    for row in lap_times:
                        data_sections.append(
                            f"  Lap {row[0]}: {row[1] / 1000:.3f}s "
                            f"(S1:{row[2] / 1000:.3f} S2:{row[3] / 1000:.3f} S3:{row[4] / 1000:.3f})"
                        )
            except Exception:
                pass

            # 4. Weather/session conditions
            try:
                session_data = await loop.run_in_executor(
                    None,
                    self.datastore.query,
                    """
                    SELECT weather, track_temperature, air_temperature,
                           safety_car_status, rain_percentage,
                           pit_stop_window_ideal_lap, pit_stop_window_latest_lap
                    FROM session_data
                    ORDER BY timestamp DESC LIMIT 1
                """,
                )
                if session_data:
                    row = session_data[0]
                    weather_map = {
                        0: "Clear",
                        1: "Light Cloud",
                        2: "Overcast",
                        3: "Light Rain",
                        4: "Heavy Rain",
                        5: "Storm",
                    }
                    data_sections.append(
                        f"Conditions: {weather_map.get(row[0], '?')}, "
                        f"Track:{row[1]}C, Air:{row[2]}C, Rain:{row[4]}%, "
                        f"Pit Window: Lap {row[5]}-{row[6]}"
                    )
            except Exception:
                pass

            # 5. Position and gaps
            try:
                position_data = await loop.run_in_executor(
                    None,
                    self.datastore.query,
                    """
                    SELECT car_position, delta_to_car_in_front_in_ms,
                           delta_to_race_leader_in_ms, current_lap_num, num_pit_stops
                    FROM lap_data
                    WHERE car_index = 0
                    ORDER BY timestamp DESC LIMIT 1
                """,
                )
                if position_data:
                    row = position_data[0]
                    data_sections.append(
                        f"Position: P{row[0]}, Gap to front: {row[1] / 1000:.3f}s, "
                        f"Gap to leader: {row[2] / 1000:.3f}s, Lap: {row[3]}, Pit stops: {row[4]}"
                    )
            except Exception:
                pass

            if not data_sections:
                return  # Not enough data yet

            # Legacy fallback: also check old telemetry table
            try:
                legacy_wear = await loop.run_in_executor(
                    None,
                    self.datastore.query,
                    """
                    SELECT lap, MAX(tire_wear_fl) as max_wear_fl, MAX(tire_wear_rr) as max_wear_rr
                    FROM telemetry GROUP BY lap ORDER BY lap DESC LIMIT 3
                """,
                )
                if legacy_wear and not any("Tire Wear" in s for s in data_sections):
                    data_sections.append("Legacy Tire Wear (Lap, FL%, RR%):")
                    for row in legacy_wear:
                        data_sections.append(
                            f"  Lap {row[0]}: FL {row[1]:.1f}%, RR {row[2]:.1f}%"
                        )
            except Exception:
                pass

            data_summary = "\n".join(data_sections)
            logger.info("Strategy Team generated data summary, invoking Analyst AI...")

            if not self.client:
                fallback_insight = StrategyInsight(
                    summary="Tire wear data collected.",
                    recommendation="Continue current pace.",
                    criticality=2,
                )
                await bus.publish("strategy_insight", fallback_insight)
                return

            system_instruction = (
                "You are the Lead Strategy Analyst for an F1 team. "
                "You review raw database aggregates including tire wear, fuel levels, "
                "lap times, weather conditions, and position data. "
                "Provide a short strategic summary and recommendation to the Race Engineer. "
                "Consider tire degradation trends, fuel management, weather changes, "
                "and pit stop timing in your analysis. "
                "Your output must be EXACTLY two lines:\n"
                "Line 1: 'Summary: <your analysis>'\n"
                "Line 2: 'Recommendation: <what the driver should do>'"
            )

            response = await self.client.aio.models.generate_content(
                model="gemini-2.5-flash",
                contents=data_summary,
                config=types.GenerateContentConfig(
                    system_instruction=system_instruction,
                    temperature=0.2,
                    max_output_tokens=150,
                ),
            )

            text = response.text
            if not text:
                return

            lines = [l.strip() for l in text.split("\n") if l.strip()]
            summary = (
                lines[0].replace("Summary:", "").strip()
                if len(lines) > 0
                else "Analysis complete."
            )
            rec = (
                lines[1].replace("Recommendation:", "").strip()
                if len(lines) > 1
                else "Keep pushing."
            )

            criticality = 3
            if (
                "box" in rec.lower()
                or "critical" in summary.lower()
                or "pit" in rec.lower()
            ):
                criticality = 5

            insight = StrategyInsight(
                summary=summary, recommendation=rec, criticality=criticality
            )

            logger.info(f"Strategy Team published insight: {summary}")
            await bus.publish("strategy_insight", insight)

        except Exception as e:
            logger.error(f"Strategy Team analysis failed: {e}")
