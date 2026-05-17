package email

import (
	"context"
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	emailacs "github.com/thand-io/agent/internal/providers/email.acs"
	ses "github.com/thand-io/agent/internal/providers/email.ses"
	smtp "github.com/thand-io/agent/internal/providers/email.smtp"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

const EmailProviderName = "email"

// emailProvider implements the ProviderImpl interface for Email
type emailProvider struct {
	*models.BaseProvider
	proxy models.Provider
}

func (p *emailProvider) Initialize(identifier string, provider models.ProviderConfig) error {

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
	ctx models.ProviderContext, notification models.NotificationRequest,
) error {

	if p.proxy == nil {
		return fmt.Errorf("email provider proxy is not initialized")
	}

	// When invoked from a Temporal workflow coroutine, dispatch the actual
	// email API call as a Temporal activity so it benefits from retry,
	// history, and replay determinism. Mirrors the AWS provider's exec*
	// helpers.
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), models.SendNotificationActivityName),
			notification,
		).Get(wfCtx, nil)
	}

	return p.sendNotificationDirect(models.ContextFromProviderContext(ctx), notification)
}

// sendNotificationDirect performs the underlying API call against the
// configured email proxy (smtp / ses / acs / mock). It is invoked both
// directly (when no Temporal workflow context is present) and as the body of
// the SendNotificationActivity Temporal activity.
func (p *emailProvider) sendNotificationDirect(
	ctx context.Context,
	notification models.NotificationRequest,
) error {
	if p.proxy == nil {
		return fmt.Errorf("email provider proxy is not initialized")
	}
	return p.proxy.SendNotification(ctx, notification)
}

func init() {
	providers.Register(EmailProviderName, &emailProvider{}, EmailCapabilities, &ConfigSchema{})
}
