package common

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryWithParams_FullSQLOutput(t *testing.T) {
	// Disable logrus output during tests to keep test output clean
	logrus.SetLevel(logrus.FatalLevel)
	defer logrus.SetLevel(logrus.InfoLevel)

	tests := []struct {
		name        string
		query       string
		args        []any
		wantErr     bool
		expectedSQL string // Expected full SQL output (can be partial for complex cases)
	}{
		{
			name:        "simple select without parameters",
			query:       "SELECT * FROM users",
			args:        []any{},
			wantErr:     false,
			expectedSQL: "SELECT * FROM users",
		},
		{
			name:        "select with single integer parameter",
			query:       "SELECT * FROM users WHERE id = ?",
			args:        []any{1},
			wantErr:     false,
			expectedSQL: "SELECT * FROM users WHERE id = 1",
		},
		{
			name:        "select with string parameter",
			query:       "SELECT * FROM users WHERE name = ?",
			args:        []any{"john"},
			wantErr:     false,
			expectedSQL: `SELECT * FROM users WHERE name = "john"`,
		},
		{
			name:        "select with multiple parameters",
			query:       "SELECT * FROM users WHERE id = ? AND name = ?",
			args:        []any{1, "john"},
			wantErr:     false,
			expectedSQL: `SELECT * FROM users WHERE id = 1 AND name = "john"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := QueryWithParams(tt.query, tt.args...)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Empty(t, result)
				return
			}

			require.NoError(t, err)

			// Print the actual SQL for debugging/inspection
			t.Logf("Input query: %s", tt.query)
			t.Logf("Input args: %v", tt.args)
			t.Logf("Generated SQL: %s", result)
			t.Logf("Expected SQL: %s", tt.expectedSQL)

			// Note: GORM might not substitute parameters exactly as expected
			// This test helps you see the actual output format
			assert.NotEmpty(t, result, "Generated SQL should not be empty")

			assert.Equal(t, tt.expectedSQL, result, "Generated SQL does not match expected SQL")

		})
	}
}
