package email_ses

import "github.com/thand-io/agent/internal/models"

var EmailCapabilities = models.NewProviderCapabilities().
	WithDefaultNotifierConfiguration()
