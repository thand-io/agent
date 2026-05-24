package thand

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// createRevokeEmailBody creates the email body for revocation notification
func (r *revokeNotifier) createRevokeEmailBody() (string, string) {
	elevationReq := r.elevationReq
	notifyReq := r.req

	// Build plain text version
	var plainText strings.Builder
	plainText.WriteString("Your access has been revoked.\n\n")

	if elevationReq.Role != nil {
		fmt.Fprintf(&plainText, "Role: %s\n", elevationReq.Role.Name)
		if len(elevationReq.Role.Description) > 0 {
			fmt.Fprintf(&plainText, "Description: %s\n", elevationReq.Role.Description)
		}
	}

	if len(elevationReq.Providers) > 0 {
		fmt.Fprintf(&plainText, "Providers: %s\n", strings.Join(elevationReq.Providers, ", "))
	}

	if len(elevationReq.Duration) > 0 {
		fmt.Fprintf(&plainText, "Duration: %s\n", elevationReq.Duration)
	}

	if elevationReq.Role != nil && len(elevationReq.Role.Permissions.Allow) > 0 {
		plainText.WriteString("\nRevoked Permissions:\n")
		for _, stmt := range sortedStatementsWithSortedFields(elevationReq.Role.Permissions.Allow) {
			if len(stmt.Operations) > 0 {
				fmt.Fprintf(&plainText, "- Operations: %s\n", strings.Join(stmt.Operations, ", "))
			}
			if len(stmt.Binding) > 0 {
				fmt.Fprintf(&plainText, "  Binding: %s\n", stmt.Binding)
			}
			if len(stmt.Targets) > 0 {
				fmt.Fprintf(&plainText, "  Targets: %s\n", strings.Join(stmt.Targets, ", "))
			}
		}
	}

	plainText.WriteString("\nYour access has been successfully revoked. If you need access again, please submit a new request.")

	// Build data map for template
	data := map[string]any{
		"Providers": strings.Join(elevationReq.Providers, ", "),
		"Duration":  elevationReq.Duration,
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
			data["Permissions"] = sortedStatementsWithSortedFields(elevationReq.Role.Permissions.Allow)
		}
	}

	// Render HTML email using template
	html, err := RenderEmailWithTemplate("Access Revoked", GetRevokeContentTemplate(), data)
	if err != nil {
		logrus.WithError(err).Error("Failed to render revocation email")
		return plainText.String(), ""
	}

	return plainText.String(), html
}
