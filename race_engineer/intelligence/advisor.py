import os
import logging
from typing import Optional

from google import genai
from google.genai import types

from race_engineer.core.event_bus import bus
from race_engineer.telemetry.models import TelemetryTick, DriverQuery, DrivingInsight
from race_engineer.telemetry.packets import (
    PacketCarStatusData,
    PacketCarDamageData,
    PacketSessionData,
    PacketLapData,
)
from race_engineer.telemetry.enums import (
    WEATHER_NAMES,
    FUEL_MIX_NAMES,
    ERS_MODE_NAMES,
    SAFETY_CAR_NAMES,
    TYRE_COMPOUND_SHORT,
)
from race_engineer.intelligence.models import StrategyInsight

logger = logging.getLogger(__name__)


class LLMAdvisor:
    """
    Acts as the brain of the Race Engineer.
    Maintains the latest telemetry state from expanded packets and uses Gemini
    to answer driver queries dynamically with full race context.
    """

    def __init__(self):
        self.api_key = os.getenv("GEMINI_API_KEY")
        self.client = genai.Client(api_key=self.api_key) if self.api_key else None

        # State tracking (legacy)
        self.latest_telemetry: Optional[TelemetryTick] = None
        self.latest_strategy: Optional[StrategyInsight] = None

        # Expanded state tracking
        self._car_status: Optional[PacketCarStatusData] = None
        self._car_damage: Optional[PacketCarDamageData] = None
        self._session: Optional[PacketSessionData] = None
        self._lap_data: Optional[PacketLapData] = None
        self._player_idx: int = 0

        # Subscribe to data
        bus.subscribe("telemetry_tick", self._update_telemetry)
        bus.subscribe("strategy_insight", self._update_strategy)
        bus.subscribe("driver_query", self._handle_query)
        bus.subscribe("packet_car_status", self._update_car_status)
        bus.subscribe("packet_car_damage", self._update_car_damage)
        bus.subscribe("packet_session", self._update_session)
        bus.subscribe("packet_lap_data", self._update_lap_data)

    async def _update_telemetry(self, tick: TelemetryTick):
        self.latest_telemetry = tick

    async def _update_strategy(self, insight: StrategyInsight):
        self.latest_strategy = insight
        if insight.criticality >= 4:
            logger.info(
                f"LLM Advisor received CRITICAL strategy: {insight.recommendation}"
            )
            await self._send_insight(
                f"Strategy Team update: {insight.recommendation}",
                "strategy",
                insight.criticality,
            )

    async def _update_car_status(self, data: PacketCarStatusData):
        self._car_status = data
        self._player_idx = data.header.player_car_index

    async def _update_car_damage(self, data: PacketCarDamageData):
        self._car_damage = data

    async def _update_session(self, data: PacketSessionData):
        self._session = data

    async def _update_lap_data(self, data: PacketLapData):
        self._lap_data = data

    def _build_context(self) -> str:
        """Build a rich context string from all available data."""
        parts = []

        # Basic telemetry
        t = self.latest_telemetry
        if t:
            parts.append(
                f"Speed: {t.speed:.0f}km/h, Gear: {t.gear}, RPM: {t.engine_rpm}"
            )
            parts.append(f"Lap: {t.lap}, Sector: {t.sector}")

        # Car status (fuel, ERS, tires)
        if self._car_status and self._player_idx < len(
            self._car_status.car_status_data
        ):
            s = self._car_status.car_status_data[self._player_idx]
            compound = TYRE_COMPOUND_SHORT.get(s.visual_tyre_compound, "?")
            fuel_mix = FUEL_MIX_NAMES.get(s.fuel_mix, "?")
            ers_mode = ERS_MODE_NAMES.get(s.ers_deploy_mode, "?")
            ers_pct = s.ers_store_energy / 4000000.0 * 100

            parts.append(f"Tire: {compound} (Age: {s.tyres_age_laps} laps)")
            parts.append(
                f"Fuel: {s.fuel_remaining_laps:.1f} laps remaining (Mix: {fuel_mix})"
            )
            parts.append(f"ERS: {ers_pct:.0f}% ({ers_mode})")
            parts.append(f"DRS: {'Available' if s.drs_allowed else 'Not available'}")

        # Car damage
        if self._car_damage and self._player_idx < len(
            self._car_damage.car_damage_data
        ):
            d = self._car_damage.car_damage_data[self._player_idx]
            parts.append(
                f"Tire Wear - FL:{d.tyres_wear[2]:.1f}%, FR:{d.tyres_wear[3]:.1f}%, "
                f"RL:{d.tyres_wear[0]:.1f}%, RR:{d.tyres_wear[1]:.1f}%"
            )
            dmg_parts = []
            if d.front_left_wing_damage > 0:
                dmg_parts.append(f"FL Wing:{d.front_left_wing_damage}%")
            if d.front_right_wing_damage > 0:
                dmg_parts.append(f"FR Wing:{d.front_right_wing_damage}%")
            if d.rear_wing_damage > 0:
                dmg_parts.append(f"Rear Wing:{d.rear_wing_damage}%")
            if d.floor_damage > 0:
                dmg_parts.append(f"Floor:{d.floor_damage}%")
            if d.gear_box_damage > 0:
                dmg_parts.append(f"Gearbox:{d.gear_box_damage}%")
            if d.engine_damage > 0:
                dmg_parts.append(f"Engine:{d.engine_damage}%")
            if dmg_parts:
                parts.append(f"Damage: {', '.join(dmg_parts)}")

        # Lap data (position, gaps)
        if self._lap_data and self._player_idx < len(self._lap_data.car_lap_data):
            lap = self._lap_data.car_lap_data[self._player_idx]
            parts.append(f"Position: P{lap.car_position}")
            if lap.delta_to_car_in_front_in_ms > 0:
                parts.append(
                    f"Gap to front: {lap.delta_to_car_in_front_in_ms / 1000:.3f}s"
                )
            if lap.last_lap_time_in_ms > 0:
                parts.append(f"Last Lap: {lap.last_lap_time_in_ms / 1000:.3f}s")
            parts.append(f"Pit Stops: {lap.num_pit_stops}")

        # Session (weather, track conditions)
        if self._session:
            weather = WEATHER_NAMES.get(self._session.weather, "Unknown")
            sc = SAFETY_CAR_NAMES.get(self._session.safety_car_status, "None")
            parts.append(
                f"Weather: {weather}, Track: {self._session.track_temperature}C, Air: {self._session.air_temperature}C"
            )
            if self._session.safety_car_status > 0:
                parts.append(f"Safety Car: {sc}")
            parts.append(f"Total Laps: {self._session.total_laps}")

        # Strategy team input
        if self.latest_strategy:
            parts.append(
                f"Strategy Team: {self.latest_strategy.summary}. Rec: {self.latest_strategy.recommendation}"
            )

        return ", ".join(parts) if parts else "No telemetry data available yet."

    async def _handle_query(self, query: DriverQuery):
        """When the driver asks a question, consult Gemini using full race context."""
        logger.info(f"LLM Advisor received query: '{query.query}'")

        if not self.latest_telemetry:
            await self._send_insight(
                "I don't have any telemetry data yet. Stand by.", "info"
            )
            return

        context = self._build_context()

        if not self.client:
            logger.warning("GEMINI_API_KEY not set. Using fallback dynamic response.")
            t = self.latest_telemetry
            fallback_msg = f"I'm offline, but I see your front left tire is at {t.tire_wear_fl:.1f} percent."
            await self._send_insight(fallback_msg, "info")
            return

        system_prompt = (
            "You are an F1 Race Engineer speaking directly over the radio to your driver. "
            "Give detailed, thorough answers. Be conversational but informative. "
            "Use the provided live telemetry to answer the driver's question accurately. "
            "You have access to tire wear, fuel levels, ERS state, weather, position, "
            "damage status, and strategy team analysis. "
            f"Live Telemetry Context: {context}"
        )

        try:
            response = await self.client.aio.models.generate_content(
                model="gemini-2.5-flash",
                contents=query.query,
                config=types.GenerateContentConfig(
                    system_instruction=system_prompt,
                    temperature=0.3,
                    max_output_tokens=500,
                ),
            )

            answer = response.text
            if answer:
                await self._send_insight(answer.strip(), "info", priority=4)

        except Exception as e:
            logger.error(f"Failed to generate Gemini response: {e}")
            await self._send_insight(
                "I'm having trouble with the data connection.", "warning", priority=5
            )

    async def _send_insight(self, message: str, insight_type: str, priority: int = 3):
        insight = DrivingInsight(message=message, type=insight_type, priority=priority)
        await bus.publish("driving_insight", insight)
