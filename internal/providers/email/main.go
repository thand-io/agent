package email

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	emailacs "github.com/thand-io/agent/internal/providers/email.acs"
	ses "github.com/thand-io/agent/internal/providers/email.ses"
	smtp "github.com/thand-io/agent/internal/providers/email.smtp"
)

const EmailProviderName = "email"

// emailProvider implements the ProviderImpl interface for Email
type emailProvider struct {
	*models.BaseProvider
	proxy models.ProviderImpl
}

func (p *emailProvider) Initialize(identifier string, provider models.Provider) error {

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		EmailCapabilities,
	)

	// Depending on the provider configuration, setup the email dialer
	// By default, we expect SMTP configuration

	emailerConfig := p.GetConfig()

	// Get platform specific configuration
	platformType := emailerConfig.GetStringWithDefault("platform", "smtp")

	switch platformType {
	case "ses":
		p.proxy = ses.NewEmailSesProvider()
	case "acs":
		p.proxy = emailacs.NewEmailAcsProvider()
	case "smtp":
		fallthrough
	default:
		p.proxy = smtp.NewEmailSmtpProvider()
	}

	if p.proxy == nil {
		return fmt.Errorf("failed to initialize email proxy for platform: %s", platformType)
	}

	return p.proxy.Initialize(identifier, provider)
}
func (p *emailProvider) SendNotification(
	ctx context.Context, notification models.NotificationRequest,
) error {

	if p.proxy == nil {
		return fmt.Errorf("email provider proxy is not initialized")
	}

	return p.proxy.SendNotification(ctx, notification)
}

func init() {
	providers.Register(EmailProviderName, &emailProvider{})
}
