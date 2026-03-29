#!/bin/bash
# ---------------------------------------------------------------------------
# Race Engineer — Concurrent launcher (3 processes)
# Starts Go core, React dashboard, and Python voice service
# ---------------------------------------------------------------------------

set -e
cd "$(dirname "$0")"

# ensure dependencies exist
if ! command -v concurrently &> /dev/null
then
    echo "concurrently is not installed. Installing it globally now..."
    npm install -g concurrently
fi

# Load .env variables
if [ -f .env ]; then
    set -a
    source .env
    set +a
fi

# ── Python venv setup ────────────────────────────────────────────────────
if [ ! -d venv ]; then
    echo "Creating Python virtual environment..."
    python3 -m venv venv
fi

# Only install Python deps on first run or when explicitly requested (REINSTALL_DEPS=1)
DEPS_MARKER="venv/.deps_installed"
if [ ! -f "$DEPS_MARKER" ] || [ "${REINSTALL_DEPS:-0}" = "1" ]; then
    echo "Installing Python voice service dependencies..."
    source venv/bin/activate

    # Core deps (always needed)
    pip install -q edge-tts faster-whisper httpx fastapi python-multipart uvicorn pydantic

    # Optional deps based on configured engines
    if [ "$TTS_ENGINE" = "qwen-tts" ]; then
        echo "  Installing Qwen TTS deps (mlx-audio, soundfile, numpy)..."
        pip install -q mlx-audio soundfile numpy
    fi

    if [ "$STT_ENGINE" = "parakeet" ]; then
        echo "  Installing Parakeet STT deps (nemo_toolkit[asr], soundfile, librosa)..."
        pip install -q "nemo_toolkit[asr]" soundfile librosa
    fi

    deactivate
    touch "$DEPS_MARKER"
else
    echo "Python deps already installed (run with REINSTALL_DEPS=1 to force reinstall)"
fi

# ── Go builds ────────────────────────────────────────────────────────────
echo "Building telemetry-core Go service..."
(cd telemetry-core && go build -o ../workspace/bin/telemetry-core cmd/server/main.go)
echo "Building racedb query tool..."
(cd telemetry-core && go build -o ../workspace/bin/racedb cmd/query/main.go)
echo "Go builds complete!"

echo "Installing dashboard dependencies..."
(cd dashboard && npm install)

ANALYST_MODE="${ANALYST_MODE:-internal}"

if [ "$ANALYST_MODE" = "opencode" ]; then
    echo "Starting all processes (with OpenCode analyst)..."
    concurrently \
        --names "GO-CORE,REACT,WHISPER,ANALYST" \
        --prefix-colors "blue,green,yellow,magenta" \
        --kill-others \
        "DB_PATH=\"${DB_PATH:-workspace/telemetry.duckdb}\" ANALYST_MODE=opencode ./workspace/bin/telemetry-core" \
        "cd dashboard && npm run dev" \
        "source venv/bin/activate && python voice_service.py" \
        "cd workspace && opencode serve --port ${OPENCODE_PORT:-4095}"
else
    echo "Starting all processes (internal analyst)..."
    concurrently \
        --names "GO-CORE,REACT,WHISPER" \
        --prefix-colors "blue,green,yellow" \
        --kill-others \
        "DB_PATH=\"${DB_PATH:-workspace/telemetry.duckdb}\" ./workspace/bin/telemetry-core" \
        "cd dashboard && npm run dev" \
        "source venv/bin/activate && python voice_service.py"
fi
