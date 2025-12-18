package thand

import (
	"context"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/models"
)

// createSlackBlocks creates the Slack Block Kit blocks for the notification
func (a *approvalsNotifier) createApprovalSlackBlocks() []slack.Block {

	notifyReq := a.req
	elevateRequest := a.elevationReq
	workflowTask := a.workflowTask

	blocks := []slack.Block{}

	// Add the user message section
	a.addUserMessageSection(&blocks, notifyReq)

	// Add divider
	blocks = append(blocks, slack.NewDividerBlock())

	// Add request details section
	a.addRequestDetailsSection(&blocks, elevateRequest)

	// Add identities section
	a.addIdentitiesSection(&blocks, elevateRequest)

	// Add inherited roles section
	a.addInheritedRolesSection(&blocks, elevateRequest)

	// Add groups section
	a.addGroupsSection(&blocks, elevateRequest)

	// Add permissions section
	a.addPermissionsSection(&blocks, elevateRequest)

	// Add resources section
	a.addResourcesSection(&blocks, elevateRequest)

	// Add user information section
	a.addUserInfoSection(&blocks, elevateRequest)

	// Add divider before action section
	blocks = append(blocks, slack.NewDividerBlock())

	// Add action section
	a.addActionSection(&blocks, workflowTask, notifyReq)

	return blocks
}

// addUserMessageSection adds the user message block if message is provided
func (a *approvalsNotifier) addUserMessageSection(blocks *[]slack.Block, approvalNotifier *ApprovalNotifier) {
	if len(approvalNotifier.Notifier.Message) > 0 {
		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				approvalNotifier.Notifier.Message,
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addRequestDetailsSection builds and adds the request details section
func (a *approvalsNotifier) addRequestDetailsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	var requestDetailsText strings.Builder
	requestDetailsText.WriteString("*Access Request Details:*\n")

	if elevateRequest.Role != nil {
		requestDetailsText.WriteString(fmt.Sprintf("- *Role:* %s\n", elevateRequest.Role.Name))
		if len(elevateRequest.Role.Description) > 0 {
			requestDetailsText.WriteString(fmt.Sprintf("- *Description:* %s\n", elevateRequest.Role.Description))
		}
	}

	if len(elevateRequest.Providers) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("- *Providers:* %s\n", strings.Join(elevateRequest.Providers, ", ")))
	}

	if len(elevateRequest.Reason) > 0 {
		requestDetailsText.WriteString(fmt.Sprintf("- *Reason:* %s\n", elevateRequest.Reason))
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

// addInheritedRolesSection adds inherited roles section if available
func (a *approvalsNotifier) addInheritedRolesSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.Role != nil && len(elevateRequest.Role.Inherits) > 0 {
		var inheritsText strings.Builder
		inheritsText.WriteString("*Inherited Roles:*\n")

		for _, inheritedRole := range elevateRequest.Role.Inherits {
			inheritsText.WriteString(fmt.Sprintf("- %s\n", inheritedRole))
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

// addGroupsSection adds groups section if available
func (a *approvalsNotifier) addGroupsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if elevateRequest.Role != nil && (len(elevateRequest.Role.Groups.Allow) > 0 || len(elevateRequest.Role.Groups.Deny) > 0) {
		var groupsText strings.Builder
		groupsText.WriteString("*Groups:*\n")

		if len(elevateRequest.Role.Groups.Allow) > 0 {
			groupsText.WriteString("*Allowed:*\n")
			for _, group := range elevateRequest.Role.Groups.Allow {
				groupsText.WriteString(fmt.Sprintf("- %s\n", group))
			}
		}

		if len(elevateRequest.Role.Groups.Deny) > 0 {
			groupsText.WriteString("*Denied:*\n")
			for _, group := range elevateRequest.Role.Groups.Deny {
				groupsText.WriteString(fmt.Sprintf("- %s\n", group))
			}
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				groupsText.String(),
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addPermissionsSection adds permissions section if available
func (a *approvalsNotifier) addPermissionsSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
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
func (a *approvalsNotifier) addResourcesSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
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
func (a *approvalsNotifier) addUserInfoSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
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

// addIdentitiesSection adds identities section if available
func (a *approvalsNotifier) addIdentitiesSection(blocks *[]slack.Block, elevateRequest *models.ElevateRequestInternal) {
	if len(elevateRequest.Identities) > 0 {
		var identitiesText strings.Builder
		identitiesText.WriteString("*Target Identities:*\n")

		// Resolve all identities to get nice display names
		resolvedIdentities := elevateRequest.ResolveIdentities(
			context.Background(),
			a.config.GetProvidersByCapability(
				models.ProviderCapabilityIdentities,
			))

		for _, identity := range elevateRequest.Identities {

			if resolved, ok := resolvedIdentities[identity]; ok {
				identitiesText.WriteString(fmt.Sprintf("- %s\n", resolved.String()))
				continue
			}

			identitiesText.WriteString(fmt.Sprintf("- %s\n", identity))
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				identitiesText.String(),
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addActionSection adds action buttons section based on approval requirements
func (a *approvalsNotifier) addActionSection(
	blocks *[]slack.Block,
	workflowTask *models.WorkflowTask,
	approvalNotifier *ApprovalNotifier,
) {
	if approvalNotifier.Approvals > 0 {
		// Get current approvals from workflow context
		workflowContext := workflowTask.GetContextAsMap()
		approvals, ok := workflowContext["approvals"].([]any)
		if !ok {
			approvals = []any{}
		}

		// Count existing approved approvals
		approvedCount := 0
		for _, approval := range approvals {
			if approvalMap, ok := approval.(map[string]any); ok {
				if approved, exists := approvalMap["approved"]; exists {
					if approvedBool, ok := approved.(bool); ok && approvedBool {
						approvedCount++
					}
				}
			}
		}

		remainingApprovals := approvalNotifier.Approvals - approvedCount

		// Create dynamic message based on approval requirements
		var actionMessage string
		if approvalNotifier.Approvals == 1 {
			actionMessage = "*Action Required:*\n*One approval is required.* Please review the request and choose an action."
		} else if remainingApprovals <= 0 {
			actionMessage = "*Action Required:*\n*Sufficient approvals have been received.* Please review the request and choose an action."
		} else if remainingApprovals == 1 {
			actionMessage = fmt.Sprintf("*Action Required:*\n*%d more approval is needed (%d of %d received).* Please review the request and choose an action.", remainingApprovals, approvedCount, approvalNotifier.Approvals)
		} else {
			actionMessage = fmt.Sprintf("*Action Required:*\n*%d more approvals are needed (%d of %d received).* Please review the request and choose an action.", remainingApprovals, approvedCount, approvalNotifier.Approvals)
		}

		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				actionMessage,
				false,
				false,
			),
			nil,
			nil,
		))

		if remainingApprovals > 0 {

			*blocks = append(*blocks, slack.NewActionBlock(
				"",
				slack.NewButtonBlockElement(
					fmt.Sprintf(
						"%s-%s-%s",
						a.workflowTask.WorkflowID,
						a.workflowTask.GetTaskName(),
						"approve",
					),
					"Approve",
					slack.NewTextBlockObject(
						slack.PlainTextType,
						"Approve",
						false,
						false,
					),
				).WithURL(a.createCallbackUrl(workflowTask, approvalNotifier, true)).WithStyle(slack.StylePrimary),
				slack.NewButtonBlockElement(
					fmt.Sprintf(
						"%s-%s-%s",
						a.workflowTask.WorkflowID,
						a.workflowTask.GetTaskName(),
						"deny",
					),
					"Deny",
					slack.NewTextBlockObject(
						slack.PlainTextType,
						"Deny",
						false,
						false,
					),
				).WithURL(a.createCallbackUrl(workflowTask, approvalNotifier, false)).WithStyle(slack.StyleDanger),
				slack.NewButtonBlockElement(
					fmt.Sprintf(
						"%s-%s-%s",
						a.workflowTask.WorkflowID,
						a.workflowTask.GetTaskName(),
						"view_request",
					),
					"View Request",
					slack.NewTextBlockObject(
						slack.PlainTextType,
						"View Request",
						false,
						false,
					),
				).WithURL(a.createViewRequestUrl(workflowTask)),
			))

		}
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

func (a *approvalsNotifier) createViewRequestUrl(workflowTask *models.WorkflowTask) string {
	return fmt.Sprintf("%s/execution/%s", a.config.GetLoginServerUrl(), workflowTask.WorkflowID)
}

func (a *approvalsNotifier) createCallbackUrl(
	workflowTask *models.WorkflowTask,
	approvalNotifier *ApprovalNotifier,
	approve bool,
) string {

	// Create an Event.
	event := cloudevents.NewEvent()
	event.SetSpecVersion("1.0")
	event.SetID(uuid.New().String())
	event.SetTime(time.Now())
	event.SetSource("urn:thand:agent")
	event.SetType(ThandApprovalEventType)
	event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": approve,
	})
	// The user who clicked the button is not known at this time
	// event.SetExtension("user", "")

	// Setup workflow for the next state
	signaledWorkflow := workflowTask.Clone().(*models.WorkflowTask)
	signaledWorkflow.SetInput(&event)

	if len(approvalNotifier.Entrypoint) > 0 {
		signaledWorkflow.SetEntrypoint(approvalNotifier.Entrypoint)
	}

	if workflowTask.HasTemporalContext() {
		return a.config.GetSignalCallbackUrl(signaledWorkflow)
	} else {
		return a.config.GetResumeCallbackUrl(signaledWorkflow)
	}
}
