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

func TestIsValidNumber(t *testing.T) {
	tests := []struct {
		value          string
		decimalAllowed bool
		expected       bool
	}{
		// Valid integers
		{"123", true, true},
		{"123", false, true},
		{"0", true, true},
		{"0", false, true},
		{"-123", true, true},
		{"-123", false, true},

		// Valid decimals
		{"123.45", true, true},
		{"0.5", true, true},
		{"-123.45", true, true},
		{"3.14159", true, true},

		// Decimals when not allowed
		{"123.45", false, false},
		{"0.5", false, false},
		{"-123.45", false, false},

		// Invalid values
		{"", true, false},
		{"abc", true, false},
		{"12.34.56", true, false},
		{"12a34", true, false},
		{" 123", true, false},
		{"123 ", true, false},
	}

	for _, test := range tests {
		result := IsValidNumber(test.value, test.decimalAllowed)
		if result != test.expected {
			t.Errorf("IsValidNumber(%q, %v) = %v, expected %v", test.value, test.decimalAllowed, result, test.expected)
		}
	}
}

func TestValidateNumberRange(t *testing.T) {
	tests := []struct {
		value    string
		minValue string
		maxValue string
		wantErr  bool
		errMsg   string
	}{
		// Valid within range
		{"50", "0", "100", false, ""},
		{"0", "0", "100", false, ""},
		{"100", "0", "100", false, ""},
		{"50.5", "0", "100", false, ""},

		// No constraints
		{"50", "", "", false, ""},
		{"-100", "", "", false, ""},
		{"999999", "", "", false, ""},

		// Only min constraint
		{"50", "10", "", false, ""},
		{"10", "10", "", false, ""},
		{"5", "10", "", true, "Value must be at least 10"},
		{"-5", "0", "", true, "Value must be at least 0"},

		// Only max constraint
		{"50", "", "100", false, ""},
		{"100", "", "100", false, ""},
		{"150", "", "100", true, "Value must be at most 100"},

		// Both constraints - below min
		{"5", "10", "100", true, "Value must be at least 10"},
		{"-1", "0", "100", true, "Value must be at least 0"},

		// Both constraints - above max
		{"150", "10", "100", true, "Value must be at most 100"},
		{"101", "0", "100", true, "Value must be at most 100"},

		// Decimal values
		{"10.5", "10", "11", false, ""},
		{"9.9", "10", "11", true, "Value must be at least 10"},
		{"11.1", "10", "11", true, "Value must be at most 11"},

		// Negative ranges
		{"-50", "-100", "-10", false, ""},
		{"-5", "-100", "-10", true, "Value must be at most -10"},
		{"-150", "-100", "-10", true, "Value must be at least -100"},

		// Invalid number
		{"abc", "0", "100", true, "Please enter a valid number"},
		{"", "0", "100", true, "Please enter a valid number"},

		// Invalid min/max values are ignored
		{"50", "abc", "100", false, ""},
		{"50", "0", "abc", false, ""},
		{"50", "abc", "abc", false, ""},
	}

	for _, test := range tests {
		result := ValidateNumberRange(test.value, test.minValue, test.maxValue)
		hasErr := len(result) != 0

		if hasErr != test.wantErr {
			t.Errorf("ValidateNumberRange(%q, %q, %q) error = %v, wantErr %v", test.value, test.minValue, test.maxValue, result, test.wantErr)
		}

		if test.wantErr && result != test.errMsg {
			t.Errorf("ValidateNumberRange(%q, %q, %q) = %q, expected %q", test.value, test.minValue, test.maxValue, result, test.errMsg)
		}
	}
}

func TestGetValidator(t *testing.T) {
	// Test that GetValidator returns a non-nil validator
	v1 := GetValidator()
	if v1 == nil {
		t.Fatal("GetValidator() returned nil")
	}

	// Test singleton pattern - should return the same instance
	v2 := GetValidator()
	if v1 != v2 {
		t.Error("GetValidator() did not return the same instance (singleton pattern violated)")
	}

	// Test that custom validators are registered
	type TestStruct struct {
		SemverField      string `validate:"semver_pattern"`
		AlphanumHyphen   string `validate:"alphanum_hyphen"`
	}

	tests := []struct {
		name    string
		input   TestStruct
		wantErr bool
	}{
		{
			name: "valid semver",
			input: TestStruct{
				SemverField:    "1.0.0",
				AlphanumHyphen: "valid-name_123",
			},
			wantErr: false,
		},
		{
			name: "invalid semver",
			input: TestStruct{
				SemverField:    "1.0",
				AlphanumHyphen: "valid-name",
			},
			wantErr: true,
		},
		{
			name: "valid alphanum_hyphen",
			input: TestStruct{
				SemverField:    "1.0.0",
				AlphanumHyphen: "aws-provider_v2.0",
			},
			wantErr: false,
		},
		{
			name: "invalid alphanum_hyphen - starts with hyphen",
			input: TestStruct{
				SemverField:    "1.0.0",
				AlphanumHyphen: "-invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid alphanum_hyphen - special chars",
			input: TestStruct{
				SemverField:    "1.0.0",
				AlphanumHyphen: "invalid@name",
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := v1.Struct(test.input)
			if (err != nil) != test.wantErr {
				t.Errorf("Validate(%+v) error = %v, wantErr %v", test.input, err, test.wantErr)
			}
		})
	}
}
