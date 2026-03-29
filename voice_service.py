"""Voice service: TTS synthesis + STT transcription with pluggable engines.

TTS engines: edge-tts (default), qwen-tts (MLX voice cloning)
STT engines: whisper (default), parakeet (NVIDIA NeMo), resemble (cloud)
"""

import io
import logging
import os
import tempfile
import time

import random

import edge_tts
import httpx
from fastapi import FastAPI, File, HTTPException, UploadFile
from fastapi.responses import Response
from pydantic import BaseModel

logging.basicConfig(
    level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s"
)
logger = logging.getLogger("voice_service")

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
TTS_ENGINE = os.getenv("TTS_ENGINE", "edge-tts")
TTS_VOICE = os.getenv("TTS_VOICE", "en-GB-RyanNeural")
TTS_VOICE_REF = os.getenv("TTS_VOICE_REF", "workspace/voices/reference.wav")
TTS_SPEED = float(os.getenv("TTS_SPEED", "1.0"))
WHISPER_MODEL = os.getenv("WHISPER_MODEL", "medium")

# STT engine: "whisper" (local, default), "parakeet" (NeMo), or "resemble" (cloud)
STT_ENGINE = os.getenv("STT_ENGINE", "whisper")
RESEMBLE_API_KEY = os.getenv("RESEMBLE_API_KEY", "")
RESEMBLE_BASE_URL = os.getenv("RESEMBLE_BASE_URL", "https://app.resemble.ai/api/v2")

SAMPLE_RATE_ASR = 16000
SAMPLE_RATE_TTS = 24000

# ---------------------------------------------------------------------------
# Lazy-loaded models
# ---------------------------------------------------------------------------
_whisper_model = None
_parakeet_model = None
_qwen_tts_model = None


def _get_whisper_model():
    """Load the faster-whisper model on first use."""
    global _whisper_model
    if _whisper_model is None:
        logger.info(
            "Loading faster-whisper model '%s' (compute_type=int8) ...", WHISPER_MODEL
        )
        from faster_whisper import WhisperModel

        _whisper_model = WhisperModel(
            WHISPER_MODEL, device="cpu", compute_type="int8"
        )
        logger.info("Whisper model loaded.")
    return _whisper_model


def _get_parakeet_model():
    """Load NVIDIA Parakeet TDT 0.6b v3 via NeMo on first use."""
    global _parakeet_model
    if _parakeet_model is None:
        logger.info("Loading ASR: nvidia/parakeet-tdt-0.6b-v3 ...")
        t0 = time.time()

        import warnings
        warnings.filterwarnings("ignore")
        os.environ.setdefault("NEMO_LOG_LEVEL", "ERROR")
        os.environ.setdefault("TRANSFORMERS_VERBOSITY", "error")
        logging.getLogger("nemo_logger").setLevel(logging.ERROR)
        logging.getLogger("pytorch_lightning").setLevel(logging.ERROR)

        import nemo.collections.asr as nemo_asr

        _parakeet_model = nemo_asr.models.ASRModel.from_pretrained(
            model_name="nvidia/parakeet-tdt-0.6b-v3"
        )
        _parakeet_model.eval()
        logger.info("Parakeet ASR loaded in %.1fs", time.time() - t0)
    return _parakeet_model


def _get_qwen_tts_model():
    """Load Qwen3-TTS 0.6B via MLX-Audio on first use."""
    global _qwen_tts_model
    if _qwen_tts_model is None:
        logger.info("Loading TTS: Qwen3-TTS-0.6B via MLX ...")
        t0 = time.time()
        from mlx_audio.tts.utils import load_model

        _qwen_tts_model = load_model("mlx-community/Qwen3-TTS-12Hz-0.6B-Base-bf16")
        logger.info("Qwen3-TTS loaded in %.1fs", time.time() - t0)
    return _qwen_tts_model


# ---------------------------------------------------------------------------
# Resemble STT client
# ---------------------------------------------------------------------------
_resemble_client = None


def _get_resemble_client() -> httpx.Client:
    """Lazy-init a persistent httpx client for Resemble API calls."""
    global _resemble_client
    if _resemble_client is None:
        _resemble_client = httpx.Client(timeout=60.0)
    return _resemble_client


def _transcribe_resemble(audio_bytes: bytes, filename: str) -> dict:
    """Transcribe audio using Resemble.ai Speech-to-Text API."""
    if not RESEMBLE_API_KEY:
        raise ValueError("RESEMBLE_API_KEY is not set")

    client = _get_resemble_client()
    headers = {"Authorization": f"Bearer {RESEMBLE_API_KEY}"}

    ext = os.path.splitext(filename)[1].lower() if filename else ""
    mime_map = {".mp3": "audio/mpeg", ".wav": "audio/wav", ".m4a": "audio/mp4"}
    mime_type = mime_map.get(ext, "application/octet-stream")
    if ext in (".webm", ".ogg", ".opus"):
        mime_type = "audio/webm"

    upload_filename = f"audio{ext}" if ext else "audio.webm"
    resp = client.post(
        f"{RESEMBLE_BASE_URL}/speech-to-text",
        headers=headers,
        files={"file": (upload_filename, audio_bytes, mime_type)},
    )

    if resp.status_code not in (200, 201):
        logger.error("Resemble STT submit failed: %d %s", resp.status_code, resp.text[:200])
        raise RuntimeError(f"Resemble STT submit failed: {resp.status_code}")

    data = resp.json()
    item = data.get("item", {})
    uuid = item.get("uuid", "")
    status = item.get("status", "")

    logger.info("Resemble STT submitted: uuid=%s status=%s", uuid, status)

    if status == "completed" and item.get("text"):
        return {"text": item["text"], "confidence": 0.95}

    if status in ("pending", "processing"):
        return _poll_resemble(client, headers, uuid)

    raise RuntimeError(f"Resemble STT unexpected status: {status}")


def _poll_resemble(client: httpx.Client, headers: dict, uuid: str) -> dict:
    """Poll Resemble STT endpoint until transcription completes."""
    max_attempts = 30
    for attempt in range(max_attempts):
        time.sleep(1)
        resp = client.get(
            f"{RESEMBLE_BASE_URL}/speech-to-text/{uuid}",
            headers=headers,
        )
        if resp.status_code != 200:
            logger.warning("Resemble poll failed: %d (attempt %d)", resp.status_code, attempt + 1)
            continue

        data = resp.json()
        item = data.get("item", {})
        status = item.get("status", "")

        if status == "completed":
            text = item.get("text", "")
            if text:
                logger.info("Resemble STT complete (attempt %d): %.80s", attempt + 1, text)
                return {"text": text, "confidence": 0.95}
            raise RuntimeError("Resemble STT completed but returned no text")

        if status == "failed":
            raise RuntimeError("Resemble STT transcription failed on server")

        logger.debug("Resemble poll: status=%s (attempt %d)", status, attempt + 1)

    raise RuntimeError(f"Resemble STT timed out after {max_attempts} attempts")


# ---------------------------------------------------------------------------
# STT implementations
# ---------------------------------------------------------------------------
def _transcribe_whisper(audio_bytes: bytes) -> dict:
    """Transcribe using local faster-whisper model."""
    model = _get_whisper_model()
    audio_stream = io.BytesIO(audio_bytes)
    # Use lenient VAD params for short push-to-talk recordings.
    # Lower min_silence_duration keeps more audio; lower threshold catches quieter speech.
    segments, info = model.transcribe(
        audio_stream,
        vad_filter=True,
        vad_parameters=dict(
            min_silence_duration_ms=500,
            speech_pad_ms=300,
            threshold=0.3,
        ),
    )

    text_parts = []
    for segment in segments:
        text_parts.append(segment.text.strip())

    full_text = " ".join(text_parts)
    confidence = round(info.language_probability, 2) if info.language_probability else 0.0

    logger.info(
        "Transcribed via Whisper (%s, conf=%.2f): %.80s",
        info.language,
        confidence,
        full_text,
    )
    return {"text": full_text, "confidence": confidence}


def _convert_to_wav(audio_bytes: bytes, filename: str = "") -> str:
    """Convert any audio format to 16kHz mono WAV using pydub/ffmpeg.

    Returns the path to a temporary WAV file. Caller must delete it.
    """
    import numpy as np
    import soundfile as sf
    from pydub import AudioSegment

    # Detect format from content or filename
    ext = os.path.splitext(filename)[1].lower() if filename else ""
    fmt = None
    if ext in (".webm", ".opus", ".ogg"):
        fmt = "webm"
    elif ext in (".mp3",):
        fmt = "mp3"
    elif ext in (".mp4", ".m4a"):
        fmt = "mp4"

    # Write raw bytes to temp file for pydub
    with tempfile.NamedTemporaryFile(suffix=ext or ".webm", delete=False) as src:
        src.write(audio_bytes)
        src_path = src.name

    wav_path = src_path + ".wav"
    try:
        # pydub handles format detection and uses ffmpeg under the hood
        audio_seg = AudioSegment.from_file(src_path, format=fmt)
        audio_seg = audio_seg.set_frame_rate(SAMPLE_RATE_ASR).set_channels(1)
        audio_seg.export(wav_path, format="wav")
    finally:
        os.unlink(src_path)

    return wav_path


def _transcribe_parakeet(audio_bytes: bytes, filename: str = "") -> dict:
    """Transcribe using NVIDIA Parakeet TDT via NeMo."""
    model = _get_parakeet_model()

    # Convert any audio format (webm, opus, mp3, etc.) to 16kHz mono WAV
    tmp_path = _convert_to_wav(audio_bytes, filename)

    try:
        t0 = time.time()
        output = model.transcribe([tmp_path], verbose=False)
        elapsed = time.time() - t0

        text = output[0].text if hasattr(output[0], "text") else str(output[0])
        logger.info("Transcribed via Parakeet (%.2fs): %.80s", elapsed, text)
        return {"text": text, "confidence": 0.95}
    finally:
        os.unlink(tmp_path)


# ---------------------------------------------------------------------------
# TTS implementations
# ---------------------------------------------------------------------------
def _synthesize_qwen(text: str) -> tuple[bytes, str]:
    """Synthesize speech using Qwen3-TTS via MLX-Audio with voice cloning.

    Returns (audio_bytes, media_type).
    """
    import numpy as np
    import soundfile as sf
    import mlx.core as mx

    model = _get_qwen_tts_model()

    ref_audio = TTS_VOICE_REF if os.path.exists(TTS_VOICE_REF) else None
    if ref_audio:
        logger.debug("Using voice reference: %s", ref_audio)

    t0 = time.time()
    generate_kwargs = {"text": text, "speed": TTS_SPEED}
    if ref_audio:
        generate_kwargs["ref_audio"] = ref_audio

    results = list(model.generate(**generate_kwargs))
    tts_elapsed = time.time() - t0

    audio = results[0].audio
    if isinstance(audio, mx.array):
        audio_np = np.array(audio)
    else:
        audio_np = np.array(audio)

    if audio_np.ndim > 1:
        audio_np = audio_np.squeeze()

    sr = getattr(results[0], "sample_rate", SAMPLE_RATE_TTS)
    audio_duration = len(audio_np) / sr

    logger.info(
        "Qwen TTS: %.2fs generation, %.1fs audio, RTF: %.2fx",
        tts_elapsed,
        audio_duration,
        tts_elapsed / audio_duration if audio_duration > 0 else 0,
    )

    # Encode as WAV bytes
    buf = io.BytesIO()
    sf.write(buf, audio_np, sr, format="WAV")
    return buf.getvalue(), "audio/wav"


async def _synthesize_edge_tts(text: str) -> tuple[bytes, str]:
    """Synthesize speech using edge-tts. Returns (audio_bytes, media_type)."""
    communicate = edge_tts.Communicate(text, TTS_VOICE)
    audio_buffer = io.BytesIO()
    async for chunk in communicate.stream():
        if chunk["type"] == "audio":
            audio_buffer.write(chunk["data"])

    audio_bytes = audio_buffer.getvalue()
    if not audio_bytes:
        raise RuntimeError("edge-tts produced no audio")

    return audio_bytes, "audio/mpeg"


# ---------------------------------------------------------------------------
# FastAPI app
# ---------------------------------------------------------------------------
app = FastAPI(title="Voice Service")


# -- Synthesize -------------------------------------------------------------
class SynthesizeRequest(BaseModel):
    text: str
    priority: int = 3


@app.post("/synthesize")
async def synthesize(req: SynthesizeRequest):
    """Synthesize speech using the configured TTS engine."""
    if not req.text.strip():
        raise HTTPException(status_code=400, detail="text must not be empty")

    try:
        if TTS_ENGINE == "qwen-tts":
            if _qwen_tts_model is None:
                raise HTTPException(status_code=503, detail="Qwen TTS model still loading, try again shortly")
            audio_bytes, media_type = _synthesize_qwen(req.text)
        else:
            audio_bytes, media_type = await _synthesize_edge_tts(req.text)

        logger.info(
            "Synthesized %d bytes via %s (pri=%d): %.60s",
            len(audio_bytes),
            TTS_ENGINE,
            req.priority,
            req.text,
        )
        return Response(content=audio_bytes, media_type=media_type)

    except HTTPException:
        raise
    except Exception as exc:
        logger.error("TTS synthesis failed (%s): %s", TTS_ENGINE, exc)
        raise HTTPException(status_code=500, detail=f"TTS synthesis failed: {exc}")


# -- Transcribe --------------------------------------------------------------
@app.post("/transcribe")
async def transcribe(audio: UploadFile = File(...)):
    """Transcribe audio using the configured STT engine."""
    try:
        audio_bytes = await audio.read()
        if not audio_bytes:
            raise HTTPException(status_code=400, detail="Empty audio file")

        if STT_ENGINE == "resemble":
            result = _transcribe_resemble(audio_bytes, audio.filename or "audio.webm")
            logger.info(
                "Transcribed via Resemble (conf=%.2f): %.80s",
                result["confidence"],
                result["text"],
            )
            return result

        if STT_ENGINE == "parakeet":
            if _parakeet_model is None:
                raise HTTPException(status_code=503, detail="Parakeet model still loading, try again shortly")
            return _transcribe_parakeet(audio_bytes, audio.filename or "recording.webm")

        # Default: local Whisper
        if STT_ENGINE == "whisper" and _whisper_model is None:
            raise HTTPException(status_code=503, detail="Whisper model still loading, try again shortly")
        return _transcribe_whisper(audio_bytes)

    except HTTPException:
        raise
    except Exception as exc:
        logger.error("Transcription failed (%s): %s", STT_ENGINE, exc)
        raise HTTPException(status_code=500, detail=f"Transcription failed: {exc}")


# -- Acknowledgement phrases -------------------------------------------------
ACK_PHRASES = [
    "Copy, checking the data.",
    "Roger, looking into it.",
    "Understood, stand by for info.",
    "Copy that, we'll come back to you.",
    "Received, give us a moment.",
    "Got it, checking now.",
    "Heard you, one second.",
    "Copy, pulling it up now.",
    "Roger, working on it.",
    "Stand by, we're on it.",
    "Copy, we'll get back to you.",
    "Understood, let us check.",
    "On it, stand by.",
    "Roger, bear with us.",
    "Copy, running the numbers.",
]

# Pre-generated ack audio cache: {phrase: (audio_bytes, media_type)}
_ack_cache: dict[str, tuple[bytes, str]] = {}


@app.post("/synthesize-ack")
async def synthesize_ack():
    """Return a short random acknowledgement audio clip. Fast, cached."""
    phrase = random.choice(ACK_PHRASES)

    # Check cache first
    if phrase in _ack_cache:
        audio_bytes, media_type = _ack_cache[phrase]
        logger.debug("Ack cache hit: %s", phrase)
        return Response(content=audio_bytes, media_type=media_type)

    try:
        if TTS_ENGINE == "qwen-tts":
            audio_bytes, media_type = _synthesize_qwen(phrase)
        else:
            audio_bytes, media_type = await _synthesize_edge_tts(phrase)

        # Cache it for next time
        _ack_cache[phrase] = (audio_bytes, media_type)

        logger.info("Ack synthesized and cached: %s (%d bytes)", phrase, len(audio_bytes))
        return Response(content=audio_bytes, media_type=media_type)

    except Exception as exc:
        logger.error("Ack synthesis failed: %s", exc)
        raise HTTPException(status_code=500, detail=f"Ack synthesis failed: {exc}")


@app.post("/warmup-acks")
async def warmup_acks():
    """Pre-generate all acknowledgement audio clips into cache."""
    generated = 0
    for phrase in ACK_PHRASES:
        if phrase in _ack_cache:
            continue
        try:
            if TTS_ENGINE == "qwen-tts":
                audio_bytes, media_type = _synthesize_qwen(phrase)
            else:
                audio_bytes, media_type = await _synthesize_edge_tts(phrase)
            _ack_cache[phrase] = (audio_bytes, media_type)
            generated += 1
        except Exception as exc:
            logger.warning("Failed to warm up ack '%s': %s", phrase, exc)

    logger.info("Warmed up %d/%d ack phrases", generated, len(ACK_PHRASES))
    return {"status": "ok", "cached": len(_ack_cache), "total": len(ACK_PHRASES)}


# -- Record voice reference --------------------------------------------------
@app.post("/record-reference")
async def record_reference(audio: UploadFile = File(...)):
    """Upload a voice reference WAV for Qwen TTS voice cloning."""
    try:
        audio_bytes = await audio.read()
        if not audio_bytes:
            raise HTTPException(status_code=400, detail="Empty audio file")

        ref_dir = os.path.dirname(TTS_VOICE_REF)
        if ref_dir:
            os.makedirs(ref_dir, exist_ok=True)

        with open(TTS_VOICE_REF, "wb") as f:
            f.write(audio_bytes)

        logger.info("Voice reference saved: %s (%d bytes)", TTS_VOICE_REF, len(audio_bytes))
        return {"status": "ok", "path": TTS_VOICE_REF, "size": len(audio_bytes)}

    except HTTPException:
        raise
    except Exception as exc:
        logger.error("Failed to save voice reference: %s", exc)
        raise HTTPException(status_code=500, detail=f"Failed to save reference: {exc}")


# -- Health ------------------------------------------------------------------
@app.get("/health")
async def health():
    return {
        "status": "ok",
        "tts_engine": TTS_ENGINE,
        "tts_voice": TTS_VOICE if TTS_ENGINE == "edge-tts" else None,
        "tts_voice_ref": TTS_VOICE_REF if TTS_ENGINE == "qwen-tts" else None,
        "tts_voice_ref_exists": os.path.exists(TTS_VOICE_REF) if TTS_ENGINE == "qwen-tts" else None,
        "stt_engine": STT_ENGINE,
        "resemble_configured": bool(RESEMBLE_API_KEY),
        "whisper_model_name": WHISPER_MODEL if STT_ENGINE == "whisper" else None,
        "whisper_model_loaded": _whisper_model is not None,
        "parakeet_model_loaded": _parakeet_model is not None,
        "qwen_tts_model_loaded": _qwen_tts_model is not None,
    }


# ---------------------------------------------------------------------------
# Startup event — pre-load models so first request isn't slow
# ---------------------------------------------------------------------------
@app.on_event("startup")
async def startup_warmup():
    import threading

    global STT_ENGINE
    logger.info("TTS engine: %s | STT engine: %s", TTS_ENGINE, STT_ENGINE)

    if TTS_ENGINE == "qwen-tts" and STT_ENGINE == "parakeet":
        logger.error(
            "INCOMPATIBLE: qwen-tts (needs transformers 5.x) and parakeet (needs transformers <5) "
            "cannot run together. Use STT_ENGINE=whisper with qwen-tts. Falling back to whisper."
        )
        STT_ENGINE = "whisper"

    if TTS_ENGINE == "qwen-tts":
        logger.info("Voice ref: %s (exists=%s)", TTS_VOICE_REF, os.path.exists(TTS_VOICE_REF))

    # Load both models in parallel background threads so the server starts immediately.
    # Requests to unloaded models return 503 until ready.
    def _load_tts():
        if TTS_ENGINE == "qwen-tts":
            _get_qwen_tts_model()

    def _load_stt():
        if STT_ENGINE == "parakeet":
            _get_parakeet_model()
        elif STT_ENGINE == "whisper":
            _get_whisper_model()

    threading.Thread(target=_load_tts, name="tts-warmup", daemon=True).start()
    threading.Thread(target=_load_stt, name="stt-warmup", daemon=True).start()
    logger.info("Model loading started in background threads")


# ---------------------------------------------------------------------------
# Entrypoint
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    import uvicorn

    port = int(os.getenv("VOICE_PORT", "8000"))
    if STT_ENGINE == "resemble":
        logger.info("Resemble API: %s (key=%s...)", RESEMBLE_BASE_URL, RESEMBLE_API_KEY[:8] if RESEMBLE_API_KEY else "NOT SET")
    uvicorn.run(app, host="0.0.0.0", port=port)
