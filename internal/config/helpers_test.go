package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/aws"
	"github.com/thand-io/agent/internal/providers/azure"
	"github.com/thand-io/agent/internal/providers/email"
	"github.com/thand-io/agent/internal/providers/gcp"
	"github.com/thand-io/agent/internal/providers/kubernetes"
)

// newMockProvider creates the correct mock implementation for the given provider type.
func newMockProvider(providerType string) (models.Provider, error) {
	switch providerType {
	case aws.AwsProviderName:
		return aws.NewMockAwsProvider(), nil
	case gcp.GcpProviderName:
		return gcp.NewMockGcpProvider(), nil
	case azure.AzureProviderName:
		return azure.NewMockAzureProvider(), nil
	case kubernetes.KubernetesProviderName:
		return kubernetes.NewMockKubernetesProvider(), nil
	case email.EmailProviderName:
		return email.NewMockEmailProvider(), nil
	default:
		return nil, fmt.Errorf("no mock available for provider type %q", providerType)
	}
}

// newTestConfig creates a Config with mock providers initialized.
// Each provider is instantiated directly using its typed mock constructor and
// Initialize is called on it, which triggers Synchronize / PreSynchronizeActivities
// so that managed policies and roles are loaded from embedded data.
func newTestConfig(t *testing.T, roles map[string]models.Role, providerDefs map[string]models.ProviderConfig) *Config {
	t.Helper()

	config := &Config{
		mode: ModeServer,
		Roles: RoleConfig{
			Definitions: roles,
		},
		Providers: ProviderDefinitionsConfig{
			Definitions: providerDefs,
		},
	}

	for name, providerCfg := range providerDefs {
		impl, err := newMockProvider(providerCfg.Provider)
		require.NoError(t, err, "Failed to create mock for provider %q (type %q)", name, providerCfg.Provider)

		err = impl.Initialize(name, providerCfg)
		require.NoError(t, err, "Failed to initialize mock provider %q", name)

		config.AddProvider(name, impl)
	}

	return config
}
