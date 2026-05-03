package localpresence

import (
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

var LocalPresenceCapabilities = models.NewProviderCapabilities().
	WithNotifierConfiguration(models.NotifierConfiguration{
		Runtime: sdkConstants.ModeAgent,
	})
