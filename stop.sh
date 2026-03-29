#!/bin/bash
# ---------------------------------------------------------------------------
# Race Engineer — Graceful shutdown (3 processes)
# ---------------------------------------------------------------------------

echo "Stopping Race Engineer components..."

# Kill Go telemetry-core
pkill -f "telemetry-core" 2>/dev/null && echo "  Go core stopped" || echo "  Go core not running"

# Kill Python voice service
pkill -f "voice_service.py" 2>/dev/null && echo "  Voice service stopped" || echo "  Voice service not running"

# Kill Vite dev server
pkill -f "vite" 2>/dev/null && echo "  Dashboard stopped" || echo "  Dashboard not running"

# Kill OpenCode analyst agent
pkill -f "opencode serve" 2>/dev/null && echo "  Analyst agent stopped" || echo "  Analyst agent not running"

echo "All components stopped cleanly!"
