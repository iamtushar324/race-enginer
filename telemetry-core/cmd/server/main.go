package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/api"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/config"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/ingestion"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/insights"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/intelligence"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/storage"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/workspace"
)

func main() {
	// Pretty console logging for development.
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"})
	log.Info().Msg("=== Telemetry Core Service ===")

	// ── 1. Configuration ─────────────────────────────────────────────────
	cfg := config.Load()

	// Apply log level from config (default: info).
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Info().Str("log_level", level.String()).Msg("Log level set")

	// ── 2. Root context + WaitGroup ──────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// ── 3. DuckDB Storage ────────────────────────────────────────────────
	store, err := storage.NewStorage(cfg.DBPath, cfg.BatchSize)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialise DuckDB storage")
	}
	defer func() {
		log.Info().Msg("Closing DuckDB storage…")
		if err := store.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing storage")
		}
	}()

	// ── 4. Channel Topology ──────────────────────────────────────────────
	//
	//   insightEngine (5s) ──→ rawInsightChan ──→ CommsGate (15s batch / immediate urgent) ──→ translatedChan ──→ FANOUT
	//   analyst (15s, dual-mode) ──→ rawInsightChan ──↗                                        ├→ dashboardChan
	//   POST /api/strategy ──→ analyst.HandleStrategyWebhook → rawInsightChan ──↗               ├→ voiceChan → synthesize → WS audio
	//   POST /api/driver_query → gate.HandleQuery() → translatedChan                            └→ workspace
	//   POST /api/voice → /transcribe → gate.HandleQuery() → translatedChan
	//
	insightLog := insights.NewLog(500)
	rawInsightChan := make(chan models.DrivingInsight, 100)
	translatedChan := make(chan models.DrivingInsight, 100)
	dashboardChan := make(chan models.DrivingInsight, 100)
	voiceChan := make(chan models.DrivingInsight, 100)
	wsChan := make(chan models.DrivingInsight, 100)

	// ── 5. LLM Provider ─────────────────────────────────────────────────
	// Resolve API key: provider-specific key takes priority, fallback to GEMINI_API_KEY.
	llmKey := cfg.GeminiAPIKey
	switch cfg.LLMProvider {
	case "anthropic", "claude":
		if cfg.AnthropicAPIKey != "" {
			llmKey = cfg.AnthropicAPIKey
		}
	case "openai":
		if cfg.OpenAIAPIKey != "" {
			llmKey = cfg.OpenAIAPIKey
		}
	}

	provider, err := intelligence.NewProvider(cfg.LLMProvider, llmKey, cfg.LLMModel)
	if err != nil {
		log.Fatal().Err(err).Str("provider", cfg.LLMProvider).Msg("Failed to create LLM provider")
	}
	defer provider.Close()

	// ── 6. Workspace Writer ──────────────────────────────────────────────
	ws := workspace.NewWriter(
		cfg.WorkspaceDir,
		func() *models.RaceState { return store.Cache().Load() },
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ws.Run(ctx)
	}()

	// ── 7. Insight Engine (reads from atomic RaceState cache) ────────────
	// Engine now sends to rawInsightChan (instead of directly to dashboard).
	engine := insights.NewEngine(
		func() *models.RaceState { return store.Cache().Load() },
		func() int32 { return cfg.TalkLevel.Load() },
		rawInsightChan,
	)
	engine.SetOnInsight(func(i models.DrivingInsight) {
		ws.RecordInsight(i)
		insightLog.Record("engine", i)
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		engine.Run(ctx)
	}()

	// ── 8. CommsGate (rawInsightChan → translatedChan) ──────────────────
	gate := intelligence.NewCommsGate(
		provider,
		func() *models.RaceState { return store.Cache().Load() },
		func() int32 { return cfg.Verbosity.Load() },
		rawInsightChan,
		translatedChan,
		cfg.WorkspaceDir,
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		gate.Run(ctx)
	}()

	// ── 9. Fanout (translatedChan → dashboard + voice + WS + workspace) ──
	wg.Add(1)
	go func() {
		defer wg.Done()
		fanout(ctx, translatedChan, dashboardChan, voiceChan, wsChan, ws, insightLog)
	}()

	// ── 10. Ingestion Pipeline (3 self-healing goroutines) ───────────────
	ingester := ingestion.NewIngester(cfg, store)
	ingester.Start(ctx, &wg) // adds 3 to wg internally

	// ── 11. WebSocket Hub ───────────────────────────────────────────────
	// Hub must be created before VoiceClient since VoiceClient broadcasts audio through it.
	startedAt := time.Now()
	wsHub := api.NewHub(
		cfg.WSPushRate,
		func() *models.RaceState { return store.Cache().Load() },
		func() interface{} {
			dbOK := true
			if _, err := store.Reader().Exec("SELECT 1"); err != nil {
				dbOK = false
			}
			return map[string]interface{}{
				"status":     "ok",
				"uptime":     time.Since(startedAt).Truncate(time.Second).String(),
				"packets_rx": ingester.PacketsReceived(),
				"duckdb_ok":  dbOK,
				"mock_mode":  cfg.MockMode.Load(),
				"talk_level": cfg.TalkLevel.Load(),
				"udp_host":   cfg.RuntimeUDPHost(),
				"udp_port":   cfg.RuntimeUDPPort(),
				"udp_mode":   cfg.RuntimeUDPMode(),
			}
		},
	)

	// Start Hub broadcast loops.
	wg.Add(1)
	go func() {
		defer wg.Done()
		wsHub.Run(ctx)
	}()

	// Drain wsChan → broadcast to WebSocket clients.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case insight, ok := <-wsChan:
				if !ok {
					return
				}
				wsHub.BroadcastInsight(insight)
			}
		}
	}()

	// Drain PTTChan → broadcast push-to-talk state to WebSocket clients.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case active, ok := <-ingester.PTTChan:
				if !ok {
					return
				}
				wsHub.BroadcastPTT(active)
			}
		}
	}()

	// ── 12. Voice Client (voiceChan → synthesize audio → broadcast via WS) ─
	voiceClient := intelligence.NewVoiceClient(cfg.VoiceURL, voiceChan, wsHub)
	wg.Add(1)
	go func() {
		defer wg.Done()
		voiceClient.Run(ctx)
	}()

	// ── 13. Strategy Analyst ────────────────────────────────────────────
	// Two modes: "internal" runs the built-in Gemini loop, "opencode" pokes
	// an external OpenCode agent server that queries DuckDB and publishes
	// insights back via POST /api/strategy.
	analyst := intelligence.NewAnalyst(
		provider,
		store.Reader(),
		cfg.WorkspaceDir,
		rawInsightChan,
		insightLog,
		func() *models.RaceState { return store.Cache().Load() },
	)

	var agentClient *intelligence.AgentClient
	switch cfg.AnalystMode {
	case "opencode":
		agentClient = intelligence.NewAgentClient(cfg.OpenCodeURL, cfg.AgentInterval)
		wg.Add(1)
		go func() {
			defer wg.Done()
			agentClient.Run(ctx)
		}()
		log.Info().
			Str("url", cfg.OpenCodeURL).
			Int("interval", cfg.AgentInterval).
			Msg("Strategy analyst: OpenCode agent mode")
	default:
		wg.Add(1)
		go func() {
			defer wg.Done()
			analyst.Run(ctx)
		}()
		log.Info().Msg("Strategy analyst: internal Gemini mode")
	}

	// ── 14. Fiber REST API ───────────────────────────────────────────────
	deps := &api.Deps{
		Store:          store,
		Cfg:            cfg,
		InsightChan:    dashboardChan,
		StartedAt:      startedAt,
		PacketsRx:      ingester.PacketsReceived,
		Workspace:      ws,
		Gate:           gate,
		Analyst:        analyst,
		AgentClient:    agentClient,
		Hub:            wsHub,
		VoiceClient:    voiceClient,
		TranslatedChan: translatedChan,
		InsightLog:     insightLog,
	}
	server := api.NewServer(cfg.APIPort, deps)
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Info().Str("port", cfg.APIPort).Msg("API server starting")
		if err := server.Start(); err != nil {
			log.Error().Err(err).Msg("API server error")
		}
	}()

	log.Info().
		Str("api", ":"+cfg.APIPort).
		Int("udp_port", cfg.UDPPort).
		Int("ws_push_rate", cfg.WSPushRate).
		Bool("mock", cfg.MockMode.Load()).
		Bool("llm", provider.Available()).
		Str("llm_provider", cfg.LLMProvider).
		Msg("All systems go — awaiting telemetry")

	// ── 16. Graceful Shutdown ────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutdown signal received — tearing down…")

	// Cancel context to stop all goroutines.
	cancel()

	// Shutdown HTTP server so no new requests are accepted.
	if err := server.Shutdown(); err != nil {
		log.Error().Err(err).Msg("HTTP shutdown error")
	}

	// Wait for all goroutines to finish (they flush buffers on exit).
	wg.Wait()

	log.Info().Msg("Graceful shutdown complete.")
}

// fanout distributes translated insights to the dashboard channel, voice
// channel, WebSocket channel, and workspace writer. Non-blocking sends — if a
// channel is full, that consumer's insight is dropped rather than blocking others.
func fanout(ctx context.Context, in <-chan models.DrivingInsight, dashboard, voice, wsCh chan<- models.DrivingInsight, ws *workspace.Writer, ilog *insights.Log) {
	log.Info().Msg("Insight fanout started")
	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Insight fanout stopping")
			return
		case insight, ok := <-in:
			if !ok {
				return
			}

			// Record to workspace and insight log (all translated insights).
			ws.RecordInsight(insight)
			ilog.Record("comms_gate", insight)

			// Dashboard (non-blocking).
			select {
			case dashboard <- insight:
			default:
				log.Warn().Msg("Dashboard insight channel full, dropping")
			}

			// Voice (non-blocking).
			select {
			case voice <- insight:
			default:
				log.Warn().Msg("Voice insight channel full, dropping")
			}

			// WebSocket (non-blocking).
			select {
			case wsCh <- insight:
			default:
				log.Warn().Msg("WebSocket insight channel full, dropping")
			}
		}
	}
}
