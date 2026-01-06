package runner

import (
	"testing"
)

func TestExpandURITemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		input    any
		expected string
		hasError bool
	}{
		{
			name:     "Simple substitution",
			template: "https://api.example.com/users/{userId}",
			input:    map[string]any{"userId": "123"},
			expected: "https://api.example.com/users/123",
			hasError: false,
		},
		{
			name:     "Multiple substitutions",
			template: "https://api.example.com/users/{userId}/posts/{postId}",
			input:    map[string]any{"userId": "123", "postId": "456"},
			expected: "https://api.example.com/users/123/posts/456",
			hasError: false,
		},
		{
			name:     "Missing variable",
			template: "https://api.example.com/users/{userId}/posts/{postId}",
			input:    map[string]any{"userId": "123"},
			expected: "https://api.example.com/users/123/posts/",
			hasError: false,
		},
		{
			name:     "No input data",
			template: "https://api.example.com/users/{userId}",
			input:    nil,
			expected: "https://api.example.com/users/",
			hasError: false,
		},
		{
			name:     "Invalid input type",
			template: "https://api.example.com/users/{userId}",
			input:    "not a map",
			expected: "",
			hasError: true,
		},
		{
			name:     "No variables to substitute",
			template: "https://api.example.com/users/static",
			input:    map[string]any{"userId": "123"},
			expected: "https://api.example.com/users/static",
			hasError: false,
		},
		{
			name:     "Mixed types in values",
			template: "https://api.example.com/users/{userId}/score/{score}/active/{active}",
			input:    map[string]any{"userId": 123, "score": 95.5, "active": true},
			expected: "https://api.example.com/users/123/score/95.5/active/true",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandURITemplate(tt.template, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
