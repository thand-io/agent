package local

import "github.com/thand-io/agent/internal/models"

var AwsCapabilities = models.NewProviderCapabilities().
	WithDefaultTenantsConfiguration()
