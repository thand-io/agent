package config

import "testing"

func TestLoadFromEnvUsesDefaultSocketPath(t *testing.T) {
	t.Setenv(EnvSocketPath, "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != DefaultSocketPath {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, DefaultSocketPath)
	}
}

func TestLoadFromEnvUsesConfiguredSocketPath(t *testing.T) {
	want := "/tmp/custom-elevate.sock"
	t.Setenv(EnvSocketPath, want)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != want {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, want)
	}
}

func TestValidateRejectsEmptySocketPath(t *testing.T) {
	cfg := &Config{SocketPath: "   "}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty socket path")
	}
}
