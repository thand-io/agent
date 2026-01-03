package common

import "testing"

func TestExtractNameFromEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "simple email with dots",
			email:    "john.doe@example.com",
			expected: "John Doe",
		},
		{
			name:     "email with underscores",
			email:    "jane_smith@example.com",
			expected: "Jane Smith",
		},
		{
			name:     "email with hyphens",
			email:    "bob-jones@example.com",
			expected: "Bob Jones",
		},
		{
			name:     "email with mixed separators",
			email:    "mary.anne_wilson@example.com",
			expected: "Mary Anne Wilson",
		},
		{
			name:     "single word email",
			email:    "admin@example.com",
			expected: "Admin",
		},
		{
			name:     "email with numbers",
			email:    "user123@example.com",
			expected: "User123",
		},
		{
			name:     "email with multiple dots",
			email:    "first.middle.last@example.com",
			expected: "First Middle Last",
		},
		{
			name:     "email with uppercase letters",
			email:    "JOHN.DOE@example.com",
			expected: "John Doe",
		},
		{
			name:     "empty string",
			email:    "",
			expected: "",
		},
		{
			name:     "email without domain",
			email:    "john.doe",
			expected: "John Doe",
		},
		{
			name:     "complex mixed format",
			email:    "john_doe-smith.jr@example.com",
			expected: "John Doe Smith Jr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractNameFromEmail(tt.email)
			if result != tt.expected {
				t.Errorf("ExtractNameFromEmail(%q) = %q, want %q", tt.email, result, tt.expected)
			}
		})
	}
}

func TestExtractUsernameFromEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "simple email with dots",
			email:    "john.doe@example.com",
			expected: "john_doe",
		},
		{
			name:     "email with underscores",
			email:    "jane_smith@example.com",
			expected: "jane_smith",
		},
		{
			name:     "email with hyphens",
			email:    "bob-jones@example.com",
			expected: "bob_jones",
		},
		{
			name:     "email with mixed separators",
			email:    "mary.anne_wilson@example.com",
			expected: "mary_anne_wilson",
		},
		{
			name:     "single word email",
			email:    "admin@example.com",
			expected: "admin",
		},
		{
			name:     "email with numbers",
			email:    "user123@example.com",
			expected: "user123",
		},
		{
			name:     "email with multiple dots",
			email:    "first.middle.last@example.com",
			expected: "first_middle_last",
		},
		{
			name:     "email with uppercase letters",
			email:    "JOHN.DOE@example.com",
			expected: "john_doe",
		},
		{
			name:     "empty string",
			email:    "",
			expected: "",
		},
		{
			name:     "email without domain",
			email:    "john.doe",
			expected: "john_doe",
		},
		{
			name:     "complex mixed format",
			email:    "john_doe-smith.jr@example.com",
			expected: "john_doe_smith_jr",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractUsernameFromEmail(tt.email)
			if result != tt.expected {
				t.Errorf("ExtractUsernameFromEmail(%q) = %q, want %q", tt.email, result, tt.expected)
			}
		})
	}
}
