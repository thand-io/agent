package gsuite

import "github.com/thand-io/agent/internal/models"

var GsuiteCapabilities = models.NewProviderCapabilities().
	WithDefaultUsersConfiguration().
	WithDefaultGroupsConfiguration().
	WithDefaultProvisioningConfiguration()
