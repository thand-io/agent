package thand

import (
	"sort"

	"github.com/thand-io/agent/internal/models"
)

// sortedStrings returns a sorted (A-Z) copy of the given string slice without
// modifying the original.
func sortedStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	sort.Strings(out)
	return out
}

// sortedStatementsWithSortedFields returns a new slice of Statements derived
// from stmts where:
//  1. Operations and Targets within each Statement are sorted A-Z.
//  2. The statements themselves are sorted A-Z by their first (sorted) operation.
//
// The original slice and its elements are not modified.
func sortedStatementsWithSortedFields(stmts models.RoleStatements) []models.Statement {
	if len(stmts) == 0 {
		return []models.Statement{}
	}
	out := make([]models.Statement, len(stmts))
	for i, stmt := range stmts {
		out[i] = models.Statement{
			Operations: sortedStrings(stmt.Operations),
			Targets:    sortedStrings(stmt.Targets),
			Conditions: stmt.Conditions,
		}
	}
	sort.Slice(out, func(i, j int) bool {
		first := ""
		if len(out[i].Operations) > 0 {
			first = out[i].Operations[0]
		}
		second := ""
		if len(out[j].Operations) > 0 {
			second = out[j].Operations[0]
		}
		return first < second
	})
	return out
}
