package localnotification

import "github.com/thand-io/agent/internal/models"

var LocalNotificationCapabilities = models.NewProviderCapabilities().
	WithDefaultNotifierConfiguration()
