package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvSocketPath     = "THAND_ELEVATE_SOCKET_PATH"
	DefaultSocketPath = "/var/run/thand/elevate.sock"
)

type Config struct {
	SocketPath string
}

func LoadFromEnv() (*Config, error) {
	socketPath := strings.TrimSpace(os.Getenv(EnvSocketPath))
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	cfg := &Config{
		SocketPath: socketPath,
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}

	if strings.TrimSpace(c.SocketPath) == "" {
		return fmt.Errorf("socket path is required")
	}

	return nil
}
