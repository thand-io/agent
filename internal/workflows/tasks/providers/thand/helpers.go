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
//     1a. Binding is preserved as-is.
//  2. The statements themselves are sorted A-Z with a fully deterministic
//     comparator: Operations are compared element-by-element (shorter slice
//     sorts first on a tie), then Targets are used as a further tie-breaker
//     using the same element-by-element rule. sort.SliceStable preserves
//     the original relative order when all fields are identical.
//
// The original slice and its elements are not modified.
func sortedStatementsWithSortedFields(stmts models.RoleStatements) models.RoleStatements {
	if len(stmts) == 0 {
		return models.RoleStatements{}
	}
	out := make(models.RoleStatements, len(stmts))
	for i, stmt := range stmts {
		out[i] = models.Statement{
			ID:         stmt.ID,
			Operations: sortedStrings(stmt.Operations),
			Targets:    sortedStrings(stmt.Targets),
			Conditions: stmt.Conditions,
			Binding:    stmt.Binding,
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := out[i], out[j]
		// Primary sort: compare Operations element-by-element
		for k := 0; k < len(si.Operations) && k < len(sj.Operations); k++ {
			if si.Operations[k] != sj.Operations[k] {
				return si.Operations[k] < sj.Operations[k]
			}
		}
		if len(si.Operations) != len(sj.Operations) {
			return len(si.Operations) < len(sj.Operations)
		}
		// Tie-break: compare Targets element-by-element
		for k := 0; k < len(si.Targets) && k < len(sj.Targets); k++ {
			if si.Targets[k] != sj.Targets[k] {
				return si.Targets[k] < sj.Targets[k]
			}
		}
		return len(si.Targets) < len(sj.Targets)
	})
	return out
}
