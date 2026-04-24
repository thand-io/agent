package models

import "testing"

func TestNormalizeLocalSudoRequestRequiresDevice(t *testing.T) {
	request := &ElevateRequest{
		Role: &Role{
			Name:       "Local Sudo",
			Identifier: LocalSudoRoleIdentifier,
			Providers:  []string{"local", "local-elevation"},
			Workflows:  []string{LocalSudoTimedWorkflowName, LocalSudoCommandWorkflowName},
		},
		Reason:   "maintenance",
		Duration: "30m",
	}

	err := NormalizeLocalSudoRequest(request, map[string]ProviderConfig{
		"local-elevation": {Provider: "local", Enabled: true},
	})
	if err == nil {
		t.Fatal("expected error when device is missing")
	}
}

func TestNormalizeLocalSudoRequestSetsDeviceMetadataAndWorkflow(t *testing.T) {
	request := &ElevateRequest{
		Role: &Role{
			Name:       "Local Sudo",
			Identifier: LocalSudoRoleIdentifier,
			Providers:  []string{"local", "local-elevation"},
			Workflows:  []string{LocalSudoTimedWorkflowName, LocalSudoCommandWorkflowName},
		},
		Device:   "device-alpha",
		Reason:   "maintenance",
		Duration: "30m",
	}

	err := NormalizeLocalSudoRequest(request, map[string]ProviderConfig{
		"local-elevation": {Provider: "local", Enabled: true},
	})
	if err != nil {
		t.Fatalf("NormalizeLocalSudoRequest returned error: %v", err)
	}

	if got, want := request.Workflow, LocalSudoTimedWorkflowName; got != want {
		t.Fatalf("workflow = %q, want %q", got, want)
	}
	if got, want := request.Metadata["device_id"], "device-alpha"; got != want {
		t.Fatalf("device_id = %#v, want %q", got, want)
	}
	if got, want := request.Providers[0], "local-elevation"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
}

func TestDeviceLocalElevationPolicyResolveLocalUsername(t *testing.T) {
	policy := &DeviceLocalElevationPolicy{
		Enabled: true,
		Accounts: []DeviceLocalElevationAccount{
			{Identity: "identity-1", LocalUsername: "alpha"},
			{Email: "user@example.com", LocalUsername: "beta"},
			{Username: "exampleuser", LocalUsername: "gamma"},
		},
	}

	username, err := policy.ResolveLocalUsername("identity-1", &Identity{
		ID: "identity-1",
		User: &User{
			Email:    "user@example.com",
			Username: "exampleuser",
		},
	})
	if err != nil {
		t.Fatalf("ResolveLocalUsername returned error: %v", err)
	}
	if got, want := username, "alpha"; got != want {
		t.Fatalf("local username = %q, want %q", got, want)
	}
}
