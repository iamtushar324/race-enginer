import asyncio
import logging
import uvicorn
from race_engineer.core.event_bus import bus
from race_engineer.telemetry.parser import BaseTelemetryParser
from race_engineer.feedback.analyzer import PerformanceAnalyzer
from race_engineer.voice.assistant import VoiceAssistant
from race_engineer.telemetry.models import DriverQuery, DrivingInsight
from race_engineer.ui.app import app

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger(__name__)

async def start_ui_server():
    """Starts the FastAPI Web UI on port 8000 using uvicorn."""
    config = uvicorn.Config(app, host="0.0.0.0", port=8000, log_level="info")
    server = uvicorn.Server(config)
    logger.info("Starting UI server on http://localhost:8000")
    await server.serve()

async def main():
    logger.info("Starting Race Engineer...")

    # Initialize components
    telemetry_parser = BaseTelemetryParser()
    feedback_analyzer = PerformanceAnalyzer()
    voice_assistant = VoiceAssistant()

    # Wire up a simple response for driver queries to demonstrate bidirectional flow
    async def handle_driver_query(query: DriverQuery):
        # Very simple routing: Feedback or Voice would normally handle LLM logic
        if "tire" in query.query.lower():
            insight = DrivingInsight(
                message="Tire wear is around 15 percent, they are holding up well.",
                type="info",
                priority=3
            )
            await bus.publish("driving_insight", insight)
    
    bus.subscribe("driver_query", handle_driver_query)

    # Start independent concurrent loops (Telemetry listener, Voice listener, UI server)
    try:
        await asyncio.gather(
            telemetry_parser.start(),
            voice_assistant.listen_for_driver(),
            start_ui_server()
        )
    except KeyboardInterrupt:
        logger.info("Shutting down Race Engineer...")
        telemetry_parser.stop()

if __name__ == "__main__":
    asyncio.run(main())
