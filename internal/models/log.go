package models

import (
	"time"

	"github.com/sirupsen/logrus"
)

type LogEntry struct {

	// Contains all the fields set by the user.
	Data logrus.Fields `json:"data,omitempty"`

	// Time at which the log entry was created
	Time time.Time `json:"time,omitempty"`

	// Level the log entry was logged at: Trace, Debug, Info, Warn, Error, Fatal or Panic
	// This field will be set on entry firing and the value will be equal to the one in Logger struct field.
	Level logrus.Level `json:"level,omitempty"`

	// Message passed to Trace, Debug, Info, Warn, Error, Fatal or Panic
	Message string `json:"message,omitempty"`
}

func NewLogEntry(entry *logrus.Entry) *LogEntry {
	logEntry := &LogEntry{
		Data:    entry.Data,
		Time:    entry.Time,
		Level:   entry.Level,
		Message: entry.Message,
	}

	return logEntry
}
