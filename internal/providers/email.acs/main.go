package email_acs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	azureProvider "github.com/thand-io/agent/internal/providers/azure"
)

const EmailAcsProviderName = "email.acs"

// emailAcsProvider implements the ProviderImpl interface for Azure Communication Services Email
type emailAcsProvider struct {
	*models.BaseProvider
	endpoint           string
	defaultFromAddress string
	credential         *azureProvider.AzureConfigurationProvider
}

func (p *emailAcsProvider) Initialize(identifier string, provider models.ProviderConfig) error {

	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		EmailCapabilities,
	)

	emailerConfig := p.GetConfig()

	// Get required configuration
	acsEndpoint, foundEndpoint := emailerConfig.GetString("endpoint")
	acsFrom, foundFrom := emailerConfig.GetString("from")

	var missingFields []string
	if !foundEndpoint {
		missingFields = append(missingFields, "endpoint")
	}
	if !foundFrom {
		missingFields = append(missingFields, "from")
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("missing email configuration field(s): %v", missingFields)
	}

	// Load Azure configuration using the shared CreateAzureConfig function
	azureConfig, err := azureProvider.CreateAzureConfig(emailerConfig)

	if err != nil {
		return fmt.Errorf("failed to create Azure config: %w", err)
	}

	p.credential = azureConfig
	p.endpoint = acsEndpoint
	p.defaultFromAddress = acsFrom

	return nil
}

func (p *emailAcsProvider) SendNotification(
	ctx context.Context, notification models.NotificationRequest,
) error {

	// Convert NotificationRequest to EmailNotificationRequest
	emailRequest := &models.EmailNotificationRequest{}
	common.ConvertMapToInterface(notification, emailRequest)

	// Validate recipients
	if len(emailRequest.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	// Determine from address
	fromAddress := p.defaultFromAddress
	if len(emailRequest.From) > 0 {
		fromAddress = emailRequest.From
	}

	// Build the email message for Azure Communication Services API
	recipients := make([]map[string]string, len(emailRequest.To))
	for i, to := range emailRequest.To {
		recipients[i] = map[string]string{"address": to}
	}

	// Build content based on what's available
	content := map[string]string{
		"subject": emailRequest.Subject,
	}

	if len(emailRequest.Body.HTML) > 0 {
		content["html"] = emailRequest.Body.HTML
	}
	if len(emailRequest.Body.Text) > 0 {
		content["plainText"] = emailRequest.Body.Text
	}

	emailMessage := map[string]any{
		"senderAddress": fromAddress,
		"recipients": map[string]any{
			"to": recipients,
		},
		"content": content,
	}

	// Marshal the request body
	requestBody, err := json.Marshal(emailMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	// Get access token
	token, err := p.credential.Token.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://communication.azure.com/.default"},
	})
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Send email using Azure Communication Services REST API
	url := fmt.Sprintf("%s/emails:send?api-version=2023-03-31", p.endpoint)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.Token))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send email via Azure Communication Services: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send email via Azure Communication Services: status %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func NewEmailAcsProvider() models.Provider {
	return &emailAcsProvider{}
}

func init() {
	providers.Register(EmailAcsProviderName, &emailAcsProvider{})
}
