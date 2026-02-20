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

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	// Struct is now serialized field-by-field: primitive fields come through,
	// the function field is converted to its string representation.
	request := dataField["request"].(map[string]any)
	assert.Equal(t, "https://example.com", request["URL"])
	assert.Equal(t, "GET", request["Method"])
	// GetBody is a func — should be serialized as a non-empty string
	getBodyStr, ok := request["GetBody"].(string)
	assert.True(t, ok, "function field should be converted to string")
	assert.NotEmpty(t, getBodyStr)
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

func TestLogEntry_MarshalJSON_SensitiveFieldsRedacted(t *testing.T) {
	// Struct with a mix of normal and sensitive-tagged fields
	type mockProviderConfig struct {
		Endpoint    string `json:"endpoint"`
		Username    string `json:"username"`
		Password    string `json:"password" sensitive:"true"`
		APIKey      string `json:"api_key" sensitive:"true"`
		ServiceName string `json:"service_name"`
	}

	cfg := mockProviderConfig{
		Endpoint:    "https://api.example.com",
		Username:    "admin",
		Password:    "super-secret-password",
		APIKey:      "sk-1234567890abcdef",
		ServiceName: "my-service",
	}

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Provider configured",
		Data: logrus.Fields{
			"config": cfg,
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	config := dataField["config"].(map[string]any)

	// Non-sensitive fields should pass through unchanged
	assert.Equal(t, "https://api.example.com", config["endpoint"])
	assert.Equal(t, "admin", config["username"])
	assert.Equal(t, "my-service", config["service_name"])

	// Sensitive fields must be redacted
	assert.Equal(t, "[REDACTED]", config["password"])
	assert.Equal(t, "[REDACTED]", config["api_key"])
}

func TestLogEntry_MarshalJSON_SensitiveFieldsNotLeakedAsString(t *testing.T) {
	// Ensure the raw secret value does not appear anywhere in the JSON output
	const secretValue = "top-secret-value-xyz"

	type mockCreds struct {
		Host  string `json:"host"`
		Token string `json:"token" sensitive:"true"`
	}

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.WarnLevel,
		Message: "Credentials used",
		Data: logrus.Fields{
			"creds": mockCreds{Host: "db.internal", Token: secretValue},
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	assert.NotContains(t, string(data), secretValue, "secret value must not appear in serialized log output")
}

func TestLogEntry_MarshalJSON_JSONDashFieldsIgnored(t *testing.T) {
	// Struct with a json:"-" field that should be completely excluded from output.
	type mockConfig struct {
		Name     string `json:"name"`
		Internal string `json:"-"`
		Debug    bool   `json:"-"`
		Value    int    `json:"value"`
	}

	cfg := mockConfig{
		Name:     "my-config",
		Internal: "should-not-appear",
		Debug:    true,
		Value:    99,
	}

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Config loaded",
		Data: logrus.Fields{
			"config": cfg,
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	config := dataField["config"].(map[string]any)

	// Normal fields should be present under their json tag names.
	assert.Equal(t, "my-config", config["name"])
	assert.Equal(t, float64(99), config["value"])

	// Fields tagged json:"-" must not appear — neither under the field name nor under "-".
	assert.NotContains(t, config, "Internal", "json:\"-\" field must be excluded (Go field name)")
	assert.NotContains(t, config, "Debug", "json:\"-\" field must be excluded (Go field name)")
	assert.NotContains(t, config, "-", "json:\"-\" field must not appear under the key \"-\"")

	// The secret value itself must not appear anywhere in the raw JSON.
	assert.NotContains(t, string(data), "should-not-appear")
}

func TestLogEntry_MarshalJSON_JSONDashFieldsIgnored_Nested(t *testing.T) {
	// Nested struct where the inner struct has a json:"-" field.
	type mockInner struct {
		ID     string `json:"id"`
		Secret string `json:"-"`
	}
	type mockOuter struct {
		Label string    `json:"label"`
		Inner mockInner `json:"inner"`
	}

	obj := mockOuter{
		Label: "outer-label",
		Inner: mockInner{
			ID:     "inner-id",
			Secret: "inner-secret-value",
		},
	}

	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.DebugLevel,
		Message: "Nested json dash test",
		Data: logrus.Fields{
			"obj": obj,
		},
	}

	data, err := json.Marshal(entry)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	dataField := result["data"].(map[string]any)
	outer := dataField["obj"].(map[string]any)
	inner := outer["inner"].(map[string]any)

	// Normal fields should be present.
	assert.Equal(t, "outer-label", outer["label"])
	assert.Equal(t, "inner-id", inner["id"])

	// The json:"-" field must be excluded from the nested struct too.
	assert.NotContains(t, inner, "Secret", "nested json:\"-\" field must be excluded (Go field name)")
	assert.NotContains(t, inner, "-", "nested json:\"-\" field must not appear under the key \"-\"")

	// Raw value must not leak into the JSON output.
	assert.NotContains(t, string(data), "inner-secret-value")
}

func TestLogEntry_MarshalJSON_NestedSensitiveFieldsRedacted(t *testing.T) {
	// Nested structs where the inner struct has sensitive fields.
	type mockNestedCreds struct {
		ProjectID    string `json:"project_id"`
		PrivateKey   string `json:"private_key" sensitive:"true"`
		PrivateKeyID string `json:"private_key_id" sensitive:"true"`
	}
	type mockGCPCredentials struct {
		Provider string          `json:"provider"`
		Region   string          `json:"region"`
		Creds    mockNestedCreds `json:"creds"`
	}
	const (
		privateKey   = "-----BEGIN PRIVATE KEY-----super-secret-key-----END PRIVATE KEY-----"
		privateKeyID = "private-key-id-12345"
	)
	gcpCreds := mockGCPCredentials{
		Provider: "gcp",
		Region:   "us-central1",
		Creds: mockNestedCreds{
			ProjectID:    "my-project",
			PrivateKey:   privateKey,
			PrivateKeyID: privateKeyID,
		},
	}
	entry := &LogEntry{
		Time:    time.Date(2026, 1, 13, 12, 0, 0, 0, time.UTC),
		Level:   logrus.InfoLevel,
		Message: "Using GCP credentials",
		Data: logrus.Fields{
			"gcp_credentials": gcpCreds,
		},
	}
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)
	dataField, ok := result["data"].(map[string]any)
	require.True(t, ok, "data field should be a map")
	credsAny, ok := dataField["gcp_credentials"]
	require.True(t, ok, "gcp_credentials field should be present")
	credsMap, ok := credsAny.(map[string]any)
	require.True(t, ok, "gcp_credentials should be a map")
	innerAny, ok := credsMap["creds"]
	require.True(t, ok, "nested creds field should be present")
	innerMap, ok := innerAny.(map[string]any)
	require.True(t, ok, "nested creds should be a map")
	// Outer non-sensitive fields should be unchanged.
	assert.Equal(t, "gcp", credsMap["provider"])
	assert.Equal(t, "us-central1", credsMap["region"])
	// Inner non-sensitive field should be unchanged.
	assert.Equal(t, "my-project", innerMap["project_id"])
	// Inner sensitive fields must be redacted.
	assert.Equal(t, "[REDACTED]", innerMap["private_key"])
	assert.Equal(t, "[REDACTED]", innerMap["private_key_id"])
	// Ensure raw sensitive values do not appear anywhere in the JSON output.
	jsonStr := string(data)
	assert.NotContains(t, jsonStr, privateKey)
	assert.NotContains(t, jsonStr, privateKeyID)
}
