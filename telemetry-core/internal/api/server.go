package api

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Server wraps the Fiber HTTP server that exposes the telemetry REST API.
type Server struct {
	app  *fiber.App
	deps *Deps
	port string
}

// NewServer creates a configured Fiber server with all routes registered.
func NewServer(port string, deps *Deps) *Server {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ReadBufferSize:        8192,
		BodyLimit:             10 * 1024 * 1024, // 10MB for audio uploads
	})

	s := &Server{
		app:  app,
		deps: deps,
		port: port,
	}
	s.setupMiddleware()
	s.setupRoutes()
	return s
}

func (s *Server) setupMiddleware() {
	s.app.Use(recover.New())
	s.app.Use(logger.New(logger.Config{
		Format:     "${time} ${status} ${method} ${path} ${latency}\n",
		TimeFormat: "15:04:05",
	}))
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
	}))
}

func (s *Server) setupRoutes() {
	// Health & status
	s.app.Get("/health", healthHandler(s.deps))
	s.app.Get("/api/settings", settingsHandler(s.deps))

	// Live telemetry from atomic cache
	s.app.Get("/api/telemetry/latest", latestTelemetryHandler(s.deps))

	// Insight consumption
	s.app.Get("/api/insights/next", nextInsightHandler(s.deps))
	s.app.Get("/api/insights/history", insightHistoryHandler(s.deps))

	// SQL query (read-only, via DuckDB reader connection)
	s.app.Post("/api/query", queryHandler(s.deps))

	// Dashboard-controllable settings
	s.app.Post("/api/settings/talk_level", talkLevelHandler(s.deps))
	s.app.Post("/api/settings/verbosity", verbosityHandler(s.deps))
	s.app.Post("/api/settings/mode", telemetryModeHandler(s.deps))
	s.app.Get("/api/settings/mock/overrides", getMockOverridesHandler(s.deps))
	s.app.Post("/api/settings/mock/overrides", setMockOverridesHandler(s.deps))

	// Workspace — compact insights context for LLM agents
	s.app.Get("/api/workspace", workspaceHandler(s.deps))

	// OpenCode agent status + logs
	s.app.Get("/api/agent/status", agentStatusHandler(s.deps))

	// Intelligence — driver queries, strategy webhooks, and voice proxy
	s.app.Post("/api/driver_query", driverQueryHandler(s.deps))
	s.app.Post("/api/strategy", strategyWebhookHandler(s.deps))
	s.app.Post("/api/voice", voiceHandler(s.deps))

	// WebSocket — live telemetry push
	if s.deps.Hub != nil {
		s.app.Use("/ws", func(c *fiber.Ctx) error {
			if websocket.IsWebSocketUpgrade(c) {
				return c.Next()
			}
			return fiber.ErrUpgradeRequired
		})
		s.app.Get("/ws", websocket.New(wsHandler(s.deps.Hub)))
	}
}

// Start begins listening on the configured port. Blocks until shutdown.
func (s *Server) Start() error {
	return s.app.Listen(":" + s.port)
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
