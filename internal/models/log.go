package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
)

type LogEntry struct {

	// Contains all the fields set by the user.
	Data logrus.Fields `json:"data,omitempty"`

	// Time at which the log entry was created
	Time time.Time `json:"time"`

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

// MarshalJSON implements custom JSON marshaling for LogEntry
// It safely handles error types and other non-serializable values
func (l *LogEntry) MarshalJSON() ([]byte, error) {

	// Create a sanitized copy of the data fields
	sanitizedData := make(map[string]any)
	for key, value := range l.Data {
		sanitizedData[key] = sanitizeValue(value)
	}

	return json.Marshal(&struct {
		Data    map[string]any `json:"data,omitempty"`
		Time    time.Time      `json:"time"`
		Level   logrus.Level   `json:"level,omitempty"`
		Message string         `json:"message,omitempty"`
	}{
		Data:    sanitizedData,
		Time:    l.Time,
		Level:   l.Level,
		Message: l.Message,
	})
}

// sanitizeValue converts a value to a JSON-safe representation
func sanitizeValue(v any) any {
	if v == nil {
		return nil
	}

	// Handle error types specially - convert to string
	if err, ok := v.(error); ok {
		return err.Error()
	}

	val := reflect.ValueOf(v)

	switch val.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		// Convert non-serializable types to string representation
		return fmt.Sprintf("%v", v)

	case reflect.Ptr:
		if val.IsNil() {
			return nil
		}
		return sanitizeValue(val.Elem().Interface())

	case reflect.Struct:
		// For structs, try to marshal them once. If it fails, return string representation
		if b, err := json.Marshal(v); err != nil {
			return fmt.Sprintf("%+v", v)
		} else {
			// Use json.RawMessage so the pre-marshaled JSON is embedded directly
			return json.RawMessage(b)
		}

	case reflect.Map:
		// Sanitize map values, converting all keys to strings
		result := make(map[string]any)
		for _, key := range val.MapKeys() {
			mapVal := val.MapIndex(key)
			if mapVal.CanInterface() {
				// JSON object keys must be strings; use fmt.Sprint to preserve key information
				keyStr := fmt.Sprint(key.Interface())
				result[keyStr] = sanitizeValue(mapVal.Interface())
			}
		}
		return result

	case reflect.Slice, reflect.Array:
		// Sanitize slice/array elements
		result := make([]any, val.Len())
		for i := 0; i < val.Len(); i++ {
			elem := val.Index(i)
			if elem.CanInterface() {
				result[i] = sanitizeValue(elem.Interface())
			}
		}
		return result

	default:
		// Primitive types and other safe types
		return v
	}
}
