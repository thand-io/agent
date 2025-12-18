package thand

import (
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/models"
)

// createFormSlackBlocks creates Slack blocks for the form notification
// For Slack, we send the form blocks directly along with a link to complete the form
func (f *formNotifier) createFormSlackBlocks() []slack.Block {
	workflowTask := f.workflowTask

	blocks := []slack.Block{}

	// Add header section with form title
	f.addFormHeaderSection(&blocks)

	// Add description if provided
	f.addFormDescriptionSection(&blocks)

	// Add divider before form preview
	blocks = append(blocks, slack.NewDividerBlock())

	// Add the form fields preview
	f.addFormBlocksSection(&blocks)

	// Add divider before action section
	blocks = append(blocks, slack.NewDividerBlock())

	// Add action section with link to HTML form
	f.addFormActionSection(&blocks, workflowTask)

	return blocks
}

// addFormHeaderSection adds the form title header
func (f *formNotifier) addFormHeaderSection(blocks *[]slack.Block) {
	title := f.req.Title
	if len(title) == 0 {
		title = "Form Request"
	}

	*blocks = append(*blocks, slack.NewHeaderBlock(
		slack.NewTextBlockObject(
			slack.PlainTextType,
			fmt.Sprintf("📝 %s", title),
			true,
			false,
		),
	))
}

// addFormDescriptionSection adds the form description if provided
func (f *formNotifier) addFormDescriptionSection(blocks *[]slack.Block) {
	if len(f.req.Description) > 0 {
		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				f.req.Description,
				false,
				false,
			),
			nil,
			nil,
		))
	}

	// Add custom message from notifier if provided
	if len(f.req.Notifier.Message) > 0 {
		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(
				slack.MarkdownType,
				f.req.Notifier.Message,
				false,
				false,
			),
			nil,
			nil,
		))
	}
}

// addFormBlocksSection adds a preview of the form fields
func (f *formNotifier) addFormBlocksSection(blocks *[]slack.Block) {
	// Add a summary of what fields are in the form
	if len(f.req.Blocks) > 0 {
		var fieldsText strings.Builder
		fieldsText.WriteString("*Form Fields:*\n")

		fieldCount := 0
		for _, block := range f.req.Blocks {
			switch b := block.(type) {
			case *slack.InputBlock:
				fieldCount++
				label := "Field"
				if b.Label != nil {
					label = b.Label.Text
				}
				optional := ""
				if b.Optional {
					optional = " _(optional)_"
				}
				fieldsText.WriteString(fmt.Sprintf("• %s%s\n", label, optional))
			case *slack.SectionBlock:
				if b.Text != nil && len(b.Text.Text) > 0 {
					// Show section text (might be instructions)
					if fieldCount == 0 {
						fieldsText.WriteString(fmt.Sprintf("_%s_\n", truncateText(b.Text.Text, 100)))
					}
				}
			}
		}

		if fieldCount > 0 {
			*blocks = append(*blocks, slack.NewSectionBlock(
				slack.NewTextBlockObject(
					slack.MarkdownType,
					fieldsText.String(),
					false,
					false,
				),
				nil,
				nil,
			))
		}
	}
}

// addFormActionSection adds the action button to open the form
func (f *formNotifier) addFormActionSection(blocks *[]slack.Block, workflowTask *models.WorkflowTask) {
	submitLabel := f.req.SubmitLabel
	if len(submitLabel) == 0 {
		submitLabel = "Open Form"
	}

	// Create the form URL
	formURL := f.createFormUrl(workflowTask)

	*blocks = append(*blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject(
			slack.MarkdownType,
			"*Action Required:* Please complete the form to continue the workflow.",
			false,
			false,
		),
		nil,
		nil,
	))

	*blocks = append(*blocks, slack.NewActionBlock(
		"form_action",
		slack.NewButtonBlockElement(
			fmt.Sprintf(
				"%s-%s-%s",
				f.workflowTask.WorkflowID,
				f.workflowTask.GetTaskName(),
				"open_form",
			),
			"open_form",
			slack.NewTextBlockObject(
				slack.PlainTextType,
				fmt.Sprintf("📝 %s", submitLabel),
				true,
				false,
			),
		).WithURL(formURL).WithStyle(slack.StylePrimary),
	))
}

// createFormUrl creates the URL to the HTML form page
func (f *formNotifier) createFormUrl(workflowTask *models.WorkflowTask) string {
	// Get the form page URL from config
	// The form page is at /execution/:workflowId/form
	baseURL := f.config.GetLoginServerUrl()
	if len(baseURL) == 0 {
		baseURL = f.config.GetLocalServerUrl()
	}

	return fmt.Sprintf("%s/execution/%s/form", baseURL, workflowTask.WorkflowID)
}

// truncateText truncates text to a maximum length with ellipsis
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}
