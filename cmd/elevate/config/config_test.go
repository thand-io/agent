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
	t.Setenv(EnvStateRetention, "")
	t.Setenv(EnvSocketUser, "")
	t.Setenv(EnvSocketGroup, "")
	t.Setenv(EnvLogLevel, "")
	t.Setenv(EnvWindowsAdminGroup, "")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv failed: %v", err)
	}

	if cfg.SocketPath != defaultSocketPath() {
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
	if cfg.StatePath != defaultStatePath() {
		t.Fatalf("unexpected state path: got %q want %q", cfg.StatePath, defaultStatePath())
	}
	if cfg.CleanupInterval != DefaultCleanup {
		t.Fatalf("unexpected cleanup interval: got %s want %s", cfg.CleanupInterval, DefaultCleanup)
	}
	if cfg.RequestTimeout != DefaultRequestTimeout {
		t.Fatalf("unexpected request timeout: got %s want %s", cfg.RequestTimeout, DefaultRequestTimeout)
	}
	if cfg.StateRetention != DefaultStateRetention {
		t.Fatalf("unexpected state retention: got %s want %s", cfg.StateRetention, DefaultStateRetention)
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
	if cfg.WindowsAdminGroup != "" {
		t.Fatalf("unexpected windows admin group: got %q want empty", cfg.WindowsAdminGroup)
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
	wantStateRetention := "48h"
	wantSocketUser := "tom"
	wantSocketGroup := "thand"
	wantLogLevel := "warn"
	wantWindowsAdminGroup := "Administrators"
	t.Setenv(EnvSocketPath, wantSocket)
	t.Setenv(EnvSudoersDir, wantSudoers)
	t.Setenv(EnvSudoersFile, wantSudoersFile)
	t.Setenv(EnvVisudoBin, wantVisudo)
	t.Setenv(EnvStatePath, wantStatePath)
	t.Setenv(EnvCleanupInterval, wantCleanup)
	t.Setenv(EnvRequestTimeout, wantRequestTimeout)
	t.Setenv(EnvStateRetention, wantStateRetention)
	t.Setenv(EnvSocketUser, wantSocketUser)
	t.Setenv(EnvSocketGroup, wantSocketGroup)
	t.Setenv(EnvLogLevel, wantLogLevel)
	t.Setenv(EnvWindowsAdminGroup, wantWindowsAdminGroup)

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
	if cfg.StateRetention != 48*time.Hour {
		t.Fatalf("unexpected state retention: got %s want %s", cfg.StateRetention, 48*time.Hour)
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
	if cfg.WindowsAdminGroup != wantWindowsAdminGroup {
		t.Fatalf("unexpected windows admin group: got %q want %q", cfg.WindowsAdminGroup, wantWindowsAdminGroup)
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

func TestLoadFromEnvRejectsInvalidStateRetention(t *testing.T) {
	t.Setenv(EnvStateRetention, "not-a-duration")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("expected state retention parse error")
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
		StateRetention:  24 * time.Hour,
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
		StateRetention:  24 * time.Hour,
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
		StateRetention:  24 * time.Hour,
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
		StateRetention:  24 * time.Hour,
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
		StateRetention:  24 * time.Hour,
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
		StateRetention:  24 * time.Hour,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for request timeout")
	}
}

func TestValidateRejectsNonPositiveStateRetention(t *testing.T) {
	cfg := &Config{
		SocketPath:      "/var/run/thand/elevate.sock",
		SudoersDir:      "/etc/sudoers.d",
		SudoersFile:     "/etc/sudoers",
		VisudoBin:       "visudo",
		StatePath:       "/var/lib/thand/elevate/state.json",
		CleanupInterval: time.Minute,
		RequestTimeout:  time.Second,
		StateRetention:  0,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for state retention")
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
		RequestTimeout:  time.Second,
		StateRetention:  24 * time.Hour,
		LogLevel:        "verbose",
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for log level")
	}
}

func TestValidateForWindowsDoesNotRequireLinuxSudoersFields(t *testing.T) {
	cfg := &Config{
		SocketPath:      `C:\ProgramData\Thand\elevate.sock`,
		SudoersDir:      "   ",
		SudoersFile:     "   ",
		VisudoBin:       "   ",
		StatePath:       `C:\ProgramData\Thand\elevate\state.json`,
		CleanupInterval: time.Minute,
		RequestTimeout:  time.Second,
		StateRetention:  24 * time.Hour,
		LogLevel:        "info",
	}
	if err := cfg.validateForOS("windows"); err != nil {
		t.Fatalf("unexpected validation error for windows config: %v", err)
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
