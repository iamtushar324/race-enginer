package insights

import (
	"sync"
	"time"

	"github.com/tusharbhardwaj/race-engineer/telemetry-core/internal/models"
)

// LogEntry is a single recorded insight with metadata.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`  // "engine", "analyst", "webhook", "comms_gate", "driver_query"
	Message   string                `json:"message"`
	Type      string                `json:"type"`     // warning, info, encouragement, strategy
	Priority  int                   `json:"priority"`
}

// Log is a thread-safe, capped ring buffer of insight entries.
type Log struct {
	mu      sync.RWMutex
	entries []LogEntry
	cap     int
}

// NewLog creates an insight log with the given capacity.
func NewLog(capacity int) *Log {
	return &Log{
		entries: make([]LogEntry, 0, capacity),
		cap:     capacity,
	}
}

// Record appends an insight to the log, evicting the oldest if at capacity.
func (l *Log) Record(source string, insight models.DrivingInsight) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Source:    source,
		Message:   insight.Message,
		Type:      insight.Type,
		Priority:  insight.Priority,
	}

	if len(l.entries) >= l.cap {
		// Shift left by 1 to evict oldest.
		copy(l.entries, l.entries[1:])
		l.entries[len(l.entries)-1] = entry
	} else {
		l.entries = append(l.entries, entry)
	}
}

// Recent returns the last N entries (newest last). If n <= 0, returns all.
func (l *Log) Recent(n int) []LogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	total := len(l.entries)
	if n <= 0 || n > total {
		n = total
	}

	result := make([]LogEntry, n)
	copy(result, l.entries[total-n:])
	return result
}

// FormatForLLM returns a compact text summary of the last N entries for prompt injection.
func (l *Log) FormatForLLM(n int) string {
	entries := l.Recent(n)
	if len(entries) == 0 {
		return "No insights have been generated yet this session."
	}

	var buf []byte
	buf = append(buf, "Recent Insight History (newest last):\n"...)
	for _, e := range entries {
		buf = append(buf, "- ["...)
		buf = append(buf, e.Timestamp.Format("15:04:05")...)
		buf = append(buf, "] ("...)
		buf = append(buf, e.Source...)
		buf = append(buf, ", P"...)
		buf = append(buf, byte('0'+e.Priority))
		buf = append(buf, ") "...)
		buf = append(buf, e.Message...)
		buf = append(buf, '\n')
	}
	return string(buf)
}
