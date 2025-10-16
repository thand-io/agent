package common

import (
	"testing"
)

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123456789012", true}, // Valid AWS account ID
		{"000000000000", true}, // Valid with leading zeros
		{"12345", true},        // Short number
		{"", false},            // Empty string
		{"12345a", false},      // Contains letter
		{"12345 ", false},      // Contains space
		{"12345-6789", false},  // Contains dash
		{"a123456789", false},  // Starts with letter
		{"123456789a", false},  // Ends with letter
		{"1234567890", true},   // Valid 10 digits
	}

	for _, test := range tests {
		result := IsAllDigits(test.input)
		if result != test.expected {
			t.Errorf("IsAllDigits(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

func BenchmarkIsAllDigits(b *testing.B) {
	testString := "123456789012" // Typical AWS account ID

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IsAllDigits(testString)
	}
}
