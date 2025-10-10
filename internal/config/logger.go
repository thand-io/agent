package config

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

type thandLogger struct {
	// Ring buffer for storing events
	eventBuffer []*models.LogEntry
	maxSize     int
	currentPos  int
	isFull      bool
	mu          sync.RWMutex
}

func NewThandLogger() *thandLogger {
	return &thandLogger{
		eventBuffer: make([]*models.LogEntry, 1000),
		maxSize:     1000,
		currentPos:  0,
		isFull:      false,
	}
}

func (t *thandLogger) Fire(entry *logrus.Entry) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Add to ring buffer
	t.eventBuffer[t.currentPos] = models.NewLogEntry(entry)
	t.currentPos = (t.currentPos + 1) % t.maxSize

	if t.currentPos == 0 {
		t.isFull = true
	}

	return nil
}

func (t *thandLogger) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		// logrus.DebugLevel,
		// logrus.TraceLevel,
	}
}

func (t *thandLogger) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.eventBuffer = make([]*models.LogEntry, t.maxSize)
	t.currentPos = 0
	t.isFull = false
}

func (t *thandLogger) GetEvents() []*models.LogEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.isFull {
		// Return only filled portion
		result := make([]*models.LogEntry, t.currentPos)
		copy(result, t.eventBuffer[:t.currentPos])
		return result
	}

	// Return in chronological order (oldest first)
	result := make([]*models.LogEntry, t.maxSize)
	copy(result, t.eventBuffer[t.currentPos:])
	copy(result[t.maxSize-t.currentPos:], t.eventBuffer[:t.currentPos])
	return result
}

func (t *thandLogger) GetRecentEvents(count int) []*models.LogEntry {
	events := t.GetEvents()
	if len(events) <= count {
		return events
	}
	return events[len(events)-count:]
}

// LogFilter contains the filtering criteria for log events
type LogFilter struct {
	// Filter by log levels (if empty, all levels are included)
	Levels []logrus.Level `json:"levels,omitempty"`
	// Filter events after this time (if nil, no time filter from start)
	Since *time.Time `json:"since,omitempty"`
	// Filter events before this time (if nil, no time filter to end)
	Until *time.Time `json:"until,omitempty"`
	// Maximum number of events to return (if 0, no limit)
	Limit int `json:"limit,omitempty"`
}

// GetEventsWithFilter returns events that match the specified filter criteria
func (t *thandLogger) GetEventsWithFilter(filter LogFilter) []*models.LogEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()

	allEvents := t.getEventsInternal()
	var filtered []*models.LogEntry

	// Create a map for quick level lookup if levels are specified
	levelMap := make(map[logrus.Level]bool)
	if len(filter.Levels) > 0 {
		for _, level := range filter.Levels {
			levelMap[level] = true
		}
	}

	for _, entry := range allEvents {
		// Filter by log level
		if len(filter.Levels) > 0 && !levelMap[entry.Level] {
			continue
		}

		// Filter by time range
		if filter.Since != nil && entry.Time.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && entry.Time.After(*filter.Until) {
			continue
		}

		filtered = append(filtered, entry)

		// Apply limit if specified
		if filter.Limit > 0 && len(filtered) >= filter.Limit {
			break
		}
	}

	return filtered
}

// getEventsInternal returns events without additional locking (assumes caller has lock)
func (t *thandLogger) getEventsInternal() []*models.LogEntry {
	if !t.isFull {
		// Return only filled portion
		result := make([]*models.LogEntry, t.currentPos)
		copy(result, t.eventBuffer[:t.currentPos])
		return result
	}

	// Return in chronological order (oldest first)
	result := make([]*models.LogEntry, t.maxSize)
	copy(result, t.eventBuffer[t.currentPos:])
	copy(result[t.maxSize-t.currentPos:], t.eventBuffer[:t.currentPos])
	return result
}
