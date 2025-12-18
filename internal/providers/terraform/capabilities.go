package terraform

import "github.com/thand-io/agent/internal/models"

var TerraformCapabilities = models.NewProviderCapabilities().
	WithDefaultProvisioningConfiguration()
