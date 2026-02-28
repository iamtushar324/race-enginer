import asyncio
import logging
from race_engineer.core.event_bus import bus
from race_engineer.telemetry.parser import BaseTelemetryParser
from race_engineer.feedback.analyzer import PerformanceAnalyzer
from race_engineer.voice.assistant import VoiceAssistant

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)

async def main():
    logger.info("Starting Race Engineer...")

    # Initialize components
    telemetry_parser = BaseTelemetryParser()
    feedback_analyzer = PerformanceAnalyzer()
    voice_assistant = VoiceAssistant()

    # Wire up a simple response for driver queries to demonstrate bidirectional flow
    async def handle_driver_query(data: dict):
        query = data.get("query", "")
        # Very simple routing: Feedback or Voice would normally handle LLM logic
        if "tire" in query.lower():
            await bus.publish(
                "driving_insight",
                {"message": "Tire wear is at 15 percent, they are holding up well.", "type": "info"}
            )
    
    bus.subscribe("driver_query", handle_driver_query)

    # Start independent concurrent loops (e.g. Telemetry listener & Voice listener)
    try:
        await asyncio.gather(
            telemetry_parser.start(),
            voice_assistant.listen_for_driver()
        )
    except KeyboardInterrupt:
        logger.info("Shutting down Race Engineer...")
        telemetry_parser.stop()

if __name__ == "__main__":
    asyncio.run(main())
