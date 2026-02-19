package providers

import (
	coreProviders "github.com/thand-io/agent/internal/providers"
	"github.com/thand-io/agent/internal/providers/azure"
)

func init() {
	// Register mock Azure provider to override the real one for all tests
	// Use Register() to properly set metadata including capabilities
	coreProviders.Register(
		azure.AzureProviderName,
		azure.NewMockAzureProvider(),
		azure.AzureCapabilities,
		&azure.ConfigSchema{},
	)
}
