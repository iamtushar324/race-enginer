import os
import json
import logging
import asyncio
from typing import Dict, Any, List

from race_engineer.core.event_bus import bus
from race_engineer.telemetry.models import DrivingInsight

logger = logging.getLogger(__name__)

# Try to import pyttsx3, fallback to mock if not available (e.g. headless CI)
try:
    import pyttsx3  # type: ignore

    HAS_TTS = True
except ImportError:
    HAS_TTS = False
    logger.warning("pyttsx3 not installed. Voice output will be mocked.")


class VoiceAssistant:
    """
    Handles two-way communication with the driver using Text-to-Speech (TTS)
    and Speech-to-Text (STT).

    Key design: _handle_incoming_insight NEVER blocks. It always queues messages
    and returns immediately, so it doesn't stall the event bus or parser loop.
    A background loop drains the queue and speaks sequentially.
    """

    def __init__(self):
        self.talk_level = 5
        self._is_speaking = False
        self._speak_lock = asyncio.Lock()

        # Priority queue: (negative_priority, sequence, insight)
        # Lower number = higher priority. Sequence breaks ties in FIFO order.
        self._priority_queue: asyncio.PriorityQueue = asyncio.PriorityQueue()
        self._seq = 0

        self.genai_client = None

        api_key = os.getenv("GEMINI_API_KEY")
        if api_key:
            try:
                from google import genai

                self.genai_client = genai.Client(api_key=api_key)
            except ImportError:
                logger.warning("google-genai not available or import failed.")
        else:
            logger.warning(
                "GEMINI_API_KEY not found in environment. Smart summarizing disabled."
            )

        # Listen for generated insights to broadcast to the driver
        bus.subscribe("driving_insight", self._handle_incoming_insight)
        bus.subscribe("talk_level_changed", self._update_talk_level)

        self.tts_engine: Any = None

        global HAS_TTS
        if HAS_TTS:
            import pyttsx3  # type: ignore

            try:
                self.tts_engine = pyttsx3.init()  # type: ignore
                self.tts_engine.setProperty("rate", 170)  # type: ignore
            except Exception as e:
                logger.error(f"Failed to initialize pyttsx3: {e}")
                HAS_TTS = False

        try:
            loop = asyncio.get_running_loop()
            loop.create_task(self._speaker_loop())
            loop.create_task(self._batch_summarize_loop())
        except RuntimeError:
            pass

    async def _update_talk_level(self, data: Dict[str, Any]):
        self.talk_level = data.get("talk_level", 5)
        logger.info(f"Race Engineer talk level updated to {self.talk_level}")

    async def _handle_incoming_insight(self, insight: DrivingInsight):
        """
        NEVER blocks. Always queues and returns immediately.
        High-priority messages get higher queue priority so they're spoken first.
        """
        self._seq += 1
        # Negative priority so higher priority number = dequeued first
        queue_priority = (
            -insight.priority
            if (insight.priority >= 4 or insight.type == "warning")
            else 0
        )
        await self._priority_queue.put((queue_priority, self._seq, insight))

    async def _speaker_loop(self):
        """
        Background loop that drains the priority queue and speaks messages
        one at a time, sequentially. Never blocks the event bus.
        """
        while True:
            # Wait for a message
            priority, seq, insight = await self._priority_queue.get()

            # If currently speaking, drop low-priority messages that pile up
            # but always speak high-priority ones (they wait for current speech)
            is_high_priority = insight.priority >= 4 or insight.type == "warning"

            if self._is_speaking and not is_high_priority:
                # Drop low-priority messages while speaking to avoid backlog
                logger.info(
                    f"VOICE ENGINE: Dropping low-pri message while speaking: {insight.message[:40]}..."
                )
                continue

            logger.info(
                f"VOICE ENGINE [{insight.type.upper()}] (Pri {insight.priority}): TTS '{insight.message[:60]}...'"
            )

            async with self._speak_lock:
                self._is_speaking = True
                try:
                    await self._speak(insight.message)
                finally:
                    self._is_speaking = False

    async def _speak(self, message: str):
        """Text-to-Speech. Runs in thread pool to avoid blocking the event loop."""
        try:
            if self.tts_engine is not None and HAS_TTS:
                loop = asyncio.get_running_loop()
                await loop.run_in_executor(None, self._speak_sync, message)
            else:
                # Mock speech delay - proportional to message length but capped
                delay = min(len(message) * 0.03, 3.0)
                await asyncio.sleep(delay)
        except Exception as e:
            logger.error(f"TTS speak error: {e}")

    def _speak_sync(self, message: str):
        """Synchronous method to run the pyttsx3 engine."""
        try:
            if self.tts_engine is not None:
                self.tts_engine.say(message)  # type: ignore
                self.tts_engine.runAndWait()  # type: ignore
        except Exception as e:
            logger.error(f"TTS Error: {e}")

    async def _batch_summarize_loop(self):
        """
        Periodically checks if there are multiple low-priority messages queued.
        If so, summarizes them into one message using Gemini.
        This runs less frequently and only handles batching/summarization.
        """
        while True:
            await asyncio.sleep(8)

            # Only summarize if not currently speaking and queue has multiple items
            if self._is_speaking or self._priority_queue.qsize() < 2:
                continue

            if not self.genai_client:
                continue

            # Drain low-priority items for summarization
            batch: List[DrivingInsight] = []
            remaining = []
            while not self._priority_queue.empty():
                try:
                    item = self._priority_queue.get_nowait()
                    priority, seq, insight = item
                    if insight.priority >= 4 or insight.type == "warning":
                        remaining.append(item)  # Keep high-pri in queue
                    else:
                        batch.append(insight)
                except asyncio.QueueEmpty:
                    break

            # Put high-priority items back
            for item in remaining:
                await self._priority_queue.put(item)

            if len(batch) < 2:
                # Not enough to summarize, put them back
                for insight in batch:
                    self._seq += 1
                    await self._priority_queue.put((0, self._seq, insight))
                continue

            messages_text = "\n".join(
                [
                    f"- [Type: {i.type} | Priority: {i.priority}] {i.message}"
                    for i in batch
                ]
            )
            prompt = f"""You are an F1 Race Engineer speaking to your driver over team radio.
Summarize these queued messages into a single, natural radio transmission.

The driver's Talk Level is {self.talk_level}/10 (1 = only critical safety/strategy, 10 = very detailed and chatty).
Filter the messages based on this preference, and prioritize critical items.

You must respond IN JSON FORMAT ONLY with exactly two keys:
1. "escalate": boolean (true if there is valuable info that needs to be spoken based on the talk level, false if silence is preferred).
2. "tts_text": string (the exact natural text the TTS engine should say. Provide an empty string if escalate is false). Do not use markdown.

Queued Messages:
{messages_text}
"""
            try:
                logger.info(
                    f"VOICE ENGINE: Requesting smart summary for {len(batch)} queued messages..."
                )

                def _call_gemini():
                    return self.genai_client.models.generate_content(
                        model="gemini-2.5-flash", contents=prompt
                    )

                loop = asyncio.get_running_loop()
                response = await loop.run_in_executor(None, _call_gemini)

                text_response = response.text.strip()
                if text_response.startswith("```json"):
                    text_response = text_response.split("```json")[1]
                    if text_response.endswith("```"):
                        text_response = text_response[:-3]
                elif text_response.startswith("```"):
                    text_response = text_response.split("```")[1]
                    if text_response.endswith("```"):
                        text_response = text_response[:-3]

                parsed = json.loads(text_response.strip())
                escalate = parsed.get("escalate", False)
                tts_text = parsed.get("tts_text", "")

                if escalate and tts_text:
                    logger.info(f"VOICE ENGINE SMART SUMMARY: {tts_text}")
                    summary_insight = DrivingInsight(
                        message=tts_text, type="info", priority=3
                    )
                    self._seq += 1
                    await self._priority_queue.put((0, self._seq, summary_insight))
                else:
                    logger.info(
                        "VOICE ENGINE SMART SUMMARY: Silence preferred based on talk level."
                    )

            except Exception as e:
                logger.error(f"Error during smart summarization: {e}")
                # On error, put one representative message back
                if batch:
                    self._seq += 1
                    await self._priority_queue.put((0, self._seq, batch[0]))
