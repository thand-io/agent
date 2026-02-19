package providers

import (
	coreProviders "github.com/thand-io/agent/internal/providers"
	"github.com/thand-io/agent/internal/providers/kubernetes"
)

func init() {
	// Register mock Kubernetes provider to override the real one for all tests
	// Use Register() to properly set metadata including capabilities
	coreProviders.Register(
		kubernetes.KubernetesProviderName,
		kubernetes.NewMockKubernetesProvider(),
		kubernetes.KubernetesCapabilities,
		&kubernetes.ConfigSchema{},
	)
}
