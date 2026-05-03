package local

import (
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

var LocalCapabilities = models.NewProviderCapabilities().
	// Notifier gets run on the agent
	WithNotifierConfiguration(models.NotifierConfiguration{
		Enabled: true,
		Runtime: sdkConstants.ModeAgent,
	}).
	// Everything else gets run on the server
	WithDefaultTenantsConfiguration()
