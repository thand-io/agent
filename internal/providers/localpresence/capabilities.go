package localpresence

import "github.com/thand-io/agent/internal/models"

var LocalPresenceCapabilities = models.NewProviderCapabilities().
	WithDefaultNotifierConfiguration()
