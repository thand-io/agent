package email

import (
	"context"
	"fmt"

	"crypto/tls"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"gopkg.in/gomail.v2"
)

// emailProvider implements the ProviderImpl interface for Email
type emailProvider struct {
	*models.BaseProvider
	mailer *gomail.Dialer
}

func (p *emailProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityNotifier,
	)

	emailerConfig := p.GetConfig()

	smtpHost, foundSmtpHost := emailerConfig.GetString("host")
	smtpPort, foundSmtpPort := emailerConfig.GetInt("port")
	smtpUser, foundSmtpUser := emailerConfig.GetString("user")
	smtpPass, foundSmtpPass := emailerConfig.GetString("pass")

	if !foundSmtpHost || !foundSmtpPort || !foundSmtpUser || !foundSmtpPass {
		return fmt.Errorf("missing email configuration")
	}

	tlsSkipVerify, foundTlsSkipVerify := emailerConfig.GetBool("tls_skip_verify")

	if !foundTlsSkipVerify {
		tlsSkipVerify = false
	}

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: tlsSkipVerify}

	p.mailer = d

	// Send emails using d.
	// TODO: Implement Email initialization logic
	return nil
}

type EmailNotificationRequest struct {
	From    string
	To      string
	Subject string
	Body    EmailNotificationBody
	Headers map[string][]string
}

type EmailNotificationBody struct {
	Text string
	HTML string
}

func (p *emailProvider) SendNotification(
	ctx context.Context, notification models.NotificationRequest,
) error {

	// Lets convert NotificationRequest to EmailNotificationRequest
	emailRequest := &EmailNotificationRequest{}
	common.ConvertMapToInterface(notification, emailRequest)

	m := &gomail.Message{}

	if len(emailRequest.Body.Text) > 0 {
		m.SetBody("text/plain", emailRequest.Body.Text, gomail.SetPartEncoding(gomail.Unencoded))
	}
	if len(emailRequest.Body.HTML) > 0 {
		m.SetBody("text/html", emailRequest.Body.HTML, gomail.SetPartEncoding(gomail.Unencoded))
	}

	if len(emailRequest.Headers) > 0 {
		m.SetHeaders(emailRequest.Headers)
	}

	if len(emailRequest.Subject) > 0 {
		m.SetHeader("Subject", emailRequest.Subject)
	}

	m.SetAddressHeader("From", emailRequest.From, "")
	m.SetAddressHeader("To", emailRequest.To, "")

	err := p.mailer.DialAndSend(m)

	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

func init() {
	providers.Register("email", &emailProvider{})
}
