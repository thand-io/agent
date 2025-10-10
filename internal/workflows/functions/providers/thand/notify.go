package thand

import (
	"errors"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	emailProvider "github.com/thand-io/agent/internal/providers/email"
	slackProvider "github.com/thand-io/agent/internal/providers/slack"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// NotifyFunction implements access request notification using LLM or user input
type notifyFunction struct {
	config *config.Config
	*functions.BaseFunction
}

// NewValidateFunction creates a new validation Function
func NewNotifyFunction(config *config.Config) *notifyFunction {
	return &notifyFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.notify",
			"This notifies an external provider of an elevation",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for validation
func (t *notifyFunction) GetRequiredParameters() []string {
	return []string{
		"provider",
	} // No strictly required parameters
}

// GetOptionalParameters returns optional parameters with defaults
func (t *notifyFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"provider": "email",
	}
}

// ValidateRequest validates the input parameters
func (t *notifyFunction) ValidateRequest(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return errors.New("request cannot be nil")
	}

	var notificationReq NotifierRequest
	common.ConvertMapToInterface(call.With, &notificationReq)

	notifierProviders := t.config.GetProvidersByCapability(
		models.ProviderCapabilityNotifier)

	// filter out providers to see if the name matches
	for _, provider := range notifierProviders {
		if strings.Compare(provider.Name, notificationReq.Provider) == 0 {
			return nil
		} else if strings.Compare(provider.Provider, notificationReq.Provider) == 0 {
			return nil
		}
	}

	return errors.New("invalid notifier")
}

/*
provider: slack # or slack, email
to: "#access-requests"
message: "Workflow validation passed for user ${ $.user.name }"
approvals: true
*/
type NotifierRequest struct {
	Provider  string `json:"provider"`
	To        string `json:"to"` // Email, channel Id, username etc.
	Message   string `json:"message"`
	Approvals bool   `json:"approvals"`
}

// Execute performs the validation logic
func (t *notifyFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	var notificationReq NotifierRequest
	err := common.ConvertMapToInterface(call.With, &notificationReq)

	if err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	elevationReq, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		return nil, fmt.Errorf("failed to get elevation request from input: %w", err)
	}

	if !elevationReq.IsValid() {
		return nil, errors.New("elevation request is not valid")
	}

	foundProvider := notificationReq.Provider

	if len(foundProvider) == 0 {
		return nil, errors.New("provider must be specified in the with block")
	}

	// Get server config to fetch provider
	providerConfig, err := t.config.Providers.GetProviderByName(foundProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"provider": providerConfig.Name,
	}).Info("Executing notification")

	// Overwrite the notification request with the converted input
	err = common.ConvertMapToInterface(call.With, &notificationReq)

	if err != nil {
		logrus.Warn("Failed to convert notification input")
		return nil, errors.New("failed to convert notification input")
	}

	var notificationPayload models.NotificationRequest

	switch providerConfig.Provider {
	case "slack":
		blocks := t.createSlackBlocks(workflowTask, elevationReq, &notificationReq)

		slackReq := slackProvider.SlackNotificationRequest{
			To: notificationReq.To,
			Text: fmt.Sprintf("Access request for role %s", func() string {
				if elevationReq.Role != nil {
					return elevationReq.Role.Name
				}
				return "unknown"
			}()),
			Blocks: slack.Blocks{
				BlockSet: blocks,
			},
		}
		err = common.ConvertInterfaceToInterface(slackReq, &notificationPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to convert slack request: %w", err)
		}
	case "email":
		emailReq := emailProvider.EmailNotificationRequest{
			To:      notificationReq.To,
			Subject: "Workflow Notification",
			Body: emailProvider.EmailNotificationBody{
				Text: notificationReq.Message,
				HTML: notificationReq.Message,
			},
		}
		err = common.ConvertInterfaceToInterface(emailReq, &notificationPayload)
		if err != nil {
			return nil, fmt.Errorf("failed to convert email request: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", providerConfig.Provider)
	}

	logrus.WithFields(logrus.Fields{
		"input": call.With,
	}).Info("Sending notification")

	err = providerConfig.GetClient().SendNotification(workflowTask.GetContext(), notificationPayload)

	if err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	return nil, err
}

// createSlackBlocks creates the Slack Block Kit blocks for the notification
func (t *notifyFunction) createSlackBlocks(
	workflowTask *models.WorkflowTask,
	elevateRequest *models.ElevateRequestInternal,
	notificationReq *NotifierRequest,
) []slack.Block {
	blocks := []slack.Block{}

	// Add the user message section
	t.addUserMessageSection(&blocks, notificationReq.Message)

	// Add divider
	blocks = append(blocks, slack.NewDividerBlock())

	// Add request details section
	t.addRequestDetailsSection(&blocks, elevateRequest)

	// Add inherited roles section
	t.addInheritedRolesSection(&blocks, elevateRequest)

	// Add permissions section
	t.addPermissionsSection(&blocks, elevateRequest)

	// Add resources section
	t.addResourcesSection(&blocks, elevateRequest)

	// Add user information section
	t.addUserInfoSection(&blocks, elevateRequest)

	// Add divider before action section
	blocks = append(blocks, slack.NewDividerBlock())

	// Add action section
	t.addActionSection(&blocks, workflowTask, notificationReq.Approvals)

	return blocks
}

// addUserMessageSection adds the user message block if message is provided
func (t *notifyFunction) addUserMessageSection(blocks *[]slack.Block, message string) {
	if len(message) > 0 {
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
}

// addRequestDetailsSection builds and adds the request details section
func (t *notifyFunction) addRequestDetailsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	var requestDetailsText strings.Builder
	requestDetailsText.WriteString("*Access Request Details:*\n")

	if elevateRequest.Role != nil {
		requestDetailsText.WriteString(fmt.Sprintf("• *Role:* %s\n", elevateRequest.Role.Name))
		if len(elevateRequest.Role.Description) > 0 {
			requestDetailsText.WriteString(fmt.Sprintf("• *Description:* %s\n", elevateRequest.Role.Description))
		}
	}

	if len(elevateRequest.Providers) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("• *Providers:* %s\n", strings.Join(elevateRequest.Providers, ", ")))
	}

	if len(elevateRequest.Reason) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("• *Reason:* %s\n", elevateRequest.Reason))
	}

	if len(elevateRequest.Duration) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("• *Duration:* %s\n", elevateRequest.Duration))
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

// addInheritedRolesSection adds inherited roles section if available
func (t *notifyFunction) addInheritedRolesSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.Role != nil && len(elevateRequest.Role.Inherits) > 0 {
		var inheritsText strings.Builder
		inheritsText.WriteString("*Inherited Roles:*\n")

		for _, inheritedRole := range elevateRequest.Role.Inherits {
			inheritsText.WriteString(fmt.Sprintf("• %s\n", inheritedRole))
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				inheritsText.String(),
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addPermissionsSection adds permissions section if available
func (t *notifyFunction) addPermissionsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.Role != nil && (len(elevateRequest.Role.Permissions.Allow) > 0 || len(elevateRequest.Role.Permissions.Deny) > 0) {
		var permissionsText strings.Builder
		permissionsText.WriteString("*Permissions:*\n")

		if len(elevateRequest.Role.Permissions.Allow) > 0 {
			permissionsText.WriteString("*Allowed:*\n")
			for _, perm := range elevateRequest.Role.Permissions.Allow {
				permissionsText.WriteString(fmt.Sprintf("- %s\n", perm))
			}
		}

		if len(elevateRequest.Role.Permissions.Deny) > 0 {
			permissionsText.WriteString("*Denied:*\n")
			for _, perm := range elevateRequest.Role.Permissions.Deny {
				permissionsText.WriteString(fmt.Sprintf("- %s\n", perm))
			}
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

// addResourcesSection adds resources section if available
func (t *notifyFunction) addResourcesSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.Role != nil && (len(elevateRequest.Role.Resources.Allow) > 0 || len(elevateRequest.Role.Resources.Deny) > 0) {
		var resourcesText strings.Builder
		resourcesText.WriteString("*Resources:*\n")

		if len(elevateRequest.Role.Resources.Allow) > 0 {
			resourcesText.WriteString("*Allowed:*\n")
			for _, resource := range elevateRequest.Role.Resources.Allow {
				resourcesText.WriteString(fmt.Sprintf("- %s\n", resource))
			}
		}

		if len(elevateRequest.Role.Resources.Deny) > 0 {
			resourcesText.WriteString("*Denied:*\n")
			for _, resource := range elevateRequest.Role.Resources.Deny {
				resourcesText.WriteString(fmt.Sprintf("- %s\n", resource))
			}
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				resourcesText.String(),
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addUserInfoSection adds user information section if available
func (t *notifyFunction) addUserInfoSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.User != nil {
		var userText strings.Builder
		userText.WriteString("*Requested by:*\n")
		userText.WriteString(fmt.Sprintf("*User:* %s\n", elevateRequest.User.Name))
		if len(elevateRequest.User.Email) > 0 {
			userText.WriteString(fmt.Sprintf("*Email:* %s\n", elevateRequest.User.Email))
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				userText.String(),
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addActionSection adds action buttons section based on approval requirements
func (t *notifyFunction) addActionSection(blocks *[]slack.Block, workflowTask *models.WorkflowTask, approvals bool) {
	if approvals {
		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				"*Action Required:*\nPlease review the request and choose an action.",
				false,
				false,
			),
			nil,
			nil,
		))

		*blocks = append(*blocks, slack.NewActionBlock(
			"",
			slack.NewButtonBlockElement(
				"approve",
				"Approve",
				slack.NewTextBlockObject(
					slack.PlainTextType,
					"✅ Approve",
					false,
					false,
				),
			).WithURL(t.createCallbackUrl(workflowTask, true)).WithStyle(slack.StylePrimary),
			slack.NewButtonBlockElement(
				"revoke",
				"Revoke",
				slack.NewTextBlockObject(
					slack.PlainTextType,
					"❌ Revoke",
					false,
					false,
				),
			).WithURL(t.createCallbackUrl(workflowTask, false)).WithStyle(slack.StyleDanger),
		))
	} else {
		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				"No action is required. This is a notification only.",
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

func (t *notifyFunction) createCallbackUrl(
	workflowTask *models.WorkflowTask,
	approve bool,
) string {

	// Create an Event.
	event := cloudevents.NewEvent()
	event.SetSource("thand/agent")
	event.SetType("com.thand.approval")
	event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": approve,
		"user":     "",
	})

	// Setup workflow for the next state
	signaledWorkflow := workflowTask.Clone().(*models.WorkflowTask)
	signaledWorkflow.SetInput(&event)
	_, nextTask := workflowTask.GetNextTask()

	if nextTask == nil {
		logrus.Warn("Failed to ascertain next task for callback URL")
		return ""
	}

	signaledWorkflow.SetEntrypoint(nextTask.Key)

	callbackUrl := t.config.GetResumeCallbackUrl(signaledWorkflow)

	return callbackUrl
}
