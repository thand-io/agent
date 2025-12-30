package thand

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// createAuthorizeEmailBody creates the email body for authorization confirmation
func (a *authorizerNotifier) createAuthorizeEmailBody() (string, string) {
	elevationReq := a.elevationReq
	notifyReq := a.req

	// Build plain text version
	var plainText strings.Builder
	plainText.WriteString("Good news! Your access request has been approved.\n\n")

	if elevationReq.Role != nil {
		plainText.WriteString(fmt.Sprintf("Role: %s\n", elevationReq.Role.Name))
		if len(elevationReq.Role.Description) > 0 {
			plainText.WriteString(fmt.Sprintf("Description: %s\n", elevationReq.Role.Description))
		}
	}

	if len(elevationReq.Providers) > 0 {
		plainText.WriteString(fmt.Sprintf("Providers: %s\n", strings.Join(elevationReq.Providers, ", ")))
	}

	if len(elevationReq.Duration) > 0 {
		plainText.WriteString(fmt.Sprintf("Duration: %s\n", elevationReq.Duration))
	}

	if len(elevationReq.Tenants) > 0 {

		// Resolve tenant names if possible
		var tenantNames []string
		for _, tenantID := range elevationReq.Tenants {

			if len(tenantID) == 0 {
				continue
			}

			if tenant, err := a.config.GetTenant(tenantID); err == nil && tenant != nil {
				tenantNames = append(tenantNames, tenant.String())
			} else {
				tenantNames = append(tenantNames, tenantID)
			}
		}

		if len(tenantNames) > 0 {
			plainText.WriteString(fmt.Sprintf("Tenants: %s\n", strings.Join(elevationReq.Tenants, ", ")))
		}
	}

	plainText.WriteString("\nYour access is now active. Please use it responsibly.")

	// Build data map for template
	data := map[string]any{
		"Providers": strings.Join(elevationReq.Providers, ", "),
		"Duration":  elevationReq.Duration,
	}

	if len(elevationReq.Tenants) > 0 {
		data["Tenants"] = strings.Join(elevationReq.Tenants, ", ")
	}

	if len(notifyReq.Message) > 0 {
		data["Message"] = notifyReq.Message
	}

	if elevationReq.Role != nil {
		data["Role"] = map[string]any{
			"Name":        elevationReq.Role.Name,
			"Description": elevationReq.Role.Description,
		}

		// Add permissions if available
		if len(elevationReq.Role.Permissions.Allow) > 0 {
			data["Permissions"] = elevationReq.Role.Permissions.Allow
		}
	}

	// Add provider access buttons
	a.addProviderAccessButtons(context.Background(), data)

	// Render HTML email using template
	html, err := RenderEmailWithTemplate("Access Request Approved", GetAuthorizeContentTemplate(), data)
	if err != nil {
		logrus.WithError(err).Error("Failed to render authorization email")
		return plainText.String(), ""
	}

	return plainText.String(), html
}

// addProviderAccessButtons adds provider access button data to the template
func (a *authorizerNotifier) addProviderAccessButtons(ctx context.Context, data map[string]any) {
	elevationReq := a.elevationReq

	if len(elevationReq.Providers) == 0 || len(a.authRequests) == 0 || len(a.authResponses) == 0 {
		return
	}

	identities := a.req.To

	if len(identities) == 0 {
		logrus.Error("No identity found for access URL generation")
		return
	}

	type ProviderButton struct {
		Name string
		URL  string
	}

	var providerButtons []ProviderButton

	for _, providerName := range elevationReq.Providers {
		// Get provider configuration
		provider, err := a.config.GetProviderByName(providerName)

		if err != nil {
			logrus.Errorf("Failed to get provider '%s' for access URL: %v", providerName, err)
			continue
		}

		providerClient := provider.GetClient()

		if providerClient == nil {
			logrus.Errorf("Provider '%s' has no client defined for access URL", providerName)
			continue
		}

		for _, identity := range identities {
			authRequest, foundReq := a.authRequests[identity]
			authResponse, foundAuth := a.authResponses[identity]

			if !foundAuth || !foundReq {
				logrus.Errorf("No authorization found for identity '%s' and provider '%s'", identity, providerName)
				continue
			}

			accessURL := providerClient.GetAuthorizedAccessUrl(
				ctx,
				authRequest,
				authResponse,
			)

			providerButtons = append(providerButtons, ProviderButton{
				Name: providerName,
				URL:  accessURL,
			})
		}
	}

	if len(providerButtons) > 0 {
		data["ProviderButtons"] = providerButtons
	}
}
