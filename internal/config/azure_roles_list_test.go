package config

import (
"testing"
"github.com/thand-io/agent/internal/data"
)

func TestListAzureRoles(t *testing.T) {
	roles, err := data.GetParsedAzureRoles()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roles {
		t.Logf("Role: %s", r.Name)
	}
}
