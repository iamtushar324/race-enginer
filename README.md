# Race Engineer 🏎️🎙️

Race Engineer is an AI-powered, real-time voice assistant and telemetry analyzer for Formula 1 racing simulations and offline analysis. It ingests live telemetry data, uses DuckDB for ultra-fast time-series storage, and leverages the Google Gemini API to act as your personal race engineer—offering voice-activated insights, dynamic strategy advice, and performance analysis.

## Features

- **Live Telemetry Parsing**: Captures and decodes UDP telemetry data in real-time.
- **Voice Assistant**: Integrated Speech-to-Text (STT) and Text-to-Speech (TTS) for hands-free interactions while driving.
- **AI Strategy & Advisor**: Uses Google Gemini to analyze race context and telemetry, providing actionable advice on tires, fuel, pace, and strategy.
- **Fast Data Storage**: Powered by DuckDB for rapid querying of race sessions and telemetry frames.
- **Web UI**: Built-in FastAPI and WebSockets server for real-time visualization and dashboarding.

## Prerequisites

- **Python 3.10+**
- **Microphone & Speakers** (for Voice Assistant)
- **Google Gemini API Key** (for the AI Advisor logic)

## Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/your-username/race-engineer.git
   cd race-engineer
   ```

2. **Create a virtual environment:**
   ```bash
   python -m venv venv
   source venv/bin/activate  # On Windows: venv\Scripts\activate
   ```

3. **Install dependencies:**
   Some audio libraries like `PyAudio` might require system-level audio development headers (e.g., `portaudio` on macOS/Linux).
   ```bash
   pip install -r requirements.txt
   ```

4. **Environment Setup:**
   Copy the example environment file and add your Gemini API Key.
   ```bash
   cp .env.example .env
   ```
   Edit `.env` and insert your `GEMINI_API_KEY`.

## Usage

To run the application, including the main server and the background AI agent, you can use the provided `start` script:

```bash
chmod +x start
./start
```

This will:
- Launch the **OpenCode Strategy Analyst Agent** (`opencode_agent.py`) in the background.
- Start the main **Race Engineer App** (`main.py`) with auto-reloading enabled via `watchdog`.

Alternatively, to start components manually:

```bash
python main.py
```

## Architecture

For an in-depth look at the internal architecture, check out our [Architecture Guide](docs/ARCHITECTURE.md).

## Contributing

Contributions are welcome! Please open an issue or submit a Pull Request if you'd like to improve the telemetry parsers, voice models, or add support for new racing sims.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
