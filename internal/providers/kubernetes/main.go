package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const KubernetesProviderName = "kubernetes"

const (
	AuthorizeClusterRoleActivityName    = "AuthorizeClusterRole"
	AuthorizeNamespacedRoleActivityName = "AuthorizeNamespacedRole"
	RevokeClusterRoleActivityName       = "RevokeClusterRole"
	RevokeNamespacedRoleActivityName    = "RevokeNamespacedRole"
)

// kubernetesProvider implements the ProviderImpl interface for Kubernetes
type kubernetesProvider struct {
	*models.BaseProvider
	client kubernetes.Interface
}

func (p *kubernetesProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		KubernetesCapabilities,
	)

	// Initialize Kubernetes client
	config, err := p.getKubernetesConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	p.client = client

	return nil
}

func (p *kubernetesProvider) GetClient() kubernetes.Interface {
	return p.client
}

// getKubernetesConfig returns the appropriate Kubernetes configuration
func (p *kubernetesProvider) getKubernetesConfig() (*rest.Config, error) {
	// Try in-cluster config first (when running inside K8s)
	if config, err := rest.InClusterConfig(); err == nil {
		logrus.Info("Using in-cluster Kubernetes configuration")
		return config, nil
	}

	// Fallback to kubeconfig file
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if len(kubeconfigPath) == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
	}

	logrus.WithField("kubeconfig", kubeconfigPath).Info("Using kubeconfig file")
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func init() {
	providers.Register(KubernetesProviderName, &kubernetesProvider{}, KubernetesCapabilities, &ConfigSchema{})
}
