package providers

import (
	coreProviders "github.com/thand-io/agent/internal/providers"
	"github.com/thand-io/agent/internal/providers/gcp"
)

func init() {
	// Register mock GCP provider to override the real one for all tests
	// Use Register() to properly set metadata including capabilities
	coreProviders.Register(
		gcp.GcpProviderName,
		gcp.NewMockGcpProvider(),
		gcp.GcpCapabilities,
		&gcp.ConfigSchema{},
	)
}
