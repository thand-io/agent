package thand

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// createApprovalEmailBody creates the email body for approval requests
func (a *approvalsNotifier) createApprovalEmailBody() (string, string) {
	elevationReq := a.elevationReq
	notifyReq := a.req
	workflowTask := a.workflowTask

	// Build plain text version
	var plainText strings.Builder
	plainText.WriteString("A user has requested elevated access and requires your approval.\n\n")

	if elevationReq.User != nil {
		plainText.WriteString(fmt.Sprintf("Requested by: %s", elevationReq.User.Name))
		if len(elevationReq.User.Email) > 0 {
			plainText.WriteString(fmt.Sprintf(" (%s)", elevationReq.User.Email))
		}
		plainText.WriteString("\n\n")
	}

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

	if len(elevationReq.Reason) > 0 {
		plainText.WriteString(fmt.Sprintf("Reason: %s\n", elevationReq.Reason))
	}

	if len(elevationReq.Identities) > 0 {

		plainText.WriteString("\nTarget Identities:\n")

		// Resolve all identities to get nice display names
		resolvedIdentities := elevationReq.ResolveIdentities(
			context.Background(),
			a.config.GetProvidersByCapability(
				models.ProviderCapabilityIdentities,
			))

		for _, identity := range elevationReq.Identities {

			if resolved, ok := resolvedIdentities[identity]; ok {
				plainText.WriteString(fmt.Sprintf("- %s\n", resolved.String()))
				continue
			}

			plainText.WriteString(fmt.Sprintf("- %s\n", identity))
		}
	}

	if elevationReq.Role != nil && (len(elevationReq.Role.Groups.Allow) > 0 || len(elevationReq.Role.Groups.Deny) > 0) {
		plainText.WriteString("\nGroups:\n")
		if len(elevationReq.Role.Groups.Allow) > 0 {
			plainText.WriteString("Allowed:\n")
			for _, group := range elevationReq.Role.Groups.Allow {
				plainText.WriteString(fmt.Sprintf("- %s\n", group))
			}
		}
		if len(elevationReq.Role.Groups.Deny) > 0 {
			plainText.WriteString("Denied:\n")
			for _, group := range elevationReq.Role.Groups.Deny {
				plainText.WriteString(fmt.Sprintf("- %s\n", group))
			}
		}
	}

	if elevationReq.Role != nil && (len(elevationReq.Role.Permissions.Allow) > 0 || len(elevationReq.Role.Permissions.Deny) > 0) {
		plainText.WriteString("\nPermissions:\n")
		if len(elevationReq.Role.Permissions.Allow) > 0 {
			plainText.WriteString("Allowed:\n")
			for _, perm := range elevationReq.Role.Permissions.Allow {
				plainText.WriteString(fmt.Sprintf("- %s\n", perm))
			}
		}
		if len(elevationReq.Role.Permissions.Deny) > 0 {
			plainText.WriteString("Denied:\n")
			for _, perm := range elevationReq.Role.Permissions.Deny {
				plainText.WriteString(fmt.Sprintf("- %s\n", perm))
			}
		}
	}

	if elevationReq.Role != nil && (len(elevationReq.Role.Resources.Allow) > 0 || len(elevationReq.Role.Resources.Deny) > 0) {
		plainText.WriteString("\nResources:\n")
		if len(elevationReq.Role.Resources.Allow) > 0 {
			plainText.WriteString("Allowed:\n")
			for _, resource := range elevationReq.Role.Resources.Allow {
				plainText.WriteString(fmt.Sprintf("- %s\n", resource))
			}
		}
		if len(elevationReq.Role.Resources.Deny) > 0 {
			plainText.WriteString("Denied:\n")
			for _, resource := range elevationReq.Role.Resources.Deny {
				plainText.WriteString(fmt.Sprintf("- %s\n", resource))
			}
		}
	}

	// Build data map for template
	data := map[string]any{
		"Providers":  strings.Join(elevationReq.Providers, ", "),
		"Duration":   elevationReq.Duration,
		"Reason":     elevationReq.Reason,
		"Identities": elevationReq.Identities,
	}

	if len(notifyReq.Notifier.Message) > 0 {
		data["Message"] = notifyReq.Notifier.Message
	}

	if elevationReq.User != nil {
		data["User"] = map[string]any{
			"Name":  elevationReq.User.Name,
			"Email": elevationReq.User.Email,
		}
	}

	if elevationReq.Role != nil {
		data["Role"] = map[string]any{
			"Name":        elevationReq.Role.Name,
			"Description": elevationReq.Role.Description,
		}

		// Add groups if available
		if len(elevationReq.Role.Groups.Allow) > 0 || len(elevationReq.Role.Groups.Deny) > 0 {
			data["Groups"] = map[string]any{
				"Allow": elevationReq.Role.Groups.Allow,
				"Deny":  elevationReq.Role.Groups.Deny,
			}
		}

		// Add permissions if available
		if len(elevationReq.Role.Permissions.Allow) > 0 || len(elevationReq.Role.Permissions.Deny) > 0 {
			data["Permissions"] = map[string]any{
				"Allow": elevationReq.Role.Permissions.Allow,
				"Deny":  elevationReq.Role.Permissions.Deny,
			}
		}

		// Add resources if available
		if len(elevationReq.Role.Resources.Allow) > 0 || len(elevationReq.Role.Resources.Deny) > 0 {
			data["Resources"] = map[string]any{
				"Allow": elevationReq.Role.Resources.Allow,
				"Deny":  elevationReq.Role.Resources.Deny,
			}
		}
	}

	// Add approval action section with approval tracking logic
	if notifyReq.Approvals > 0 {
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

		remainingApprovals := notifyReq.Approvals - approvedCount

		// Create dynamic message based on approval requirements
		var actionMessage string
		if notifyReq.Approvals == 1 {
			actionMessage = "Action Required:\nOne approval is required. Please review the request and choose an action."
		} else if remainingApprovals <= 0 {
			actionMessage = "Action Required:\nSufficient approvals have been received. Please review the request and choose an action."
		} else if remainingApprovals == 1 {
			actionMessage = fmt.Sprintf("Action Required:\n%d more approval is needed (%d of %d received). Please review the request and choose an action.", remainingApprovals, approvedCount, notifyReq.Approvals)
		} else {
			actionMessage = fmt.Sprintf("Action Required:\n%d more approvals are needed (%d of %d received). Please review the request and choose an action.", remainingApprovals, approvedCount, notifyReq.Approvals)
		}

		plainText.WriteString(fmt.Sprintf("\n%s\n\n", actionMessage))

		// Add action buttons with URLs
		if remainingApprovals > 0 {
			approveURL := a.createCallbackUrl(workflowTask, notifyReq, true)
			denyURL := a.createCallbackUrl(workflowTask, notifyReq, false)
			viewRequestURL := a.createViewRequestUrl(workflowTask)

			plainText.WriteString(fmt.Sprintf("Approve: %s\n", approveURL))
			plainText.WriteString(fmt.Sprintf("Deny: %s\n", denyURL))
			plainText.WriteString(fmt.Sprintf("View Request: %s\n", viewRequestURL))

			// Add URLs to template data
			data["ActionMessage"] = actionMessage
			data["ApproveURL"] = approveURL
			data["DenyURL"] = denyURL
			data["ViewRequestURL"] = viewRequestURL
			data["ShowActions"] = true
		} else {
			data["ActionMessage"] = actionMessage
			data["ShowActions"] = false
		}
	} else {
		plainText.WriteString("\nNo action is required. This is a notification only.\n")
		data["ActionMessage"] = "No action is required. This is a notification only."
		data["ShowActions"] = false
	}

	// Render HTML email using template
	html, err := RenderEmailWithTemplate("Access Request - Approval Required", GetApprovalContentTemplate(), data)
	if err != nil {
		logrus.WithError(err).Error("Failed to render approval email")
		return plainText.String(), ""
	}

	return plainText.String(), html
}
