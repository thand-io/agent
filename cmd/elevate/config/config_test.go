package config

import (
	"testing"
	"time"
)

func TestLoadFromEnvUsesDefaultSocketPath(t *testing.T) {
	t.Setenv(EnvSocketPath, "")
	t.Setenv(EnvSudoersDir, "")
	t.Setenv(EnvSudoersFile, "")
	t.Setenv(EnvVisudoBin, "")
	t.Setenv(EnvStatePath, "")
	t.Setenv(EnvCleanupInterval, "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != DefaultSocketPath {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, DefaultSocketPath)
	}
	if cfg.SudoersDir != DefaultSudoersDir {
		t.Fatalf("unexpected sudoers dir: got %q want %q", cfg.SudoersDir, DefaultSudoersDir)
	}
	if cfg.SudoersFile != DefaultSudoersFile {
		t.Fatalf("unexpected sudoers file: got %q want %q", cfg.SudoersFile, DefaultSudoersFile)
	}
	if cfg.VisudoBin != DefaultVisudoBin {
		t.Fatalf("unexpected visudo bin: got %q want %q", cfg.VisudoBin, DefaultVisudoBin)
	}
	if cfg.StatePath != DefaultStatePath {
		t.Fatalf("unexpected state path: got %q want %q", cfg.StatePath, DefaultStatePath)
	}
	if cfg.CleanupInterval != DefaultCleanup {
		t.Fatalf("unexpected cleanup interval: got %s want %s", cfg.CleanupInterval, DefaultCleanup)
	}
}

func TestLoadFromEnvUsesConfiguredSocketPath(t *testing.T) {
	wantSocket := "/tmp/custom-elevate.sock"
	wantSudoers := "/tmp/custom-sudoers.d"
	wantSudoersFile := "/tmp/custom-sudoers"
	wantVisudo := "/usr/local/bin/visudo"
	wantStatePath := "/tmp/custom-state.json"
	wantCleanup := "30s"
	t.Setenv(EnvSocketPath, wantSocket)
	t.Setenv(EnvSudoersDir, wantSudoers)
	t.Setenv(EnvSudoersFile, wantSudoersFile)
	t.Setenv(EnvVisudoBin, wantVisudo)
	t.Setenv(EnvStatePath, wantStatePath)
	t.Setenv(EnvCleanupInterval, wantCleanup)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != wantSocket {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, wantSocket)
	}
	if cfg.SudoersDir != wantSudoers {
		t.Fatalf("unexpected sudoers dir: got %q want %q", cfg.SudoersDir, wantSudoers)
	}
	if cfg.SudoersFile != wantSudoersFile {
		t.Fatalf("unexpected sudoers file: got %q want %q", cfg.SudoersFile, wantSudoersFile)
	}
	if cfg.VisudoBin != wantVisudo {
		t.Fatalf("unexpected visudo bin: got %q want %q", cfg.VisudoBin, wantVisudo)
	}
	if cfg.StatePath != wantStatePath {
		t.Fatalf("unexpected state path: got %q want %q", cfg.StatePath, wantStatePath)
	}
	if cfg.CleanupInterval != 30*time.Second {
		t.Fatalf("unexpected cleanup interval: got %s want %s", cfg.CleanupInterval, 30*time.Second)
	}
}

func TestLoadFromEnvRejectsInvalidCleanupInterval(t *testing.T) {
	t.Setenv(EnvCleanupInterval, "not-a-duration")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected cleanup interval parse error")
	}
}

func TestValidateRejectsEmptySocketPath(t *testing.T) {
	cfg := &Config{
		SocketPath:      "   ",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty socket path")
	}
}

func TestValidateRejectsEmptySudoersDir(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "   ",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty sudoers dir")
	}
}

func TestValidateRejectsEmptySudoersFile(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "   ",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty sudoers file")
	}
}

func TestValidateRejectsEmptyVisudoBin(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "   ",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty visudo binary")
	}
}

func TestValidateRejectsEmptyStatePath(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "   ",
		CleanupInterval: time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for empty state path")
	}
}

func TestValidateRejectsNonPositiveCleanupInterval(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: 0,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for cleanup interval")
	}
}
