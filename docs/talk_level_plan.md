# Race Engineer Talk Level & Smart Queue Implementation Plan

## 1. UI Updates (`race_engineer/ui/app.py`)
*   **Slider Control:** Add a visual slider (range 1-10, default 5) in the "Control Panel" section of the dashboard labeled "Race Engineer Talk Level".
*   **API Integration:** Add JavaScript to listen for slider changes and send an HTTP POST request to a new backend endpoint.
*   **New API Endpoint:** Create `POST /api/talk_level` that accepts the slider value (1-10).
*   **Event Publishing:** The `/api/talk_level` endpoint will publish a `talk_level_changed` event to the global `event_bus`.

## 2. Voice Assistant Updates (`race_engineer/voice/assistant.py`)
*   **State Management:**
    *   Subscribe `VoiceAssistant` to the `talk_level_changed` event.
    *   Store the current `talk_level` (default 5).
*   **Message Queue (`asyncio.Queue`) & Priority Bypass:**
    *   Check message priority upon receipt. 
    *   If a message is `CRITICAL` (e.g., Safety Car, Engine Overheating), **bypass the queue** and speak it immediately.
    *   For `INFO` or `INSIGHT` events, push them into an internal `asyncio.Queue` or list.
*   **Background Processor Loop:**
    *   Create a continuous background task (`asyncio.create_task`) that loops every few seconds (e.g., 5-8 seconds).
    *   **Rate Limiting & Concurrency Safety:** The loop should only proceed if the queue is NOT empty AND the TTS engine is NOT currently speaking (`self.is_speaking` flag check). Otherwise, it should `await asyncio.sleep(5)` and retry.
    *   If conditions are met, extract all queued messages for processing.
*   **Smart Summarization (LLM with Structured JSON):**
    *   Initialize a `google.genai` client within the `VoiceAssistant` (using `GEMINI_API_KEY`).
    *   Send the batched messages to Gemini with a system prompt that includes the current `talk_level`. Instruct Gemini to return its response in strict JSON format.
    *   **Prompt Strategy:**
        > *"You are an F1 Race Engineer speaking to your driver. Summarize these queued messages into a single, natural radio transmission. The driver's Talk Level is {talk_level}/10 (1=only critical safety/strategy, 10=very detailed and chatty). Filter the messages based on this preference, prioritize critical items. You must respond IN JSON FORMAT ONLY with two keys: `escalate` (boolean, true if there is valuable information that needs to be spoken to the driver, false if silence is preferred at this talk level) and `tts_text` (string, the exact text the TTS engine should say. Provide empty string if escalate is false)."*
    *   Parse the JSON response.
*   **TTS Output:** If `escalate` is `true`, pass the `tts_text` to the `pyttsx3` text-to-speech engine.

## 3. Data Model Updates (`race_engineer/telemetry/models.py`)
*   Create a new Pydantic model `TalkLevelPayload(BaseModel)` to handle the `/api/talk_level` endpoint request body.
    *   *Note: Use the `fastapi` skill knowledge if needed when designing the endpoint and model.*

## 4. Dependencies
*   Ensure `google.genai` is imported and configured in `assistant.py` to handle the smart summarization.
*   Ensure `json` library is ready to parse the LLM's structured output.
