package thand

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/models"
)

// createFormEmailBody creates the email body for form requests
// For email, we send a link to the HTML form page
func (f *formNotifier) createFormEmailBody(toIdentity *models.Identity) (string, string) {
	workflowTask := f.workflowTask

	// Create the form URL
	formURL := f.createFormUrl(workflowTask)

	// Build plain text version
	var plainText strings.Builder
	plainText.WriteString("A form requires your input.\n\n")

	if len(f.req.Title) > 0 {
		plainText.WriteString(fmt.Sprintf("Form: %s\n", f.req.Title))
	}

	if len(f.req.Description) > 0 {
		plainText.WriteString(fmt.Sprintf("Description: %s\n", f.req.Description))
	}

	// Add form fields summary
	plainText.WriteString("\nForm Fields:\n")
	for _, block := range f.req.Blocks {
		if inputBlock, ok := block.(*slack.InputBlock); ok {
			label := "Field"
			if inputBlock.Label != nil {
				label = inputBlock.Label.Text
			}
			optional := ""
			if inputBlock.Optional {
				optional = " (optional)"
			}
			plainText.WriteString(fmt.Sprintf("- %s%s\n", label, optional))
		}
	}

	plainText.WriteString(fmt.Sprintf("\nPlease complete the form at: %s\n", formURL))

	// Build data map for template
	data := map[string]any{
		"FormURL":     formURL,
		"Title":       f.req.Title,
		"Description": f.req.Description,
		"SubmitLabel": f.req.SubmitLabel,
	}

	if len(f.req.Notifier.Message) > 0 {
		data["Message"] = f.req.Notifier.Message
	}

	// Build form fields list
	var formFields []map[string]any
	for _, block := range f.req.Blocks {
		if inputBlock, ok := block.(*slack.InputBlock); ok {
			field := map[string]any{
				"Optional": inputBlock.Optional,
			}
			if inputBlock.Label != nil {
				field["Label"] = inputBlock.Label.Text
			}
			if inputBlock.Hint != nil {
				field["Hint"] = inputBlock.Hint.Text
			}
			formFields = append(formFields, field)
		}
	}
	data["FormFields"] = formFields

	// Render HTML email using template
	html, err := RenderEmailWithTemplate(f.getEmailSubject(), GetFormContentTemplate(), data)
	if err != nil {
		logrus.WithError(err).Error("Failed to render form email")
		return plainText.String(), ""
	}

	return plainText.String(), html
}
