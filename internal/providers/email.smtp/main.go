package email_smtp

import (
	"fmt"

	"crypto/tls"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"gopkg.in/gomail.v2"
)

const EmailSmtpProviderName = "email.smtp"

// emailSmtpProvider implements the ProviderImpl interface for Email
type emailSmtpProvider struct {
	*models.BaseProvider
	mailer             *gomail.Dialer
	defaultFromAddress string
}

func (p *emailSmtpProvider) Initialize(identifier string, provider models.ProviderConfig) error {

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		EmailCapabilities,
	)

	// Depending on the provider configuration, setup the email dialer
	// By default, we expect SMTP configuration

	emailerConfig := p.GetConfig()

	smtpHost, foundSmtpHost := emailerConfig.GetString("host")
	smtpPort, foundSmtpPort := emailerConfig.GetInt("port")
	smtpUser, foundSmtpUser := emailerConfig.GetString("user")
	smtpPass, foundSmtpPass := emailerConfig.GetString("pass")
	smtpFrom, foundFrom := emailerConfig.GetString("from")

	var missingFields []string
	if !foundSmtpHost {
		missingFields = append(missingFields, "host")
	}
	if !foundSmtpPort {
		missingFields = append(missingFields, "port")
	}
	if !foundSmtpUser {
		missingFields = append(missingFields, "user")
	}
	if !foundSmtpPass {
		missingFields = append(missingFields, "pass")
	}
	if !foundFrom {
		missingFields = append(missingFields, "from")
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("missing email configuration field(s): %v", missingFields)
	}

	tlsSkipVerify, foundTlsSkipVerify := emailerConfig.GetBool("tls_skip_verify")

	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)

	if foundTlsSkipVerify {
		d.TLSConfig = &tls.Config{
			ServerName:         smtpHost,
			InsecureSkipVerify: tlsSkipVerify,
		}
	}

	p.mailer = d
	p.defaultFromAddress = smtpFrom

	return nil
}

func (p *emailSmtpProvider) SendNotification(
	ctx models.ProviderContext, notification models.NotificationRequest,
) error {

	// gomail's dialer does not accept a context; the ProviderContext is only
	// used to satisfy the ProviderNotifier interface and may also carry a
	// workflow.Context when invoked from within a Temporal workflow (the
	// outer email provider performs the activity dispatch).
	_ = ctx

	// Lets convert NotificationRequest to EmailNotificationRequest
	emailRequest := &models.EmailNotificationRequest{}
	common.ConvertMapToInterface(notification, emailRequest)

	m := gomail.NewMessage()

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

	// From field is required
	if len(emailRequest.From) > 0 {
		m.SetAddressHeader("From", emailRequest.From, "")
	} else {
		m.SetAddressHeader("From", p.defaultFromAddress, "")
	}

	// Set multiple recipients
	if len(emailRequest.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	m.SetHeader("To", emailRequest.To...)

	err := p.mailer.DialAndSend(m)

	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

func NewEmailSmtpProvider() models.Provider {
	return &emailSmtpProvider{}
}

func init() {
	providers.Register(EmailSmtpProviderName, &emailSmtpProvider{}, EmailCapabilities, &ConfigSchema{})
}
