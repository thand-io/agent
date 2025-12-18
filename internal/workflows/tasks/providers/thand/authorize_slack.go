package thand

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
)

// createAuthorizeSlackBlocks creates the Slack Block Kit blocks for the approval notification
func (a *authorizerNotifier) createAuthorizeSlackBlocks(
	toIdentity *models.Identity,
) []slack.Block {

	notifyReq := a.req
	elevateRequest := a.elevationReq

	blocks := []slack.Block{}

	// Add the success message
	a.addSuccessMessageSection(&blocks, notifyReq)

	// Add divider
	blocks = append(blocks, slack.NewDividerBlock())

	// Add request details section
	a.addAuthorizeRequestDetailsSection(&blocks, elevateRequest)

	// Add permissions section (what they can do)
	a.addAuthorizePermissionsSection(&blocks, elevateRequest)

	// Add access button if single provider
	a.addProviderAccessButton(&blocks, elevateRequest, toIdentity)

	// Add divider before closing message
	blocks = append(blocks, slack.NewDividerBlock())

	// Add closing message
	a.addClosingMessageSection(&blocks)

	return blocks
}

// addSuccessMessageSection adds the success message block
func (a *authorizerNotifier) addSuccessMessageSection(blocks *[]slack.Block, notifyReq *thandFunction.NotifierRequest) {
	message := "*Your access request has been approved!*"

	if len(notifyReq.Message) > 0 {
		message = notifyReq.Message
	}

	*blocks = append(*blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject(
			slack.MarkdownType,
			message,
			false,
			false,
		),
		nil,
		nil,
	))
}

// addAuthorizeRequestDetailsSection builds and adds the request details section
func (a *authorizerNotifier) addAuthorizeRequestDetailsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	var requestDetailsText strings.Builder
	requestDetailsText.WriteString("*Approved Request Details:*\n")

	if elevateRequest.Role != nil {
		requestDetailsText.WriteString(fmt.Sprintf("- *Role:* %s\n", elevateRequest.Role.Name))
		if len(elevateRequest.Role.Description) > 0 {
			requestDetailsText.WriteString(fmt.Sprintf("- *Description:* %s\n", elevateRequest.Role.Description))
		}
	}

	if len(elevateRequest.Providers) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("- *Providers:* %s\n", strings.Join(elevateRequest.Providers, ", ")))
	}

	if len(elevateRequest.Duration) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("- *Duration:* %s\n", elevateRequest.Duration))
	}

	*blocks = append(*blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject(
			slack.MarkdownType,
			requestDetailsText.String(),
			false,
			false,
		),
		nil,
		nil,
	))
}

// addAuthorizePermissionsSection adds permissions section if available
func (a *authorizerNotifier) addAuthorizePermissionsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.Role != nil && len(elevateRequest.Role.Permissions.Allow) > 0 {
		var permissionsText strings.Builder
		permissionsText.WriteString("*Granted Permissions:*\n")

		for _, perm := range elevateRequest.Role.Permissions.Allow {
			permissionsText.WriteString(fmt.Sprintf("✓ %s\n", perm))
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				permissionsText.String(),
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addProviderAccessButton adds a button for each provider
func (a *authorizerNotifier) addProviderAccessButton(
	blocks *[]slack.Block,
	elevateRequest *models.ElevateRequestInternal,
	toIdentity *models.Identity,
) {

	if len(elevateRequest.Providers) == 0 || len(a.authRequests) == 0 || len(a.authResponses) == 0 {
		return
	}

	buttonElements := []slack.BlockElement{}

	for _, providerName := range elevateRequest.Providers {
		// Customize URL based on your provider setup
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

		// See if the user ids or emails match in the auth requests/responses

		authRequest, foundReq := a.authRequests[toIdentity.GetId()]
		authResponse, foundAuth := a.authResponses[toIdentity.GetId()]

		if !foundAuth || !foundReq {
			logrus.Errorf("No authorization found for identity '%s' and provider '%s'", toIdentity.GetId(), providerName)
			continue
		}

		accessUrl := providerClient.GetAuthorizedAccessUrl(
			context.TODO(),
			authRequest,
			authResponse,
		)

		buttonElement := slack.NewButtonBlockElement(
			fmt.Sprintf(
				"%s-%s-%s-%s",
				a.workflowTask.WorkflowID,
				a.workflowTask.GetTaskName(),
				"access_provider",
				toIdentity.GetId(),
			),
			fmt.Sprintf("access_provider_button_%s_%s", providerName, toIdentity.GetId()),
			slack.NewTextBlockObject(slack.PlainTextType, fmt.Sprintf("Access %s", providerName), false, false),
		)
		buttonElement.URL = accessUrl
		buttonElement.Style = slack.StylePrimary

		buttonElements = append(buttonElements, buttonElement)

	}

	if len(buttonElements) > 0 {

		*blocks = append(*blocks, slack.NewActionBlock(
			"provider_access_actions",
			buttonElements...,
		))

	}
}

// addClosingMessageSection adds a closing message
func (a *authorizerNotifier) addClosingMessageSection(blocks *[]slack.Block) {
	*blocks = append(*blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject(
			slack.MarkdownType,
			"_Your access is now active. Please use it responsibly._",
			false,
			false,
		),
		nil,
		nil,
	))
}
