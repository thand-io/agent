package thand

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

// TestSortedStrings tests the sortedStrings helper function.
func TestSortedStrings(t *testing.T) {
	t.Run("nil slice returns nil", func(t *testing.T) {
		result := sortedStrings(nil)
		assert.Nil(t, result)
	})

	t.Run("empty slice returns empty", func(t *testing.T) {
		result := sortedStrings([]string{})
		assert.Empty(t, result)
	})

	t.Run("single element returned unchanged", func(t *testing.T) {
		result := sortedStrings([]string{"a"})
		assert.Equal(t, []string{"a"}, result)
	})

	t.Run("unsorted slice is sorted A-Z", func(t *testing.T) {
		result := sortedStrings([]string{"ec2:TerminateInstances", "ec2:DescribeInstances", "s3:GetObject"})
		assert.Equal(t, []string{"ec2:DescribeInstances", "ec2:TerminateInstances", "s3:GetObject"}, result)
	})

	t.Run("already sorted slice is unchanged", func(t *testing.T) {
		input := []string{"a", "b", "c"}
		result := sortedStrings(input)
		assert.Equal(t, []string{"a", "b", "c"}, result)
	})

	t.Run("duplicate elements are preserved", func(t *testing.T) {
		result := sortedStrings([]string{"b", "a", "b"})
		assert.Equal(t, []string{"a", "b", "b"}, result)
	})

	t.Run("original slice is not mutated", func(t *testing.T) {
		original := []string{"z", "m", "a"}
		_ = sortedStrings(original)
		assert.Equal(t, []string{"z", "m", "a"}, original)
	})
}

// TestSortedStatementsWithSortedFields tests the sortedStatementsWithSortedFields helper function.
func TestSortedStatementsWithSortedFields(t *testing.T) {
	t.Run("nil input returns empty slice", func(t *testing.T) {
		result := sortedStatementsWithSortedFields(nil)
		assert.Empty(t, result)
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		result := sortedStatementsWithSortedFields(models.RoleStatements{})
		assert.Empty(t, result)
	})

	t.Run("single statement returned with sorted fields", func(t *testing.T) {
		stmts := models.RoleStatements{
			{
				Operations: []string{"s3:PutObject", "s3:GetObject"},
				Targets:    []string{"arn:aws:s3:::bucket-b/*", "arn:aws:s3:::bucket-a/*"},
			},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Len(t, result, 1)
		assert.Equal(t, []string{"s3:GetObject", "s3:PutObject"}, result[0].Operations)
		assert.Equal(t, []string{"arn:aws:s3:::bucket-a/*", "arn:aws:s3:::bucket-b/*"}, result[0].Targets)
	})

	t.Run("statements are sorted A-Z by first operation", func(t *testing.T) {
		stmts := models.RoleStatements{
			{Operations: []string{"s3:GetObject"}, Targets: []string{"*"}},
			{Operations: []string{"ec2:DescribeInstances"}, Targets: []string{"*"}},
			{Operations: []string{"iam:ListRoles"}, Targets: []string{"*"}},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Len(t, result, 3)
		assert.Equal(t, "ec2:DescribeInstances", result[0].Operations[0])
		assert.Equal(t, "iam:ListRoles", result[1].Operations[0])
		assert.Equal(t, "s3:GetObject", result[2].Operations[0])
	})

	t.Run("operations within each statement are sorted A-Z", func(t *testing.T) {
		stmts := models.RoleStatements{
			{
				Operations: []string{"s3:PutObject", "ec2:DescribeInstances", "iam:GetRole"},
				Targets:    []string{"*"},
			},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Equal(t, []string{"ec2:DescribeInstances", "iam:GetRole", "s3:PutObject"}, result[0].Operations)
	})

	t.Run("targets within each statement are sorted A-Z", func(t *testing.T) {
		stmts := models.RoleStatements{
			{
				Operations: []string{"s3:GetObject"},
				Targets:    []string{"arn:aws:s3:::z-bucket/*", "arn:aws:s3:::a-bucket/*", "arn:aws:s3:::m-bucket/*"},
			},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Equal(t, []string{"arn:aws:s3:::a-bucket/*", "arn:aws:s3:::m-bucket/*", "arn:aws:s3:::z-bucket/*"}, result[0].Targets)
	})

	t.Run("statement with no operations sorts before one with operations", func(t *testing.T) {
		stmts := models.RoleStatements{
			{Operations: []string{"s3:GetObject"}, Targets: []string{"*"}},
			{Operations: []string{}, Targets: []string{"*"}},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Len(t, result, 2)
		assert.Empty(t, result[0].Operations)
		assert.Equal(t, []string{"s3:GetObject"}, result[1].Operations)
	})

	t.Run("conditions are preserved on each statement", func(t *testing.T) {
		conditions := map[string]any{
			"IpAddress": map[string]any{"aws:SourceIp": "10.0.0.0/8"},
		}
		stmts := models.RoleStatements{
			{
				Operations: []string{"s3:GetObject"},
				Targets:    []string{"*"},
				Conditions: conditions,
			},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Equal(t, conditions, result[0].Conditions)
	})

	t.Run("original slice and its elements are not mutated", func(t *testing.T) {
		original := models.RoleStatements{
			{
				Operations: []string{"s3:PutObject", "ec2:DescribeInstances"},
				Targets:    []string{"arn:aws:s3:::z/*", "arn:aws:s3:::a/*"},
			},
			{
				Operations: []string{"iam:GetRole"},
				Targets:    []string{"*"},
			},
		}
		ops0Before := make([]string, len(original[0].Operations))
		copy(ops0Before, original[0].Operations)
		targets0Before := make([]string, len(original[0].Targets))
		copy(targets0Before, original[0].Targets)

		_ = sortedStatementsWithSortedFields(original)

		assert.Equal(t, ops0Before, original[0].Operations, "original operations should not be mutated")
		assert.Equal(t, targets0Before, original[0].Targets, "original targets should not be mutated")
	})

	t.Run("nil targets and conditions handled gracefully", func(t *testing.T) {
		stmts := models.RoleStatements{
			{Operations: []string{"s3:GetObject"}},
		}
		result := sortedStatementsWithSortedFields(stmts)
		assert.Len(t, result, 1)
		assert.Equal(t, []string{"s3:GetObject"}, result[0].Operations)
		assert.Empty(t, result[0].Targets)
		assert.Nil(t, result[0].Conditions)
	})

	t.Run("multiple statements with multi-operation sort interplay", func(t *testing.T) {
		stmts := models.RoleStatements{
			{Operations: []string{"s3:PutObject", "s3:GetObject"}, Targets: []string{"bucket-b", "bucket-a"}},
			{Operations: []string{"iam:PutRole", "iam:GetRole"}, Targets: []string{"role-b", "role-a"}},
			{Operations: []string{"ec2:TerminateInstances", "ec2:DescribeInstances"}, Targets: []string{"i-002", "i-001"}},
		}
		result := sortedStatementsWithSortedFields(stmts)

		// Statements sorted A-Z by first (already-sorted) operation
		assert.Equal(t, []string{"ec2:DescribeInstances", "ec2:TerminateInstances"}, result[0].Operations)
		assert.Equal(t, []string{"i-001", "i-002"}, result[0].Targets)

		assert.Equal(t, []string{"iam:GetRole", "iam:PutRole"}, result[1].Operations)
		assert.Equal(t, []string{"role-a", "role-b"}, result[1].Targets)

		assert.Equal(t, []string{"s3:GetObject", "s3:PutObject"}, result[2].Operations)
		assert.Equal(t, []string{"bucket-a", "bucket-b"}, result[2].Targets)
	})
}
