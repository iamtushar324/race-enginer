package ingestion

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
	backoffFactor  = 1.5
	maxFailures    = 10
)

// selfHeal wraps a long-running function in a goroutine that automatically
// restarts on panic with exponential backoff. On a clean return (no panic)
// or context cancellation, the goroutine exits normally.
//
// The backoff resets to the initial value when the wrapped function runs
// for at least 10 seconds without panicking, indicating a healthy state.
func selfHeal(ctx context.Context, wg *sync.WaitGroup, name string, fn func(ctx context.Context)) {
	defer wg.Done()

	backoff := initialBackoff
	consecutiveFailures := 0

	for {
		startedAt := time.Now()
		panicked := false

		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
					consecutiveFailures++
					log.Error().
						Str("goroutine", name).
						Interface("panic", r).
						Int("failures", consecutiveFailures).
						Msg("Panicked — will restart")
				}
			}()
			fn(ctx)
		}()

		// Clean exit (no panic) — the function returned normally.
		// This usually means ctx was cancelled.
		if !panicked {
			return
		}

		// Bail out after too many consecutive failures.
		if consecutiveFailures >= maxFailures {
			log.Error().
				Str("goroutine", name).
				Int("failures", consecutiveFailures).
				Msg("Too many consecutive failures — giving up")
			return
		}

		// If the function survived for a decent stretch, reset backoff.
		if time.Since(startedAt) > 10*time.Second {
			backoff = initialBackoff
			consecutiveFailures = 0
		}

		// Wait with exponential backoff before restarting.
		log.Info().
			Str("goroutine", name).
			Dur("backoff", backoff).
			Msg("Restarting after backoff")

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		// Grow backoff for next potential failure.
		backoff = time.Duration(float64(backoff) * backoffFactor)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
