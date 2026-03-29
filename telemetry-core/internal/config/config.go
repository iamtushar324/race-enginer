package config

import (
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// Config holds all service configuration. Static fields are set once at startup
// from env vars. Dynamic fields use atomic types so the dashboard API can
// change them at runtime without locks.
type Config struct {
	// --- Static (set once at startup) ---
	UDPHost      string // e.g. "0.0.0.0"
	UDPPort      int    // e.g. 20777
	APIPort      string // e.g. "8081"
	DBPath       string // e.g. "workspace/telemetry.duckdb"
	PythonAPI    string // e.g. "http://localhost:8000"
	BatchSize    int    // flush threshold for DuckDB buffers
	SampleRate   int    // 20Hz → 1Hz = sample every 20th packet
	GeminiAPIKey    string // Google Gemini API key (empty = LLM disabled)
	LLMProvider     string // "gemini" (default), "anthropic", "openai"
	LLMModel        string // Override model name (empty = provider default)
	AnthropicAPIKey string // Anthropic API key for Claude
	OpenAIAPIKey    string // OpenAI API key
	VoiceURL        string // Python voice service URL, e.g. "http://localhost:8000"
	WorkspaceDir string // path to workspace/ directory with markdown context files
	LogLevel      string // zerolog level: trace, debug, info, warn, error, fatal, panic
	WSPushRate    int    // WebSocket push rate in Hz (e.g. 10 = 100ms interval)
	AnalystMode   string // "internal" = built-in Gemini loop, "opencode" = external OpenCode agent
	OpenCodeURL   string // OpenCode headless server URL, e.g. "http://localhost:4095"
	AgentInterval int    // seconds between OpenCode analysis cycles
	PTTButton     uint32 // bitmask for push-to-talk button in F1 BUTN events

	// --- Dynamic (changeable via dashboard at runtime) ---
	MockMode      atomic.Bool          // true = generate fake telemetry instead of listening to UDP
	TalkLevel     atomic.Int32         // 1 (quiet) to 10 (chatty) — controls insight verbosity
	Verbosity     atomic.Int32         // 1 (terse) to 10 (detailed) — controls engineer response length
	MockOverrides models.MockOverrides // manual physics/weather overrides
	RestartGen    atomic.Uint64        // bumped on config change to signal listener restart
	mu            sync.RWMutex         // protects non-atomic dynamic fields below
	udpPort       int                  // runtime-changeable UDP port (requires listener restart)
	udpHost    string        // runtime-changeable UDP host
	udpMode    string        // "broadcast" or "unicast"
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	c := &Config{
		UDPHost:      envStr("TELEMETRY_HOST", "0.0.0.0"),
		UDPPort:      envInt("TELEMETRY_PORT", 20777),
		APIPort:      envStr("API_PORT", "8081"),
		DBPath:       envStr("DB_PATH", "workspace/telemetry.duckdb"),
		PythonAPI:    envStr("PYTHON_API_URL", "http://localhost:8000"),
		BatchSize:    envInt("BATCH_SIZE", 20),
		SampleRate:   envInt("SAMPLE_RATE", 20),
		GeminiAPIKey:    envStr("GEMINI_API_KEY", ""),
		LLMProvider:     envStr("LLM_PROVIDER", "gemini"),
		LLMModel:        envStr("LLM_MODEL", ""),
		AnthropicAPIKey: envStr("ANTHROPIC_API_KEY", ""),
		OpenAIAPIKey:    envStr("OPENAI_API_KEY", ""),
		VoiceURL:        envStr("VOICE_URL", "http://localhost:8000"),
		WorkspaceDir: envStr("WORKSPACE_DIR", "workspace"),
		LogLevel:     envStr("LOG_LEVEL", "info"),
		WSPushRate:    envInt("WS_PUSH_RATE", 10),
		AnalystMode:   envStr("ANALYST_MODE", "internal"),
		OpenCodeURL:   envStr("OPENCODE_URL", "http://localhost:4095"),
		AgentInterval: envInt("AGENT_INTERVAL", 15),
		PTTButton:     uint32(envHex("PTT_BUTTON", 0x00000002)), // default: Triangle / Y
	}

	c.MockMode.Store(envStr("TELEMETRY_MODE", "real") == "mock")
	c.TalkLevel.Store(int32(envInt("TALK_LEVEL", 5)))
	c.Verbosity.Store(int32(envInt("VERBOSITY", 5)))
	c.MockOverrides = models.MockOverrides{
		TireWearMultiplier: 1.0,
		FuelBurnMultiplier: 1.0,
		TireTempOffset:     0.0,
	}
	c.udpPort = c.UDPPort
	c.udpHost = c.UDPHost
	c.udpMode = envStr("UDP_MODE", "broadcast")

	log.Info().
		Str("udp_host", c.UDPHost).
		Int("udp_port", c.UDPPort).
		Str("udp_mode", c.udpMode).
		Str("api_port", c.APIPort).
		Str("db_path", c.DBPath).
		Str("python_api", c.PythonAPI).
		Bool("mock_mode", c.MockMode.Load()).
		Int32("talk_level", c.TalkLevel.Load()).
		Str("llm_provider", c.LLMProvider).
		Str("llm_model", c.LLMModel).
		Str("analyst_mode", c.AnalystMode).
		Msg("Configuration loaded")

	return c
}

// SetMockMode toggles mock mode at runtime (from dashboard API).
func (c *Config) SetMockMode(mock bool) {
	c.MockMode.Store(mock)
	c.RestartGen.Add(1)
	log.Info().Bool("mock_mode", mock).Msg("Mock mode updated")
}

// SetTalkLevel adjusts verbosity at runtime (from dashboard API).
func (c *Config) SetTalkLevel(level int) {
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	c.TalkLevel.Store(int32(level))
	log.Info().Int("talk_level", level).Msg("Talk level updated")
}

// SetVerbosity adjusts the engineer's response detail level at runtime.
func (c *Config) SetVerbosity(level int) {
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	c.Verbosity.Store(int32(level))
	log.Info().Int("verbosity", level).Msg("Verbosity updated")
}

// SetMockOverrides updates the manual overrides for physics/weather simulation.
func (c *Config) SetMockOverrides(overrides models.MockOverrides) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MockOverrides = overrides
}

// GetMockOverrides returns a snapshot of the current mock simulation overrides.
func (c *Config) GetMockOverrides() models.MockOverrides {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MockOverrides
}

// RuntimeUDPHost returns the current runtime UDP host.
func (c *Config) RuntimeUDPHost() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.udpHost
}

// RuntimeUDPPort returns the current runtime UDP port (may differ from startup).
func (c *Config) RuntimeUDPPort() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.udpPort
}

// RuntimeUDPMode returns the current UDP stream mode ("broadcast" or "unicast").
func (c *Config) RuntimeUDPMode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.udpMode
}

// SetRuntimeUDP changes the UDP host, port, and/or mode at runtime and signals
// the ingestion layer to restart the listener.
func (c *Config) SetRuntimeUDP(host string, port int, udpMode string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if host != "" {
		c.udpHost = host
	}
	if port > 0 {
		c.udpPort = port
	}
	if udpMode == "broadcast" || udpMode == "unicast" {
		c.udpMode = udpMode
	}
	c.RestartGen.Add(1)
	log.Info().Str("udp_host", c.udpHost).Int("udp_port", c.udpPort).Str("udp_mode", c.udpMode).Msg("Runtime UDP config updated")
}

// SetRuntimeUDPPort changes the UDP port at runtime (backwards compat).
func (c *Config) SetRuntimeUDPPort(port int) {
	c.SetRuntimeUDP("", port, "")
}

// --- helpers ---

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envHex(key string, fallback uint64) uint64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 0, 32); err == nil {
			return n
		}
	}
	return fallback
}
