package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock types to simulate complex structures with function fields
type mockHTTPRequest struct {
	URL     string
	Method  string
	GetBody func() (io.ReadCloser, error) // This is the problematic function field
}

type mockAWSError struct {
	ServiceID     string
	OperationName string
	Request       *mockHTTPRequest
}

func (e *mockAWSError) Error() string {
	return fmt.Sprintf("operation error %s: %s", e.ServiceID, e.OperationName)
}

func TestLogEntry_MarshalJSON_BasicTypes(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Test message",
		Data: logrus.Fields{
			"string": "value",
			"int":    42,
			"float":  3.14,
			"bool":   true,
			"nil":    nil,
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "Test message", result["message"])
	assert.Equal(t, "info", result["level"]) // logrus.Level marshals as string

	dataField := result["data"].(map[string]any)
	assert.Equal(t, "value", dataField["string"])
	assert.Equal(t, float64(42), dataField["int"])
	assert.Equal(t, 3.14, dataField["float"])
	assert.Equal(t, true, dataField["bool"])
	assert.Nil(t, dataField["nil"])
}

func TestLogEntry_MarshalJSON_ErrorTypes(t *testing.T) {
	testErr := errors.New("test error message")

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.ErrorLevel,
		Message: "Error occurred",
		Data: logrus.Fields{
			"error": testErr,
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	assert.Equal(t, "test error message", dataField["error"])
}

func TestLogEntry_MarshalJSON_ComplexErrorWithFunctionFields(t *testing.T) {
	// Simulate AWS SDK error with HTTP request containing function field
	awsErr := &mockAWSError{
		ServiceID:     "Organizations",
		OperationName: "DescribeOrganization",
		Request: &mockHTTPRequest{
			URL:    "https://organizations.amazonaws.com",
			Method: "POST",
			GetBody: func() (io.ReadCloser, error) {
				return nil, nil
			},
		},
	}

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.WarnLevel,
		Message: "Unable to access AWS Organizations",
		Data: logrus.Fields{
			"error": awsErr,
		},
	}

	// This should not panic or return an error
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	// Error should be converted to its string representation
	dataField := result["data"].(map[string]any)
	assert.Equal(t, "operation error Organizations: DescribeOrganization", dataField["error"])
}

func TestLogEntry_MarshalJSON_FunctionFields(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.DebugLevel,
		Message: "Debug info",
		Data: logrus.Fields{
			"function": func() string { return "test" },
			"normal":   "value",
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	assert.Equal(t, "value", dataField["normal"])
	// Function should be converted to string representation (memory address)
	functionStr, ok := dataField["function"].(string)
	assert.True(t, ok, "function field should be converted to string")
	assert.NotEmpty(t, functionStr, "function field should have a value")
}

func TestLogEntry_MarshalJSON_NestedStructs(t *testing.T) {
	type SimpleStruct struct {
		Name  string
		Value int
	}

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Nested struct test",
		Data: logrus.Fields{
			"simple": SimpleStruct{Name: "test", Value: 42},
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	simpleStruct := dataField["simple"].(map[string]any)
	assert.Equal(t, "test", simpleStruct["Name"])
	assert.Equal(t, float64(42), simpleStruct["Value"])
}

func TestLogEntry_MarshalJSON_StructWithFunctionField(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.WarnLevel,
		Message: "Struct with function",
		Data: logrus.Fields{
			"request": &mockHTTPRequest{
				URL:    "https://example.com",
				Method: "GET",
				GetBody: func() (io.ReadCloser, error) {
					return nil, nil
				},
			},
		},
	}

	// Should convert struct to string representation since it can't be marshaled
	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	// Should be a string representation of the struct
	assert.IsType(t, "", dataField["request"])
	assert.Contains(t, dataField["request"].(string), "example.com")
}

func TestLogEntry_MarshalJSON_Maps(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Map test",
		Data: logrus.Fields{
			"metadata": map[string]any{
				"key1": "value1",
				"key2": 42,
				"key3": map[string]any{
					"nested": "value",
				},
			},
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	metadata := dataField["metadata"].(map[string]any)
	assert.Equal(t, "value1", metadata["key1"])
	assert.Equal(t, float64(42), metadata["key2"])

	nested := metadata["key3"].(map[string]any)
	assert.Equal(t, "value", nested["nested"])
}

func TestLogEntry_MarshalJSON_Slices(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Slice test",
		Data: logrus.Fields{
			"items":   []string{"item1", "item2", "item3"},
			"numbers": []int{1, 2, 3},
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	items := dataField["items"].([]any)
	assert.Len(t, items, 3)
	assert.Equal(t, "item1", items[0])

	numbers := dataField["numbers"].([]any)
	assert.Len(t, numbers, 3)
	assert.Equal(t, float64(1), numbers[0])
}

func TestLogEntry_MarshalJSON_EmptyData(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Empty data test",
		Data:    logrus.Fields{},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "Empty data test", result["message"])
	// Empty data should be omitted or empty
	if dataField, exists := result["data"]; exists {
		assert.Empty(t, dataField)
	}
}

func TestLogEntry_MarshalJSON_NilData(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Nil data test",
		Data:    nil,
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "Nil data test", result["message"])
}

func TestLogEntry_MarshalJSON_Pointers(t *testing.T) {
	stringVal := "pointer value"
	intVal := 42

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Pointer test",
		Data: logrus.Fields{
			"stringPtr": &stringVal,
			"intPtr":    &intVal,
			"nilPtr":    (*string)(nil),
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	assert.Equal(t, "pointer value", dataField["stringPtr"])
	assert.Equal(t, float64(42), dataField["intPtr"])
	assert.Nil(t, dataField["nilPtr"])
}

func TestLogEntry_MarshalJSON_MultipleDataFields(t *testing.T) {
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.ErrorLevel,
		Message: "Complex log entry",
		Data: logrus.Fields{
			"error":      errors.New("something went wrong"),
			"statusCode": 500,
			"endpoint":   "https://api.example.com/users",
			"user":       "john.doe",
			"metadata": map[string]any{
				"retries": 3,
				"timeout": "30s",
			},
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "Complex log entry", result["message"])

	dataField := result["data"].(map[string]any)
	assert.Equal(t, "something went wrong", dataField["error"])
	assert.Equal(t, float64(500), dataField["statusCode"])
	assert.Equal(t, "https://api.example.com/users", dataField["endpoint"])
	assert.Equal(t, "john.doe", dataField["user"])

	metadata := dataField["metadata"].(map[string]any)
	assert.Equal(t, float64(3), metadata["retries"])
	assert.Equal(t, "30s", metadata["timeout"])
}
