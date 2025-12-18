package email_acs

import "github.com/thand-io/agent/internal/models"

var EmailCapabilities = models.NewProviderCapabilities().
	WithDefaultNotifierConfiguration()
