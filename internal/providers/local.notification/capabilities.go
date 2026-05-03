package localnotification

import (
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

var LocalNotificationCapabilities = models.NewProviderCapabilities().
	WithNotifierConfiguration(models.NotifierConfiguration{
		Runtime: sdkConstants.ModeAgent,
	})
