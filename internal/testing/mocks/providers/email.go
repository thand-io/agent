package providers

import (
	coreProviders "github.com/thand-io/agent/internal/providers"
	"github.com/thand-io/agent/internal/providers/email"
)

func init() {
	// Register mock Email provider to override the real one for all tests
	// Use Register() to properly set metadata including capabilities
	coreProviders.Register(
		email.EmailProviderName,
		email.NewMockEmailProvider(),
		email.EmailCapabilities,
		&email.ConfigSchema{},
	)
}
