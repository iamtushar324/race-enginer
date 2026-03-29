package intelligence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// GateResponse is the structured JSON Gemini returns for each evaluation.
type GateResponse struct {
	CommunicateToDriver bool   `json:"communicate_to_driver"`
	VoiceText           string `json:"voice_text"`
	AnalystInputNeeded  bool   `json:"analyst_input_needed"`
	AnalystPrompt       string `json:"analyst_prompt"`
}

// QueueItem wraps an insight with age tracking.
type QueueItem struct {
	Insight models.DrivingInsight
	Age     int // batch cycles survived without being communicated
}

// driverQuery is an internal request from HTTP handlers.
type driverQuery struct {
	text   string
	result chan<- models.DrivingInsight
}

// CommsGate stages insights, evaluates them via an LLM provider with structured JSON
// output, and decides whether/when to communicate to the driver.
type CommsGate struct {
	provider       LLMProvider
	stateLoader    func() *models.RaceState
	verbosityFn    func() int32
	rawChan        <-chan models.DrivingInsight
	translatedChan chan<- models.DrivingInsight
	driverQueryCh  chan driverQuery
	soulMD         string
	userMD         string
	driverMD       string
	trackMD        string
	learningsMD    string
	normalQueue    []QueueItem
}

// NewCommsGate creates a CommsGate wired to an LLM provider and channels.
func NewCommsGate(
	provider LLMProvider,
	stateLoader func() *models.RaceState,
	verbosityFn func() int32,
	rawChan <-chan models.DrivingInsight,
	translatedChan chan<- models.DrivingInsight,
	workspaceDir string,
) *CommsGate {
	// Load personality files from workspace/, falling back to project root.
	soulMD := loadFileWithFallback(workspaceDir, "soul.md", "SOUL.md", "You are an assertive but calm F1 Race Engineer.")
	userMD := loadFileWithFallback(workspaceDir, "user.md", "USER.md", "The driver you are talking to.")
	driverMD := loadFileWithFallback(workspaceDir, "driver_profile.md", "driver_profile.md", "No driver profile available.")
	trackMD := loadFileWithFallback(workspaceDir, "track_setup.md", "track_setup.md", "No track setup available.")
	learningsMD := loadFileWithFallback(workspaceDir, "past_learnings.md", "past_learnings.md", "No past learnings available.")

	return &CommsGate{
		provider:       provider,
		stateLoader:    stateLoader,
		verbosityFn:    verbosityFn,
		rawChan:        rawChan,
		translatedChan: translatedChan,
		driverQueryCh:  make(chan driverQuery, 10),
		soulMD:         soulMD,
		userMD:         userMD,
		driverMD:       driverMD,
		trackMD:        trackMD,
		learningsMD:    learningsMD,
	}
}

// Run starts the CommsGate loop. Blocks until ctx is cancelled.
func (g *CommsGate) Run(ctx context.Context) {
	log.Info().Msg("CommsGate started")

	batchTimer := time.NewTimer(15 * time.Second)
	defer batchTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("CommsGate stopping")
			return

		case insight, ok := <-g.rawChan:
			if !ok {
				return
			}
			if insight.Priority >= 4 {
				g.processUrgent(ctx, insight)
			} else {
				g.normalQueue = append(g.normalQueue, QueueItem{Insight: insight, Age: 0})
				log.Debug().Int("queue_size", len(g.normalQueue)).Msg("CommsGate: queued normal insight")
			}

		case dq := <-g.driverQueryCh:
			g.handleDriverQuery(ctx, dq)

		case <-batchTimer.C:
			g.processNormalBatch(ctx)
			batchTimer.Reset(15 * time.Second)
		}
	}
}

// HandleQuery is the public API for HTTP handlers. Sends a driver query through
// the CommsGate pipeline and blocks until a response is ready.
func (g *CommsGate) HandleQuery(ctx context.Context, text string) models.DrivingInsight {
	resultCh := make(chan models.DrivingInsight, 1)
	dq := driverQuery{text: text, result: resultCh}

	select {
	case g.driverQueryCh <- dq:
	case <-ctx.Done():
		return models.DrivingInsight{
			Message:  "Radio timeout. Stand by.",
			Type:     "warning",
			Priority: 3,
		}
	}

	select {
	case result := <-resultCh:
		return result
	case <-time.After(30 * time.Second):
		return models.DrivingInsight{
			Message:  "I'm having trouble with the data link. Stand by.",
			Type:     "warning",
			Priority: 3,
		}
	}
}

// processUrgent evaluates a high-priority insight immediately.
func (g *CommsGate) processUrgent(ctx context.Context, insight models.DrivingInsight) {
	log.Info().Str("msg", insight.Message).Int("priority", insight.Priority).Msg("CommsGate: urgent insight")

	if !g.provider.Available() {
		g.emit(models.DrivingInsight{
			Message:  "Team: " + insight.Message,
			Type:     "strategy",
			Priority: 4,
		})
		return
	}

	telemetryCtx := g.buildTelemetryContext()
	verbosityInstr := g.verbosityInstruction()

	prompt := fmt.Sprintf(
		"URGENT INSIGHT (Priority %d):\n- %s (type: %s)\n\nThis is a critical, time-sensitive message. Evaluate and respond.",
		insight.Priority, insight.Message, insight.Type,
	)

	system, pcfg := g.gateConfig(verbosityInstr, telemetryCtx, true)

	resp, err := GenerateJSON[GateResponse](g.provider, ctx, prompt, system, pcfg)
	if err != nil {
		log.Warn().Err(err).Msg("CommsGate: urgent LLM call failed, passing through")
		g.emit(models.DrivingInsight{
			Message:  "Team: " + insight.Message,
			Type:     "strategy",
			Priority: 4,
		})
		return
	}

	if resp.CommunicateToDriver && resp.VoiceText != "" {
		log.Info().Str("radio", resp.VoiceText).Msg("CommsGate: urgent → driver")
		g.emit(models.DrivingInsight{
			Message:  strings.TrimSpace(resp.VoiceText),
			Type:     "strategy",
			Priority: 4,
		})
	} else {
		// Rare for urgent, but move to normal queue for re-eval.
		g.normalQueue = append(g.normalQueue, QueueItem{Insight: insight, Age: 0})
		log.Info().Msg("CommsGate: urgent held for re-evaluation")
	}

	if resp.AnalystInputNeeded {
		log.Info().Str("prompt", resp.AnalystPrompt).Msg("CommsGate: analyst input requested")
	}
}

// processNormalBatch evaluates all queued normal insights.
// Strategy analyst insights (type=strategy) bypass the LLM gate after 1 cycle —
// they were already vetted by the analyst. Rule-engine insights go through the gate.
func (g *CommsGate) processNormalBatch(ctx context.Context) {
	if len(g.normalQueue) == 0 {
		return
	}

	// Increment age and evict stale items (>= 3 cycles = 45s).
	var kept []QueueItem
	for _, item := range g.normalQueue {
		item.Age++
		if item.Age >= 3 {
			log.Warn().Str("msg", item.Insight.Message).Int("age", item.Age).Msg("CommsGate: evicting stale insight")
			continue
		}
		kept = append(kept, item)
	}
	g.normalQueue = kept

	if len(g.normalQueue) == 0 {
		return
	}

	// Split: strategy analyst insights get translated directly (they're already vetted).
	// Rule-engine insights go through the LLM gate.
	var strategyItems []QueueItem
	var ruleItems []QueueItem
	for _, item := range g.normalQueue {
		if item.Insight.Type == "strategy" && item.Age >= 1 {
			strategyItems = append(strategyItems, item)
		} else {
			ruleItems = append(ruleItems, item)
		}
	}

	// Translate strategy insights directly — one call per insight.
	for _, item := range strategyItems {
		g.translateAndEmit(ctx, item.Insight)
	}

	// Keep only non-strategy items in the queue.
	g.normalQueue = ruleItems

	if len(ruleItems) == 0 {
		return
	}

	log.Debug().Int("items", len(ruleItems)).Msg("CommsGate: evaluating rule-engine batch")

	if !g.provider.Available() {
		var messages []string
		for _, item := range ruleItems {
			messages = append(messages, item.Insight.Message)
		}
		g.emit(models.DrivingInsight{
			Message:  "Team: " + strings.Join(messages, "; "),
			Type:     "strategy",
			Priority: 3,
		})
		g.normalQueue = nil
		return
	}

	telemetryCtx := g.buildTelemetryContext()
	verbosityInstr := g.verbosityInstruction()

	var insightLines []string
	for _, item := range ruleItems {
		insightLines = append(insightLines, fmt.Sprintf("- %s (type: %s, priority: %d, age: %d cycles)",
			item.Insight.Message, item.Insight.Type, item.Insight.Priority, item.Age))
	}

	prompt := fmt.Sprintf(
		"PENDING INSIGHTS (%d items):\n%s\n\nEvaluate whether these insights, taken together, warrant a radio message to the driver right now.",
		len(ruleItems), strings.Join(insightLines, "\n"),
	)

	system, pcfg := g.gateConfig(verbosityInstr, telemetryCtx, false)

	resp, err := GenerateJSON[GateResponse](g.provider, ctx, prompt, system, pcfg)
	if err != nil {
		log.Warn().Err(err).Msg("CommsGate: normal batch LLM call failed")
		return
	}

	if resp.CommunicateToDriver {
		voiceText := strings.TrimSpace(resp.VoiceText)
		if voiceText == "" {
			// LLM approved but returned no text — fall back to raw messages.
			var msgs []string
			for _, item := range ruleItems {
				msgs = append(msgs, item.Insight.Message)
			}
			voiceText = strings.Join(msgs, ". ")
		}
		log.Info().Str("radio", voiceText).Int("items", len(ruleItems)).Msg("CommsGate: batch → driver")
		g.emit(models.DrivingInsight{
			Message:  voiceText,
			Type:     "strategy",
			Priority: 3,
		})
		g.normalQueue = nil
	} else {
		log.Debug().Int("items", len(ruleItems)).Msg("CommsGate: batch held for next cycle")
	}

	if resp.AnalystInputNeeded {
		log.Info().Str("prompt", resp.AnalystPrompt).Msg("CommsGate: analyst input requested")
	}
}

// handleDriverQuery evaluates a driver question with pending context.
func (g *CommsGate) handleDriverQuery(ctx context.Context, dq driverQuery) {
	log.Info().Str("query", dq.text).Msg("CommsGate: driver query")

	state := g.stateLoader()
	if state == nil {
		result := models.DrivingInsight{
			Message:  "I don't have any telemetry data yet. Stand by.",
			Type:     "info",
			Priority: 3,
		}
		g.emitAndRespond(result, dq)
		return
	}

	if !g.provider.Available() {
		result := models.DrivingInsight{
			Message:  "I don't have an AI connection right now. Stand by.",
			Type:     "info",
			Priority: 3,
		}
		g.emitAndRespond(result, dq)
		return
	}

	telemetryCtx := BuildContext(state)
	verbosityInstr := g.verbosityInstruction()

	// Include pending queue items for context.
	var pendingContext string
	if len(g.normalQueue) > 0 {
		var lines []string
		for _, item := range g.normalQueue {
			lines = append(lines, fmt.Sprintf("- %s (type: %s, priority: %d)", item.Insight.Message, item.Insight.Type, item.Insight.Priority))
		}
		pendingContext = fmt.Sprintf("\n\nPENDING UNSENT INSIGHTS:\n%s", strings.Join(lines, "\n"))
	}

	prompt := fmt.Sprintf(
		"DRIVER IS ASKING A QUESTION — you MUST set communicate_to_driver to true.\n\n"+
			"Driver's question: \"%s\"%s\n\n"+
			"Answer the driver's question using the live telemetry context. "+
			"If there are pending insights relevant to the question, incorporate them into your answer.",
		dq.text, pendingContext,
	)

	system, pcfg := g.gateConfig(verbosityInstr, telemetryCtx, true)

	resp, err := GenerateJSON[GateResponse](g.provider, ctx, prompt, system, pcfg)
	if err != nil {
		log.Warn().Err(err).Msg("CommsGate: driver query LLM call failed")
		result := models.DrivingInsight{
			Message:  "I'm having trouble with the data connection.",
			Type:     "warning",
			Priority: 4,
		}
		g.emitAndRespond(result, dq)
		return
	}

	voiceText := resp.VoiceText
	if voiceText == "" {
		voiceText = "Copy, stand by."
	}

	result := models.DrivingInsight{
		Message:  strings.TrimSpace(voiceText),
		Type:     "info",
		Priority: 4,
	}

	// Clear normal queue — driver query context subsumes pending items.
	g.normalQueue = nil

	g.emitAndRespond(result, dq)

	if resp.AnalystInputNeeded {
		log.Info().Str("prompt", resp.AnalystPrompt).Msg("CommsGate: analyst input requested (driver query)")
	}
}

// emitAndRespond sends the insight to the fanout pipeline and responds to the driver query.
func (g *CommsGate) emitAndRespond(insight models.DrivingInsight, dq driverQuery) {
	g.emit(insight)
	select {
	case dq.result <- insight:
	default:
	}
}

// translateAndEmit translates a strategy analyst insight into radio speech and emits it.
// Used for insights that bypass the LLM gate (already vetted by the analyst).
func (g *CommsGate) translateAndEmit(ctx context.Context, insight models.DrivingInsight) {
	if !g.provider.Available() {
		g.emit(models.DrivingInsight{
			Message:  insight.Message,
			Type:     "strategy",
			Priority: insight.Priority,
		})
		return
	}

	telemetryCtx := g.buildTelemetryContext()
	verbosityInstr := g.verbosityInstruction()

	prompt := fmt.Sprintf(
		"STRATEGY ANALYST INSIGHT (already vetted — translate to radio speech, always communicate):\n%s",
		insight.Message,
	)

	system, pcfg := g.gateConfig(verbosityInstr, telemetryCtx, false)

	resp, err := GenerateJSON[GateResponse](g.provider, ctx, prompt, system, pcfg)
	if err != nil || strings.TrimSpace(resp.VoiceText) == "" {
		// Fall back to passing the raw message through.
		log.Warn().Err(err).Str("msg", insight.Message).Msg("CommsGate: translation failed, passing through raw")
		g.emit(models.DrivingInsight{
			Message:  insight.Message,
			Type:     "strategy",
			Priority: insight.Priority,
		})
		return
	}

	log.Info().Str("radio", resp.VoiceText).Msg("CommsGate: strategy insight → driver")
	g.emit(models.DrivingInsight{
		Message:  strings.TrimSpace(resp.VoiceText),
		Type:     "strategy",
		Priority: insight.Priority,
	})
}

// emit sends a translated insight to the output channel (non-blocking).
func (g *CommsGate) emit(insight models.DrivingInsight) {
	select {
	case g.translatedChan <- insight:
	default:
		log.Warn().Msg("CommsGate: translated insight channel full, dropping")
	}
}

// gateConfig builds a system prompt and ProviderConfig for CommsGate evaluations.
func (g *CommsGate) gateConfig(verbosityInstr, telemetryCtx string, isCritical bool) (string, ProviderConfig) {
	systemPrompt := fmt.Sprintf(
		"%s\n\nDRIVER PREFERENCES:\n%s\n\n"+
			"You are an F1 Race Engineer evaluating incoming data to decide what to communicate "+
			"to your driver over the radio. You must respond with a JSON object with these fields:\n"+
			"- communicate_to_driver (boolean): true if this should be communicated now\n"+
			"- voice_text (string): the radio message in authentic F1 style, empty if not communicating\n"+
			"- analyst_input_needed (boolean): true if deeper data analysis is needed\n"+
			"- analyst_prompt (string): what analysis is needed, 'NONE' if not needed\n\n"+
			"If you decide to communicate, the voice_text must sound like authentic F1 radio "+
			"(e.g., 'Box box', 'Target lap time X', 'Scenario 7'). "+
			"Do NOT add conversational filler like 'Hey' or 'Alright'. Get straight to the point.\n"+
			"Do NOT start with acknowledgement phrases like 'Copy that', 'Roger', 'Understood', "+
			"'Got it', 'Received', 'Affirm', 'Solid copy' — the radio system already sends an "+
			"automatic acknowledgement before your response. Jump straight into the information.\n"+
			"VERBOSITY INSTRUCTION: %s\n"+
			"CRITICAL MESSAGE: %v\n\n"+
			"ADDITIONAL CONTEXT:\n"+
			"Driver Profile: %s\n"+
			"Track Setup: %s\n"+
			"Past Learnings: %s\n\n"+
			"Live Telemetry Context: %s",
		g.soulMD, g.userMD, verbosityInstr, isCritical, g.driverMD, g.trackMD, g.learningsMD, telemetryCtx,
	)

	return systemPrompt, ProviderConfig{
		MaxTokens:   5000,
		Temperature: 0.3,
	}
}

// buildTelemetryContext loads the current state and builds a context string.
func (g *CommsGate) buildTelemetryContext() string {
	state := g.stateLoader()
	if state == nil {
		return "No telemetry data available yet."
	}
	return BuildContext(state)
}

// verbosityInstruction returns a prompt instruction based on the current verbosity level.
func (g *CommsGate) verbosityInstruction() string {
	verbosity := g.verbosityFn()
	switch {
	case verbosity <= 2:
		return "Be EXTREMELY brief — 5 words max. Pure commands only: 'Box box', 'Push now', 'Copy'."
	case verbosity <= 4:
		return "Be very concise — one short sentence max. No explanations, just the action."
	case verbosity <= 6:
		return "Be concise but include key context — one or two sentences."
	case verbosity <= 8:
		return "Give a clear message with supporting details — two to three sentences."
	default:
		return "Give a detailed explanation with data and reasoning — up to four sentences."
	}
}
