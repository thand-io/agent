package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	EnvSocketPath      = "THAND_ELEVATE_SOCKET_PATH"
	DefaultSocketPath  = "/var/run/thand/elevate.sock"
	EnvSudoersDir      = "THAND_ELEVATE_SUDOERS_DIR"
	DefaultSudoersDir  = "/etc/sudoers.d"
	EnvSudoersFile     = "THAND_ELEVATE_SUDOERS_FILE"
	DefaultSudoersFile = "/etc/sudoers"
	EnvVisudoBin       = "THAND_ELEVATE_VISUDO_BIN"
	DefaultVisudoBin   = "visudo"
)

type Config struct {
	SocketPath  string
	SudoersDir  string
	SudoersFile string
	VisudoBin   string
}

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

	cfg := &Config{
		SocketPath:  socketPath,
		SudoersDir:  sudoersDir,
		SudoersFile: sudoersFile,
		VisudoBin:   visudoBin,
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
	if strings.TrimSpace(c.SudoersDir) == "" {
		return fmt.Errorf("sudoers dir is required")
	}
	if strings.TrimSpace(c.SudoersFile) == "" {
		return fmt.Errorf("sudoers file is required")
	}
	if strings.TrimSpace(c.VisudoBin) == "" {
		return fmt.Errorf("visudo binary is required")
	}

	return nil
}
