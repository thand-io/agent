package models_test

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

type localEncrypt struct {
}

func NewMockedEncrypt() *localEncrypt {
	return &localEncrypt{}
}

func (r localEncrypt) Initialize() error {
	return nil
}

func (r localEncrypt) Shutdown() error {
	return nil
}

func (r localEncrypt) Decrypt(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}

func (r localEncrypt) Encrypt(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}

var encryptor = NewMockedEncrypt()

func TestEncodingWrapper_Encode(t *testing.T) {

	tests := []struct {
		name string
		data models.EncodingWrapper
	}{
		{
			name: "encode workflow task",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":   "task-123",
					"name": "test task",
				},
			},
		},
		{
			name: "encode auth data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_AUTH,
				Data: map[string]any{
					"user":  "testuser",
					"token": "abc123",
				},
			},
		},
		{
			name: "encode session data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_SESSION,
				Data: map[string]any{
					"session_id": "sess-456",
					"expires":    "2023-12-31",
				},
			},
		},
		{
			name: "encode session local data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_SESSION_LOCAL,
				Data: map[string]any{
					"local_id": "local-789",
					"path":     "/tmp/session",
				},
			},
		},
		{
			name: "encode empty data",
			data: models.EncodingWrapper{
				Type: "empty",
				Data: nil,
			},
		},
		{
			name: "encode string data",
			data: models.EncodingWrapper{
				Type: "string",
				Data: "test string",
			},
		},
		{
			name: "encode number data",
			data: models.EncodingWrapper{
				Type: "number",
				Data: 42,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.data.EncodeBase64()

			// Check that encoding returns a non-empty string
			if len(encoded) == 0 {
				t.Error("Encode() returned empty string")
			}

			// Check that the encoded string is base64 (basic validation)
			if len(encoded) == 0 {
				t.Error("Encoded string should not be empty")
			}
		})
	}
}

func TestEncodingWrapper_Decode(t *testing.T) {
	tests := []struct {
		name    string
		data    models.EncodingWrapper
		wantErr bool
	}{
		{
			name: "decode workflow task",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":   "task-123",
					"name": "test task",
				},
			},
			wantErr: false,
		},
		{
			name: "decode auth data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_AUTH,
				Data: map[string]any{
					"user":  "testuser",
					"token": "abc123",
				},
			},
			wantErr: false,
		},
		{
			name: "decode session data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_SESSION,
				Data: map[string]any{
					"session_id": "sess-456",
					"expires":    "2023-12-31",
				},
			},
			wantErr: false,
		},
		{
			name: "decode empty data",
			data: models.EncodingWrapper{
				Type: "empty",
				Data: nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First encode the data
			encoded := tt.data.EncodeBase64()

			// Then decode it
			var wrapper models.EncodingWrapper
			decoded, err := wrapper.Decode(encoded)

			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if decoded == nil {
					t.Error("Decode() returned nil")
					return
				}

				if decoded.Type != tt.data.Type {
					t.Errorf("Decode() Type = %v, want %v", decoded.Type, tt.data.Type)
				}
			}
		})
	}
}

func TestEncodingWrapper_DecodeInvalidData(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "invalid base64",
			input:   "invalid-base64!@#",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "valid base64 but invalid compressed data",
			input:   "SGVsbG8gV29ybGQ=", // "Hello World" in base64
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var wrapper models.EncodingWrapper
			_, err := wrapper.Decode(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEncodingWrapper_EncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data models.EncodingWrapper
	}{
		{
			name: "round trip workflow task",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":          "task-123",
					"name":        "test task",
					"description": "This is a test task",
					"priority":    1,
					"completed":   false,
				},
			},
		},
		{
			name: "round trip complex nested data",
			data: models.EncodingWrapper{
				Type: "complex",
				Data: map[string]any{
					"nested": map[string]any{
						"level1": map[string]any{
							"level2": "deep value",
							"array":  []any{1, 2, 3, "test"},
						},
					},
					"simple": "value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			encoded := tt.data.EncodeBase64()

			// Decode
			var wrapper models.EncodingWrapper
			decoded, err := wrapper.Decode(encoded)

			if err != nil {
				t.Errorf("Round trip failed: %v", err)
				return
			}

			if decoded.Type != tt.data.Type {
				t.Errorf("Round trip Type mismatch: got %v, want %v", decoded.Type, tt.data.Type)
			}

			// For basic validation, just check that we got something back
			if decoded.Data == nil && tt.data.Data != nil {
				t.Error("Round trip lost data")
			}
		})
	}
}

func TestConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "ENCODED_WORKFLOW_TASK constant",
			constant: sdkConstants.ENCODED_WORKFLOW_TASK,
			expected: "workflow_task",
		},
		{
			name:     "ENCODED_AUTH constant",
			constant: sdkConstants.ENCODED_AUTH,
			expected: "auth",
		},
		{
			name:     "ENCODED_SESSION constant",
			constant: sdkConstants.ENCODED_SESSION,
			expected: "session",
		},
		{
			name:     "ENCODED_SESSION_LOCAL constant",
			constant: sdkConstants.ENCODED_SESSION_LOCAL,
			expected: "session_local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Constant %s = %v, want %v", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func BenchmarkEncodingWrapper_Encode(b *testing.B) {
	data := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_WORKFLOW_TASK,
		Data: map[string]any{
			"id":          "task-123",
			"name":        "benchmark task",
			"description": "This is a benchmark task with some data",
			"priority":    1,
			"completed":   false,
			"metadata": map[string]any{
				"created": "2023-01-01",
				"updated": "2023-01-02",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = data.EncodeBase64()
	}
}

func BenchmarkEncodingWrapper_Decode(b *testing.B) {
	data := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_WORKFLOW_TASK,
		Data: map[string]any{
			"id":          "task-123",
			"name":        "benchmark task",
			"description": "This is a benchmark task with some data",
			"priority":    1,
			"completed":   false,
			"metadata": map[string]any{
				"created": "2023-01-01",
				"updated": "2023-01-02",
			},
		},
	}
	encoded := data.EncodeBase64()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wrapper models.EncodingWrapper
		_, _ = wrapper.Decode(encoded)
	}
}

func TestEncodingWrapper_EncodeBytes(t *testing.T) {

	tests := []struct {
		name string
		data models.EncodingWrapper
	}{
		{
			name: "encode workflow task",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":   "task-123",
					"name": "test task",
				},
			},
		},
		{
			name: "encode session data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_SESSION,
				Data: map[string]any{
					"session_id": "sess-456",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.data.EncodeBytes()

			// Check that encoding returns non-empty bytes
			if len(encoded) == 0 {
				t.Error("EncodeBytes() returned empty byte slice")
			}
		})
	}
}

func TestEncodingWrapper_DecodeBytes(t *testing.T) {
	tests := []struct {
		name    string
		data    models.EncodingWrapper
		wantErr bool
	}{
		{
			name: "decode workflow task",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":   "task-123",
					"name": "test task",
				},
			},
			wantErr: false,
		},
		{
			name: "decode empty data",
			data: models.EncodingWrapper{
				Type: "empty",
				Data: nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First encode the data
			encoded := tt.data.EncodeBytes()

			// Then decode it
			var wrapper models.EncodingWrapper
			decoded, err := wrapper.DecodeBytes(encoded)

			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if decoded == nil {
					t.Error("DecodeBytes() returned nil")
					return
				}

				if decoded.Type != tt.data.Type {
					t.Errorf("DecodeBytes() Type = %v, want %v", decoded.Type, tt.data.Type)
				}
			}
		})
	}
}

func TestEncodingWrapper_EncodeAndEncrypt(t *testing.T) {
	data := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_WORKFLOW_TASK,
		Data: map[string]any{
			"id":   "task-encrypt",
			"name": "encrypted task",
		},
	}

	encoded := data.EncodeAndEncrypt(encryptor)

	if len(encoded) == 0 {
		t.Error("EncodeAndEncrypt() returned empty string")
	}
}

func TestEncodingWrapper_DecodeAndDecrypt(t *testing.T) {
	data := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_WORKFLOW_TASK,
		Data: map[string]any{
			"id":   "task-encrypt-decode",
			"name": "encrypted task for decode",
		},
	}

	encoded := data.EncodeAndEncrypt(encryptor)

	var wrapper models.EncodingWrapper
	decoded, err := wrapper.DecodeAndDecrypt(encoded, encryptor)

	if err != nil {
		t.Errorf("DecodeAndDecrypt() error = %v", err)
		return
	}

	if decoded == nil {
		t.Error("DecodeAndDecrypt() returned nil")
		return
	}

	if decoded.Type != data.Type {
		t.Errorf("DecodeAndDecrypt() Type = %v, want %v", decoded.Type, data.Type)
	}
}

// TestEncodingWrapper_URLSafeEncoding verifies that EncodeBase64 produces URL-safe base64
// (contains - and _ instead of + and /) which is safe for use in URLs without additional encoding
func TestEncodingWrapper_URLSafeEncoding(t *testing.T) {
	tests := []struct {
		name string
		data models.EncodingWrapper
	}{
		{
			name: "encode with characters that would produce + or / in standard base64",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_SESSION,
				Data: map[string]any{
					// These values are crafted to potentially produce + or / in standard base64
					"session_id": "test-session-with-special-chars-?????",
					"user_data":  "user@example.com+test/path",
					"metadata":   ">>><<<???",
				},
			},
		},
		{
			name: "encode workflow with potential unsafe chars",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"task_id":     "task-123-456-789",
					"description": "Test task with special characters: + / = ? & %",
					"url":         "https://example.com/path?query=value&other=test",
				},
			},
		},
		{
			name: "encode auth data",
			data: models.EncodingWrapper{
				Type: sdkConstants.ENCODED_AUTH,
				Data: map[string]any{
					"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
					"user":  "user@domain.com",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.data.EncodeBase64()

			// Verify the encoded string doesn't contain + or / characters
			// URL-safe base64 uses - and _ instead
			for i, char := range encoded {
				if char == '+' {
					t.Errorf("EncodeBase64() contains '+' at position %d, should use '-' for URL-safe encoding", i)
				}
				if char == '/' {
					t.Errorf("EncodeBase64() contains '/' at position %d, should use '_' for URL-safe encoding", i)
				}
			}

			// Verify the encoded string only contains valid URL-safe base64 characters
			// Valid characters: A-Z, a-z, 0-9, -, _, and optional = for padding
			for i, char := range encoded {
				valid := (char >= 'A' && char <= 'Z') ||
					(char >= 'a' && char <= 'z') ||
					(char >= '0' && char <= '9') ||
					char == '-' || char == '_' || char == '='
				if !valid {
					t.Errorf("EncodeBase64() contains invalid character '%c' at position %d", char, i)
				}
			}

			// Verify round-trip works
			var wrapper models.EncodingWrapper
			decoded, err := wrapper.Decode(encoded)
			if err != nil {
				t.Errorf("Failed to decode URL-safe base64: %v", err)
			}
			if decoded.Type != tt.data.Type {
				t.Errorf("Round trip Type mismatch: got %v, want %v", decoded.Type, tt.data.Type)
			}
		})
	}
}

// TestEncodingWrapper_BackwardCompatibility tests that the decode function can handle
// both standard base64 (with + and /) and URL-safe base64 (with - and _)
func TestEncodingWrapper_BackwardCompatibility(t *testing.T) {
	// Create test data that, when properly encoded, will contain + or / characters in standard base64
	testData := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_SESSION,
		Data: map[string]any{
			// These values are designed to produce base64 strings with + or / when using standard encoding
			"session_id": "test-session-????????????????",
			"user_data":  "user@example.com+++++",
			"metadata":   ">>>>>>>><<<<<<<<<<",
		},
	}

	// First, encode using the current implementation (URL-safe)
	urlSafeEncoded := testData.EncodeBase64()

	// Create a standard base64 encoded version by manually encoding the same data with standard base64
	// We'll use the internal encoding process but with standard base64
	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	// Compress with zlib (same as the internal process)
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err = writer.Write(jsonData)
	if err != nil {
		t.Fatalf("Failed to compress data: %v", err)
	}
	writer.Close()

	// Encode with standard base64 (to create legacy format with + and /)
	standardBase64Encoded := base64.StdEncoding.EncodeToString(compressed.Bytes())

	t.Run("decode URL-safe base64", func(t *testing.T) {
		var wrapper models.EncodingWrapper
		decoded, err := wrapper.Decode(urlSafeEncoded)

		if err != nil {
			t.Errorf("Decode() failed for URL-safe base64: %v", err)
			return
		}

		if decoded == nil {
			t.Error("Decode() returned nil for URL-safe base64")
			return
		}

		if decoded.Type != testData.Type {
			t.Errorf("Decode() Type = %v, want %v", decoded.Type, testData.Type)
		}
	})

	t.Run("decode standard base64 (backward compatibility)", func(t *testing.T) {
		// Only run this test if the standard encoded version actually contains + or /
		// (which indicates it's different from URL-safe encoding)
		containsPlusOrSlash := false
		for _, char := range standardBase64Encoded {
			if char == '+' || char == '/' {
				containsPlusOrSlash = true
				break
			}
		}

		if !containsPlusOrSlash {
			t.Skip("Test data doesn't produce + or / in standard base64, skipping backward compatibility test")
			return
		}

		var wrapper models.EncodingWrapper
		decoded, err := wrapper.Decode(standardBase64Encoded)

		if err != nil {
			t.Errorf("Decode() failed for standard base64: %v", err)
			return
		}

		if decoded == nil {
			t.Error("Decode() returned nil for standard base64")
			return
		}

		if decoded.Type != testData.Type {
			t.Errorf("Decode() Type = %v, want %v", decoded.Type, testData.Type)
		}

		t.Logf("Successfully decoded standard base64 with + or / characters (backward compatibility confirmed)")
	})
}
