package slack

import "github.com/thand-io/agent/internal/models"

var SlackCapabilities = models.NewProviderCapabilities().
	WithDefaultNotifierConfiguration()
