package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
)

func TestLoad_PrefersExplicitConfigFileOverEnvOverride(t *testing.T) {
	envConfigPath := writeTestConfigFile(t, filepath.Join(t.TempDir(), "env.yaml"), "env-secret")
	explicitConfigPath := writeTestConfigFile(t, filepath.Join(t.TempDir(), "explicit.yaml"), "explicit-secret")

	t.Setenv(configPathEnvVar, envConfigPath)

	cfg, err := Load(explicitConfigPath)
	require.NoError(t, err)

	assert.Equal(t, "explicit-secret", cfg.Secret)
}

func TestLoad_UsesConfigPathEnvOverrideWhenFlagNotProvided(t *testing.T) {
	configPath := writeTestConfigFile(t, filepath.Join(t.TempDir(), "env.yaml"), "env-secret")

	t.Setenv(configPathEnvVar, configPath)

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, "env-secret", cfg.Secret)
}

func TestLoad_FindsDefaultConfigInHomeConfigDir(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv(configPathEnvVar, "")

	configPath := filepath.Join(homeDir, ".config", "thand", "config.yaml")
	writeTestConfigFile(t, configPath, "home-secret")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, "home-secret", cfg.Secret)
}

func TestLoad_DoesNotUseLegacyThandConfigPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv(configPathEnvVar, "")

	legacyConfigPath := filepath.Join(homeDir, ".thand", "config.yaml")
	writeTestConfigFile(t, legacyConfigPath, "legacy-secret")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, common.DefaultServerSecret, cfg.Secret)
}

func writeTestConfigFile(t *testing.T, path string, secret string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("secret: "+secret+"\n"), 0o644))

	return path
}
