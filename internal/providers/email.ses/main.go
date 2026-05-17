package email_ses

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	awsProvider "github.com/thand-io/agent/internal/providers/aws"
)

const EmailSesProviderName = "email.ses"

// emailSesProvider implements the ProviderImpl interface for AWS SES
type emailSesProvider struct {
	*models.BaseProvider
	sesClient          *sesv2.Client
	defaultFromAddress string
}

func (p *emailSesProvider) Initialize(identifier string, provider models.ProviderConfig) error {

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		EmailCapabilities,
	)

	emailerConfig := p.GetConfig()

	// Get required configuration
	sesFrom, foundFrom := emailerConfig.GetString("from")

	var missingFields []string
	if !foundFrom {
		missingFields = append(missingFields, "from")
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("missing email configuration field(s): %v", missingFields)
	}

	// Load AWS configuration using the shared CreateAwsConfig function
	sdkConfig, err := awsProvider.CreateAwsConfig(emailerConfig)

	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}

	// Create SES client
	p.sesClient = sesv2.NewFromConfig(sdkConfig.Config)
	p.defaultFromAddress = sesFrom

	return nil
}

func (p *emailSesProvider) SendNotification(
	ctx models.ProviderContext, notification models.NotificationRequest,
) error {

	goCtx := models.ContextFromProviderContext(ctx)

	// Convert NotificationRequest to EmailNotificationRequest
	emailRequest := &models.EmailNotificationRequest{}
	common.ConvertMapToInterface(notification, emailRequest)

	// Validate recipients
	if len(emailRequest.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	// Prepare email content
	emailContent := &types.EmailContent{}

	// Build the body
	body := &types.Body{}
	if len(emailRequest.Body.Text) > 0 {
		body.Text = &types.Content{
			Data:    aws.String(emailRequest.Body.Text),
			Charset: aws.String("UTF-8"),
		}
	}
	if len(emailRequest.Body.HTML) > 0 {
		body.Html = &types.Content{
			Data:    aws.String(emailRequest.Body.HTML),
			Charset: aws.String("UTF-8"),
		}
	}

	// Build the message
	message := &types.Message{
		Subject: &types.Content{
			Data:    aws.String(emailRequest.Subject),
			Charset: aws.String("UTF-8"),
		},
		Body: body,
	}

	emailContent.Simple = message

	// Prepare destination
	destination := &types.Destination{
		ToAddresses: emailRequest.To,
	}

	// Determine from address
	fromAddress := p.defaultFromAddress
	if len(emailRequest.From) > 0 {
		fromAddress = emailRequest.From
	}

	// Send email using SES
	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(fromAddress),
		Destination:      destination,
		Content:          emailContent,
	}

	_, err := p.sesClient.SendEmail(goCtx, input)
	if err != nil {
		return fmt.Errorf("failed to send email via SES: %w", err)
	}

	return nil
}

func NewEmailSesProvider() models.Provider {
	return &emailSesProvider{}
}

func init() {
	providers.Register(EmailSesProviderName, &emailSesProvider{}, EmailCapabilities, &ConfigSchema{})
}
