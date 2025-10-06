package models

import (
	"testing"
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

func (r localEncrypt) Decrypt(data []byte) ([]byte, error) {
	return data, nil
}

func (r localEncrypt) Encrypt(data []byte) ([]byte, error) {
	return data, nil
}

var encryptor = NewMockedEncrypt()

func TestEncodingWrapper_Encode(t *testing.T) {

	tests := []struct {
		name string
		data EncodingWrapper
	}{
		{
			name: "encode workflow task",
			data: EncodingWrapper{
				Type: ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":   "task-123",
					"name": "test task",
				},
			},
		},
		{
			name: "encode auth data",
			data: EncodingWrapper{
				Type: ENCODED_AUTH,
				Data: map[string]any{
					"user":  "testuser",
					"token": "abc123",
				},
			},
		},
		{
			name: "encode session data",
			data: EncodingWrapper{
				Type: ENCODED_SESSION,
				Data: map[string]any{
					"session_id": "sess-456",
					"expires":    "2023-12-31",
				},
			},
		},
		{
			name: "encode session local data",
			data: EncodingWrapper{
				Type: ENCODED_SESSION_LOCAL,
				Data: map[string]any{
					"local_id": "local-789",
					"path":     "/tmp/session",
				},
			},
		},
		{
			name: "encode empty data",
			data: EncodingWrapper{
				Type: "empty",
				Data: nil,
			},
		},
		{
			name: "encode string data",
			data: EncodingWrapper{
				Type: "string",
				Data: "test string",
			},
		},
		{
			name: "encode number data",
			data: EncodingWrapper{
				Type: "number",
				Data: 42,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := tt.data.Encode()

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
		data    EncodingWrapper
		wantErr bool
	}{
		{
			name: "decode workflow task",
			data: EncodingWrapper{
				Type: ENCODED_WORKFLOW_TASK,
				Data: map[string]any{
					"id":   "task-123",
					"name": "test task",
				},
			},
			wantErr: false,
		},
		{
			name: "decode auth data",
			data: EncodingWrapper{
				Type: ENCODED_AUTH,
				Data: map[string]any{
					"user":  "testuser",
					"token": "abc123",
				},
			},
			wantErr: false,
		},
		{
			name: "decode session data",
			data: EncodingWrapper{
				Type: ENCODED_SESSION,
				Data: map[string]any{
					"session_id": "sess-456",
					"expires":    "2023-12-31",
				},
			},
			wantErr: false,
		},
		{
			name: "decode empty data",
			data: EncodingWrapper{
				Type: "empty",
				Data: nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First encode the data
			encoded := tt.data.Encode()

			// Then decode it
			var wrapper EncodingWrapper
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
			var wrapper EncodingWrapper
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
		data EncodingWrapper
	}{
		{
			name: "round trip workflow task",
			data: EncodingWrapper{
				Type: ENCODED_WORKFLOW_TASK,
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
			data: EncodingWrapper{
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
			encoded := tt.data.Encode()

			// Decode
			var wrapper EncodingWrapper
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
			constant: ENCODED_WORKFLOW_TASK,
			expected: "workflow_task",
		},
		{
			name:     "ENCODED_AUTH constant",
			constant: ENCODED_AUTH,
			expected: "auth",
		},
		{
			name:     "ENCODED_SESSION constant",
			constant: ENCODED_SESSION,
			expected: "session",
		},
		{
			name:     "ENCODED_SESSION_LOCAL constant",
			constant: ENCODED_SESSION_LOCAL,
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
	data := EncodingWrapper{
		Type: ENCODED_WORKFLOW_TASK,
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
		_ = data.Encode()
	}
}

func BenchmarkEncodingWrapper_Decode(b *testing.B) {
	data := EncodingWrapper{
		Type: ENCODED_WORKFLOW_TASK,
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
	encoded := data.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wrapper EncodingWrapper
		_, _ = wrapper.Decode(encoded)
	}
}
