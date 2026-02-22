package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	// EnvSocketPath overrides the helper IPC socket path.
	EnvSocketPath = "THAND_ELEVATE_SOCKET_PATH"
	// DefaultSocketPath is the default helper IPC socket path.
	DefaultSocketPath = "/var/run/thand/elevate.sock"
	// EnvSudoersDir overrides the sudoers include directory for grant files.
	EnvSudoersDir = "THAND_ELEVATE_SUDOERS_DIR"
	// DefaultSudoersDir is the default sudoers include directory for grant files.
	DefaultSudoersDir = "/etc/sudoers.d"
	// EnvSudoersFile overrides the base sudoers file used for #includedir checks.
	EnvSudoersFile = "THAND_ELEVATE_SUDOERS_FILE"
	// DefaultSudoersFile is the default base sudoers file used for #includedir checks.
	DefaultSudoersFile = "/etc/sudoers"
	// EnvVisudoBin overrides the visudo binary path/name.
	EnvVisudoBin = "THAND_ELEVATE_VISUDO_BIN"
	// DefaultVisudoBin is the default visudo binary name.
	DefaultVisudoBin = "visudo"
	// EnvStatePath overrides the persisted grant state file path.
	EnvStatePath = "THAND_ELEVATE_STATE_PATH"
	// DefaultStatePath is the default persisted grant state file path.
	DefaultStatePath = "/var/lib/thand/elevate/state.json"
	// EnvCleanupInterval overrides the periodic cleanup interval duration.
	EnvCleanupInterval = "THAND_ELEVATE_CLEANUP_INTERVAL"
	// DefaultCleanup is the default periodic cleanup interval.
	DefaultCleanup = 1 * time.Minute
	// EnvLogLevel overrides helper log level.
	EnvLogLevel = "THAND_ELEVATE_LOG_LEVEL"
	// DefaultLogLevel is the default helper log level.
	DefaultLogLevel = "info"
)

// Config contains runtime configuration for the elevate helper.
type Config struct {
	SocketPath  string
	SudoersDir  string
	SudoersFile string
	VisudoBin   string
	StatePath   string

	CleanupInterval time.Duration
	LogLevel        string
}

// LoadFromEnv loads helper configuration from environment variables with defaults.
func LoadFromEnv() (*Config, error) {
	socketPath := strings.TrimSpace(os.Getenv(EnvSocketPath))
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	sudoersDir := strings.TrimSpace(os.Getenv(EnvSudoersDir))
	if sudoersDir == "" {
		sudoersDir = DefaultSudoersDir
	}

	sudoersFile := strings.TrimSpace(os.Getenv(EnvSudoersFile))
	if sudoersFile == "" {
		sudoersFile = DefaultSudoersFile
	}

	visudoBin := strings.TrimSpace(os.Getenv(EnvVisudoBin))
	if visudoBin == "" {
		visudoBin = DefaultVisudoBin
	}

	statePath := strings.TrimSpace(os.Getenv(EnvStatePath))
	if statePath == "" {
		statePath = DefaultStatePath
	}

	cleanupInterval := DefaultCleanup
	if configured := strings.TrimSpace(os.Getenv(EnvCleanupInterval)); configured != "" {
		parsed, err := time.ParseDuration(configured)
		if err != nil {
			return nil, fmt.Errorf("parse cleanup interval: %w", err)
		}
		cleanupInterval = parsed
	}
	logLevel := strings.TrimSpace(os.Getenv(EnvLogLevel))
	if logLevel == "" {
		logLevel = DefaultLogLevel
	}

	cfg := &Config{
		SocketPath:  socketPath,
		SudoersDir:  sudoersDir,
		SudoersFile: sudoersFile,
		VisudoBin:   visudoBin,
		StatePath:   statePath,

		CleanupInterval: cleanupInterval,
		LogLevel:        logLevel,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate ensures required configuration fields are present and safe.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}

	if strings.TrimSpace(c.SocketPath) == "" {
		return fmt.Errorf("socket path is required")
	}
	if strings.TrimSpace(c.SudoersDir) == "" {
		return fmt.Errorf("sudoers dir is required")
	}
	if strings.TrimSpace(c.SudoersFile) == "" {
		return fmt.Errorf("sudoers file is required")
	}
	if strings.TrimSpace(c.VisudoBin) == "" {
		return fmt.Errorf("visudo binary is required")
	}
	if strings.TrimSpace(c.StatePath) == "" {
		return fmt.Errorf("state path is required")
	}
	if c.CleanupInterval <= 0 {
		return fmt.Errorf("cleanup interval must be > 0")
	}
	if _, err := ParseLogLevel(c.LogLevel); err != nil {
		return err
	}

	return nil
}

// ParseLogLevel converts textual configuration into an slog level.
func ParseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level: %q", raw)
	}
}
