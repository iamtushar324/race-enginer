package insights

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// Engine evaluates insight rules against the live RaceState on a fixed ticker.
// It reads from an atomic *models.RaceState pointer (never from DuckDB) for
// sub-microsecond evaluations.
type Engine struct {
	stateLoader func() *models.RaceState    // returns the current atomic RaceState snapshot
	rules       []Rule                       // the 12 ported PerformanceAnalyzer rules
	cooldown    *CooldownManager             // per-rule cooldown and debounce state
	insightChan chan<- models.DrivingInsight  // non-blocking send target
	interval    time.Duration                // evaluation period (5 s)
	talkLevel   func() int32                 // returns current verbosity from config
	onInsight   func(models.DrivingInsight)  // optional callback for every generated insight
}

// NewEngine creates a ready-to-run insight engine.
//
//   - stateLoader: a closure that loads the current *models.RaceState from an
//     atomic pointer. Returns nil when no telemetry has been received yet.
//   - talkLevel: a closure that reads the current talk level (1-10) from the
//     config's atomic Int32.
//   - insightChan: insights that pass the talk-level filter are sent here
//     (non-blocking).
func NewEngine(
	stateLoader func() *models.RaceState,
	talkLevel func() int32,
	insightChan chan<- models.DrivingInsight,
) *Engine {
	return &Engine{
		stateLoader: stateLoader,
		rules:       DefaultRules(),
		cooldown:    NewCooldownManager(),
		insightChan: insightChan,
		interval:    5 * time.Second,
		talkLevel:   talkLevel,
	}
}

// SetOnInsight registers a callback that fires for every generated insight
// (before talk-level filtering). Used by the workspace writer to record findings.
func (e *Engine) SetOnInsight(fn func(models.DrivingInsight)) {
	e.onInsight = fn
}

// Run starts the evaluation loop. It ticks every 5 seconds, evaluates all 12
// rules against the latest RaceState, filters by talk level, and pushes
// surviving insights to insightChan. It blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	log.Info().Dur("interval", e.interval).Msg("Insight engine started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Insight engine stopping")
			return
		case <-ticker.C:
			e.evaluate()
		}
	}
}

// evaluate runs a single evaluation cycle.
func (e *Engine) evaluate() {
	state := e.stateLoader()
	if state == nil {
		return
	}

	// Calculate the minimum priority an insight must have to pass the
	// talk-level filter.  talkLevel=10 (chatty) lets everything through
	// (minPriority=1), talkLevel=1 (quiet) requires priority >= 10.
	tl := e.talkLevel()
	minPriority := int(11 - tl)

	for _, rule := range e.rules {
		insight := rule.Evaluate(state, e.cooldown)
		if insight == nil {
			continue
		}

		// Record to workspace (all findings, regardless of talk-level)
		if e.onInsight != nil {
			e.onInsight(*insight)
		}

		// Talk-level filter
		if insight.Priority < minPriority {
			log.Debug().
				Str("rule", rule.Name).
				Int("priority", insight.Priority).
				Int("min_priority", minPriority).
				Msg("Insight filtered by talk level")
			continue
		}

		// Non-blocking send — drop if the channel is full rather than
		// blocking the evaluation loop.
		select {
		case e.insightChan <- *insight:
			log.Info().Str("rule", rule.Name).Str("type", insight.Type).Msg("Insight generated")
		default:
			log.Warn().Str("rule", rule.Name).Msg("Insight channel full, dropping insight")
		}
	}
}
