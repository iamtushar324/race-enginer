import asyncio
import logging
import uvicorn
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

from race_engineer.core.event_bus import bus
from race_engineer.data.store import DataStore
from race_engineer.telemetry.parser import BaseTelemetryParser
from race_engineer.telemetry.session_state import SessionState
from race_engineer.feedback.analyzer import PerformanceAnalyzer
from race_engineer.voice.assistant import VoiceAssistant
from race_engineer.intelligence.advisor import LLMAdvisor
from race_engineer.ui.app import app, set_datastore, set_parser_manager

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s"
)
logger = logging.getLogger(__name__)


class ParserManager:
    """Manages switching between mock and real telemetry parsers at runtime."""

    def __init__(self):
        self.mode = "mock"
        self._mock_parser = BaseTelemetryParser()
        self._real_parser = None
        self._active_task = None
        self.host = "0.0.0.0"
        self.port = 20777

    async def start(self):
        """Start the parser in the current mode."""
        self._active_task = asyncio.create_task(self._mock_parser.start())
        await bus.publish("telemetry_status", {"mode": "mock", "status": "running"})
        try:
            await self._active_task
        except asyncio.CancelledError:
            pass

    async def switch_mode(self, mode: str, host: str = None, port: int = None):
        """Switch between 'mock' and 'real' modes."""
        if mode == self.mode:
            return

        # Stop current parser
        if self.mode == "mock":
            self._mock_parser.stop()
        elif self._real_parser:
            self._real_parser.stop()

        if self._active_task and not self._active_task.done():
            self._active_task.cancel()
            try:
                await self._active_task
            except asyncio.CancelledError:
                pass

        self.mode = mode
        if host is not None:
            self.host = host
        if port is not None:
            self.port = port

        if mode == "mock":
            self._mock_parser = BaseTelemetryParser()
            self._active_task = asyncio.create_task(self._mock_parser.start())
            await bus.publish("telemetry_status", {"mode": "mock", "status": "running"})
        else:
            from race_engineer.telemetry.udp_parser import RealTelemetryParser

            self._real_parser = RealTelemetryParser(host=self.host, port=self.port)
            self._active_task = asyncio.create_task(self._real_parser.start())

        logger.info(f"Switched telemetry mode to: {mode}")

    def stop(self):
        if self.mode == "mock":
            self._mock_parser.stop()
        elif self._real_parser:
            self._real_parser.stop()
        if self._active_task and not self._active_task.done():
            self._active_task.cancel()

    def get_status(self) -> dict:
        if self.mode == "mock":
            return {"mode": "mock", "status": "running"}
        if self._real_parser:
            status = "connected" if self._real_parser.is_connected else "disconnected"
            return {
                "mode": "real",
                "status": status,
                "host": self.host,
                "port": self.port,
            }
        return {
            "mode": "real",
            "status": "not_started",
            "host": self.host,
            "port": self.port,
        }


async def start_ui_server():
    """Starts the FastAPI Web UI on port 8000 using uvicorn."""
    config = uvicorn.Config(app, host="0.0.0.0", port=8000, log_level="info")
    server = uvicorn.Server(config)
    logger.info("Starting UI server on http://localhost:8000")
    await server.serve()


async def main():
    logger.info("Starting Race Engineer (Full Telemetry Mode)...")

    # Initialize components
    datastore = DataStore("live_session.duckdb")
    set_datastore(datastore)  # Make DB accessible via /api/query endpoint
    session_state = SessionState()
    parser_manager = ParserManager()
    set_parser_manager(parser_manager)
    feedback_analyzer = PerformanceAnalyzer()
    voice_assistant = VoiceAssistant()

    # Initialize the LLM intelligence layer (Race Engineer)
    llm_advisor = LLMAdvisor()

    # Start independent concurrent loops
    try:
        await asyncio.gather(parser_manager.start(), start_ui_server())
    except (KeyboardInterrupt, asyncio.CancelledError):
        pass
    finally:
        logger.info("Shutting down Race Engineer...")
        parser_manager.stop()
        datastore.close()


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        pass
