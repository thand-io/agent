package kubernetes

import "github.com/thand-io/agent/internal/models"

var KubernetesCapabilities = models.NewProviderCapabilities().
	WithDefaultPermissionsConfiguration().
	WithDefaultProvisioningConfiguration()
