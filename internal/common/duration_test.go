package common

import (
	"testing"
	"time"
)

func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Valid Go duration strings (>= 1 minute)
		{
			name:    "simple seconds - should fail",
			input:   "30s",
			wantErr: true,
		},
		{
			name:     "minutes",
			input:    "5m",
			expected: 5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "hours",
			input:    "2h",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "combined duration",
			input:    "1h30m45s",
			expected: 1*time.Hour + 30*time.Minute + 45*time.Second,
			wantErr:  false,
		},
		{
			name:    "milliseconds - should fail",
			input:   "500ms",
			wantErr: true,
		},
		{
			name:    "microseconds - should fail",
			input:   "100µs",
			wantErr: true,
		},
		{
			name:    "nanoseconds - should fail",
			input:   "1000ns",
			wantErr: true,
		},
		{
			name:    "zero duration - should fail",
			input:   "0s",
			wantErr: true,
		},
		{
			name:    "zero duration without unit - should fail",
			input:   "0",
			wantErr: true,
		},
		{
			name:     "duration with whitespace",
			input:    "  10m  ",
			expected: 10 * time.Minute,
			wantErr:  false,
		},

		// Valid ISO 8601 durations (>= 1 minute)
		{
			name:    "ISO 8601 seconds - should fail",
			input:   "PT30S",
			wantErr: true,
		},
		{
			name:     "ISO 8601 minutes",
			input:    "PT5M",
			expected: 5 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "ISO 8601 hours",
			input:    "PT2H",
			expected: 2 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "ISO 8601 combined time",
			input:    "PT1H30M45S",
			expected: 1*time.Hour + 30*time.Minute + 45*time.Second,
			wantErr:  false,
		},
		{
			name:     "ISO 8601 days",
			input:    "P1D",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "ISO 8601 weeks",
			input:    "P1W",
			expected: 7 * 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "ISO 8601 months (approximate)",
			input:    "P1M",
			expected: 744 * time.Hour, // Actual month duration from ISO8601 library (31 days)
			wantErr:  false,
		},
		{
			name:     "ISO 8601 years (approximate)",
			input:    "P1Y",
			expected: 8784 * time.Hour, // Actual year duration from ISO8601 library (366 days)
			wantErr:  false,
		},
		{
			name:     "ISO 8601 mixed date and time",
			input:    "P1DT12H30M",
			expected: 24*time.Hour + 12*time.Hour + 30*time.Minute,
			wantErr:  false,
		},
		{
			name:     "ISO 8601 with whitespace",
			input:    "  PT1H  ",
			expected: 1 * time.Hour,
			wantErr:  false,
		},

		// Invalid durations
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only whitespace",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "number without unit",
			input:   "123",
			wantErr: true,
		},
		{
			name:    "invalid unit",
			input:   "10x",
			wantErr: true,
		},
		{
			name:    "malformed ISO 8601",
			input:   "P1",
			wantErr: true,
		},
		{
			name:    "malformed ISO 8601 time - should fail",
			input:   "PT",
			wantErr: true, // PT is 0 duration, which is less than 1 minute
		},
		{
			name:    "negative duration - should fail",
			input:   "-5m",
			wantErr: true, // Negative durations are less than 1 minute
		},
		{
			name:    "fractional seconds in Go format - should fail",
			input:   "1.5s",
			wantErr: true, // 1.5 seconds is less than 1 minute
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateDuration(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateDuration(%q) expected error but got none", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ValidateDuration(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("ValidateDuration(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateDurationErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "empty string error message",
			input:   "",
			wantErr: "invalid duration format: . Expect ISO 8601 or duration string",
		},
		{
			name:    "invalid format error message",
			input:   "invalid",
			wantErr: "invalid duration format: invalid. Expect ISO 8601 or duration string",
		},
		{
			name:    "number without unit error message",
			input:   "123",
			wantErr: "invalid duration format: 123. Expect ISO 8601 or duration string",
		},
		{
			name:    "duration too short error message",
			input:   "30s",
			wantErr: "duration must be at least 1 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateDuration(tt.input)

			if err == nil {
				t.Errorf("ValidateDuration(%q) expected error but got none", tt.input)
				return
			}

			if err.Error() != tt.wantErr {
				t.Errorf("ValidateDuration(%q) error = %q, expected %q", tt.input, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateDurationWhitespaceHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "leading whitespace",
			input:    "  5m",
			expected: 5 * time.Minute,
		},
		{
			name:     "trailing whitespace",
			input:    "5m  ",
			expected: 5 * time.Minute,
		},
		{
			name:     "both leading and trailing whitespace",
			input:    "  5m  ",
			expected: 5 * time.Minute,
		},
		{
			name:     "tabs and spaces",
			input:    "\t 5m \t",
			expected: 5 * time.Minute,
		},
		{
			name:     "ISO 8601 with whitespace",
			input:    "  PT5M  ",
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateDuration(tt.input)

			if err != nil {
				t.Errorf("ValidateDuration(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result != tt.expected {
				t.Errorf("ValidateDuration(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkValidateDurationGoFormat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ValidateDuration("1h30m45s")
	}
}

func BenchmarkValidateDurationISO8601(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ValidateDuration("PT1H30M45S")
	}
}

func BenchmarkValidateDurationInvalid(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ValidateDuration("invalid")
	}
}
