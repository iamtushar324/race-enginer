import os
import logging
from typing import Optional

from openai import AsyncOpenAI
from race_engineer.core.event_bus import bus
from race_engineer.telemetry.models import TelemetryTick, DriverQuery, DrivingInsight

logger = logging.getLogger(__name__)

class LLMAdvisor:
    """
    Acts as the brain of the Race Engineer.
    Maintains the latest telemetry state and uses an LLM to answer driver queries dynamically.
    """
    def __init__(self):
        self.api_key = os.getenv("OPENAI_API_KEY")
        self.client = AsyncOpenAI(api_key=self.api_key) if self.api_key else None
        
        # State tracking
        self.latest_telemetry: Optional[TelemetryTick] = None
        
        # Subscribe to data
        bus.subscribe("telemetry_tick", self._update_telemetry)
        bus.subscribe("driver_query", self._handle_query)

    async def _update_telemetry(self, tick: TelemetryTick):
        """Keep the latest telemetry in memory to provide context to the LLM."""
        self.latest_telemetry = tick

    async def _handle_query(self, query: DriverQuery):
        """When the driver asks a question, consult the LLM using the latest telemetry."""
        logger.info(f"LLM Advisor received query: '{query.query}'")
        
        if not self.latest_telemetry:
            # We don't have data yet
            await self._send_insight("I don't have any telemetry data yet. Stand by.", "info")
            return

        # Prepare the context from the latest telemetry
        t = self.latest_telemetry
        context = (
            f"Speed: {t.speed}km/h, Gear: {t.gear}, RPM: {t.engine_rpm}, "
            f"Lap: {t.lap}, Sector: {t.sector}, "
            f"Tire Wear - FL:{t.tire_wear_fl:.1f}%, FR:{t.tire_wear_fr:.1f}%, "
            f"RL:{t.tire_wear_rl:.1f}%, RR:{t.tire_wear_rr:.1f}%"
        )

        if not self.client:
            logger.warning("OPENAI_API_KEY not set. Using fallback dynamic response.")
            # Fallback if no API key is provided, still using real data instead of static string
            fallback_msg = f"I'm offline, but I see your front left tire is at {t.tire_wear_fl} percent."
            await self._send_insight(fallback_msg, "info")
            return

        # Construct the prompt
        system_prompt = (
            "You are an F1 Race Engineer speaking directly over the radio to your driver. "
            "Keep your answers extremely concise (under 20 words) and conversational. "
            "Use the provided live telemetry to answer the driver's question accurately."
        )

        try:
            response = await self.client.chat.completions.create(
                model="gpt-3.5-turbo",
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "system", "content": f"Live Telemetry Context: {context}"},
                    {"role": "user", "content": query.query}
                ],
                max_tokens=50,
                temperature=0.3 # Keep it factual and less creative
            )
            
            answer = response.choices[0].message.content
            if answer:
                await self._send_insight(answer.strip(), "info", priority=4)
                
        except Exception as e:
            logger.error(f"Failed to generate LLM response: {e}")
            await self._send_insight("I'm having trouble with the data connection.", "warning", priority=5)

    async def _send_insight(self, message: str, insight_type: str, priority: int = 3):
        """Helper to publish the generated insight."""
        insight = DrivingInsight(
            message=message,
            type=insight_type,
            priority=priority
        )
        await bus.publish("driving_insight", insight)
