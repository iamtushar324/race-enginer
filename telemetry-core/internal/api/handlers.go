package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"strconv"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/config"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/insights"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/intelligence"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/storage"
	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/workspace"
)

// Deps bundles the dependencies handlers need. Avoids a massive constructor.
type Deps struct {
	Store          *storage.Storage
	Cfg            *config.Config
	InsightChan    <-chan models.DrivingInsight // dashboard insight channel
	StartedAt      time.Time
	PacketsRx      func() uint64 // closure to read ingester packet count
	Workspace      *workspace.Writer
	Gate           *intelligence.CommsGate             // unified insight evaluation & communication
	Analyst        *intelligence.Analyst              // strategy analyst for webhook
	AgentClient    *intelligence.AgentClient          // OpenCode agent client (nil if internal mode)
	Hub            *Hub                               // WebSocket hub for live push
	VoiceClient    *intelligence.VoiceClient          // voice synthesis client (for acks)
	TranslatedChan chan<- models.DrivingInsight        // fanout channel (used by CommsGate internally)
	InsightLog     *insights.Log                      // global insight history log
}

// healthHandler returns structured health info: uptime, packet rate, DuckDB ok.
func healthHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uptime := time.Since(deps.StartedAt).Truncate(time.Second)

		// Quick DuckDB liveness check.
		dbOK := true
		if _, err := deps.Store.Reader().Exec("SELECT 1"); err != nil {
			dbOK = false
		}

		return c.JSON(fiber.Map{
			"status":      "ok",
			"uptime":      uptime.String(),
			"packets_rx":  deps.PacketsRx(),
			"duckdb_ok":   dbOK,
			"mock_mode":   deps.Cfg.MockMode.Load(),
			"talk_level":  deps.Cfg.TalkLevel.Load(),
			"udp_host":    deps.Cfg.RuntimeUDPHost(),
			"udp_port":    deps.Cfg.RuntimeUDPPort(),
			"udp_mode":    deps.Cfg.RuntimeUDPMode(),
		})
	}
}

// latestTelemetryHandler returns the current RaceState from the atomic cache.
func latestTelemetryHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		state := deps.Store.Cache().Load()
		if state == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "No telemetry data received yet",
			})
		}
		return c.JSON(state)
	}
}

// nextInsightHandler consumes and returns the next pending insight.
func nextInsightHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		select {
		case insight := <-deps.InsightChan:
			return c.JSON(insight)
		default:
			return c.Status(fiber.StatusNoContent).Send(nil)
		}
	}
}

// queryHandler executes a read-only SQL statement against DuckDB.
func queryHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload models.SQLQueryPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid JSON payload",
			})
		}
		if payload.SQL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "SQL query is required",
			})
		}

		rows, err := deps.Store.Query(c.Context(), payload.SQL)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(fiber.Map{
			"status": "success",
			"rows":   rows,
		})
	}
}

// talkLevelHandler updates the talk level from the dashboard.
func talkLevelHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload models.TalkLevelPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid payload",
			})
		}
		deps.Cfg.SetTalkLevel(payload.TalkLevel)
		return c.JSON(fiber.Map{
			"status":     "success",
			"talk_level": deps.Cfg.TalkLevel.Load(),
		})
	}
}

// verbosityHandler updates how detailed the engineer's responses are.
func verbosityHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload models.VerbosityPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid payload",
			})
		}
		deps.Cfg.SetVerbosity(payload.Verbosity)
		return c.JSON(fiber.Map{
			"status":    "success",
			"verbosity": deps.Cfg.Verbosity.Load(),
		})
	}
}

// telemetryModeHandler switches between mock and real UDP mode.
func telemetryModeHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var payload models.TelemetryModePayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid payload",
			})
		}

		switch payload.Mode {
		case "mock":
			deps.Cfg.SetMockMode(true)
		case "real":
			deps.Cfg.SetMockMode(false)
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Mode must be 'mock' or 'real'",
			})
		}

		if payload.Host != "" || payload.Port > 0 || payload.UDPMode != "" {
			deps.Cfg.SetRuntimeUDP(payload.Host, payload.Port, payload.UDPMode)
		}

		return c.JSON(fiber.Map{
			"status":    "success",
			"mode":      payload.Mode,
			"mock_mode": deps.Cfg.MockMode.Load(),
			"udp_host":  deps.Cfg.RuntimeUDPHost(),
			"udp_port":  deps.Cfg.RuntimeUDPHost(),
			"udp_mode":  deps.Cfg.RuntimeUDPMode(),
		})
		}
		}

		// getMockOverridesHandler returns the current simulation overrides.
		func getMockOverridesHandler(deps *Deps) fiber.Handler {
		return func(c *fiber.Ctx) error {
		return c.JSON(deps.Cfg.GetMockOverrides())
		}
		}

		// setMockOverridesHandler updates the simulation overrides.
		func setMockOverridesHandler(deps *Deps) fiber.Handler {
		return func(c *fiber.Ctx) error {
		var payload models.MockOverrides
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid JSON payload",
			})
		}
		deps.Cfg.SetMockOverrides(payload)
		return c.JSON(fiber.Map{
			"status": "success",
		})
		}
		}


// settingsHandler returns all current dynamic settings.
func settingsHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"mock_mode":  deps.Cfg.MockMode.Load(),
			"talk_level": deps.Cfg.TalkLevel.Load(),
			"verbosity":  deps.Cfg.Verbosity.Load(),
			"udp_host":   deps.Cfg.RuntimeUDPHost(),
			"udp_port":   deps.Cfg.RuntimeUDPPort(),
			"udp_mode":   deps.Cfg.RuntimeUDPMode(),
			"api_port":   deps.Cfg.APIPort,
			"db_path":    deps.Cfg.DBPath,
			"python_api": deps.Cfg.PythonAPI,
			"mock_overrides": deps.Cfg.GetMockOverrides(),
		})
	}
}

// workspaceHandler returns the current insights.md content as plain text.
// LLM agents call this to get compact race context for their prompts.
func workspaceHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.Workspace == nil {
			return c.Status(fiber.StatusServiceUnavailable).SendString("Workspace not initialized")
		}
		c.Set("Content-Type", "text/markdown; charset=utf-8")
		return c.SendString(deps.Workspace.Content())
	}
}

// driverQueryHandler handles POST /api/driver_query.
// Accepts {"query":"How are my tires?"} and returns a Gemini-powered response.
func driverQueryHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.Gate == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "CommsGate not initialized",
			})
		}

		var payload models.DriverQuery
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid JSON payload",
			})
		}
		if payload.Query == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Query is required",
			})
		}

		// HandleQuery sends through the CommsGate pipeline which emits to
		// translatedChan internally (reaches fanout → WS, voice, workspace).
		insight := deps.Gate.HandleQuery(c.Context(), payload.Query)

		return c.JSON(insight)
	}
}

// strategyWebhookHandler handles POST /api/strategy.
// External agents push strategy insights here for translation and voice delivery.
func strategyWebhookHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.Analyst == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error": "Analyst not initialized",
			})
		}

		var payload models.StrategyPayload
		if err := c.BodyParser(&payload); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid JSON payload",
			})
		}

		deps.Analyst.HandleStrategyWebhook(payload)
		return c.JSON(fiber.Map{
			"status": "accepted",
		})
	}
}

// insightHistoryHandler returns the insight log as JSON.
// Optional query param: ?limit=N (default 50, max 500).
func insightHistoryHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.InsightLog == nil {
			return c.JSON([]insights.LogEntry{})
		}
		limit := 50
		if l := c.Query("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				if n > 500 {
					n = 500
				}
				limit = n
			}
		}
		return c.JSON(deps.InsightLog.Recent(limit))
	}
}

// agentStatusHandler returns the OpenCode agent client status and logs.
func agentStatusHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if deps.AgentClient == nil {
			return c.JSON(intelligence.AgentStatus{
				Enabled: false,
				Logs:    []intelligence.AgentLogEntry{},
			})
		}
		return c.JSON(deps.AgentClient.Status())
	}
}

// voiceHandler handles POST /api/voice.
// Accepts a multipart audio file, proxies it to the Python voice service
// /transcribe endpoint for speech-to-text, then feeds the transcription to
// the Gemini advisor and returns the combined result.
func voiceHandler(deps *Deps) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. Get the audio file from multipart form.
		file, err := c.FormFile("audio")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Audio file required (field: audio)",
			})
		}

		// 2. Open the uploaded file.
		src, err := file.Open()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to read audio file",
			})
		}
		defer src.Close()

		// 3. Read audio bytes.
		audioBytes, err := io.ReadAll(src)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to read audio data",
			})
		}

		// 4. Fire acknowledgement audio immediately (non-blocking).
		// Driver hears "Copy, checking the data" while transcription + LLM run in parallel.
		if deps.VoiceClient != nil {
			go deps.VoiceClient.SynthesizeAck(context.Background())
		}

		// 5. Proxy to Python POST /transcribe as multipart.
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("audio", file.Filename)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create multipart request",
			})
		}
		if _, err := part.Write(audioBytes); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to write audio data",
			})
		}
		writer.Close()

		url := fmt.Sprintf("%s/transcribe", deps.Cfg.VoiceURL)
		req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, url, body)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to create transcribe request",
			})
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		client := &http.Client{Timeout: 120 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"error": "Voice service unavailable",
			})
		}
		defer resp.Body.Close()

		var transcription struct {
			Text       string  `json:"text"`
			Confidence float64 `json:"confidence"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&transcription); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to parse transcription",
			})
		}

		if transcription.Text == "" {
			return c.JSON(fiber.Map{
				"transcription": "",
				"confidence":    0,
				"insight":       nil,
			})
		}

		// 6. Feed transcription to CommsGate (handles fanout internally).
		var insight *models.DrivingInsight
		if deps.Gate != nil {
			result := deps.Gate.HandleQuery(c.Context(), transcription.Text)
			insight = &result
		}

		return c.JSON(fiber.Map{
			"transcription": transcription.Text,
			"confidence":    transcription.Confidence,
			"insight":       insight,
		})
	}
}
