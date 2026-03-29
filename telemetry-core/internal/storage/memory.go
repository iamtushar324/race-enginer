package storage

import (
	"sync/atomic"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// LiveCache provides lock-free access to the most recent RaceState snapshot
// using sync/atomic.Pointer. The insight engine and dashboard API read from
// this cache for sub-microsecond latency without touching DuckDB on disk.
type LiveCache struct {
	ptr atomic.Pointer[models.RaceState]
}

// NewLiveCache creates a new in-memory cache with no initial state.
func NewLiveCache() *LiveCache {
	return &LiveCache{}
}

// Store atomically replaces the cached RaceState pointer.
func (c *LiveCache) Store(state *models.RaceState) {
	c.ptr.Store(state)
}

// Load atomically returns the latest RaceState, or nil if none has been stored.
func (c *LiveCache) Load() *models.RaceState {
	return c.ptr.Load()
}
