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
	t.Setenv(EnvRequestTimeout, "")
	t.Setenv(EnvSocketUser, "")
	t.Setenv(EnvSocketGroup, "")
	t.Setenv(EnvLogLevel, "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != DefaultSocketPath {
		t.Fatalf("unexpected socket path: got %q want %q", cfg.SocketPath, defaultSocketPath())
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
	if cfg.RequestTimeout != DefaultRequestTimeout {
		t.Fatalf("unexpected request timeout: got %s want %s", cfg.RequestTimeout, DefaultRequestTimeout)
	}
	if cfg.SocketUser != "" {
		t.Fatalf("unexpected socket user: got %q want empty", cfg.SocketUser)
	}
	if cfg.SocketGroup != "" {
		t.Fatalf("unexpected socket group: got %q want empty", cfg.SocketGroup)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("unexpected log level: got %q want %q", cfg.LogLevel, DefaultLogLevel)
	}
}

func TestLoadFromEnvUsesConfiguredSocketPath(t *testing.T) {
	wantSocket := "/tmp/custom-elevate.sock"
	wantSudoers := "/tmp/custom-sudoers.d"
	wantSudoersFile := "/tmp/custom-sudoers"
	wantVisudo := "/usr/local/bin/visudo"
	wantStatePath := "/tmp/custom-state.json"
	wantCleanup := "30s"
	wantRequestTimeout := "5m"
	wantSocketUser := "tom"
	wantSocketGroup := "thand"
	wantLogLevel := "warn"
	t.Setenv(EnvSocketPath, wantSocket)
	t.Setenv(EnvSudoersDir, wantSudoers)
	t.Setenv(EnvSudoersFile, wantSudoersFile)
	t.Setenv(EnvVisudoBin, wantVisudo)
	t.Setenv(EnvStatePath, wantStatePath)
	t.Setenv(EnvCleanupInterval, wantCleanup)
	t.Setenv(EnvRequestTimeout, wantRequestTimeout)
	t.Setenv(EnvSocketUser, wantSocketUser)
	t.Setenv(EnvSocketGroup, wantSocketGroup)
	t.Setenv(EnvLogLevel, wantLogLevel)

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
	if cfg.RequestTimeout != 5*time.Minute {
		t.Fatalf("unexpected request timeout: got %s want %s", cfg.RequestTimeout, 5*time.Minute)
	}
	if cfg.SocketUser != wantSocketUser {
		t.Fatalf("unexpected socket user: got %q want %q", cfg.SocketUser, wantSocketUser)
	}
	if cfg.SocketGroup != wantSocketGroup {
		t.Fatalf("unexpected socket group: got %q want %q", cfg.SocketGroup, wantSocketGroup)
	}
	if cfg.LogLevel != wantLogLevel {
		t.Fatalf("unexpected log level: got %q want %q", cfg.LogLevel, wantLogLevel)
	}
}

func TestLoadFromEnvRejectsInvalidCleanupInterval(t *testing.T) {
	t.Setenv(EnvCleanupInterval, "not-a-duration")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected cleanup interval parse error")
	}
}

func TestLoadFromEnvRejectsInvalidRequestTimeout(t *testing.T) {
	t.Setenv(EnvRequestTimeout, "not-a-duration")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected request timeout parse error")
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

func TestValidateRejectsNonPositiveRequestTimeout(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
		RequestTimeout:  0,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for request timeout")
	}
}

func TestValidateRejectsInvalidLogLevel(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
		LogLevel:        "verbose",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for log level")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{in: "debug"},
		{in: "info"},
		{in: "warn"},
		{in: "warning"},
		{in: "error"},
		{in: ""},
		{in: "bad", wantErr: true},
	}
	for _, tc := range tests {
		_, err := ParseLogLevel(tc.in)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for %q", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.in, err)
		}
	}
}
