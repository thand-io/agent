package salesforce

import "github.com/thand-io/agent/internal/models"

var SalesforceCapabilities = models.NewProviderCapabilities().
	WithDefaultRolesConfiguration().
	WithDefaultProvisioningConfiguration()
