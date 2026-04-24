package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDecodesDeviceLocalElevationFields(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	configBody := []byte(`
version: "1.0"
environment:
  name: test
  platform: local
devices:
  device-alpha:
    device_id: "device-alpha"
    name: "Example Workstation"
    enabled: true
    local_elevation:
      enabled: true
      allowed_modes:
        - timed
        - command
      accounts:
        - email: user@example.com
          local_username: exampleuser
`)
	if err := os.WriteFile(configPath, configBody, 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	device, err := cfg.GetDevice("device-alpha")
	if err != nil {
		t.Fatalf("GetDevice returned error: %v", err)
	}

	if got, want := device.ID, "device-alpha"; got != want {
		t.Fatalf("device id = %q, want %q", got, want)
	}
	if device.LocalElevation == nil {
		t.Fatal("expected local_elevation to be decoded")
	}
	if got, want := len(device.LocalElevation.Accounts), 1; got != want {
		t.Fatalf("accounts len = %d, want %d", got, want)
	}
	if got, want := device.LocalElevation.Accounts[0].LocalUsername, "exampleuser"; got != want {
		t.Fatalf("local username = %q, want %q", got, want)
	}
}
