package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"

	thandProvider "github.com/thand-io/agent/internal/workflows/tasks/providers/thand"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/api/enums/v1"
)

// FormPageData contains data for rendering the form page
type FormPageData struct {
	TemplateData
	WorkflowID   string      `json:"workflow_id"`
	WorkflowName string      `json:"workflow_name"`
	TaskName     string      `json:"task_name"`
	Blocks       []FormBlock `json:"blocks"`
	BlocksJSON   string      `json:"blocks_json"`
	FormTitle    string      `json:"form_title"`
	SubmitLabel  string      `json:"submit_label"`
}

// FormBlock represents a converted Slack block for HTML rendering
type FormBlock struct {
	Type      string            `json:"type"`
	BlockID   string            `json:"block_id"`
	Text      *FormTextObject   `json:"text,omitempty"`
	Fields    []*FormTextObject `json:"fields,omitempty"`
	Label     *FormTextObject   `json:"label,omitempty"`
	Hint      *FormTextObject   `json:"hint,omitempty"`
	Element   *FormElement      `json:"element,omitempty"`
	Elements  []*FormElement    `json:"elements,omitempty"`
	Accessory *FormElement      `json:"accessory,omitempty"`
	Optional  bool              `json:"optional,omitempty"`
	ImageURL  string            `json:"image_url,omitempty"`
	AltText   string            `json:"alt_text,omitempty"`
	Title     *FormTextObject   `json:"title,omitempty"`
}

// FormTextObject represents text content for form elements
type FormTextObject struct {
	Type     string `json:"type"` // "plain_text" or "mrkdwn"
	Text     string `json:"text"`
	Emoji    bool   `json:"emoji,omitempty"`
	Verbatim bool   `json:"verbatim,omitempty"`
}

// FormElement represents an interactive element in the form
type FormElement struct {
	Type             string              `json:"type"`
	ActionID         string              `json:"action_id"`
	Placeholder      *FormTextObject     `json:"placeholder,omitempty"`
	InitialValue     string              `json:"initial_value,omitempty"`
	InitialDate      string              `json:"initial_date,omitempty"`
	InitialTime      string              `json:"initial_time,omitempty"`
	InitialOption    *FormOptionObject   `json:"initial_option,omitempty"`
	InitialOptions   []*FormOptionObject `json:"initial_options,omitempty"`
	Multiline        bool                `json:"multiline,omitempty"`
	MinLength        int                 `json:"min_length,omitempty"`
	MaxLength        int                 `json:"max_length,omitempty"`
	Options          []*FormOptionObject `json:"options,omitempty"`
	OptionGroups     []*FormOptionGroup  `json:"option_groups,omitempty"`
	IsDecimalAllowed bool                `json:"is_decimal_allowed,omitempty"`
	MinValue         string              `json:"min_value,omitempty"`
	MaxValue         string              `json:"max_value,omitempty"`
	ImageURL         string              `json:"image_url,omitempty"`
	AltText          string              `json:"alt_text,omitempty"`
	Text             *FormTextObject     `json:"text,omitempty"`
	URL              string              `json:"url,omitempty"`
	Value            string              `json:"value,omitempty"`
	Style            string              `json:"style,omitempty"`
}

// FormOptionObject represents a selectable option
type FormOptionObject struct {
	Text        *FormTextObject `json:"text"`
	Value       string          `json:"value"`
	Description *FormTextObject `json:"description,omitempty"`
}

// FormOptionGroup represents a group of options
type FormOptionGroup struct {
	Label   *FormTextObject     `json:"label"`
	Options []*FormOptionObject `json:"options"`
}

// FormSubmission represents the submitted form data
type FormSubmission struct {
	WorkflowID string            `json:"workflow_id"`
	TaskName   string            `json:"task_name"`
	Values     map[string]string `json:"values"`
}

// FormValidationError represents a validation error
type FormValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// FormSubmissionResponse is the response for form submission
type FormSubmissionResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message,omitempty"`
	Errors  []FormValidationError `json:"errors,omitempty"`
}

// getFormPage handles GET requests to display the form
func (s *Server) getFormPage(c *gin.Context) {
	workflowID := c.Param("id")

	if len(workflowID) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Workflow ID is required")
		return
	}

	temporal := s.Config.GetServices().GetTemporal()

	if temporal == nil || !temporal.HasClient() {
		s.getErrorPage(c, http.StatusNotImplemented, "Temporal service is not configured")
		return
	}

	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusNotImplemented, "Form display is only available in server mode")
		return
	}

	_, _, err := s.getSession(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: please log in to access this form", err)
		return
	}

	formData, err := s.getFormDataFromWorkflow(c, workflowID)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, err.Error())
		return
	}

	if s.canAcceptHtml(c) {
		s.renderHtml(c, "execution_form.html", formData)
	} else {
		c.JSON(http.StatusOK, formData)
	}
}

// submitForm handles POST requests to submit the form
func (s *Server) submitForm(c *gin.Context) {
	workflowID := c.Param("id")

	if len(workflowID) == 0 {
		c.JSON(http.StatusBadRequest, FormSubmissionResponse{
			Success: false,
			Message: "Workflow ID is required",
		})
		return
	}

	temporal := s.Config.GetServices().GetTemporal()

	if temporal == nil || !temporal.HasClient() {
		c.JSON(http.StatusNotImplemented, FormSubmissionResponse{
			Success: false,
			Message: "Temporal service is not configured",
		})
		return
	}

	if !s.Config.IsServer() {
		c.JSON(http.StatusNotImplemented, FormSubmissionResponse{
			Success: false,
			Message: "Form submission is only available in server mode",
		})
		return
	}

	_, foundUser, err := s.getSession(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, FormSubmissionResponse{
			Success: false,
			Message: "Unauthorized: please log in to submit this form",
		})
		return
	}

	// Parse the form submission
	var submission FormSubmission
	if err := c.ShouldBindJSON(&submission); err != nil {
		c.JSON(http.StatusBadRequest, FormSubmissionResponse{
			Success: false,
			Message: "Invalid form data: " + err.Error(),
		})
		return
	}

	submission.WorkflowID = workflowID

	// Get the current form configuration to validate against
	formData, err := s.getFormDataFromWorkflow(c, workflowID)
	if err != nil {
		c.JSON(http.StatusBadRequest, FormSubmissionResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	// Validate the submission
	validationErrors := s.validateFormSubmission(formData, &submission)
	if len(validationErrors) > 0 {
		c.JSON(http.StatusBadRequest, FormSubmissionResponse{
			Success: false,
			Message: "Validation failed",
			Errors:  validationErrors,
		})
		return
	}

	// Create CloudEvent for signaling the workflow
	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource("thand/form")
	event.SetType(thandProvider.ThandFormEventType)
	event.SetTime(time.Now().UTC())
	event.SetSubject(workflowID)

	// Set user extension
	if foundUser != nil && foundUser.User != nil {
		event.SetExtension(sdkConstants.VarsContextUser,
			foundUser.AsIdentity().EncodeBase64())
	}

	// Set form data
	formDataPayload := map[string]any{
		"workflow_id":  workflowID,
		"task_name":    submission.TaskName,
		"values":       submission.Values,
		"submitted_at": time.Now().UTC().Format(time.RFC3339),
	}

	if foundUser != nil && foundUser.User != nil {
		formDataPayload["submitted_by"] = map[string]string{
			"email": foundUser.User.Email,
			"name":  foundUser.User.Name,
		}
	}

	if err := event.SetData(cloudevents.ApplicationJSON, formDataPayload); err != nil {
		logrus.WithError(err).Error("Failed to set CloudEvent data")
		c.JSON(http.StatusInternalServerError, FormSubmissionResponse{
			Success: false,
			Message: "Failed to prepare form submission",
		})
		return
	}

	// Signal the workflow
	ctx := context.Background()
	temporalClient := temporal.GetClient()

	err = temporalClient.SignalWorkflow(
		ctx, workflowID, sdkWorkflowsModel.TemporalEmptyRunId,
		sdkWorkflowsModel.TemporalEventSignalName, event)

	if err != nil {
		logrus.WithError(err).Error("Failed to signal workflow with form submission")
		c.JSON(http.StatusInternalServerError, FormSubmissionResponse{
			Success: false,
			Message: "Failed to submit form: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, FormSubmissionResponse{
		Success: true,
		Message: "Form submitted successfully",
	})
}

// getFormDataFromWorkflow retrieves form data from the workflow's current state
func (s *Server) getFormDataFromWorkflow(c *gin.Context, workflowID string) (*FormPageData, error) {
	ctx := context.Background()

	temporal := s.Config.GetServices().GetTemporal()
	temporalClient := temporal.GetClient()

	// Get workflow info from typed search attributes
	workflowRun, err := temporalClient.DescribeWorkflow(ctx, workflowID, sdkWorkflowsModel.TemporalEmptyRunId)
	if err != nil {
		return nil, fmt.Errorf("workflow not found: %w", err)
	}

	// Check if workflow is still running
	if workflowRun.Status != enums.WORKFLOW_EXECUTION_STATUS_RUNNING {
		return nil, fmt.Errorf("workflow is not running (status: %s)", workflowRun.Status.String())
	}

	// Get workflow name from search attributes
	workflowName, found := workflowRun.TypedSearchAttributes.GetKeyword(sdkConstants.TypedSearchAttributeWorkflow)
	if !found || len(workflowName) == 0 {
		return nil, fmt.Errorf("workflow name not found in search attributes")
	}

	// Get current task from search attributes
	currentTask, found := workflowRun.TypedSearchAttributes.GetKeyword(sdkConstants.TypedSearchAttributeTask)
	if !found || len(currentTask) == 0 {
		return nil, fmt.Errorf("no active task in workflow")
	}

	// Get the workflow definition from config
	foundWorkflow, err := s.Config.GetWorkflowByName(workflowName)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow definition: %w", err)
	}

	workflowDef := foundWorkflow.GetWorkflow()
	if workflowDef == nil {
		return nil, fmt.Errorf("workflow definition is nil")
	}

	// Get form configuration from the workflow definition task
	formConfig, err := s.extractFormConfig(workflowDef, currentTask)
	if err != nil {
		return nil, err
	}

	// Convert Slack blocks to form blocks
	formBlocks := s.convertSlackBlocksToFormBlocks(formConfig.Blocks)

	// Marshal blocks to JSON for Alpine.js
	blocksJSON, _ := json.Marshal(formBlocks)

	formData := &FormPageData{
		TemplateData: s.GetTemplateData(c),
		WorkflowID:   workflowID,
		WorkflowName: workflowName,
		TaskName:     currentTask,
		Blocks:       formBlocks,
		BlocksJSON:   string(blocksJSON),
		FormTitle:    formConfig.Title,
		SubmitLabel:  formConfig.SubmitLabel,
	}

	if len(formData.FormTitle) == 0 {
		formData.FormTitle = "Form"
	}
	if len(formData.SubmitLabel) == 0 {
		formData.SubmitLabel = "Submit"
	}

	return formData, nil
}

// FormConfig represents the form configuration from the workflow
type FormConfig struct {
	Title       string        `json:"title"`
	SubmitLabel string        `json:"submit_label"`
	Blocks      []slack.Block `json:"blocks"`
}

// extractFormConfig extracts form configuration from workflow definition task
func (s *Server) extractFormConfig(workflowDef *model.Workflow, currentTaskName string) (*FormConfig, error) {
	if workflowDef == nil || workflowDef.Do == nil {
		return nil, fmt.Errorf("workflow definition or task list is nil")
	}

	// Find the current task in the workflow's Do list
	_, taskItem := workflowDef.Do.KeyAndIndex(currentTaskName)
	if taskItem == nil {
		return nil, fmt.Errorf("task '%s' not found in workflow definition", currentTaskName)
	}

	// Check if the task is a thand task with form type
	task := taskItem.Task
	if task == nil {
		return nil, fmt.Errorf("task definition is nil for task '%s'", currentTaskName)
	}

	// Try to cast to ThandTask
	thandTaskDef, ok := task.(*taskModel.ThandTask)
	if !ok {
		return nil, fmt.Errorf("task '%s' is not a thand task", currentTaskName)
	}

	// Check if it's a form task
	if thandTaskDef.Thand != thandProvider.ThandFormTask {
		return nil, fmt.Errorf("task '%s' is not a form task (type: %s)", currentTaskName, thandTaskDef.Thand)
	}

	// Extract the form configuration from the 'with' field
	if thandTaskDef.With == nil {
		return nil, fmt.Errorf("form task '%s' has no 'with' configuration", currentTaskName)
	}

	// Parse the FormTask from the with configuration
	var formTask thandProvider.FormTask
	if err := common.ConvertInterfaceToInterface(thandTaskDef.With, &formTask); err != nil {
		return nil, fmt.Errorf("failed to parse form task configuration: %w", err)
	}

	if !formTask.IsValid() {
		return nil, fmt.Errorf("form task '%s' has no blocks defined", currentTaskName)
	}

	return &FormConfig{
		Title:       formTask.Title,
		SubmitLabel: formTask.SubmitLabel,
		Blocks:      formTask.Blocks.BlockSet,
	}, nil
}

// convertSlackBlocksToFormBlocks converts Slack blocks to HTML-friendly form blocks
func (s *Server) convertSlackBlocksToFormBlocks(blocks []slack.Block) []FormBlock {
	var formBlocks []FormBlock

	for _, block := range blocks {
		formBlock := s.convertSlackBlock(block)
		if formBlock != nil {
			formBlocks = append(formBlocks, *formBlock)
		}
	}

	return formBlocks
}

// convertSlackBlock converts a single Slack block to a FormBlock
func (s *Server) convertSlackBlock(block slack.Block) *FormBlock {
	if block == nil {
		return nil
	}

	formBlock := &FormBlock{
		Type:    string(block.BlockType()),
		BlockID: block.ID(),
	}

	switch b := block.(type) {
	case *slack.SectionBlock:
		if b.Text != nil {
			formBlock.Text = convertTextBlockObject(b.Text)
		}
		if len(b.Fields) > 0 {
			formBlock.Fields = make([]*FormTextObject, len(b.Fields))
			for i, field := range b.Fields {
				formBlock.Fields[i] = convertTextBlockObject(field)
			}
		}
		if b.Accessory != nil {
			formBlock.Accessory = convertAccessory(b.Accessory)
		}

	case *slack.InputBlock:
		if b.Label != nil {
			formBlock.Label = convertTextBlockObject(b.Label)
		}
		if b.Hint != nil {
			formBlock.Hint = convertTextBlockObject(b.Hint)
		}
		formBlock.Optional = b.Optional
		if b.Element != nil {
			formBlock.Element = convertBlockElement(b.Element)
		}

	case *slack.ActionBlock:
		if b.Elements != nil {
			formBlock.Elements = make([]*FormElement, 0)
			for _, elem := range b.Elements.ElementSet {
				if converted := convertBlockElement(elem); converted != nil {
					formBlock.Elements = append(formBlock.Elements, converted)
				}
			}
		}

	case *slack.DividerBlock:
		// Divider has no additional properties

	case *slack.HeaderBlock:
		if b.Text != nil {
			formBlock.Text = convertTextBlockObject(b.Text)
		}

	case *slack.ContextBlock:
		formBlock.Elements = make([]*FormElement, 0)
		for _, elem := range b.ContextElements.Elements {
			if converted := convertMixedElement(elem); converted != nil {
				formBlock.Elements = append(formBlock.Elements, converted)
			}
		}

	case *slack.ImageBlock:
		formBlock.ImageURL = b.ImageURL
		formBlock.AltText = b.AltText
		if b.Title != nil {
			formBlock.Title = convertTextBlockObject(b.Title)
		}
	}

	return formBlock
}

// convertTextBlockObject converts a Slack TextBlockObject to FormTextObject
func convertTextBlockObject(obj *slack.TextBlockObject) *FormTextObject {
	if obj == nil {
		return nil
	}
	emoji := false
	if obj.Emoji != nil {
		emoji = *obj.Emoji
	}
	return &FormTextObject{
		Type:     obj.Type,
		Text:     obj.Text,
		Emoji:    emoji,
		Verbatim: obj.Verbatim,
	}
}

// convertBlockElement converts a Slack BlockElement to FormElement
func convertBlockElement(elem slack.BlockElement) *FormElement {
	if elem == nil {
		return nil
	}

	formElem := &FormElement{
		Type: string(elem.ElementType()),
	}

	switch e := elem.(type) {
	case *slack.PlainTextInputBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.InitialValue = e.InitialValue
		formElem.Multiline = e.Multiline
		formElem.MinLength = e.MinLength
		formElem.MaxLength = e.MaxLength

	case *slack.SelectBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.Options = convertOptions(e.Options)
		formElem.OptionGroups = convertOptionGroups(e.OptionGroups)
		if e.InitialOption != nil {
			formElem.InitialOption = convertOption(e.InitialOption)
		}

	case *slack.MultiSelectBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.Options = convertOptions(e.Options)
		formElem.OptionGroups = convertOptionGroups(e.OptionGroups)
		formElem.InitialOptions = convertOptions(e.InitialOptions)

	case *slack.DatePickerBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.InitialDate = e.InitialDate

	case *slack.TimePickerBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.InitialTime = e.InitialTime

	case *slack.CheckboxGroupsBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Options = convertOptions(e.Options)
		formElem.InitialOptions = convertOptions(e.InitialOptions)

	case *slack.RadioButtonsBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Options = convertOptions(e.Options)
		if e.InitialOption != nil {
			formElem.InitialOption = convertOption(e.InitialOption)
		}

	case *slack.NumberInputBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.InitialValue = e.InitialValue
		formElem.IsDecimalAllowed = e.IsDecimalAllowed
		formElem.MinValue = e.MinValue
		formElem.MaxValue = e.MaxValue

	case *slack.EmailTextInputBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.InitialValue = e.InitialValue

	case *slack.URLTextInputBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Placeholder = convertTextBlockObject(e.Placeholder)
		formElem.InitialValue = e.InitialValue

	case *slack.ButtonBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Text = convertTextBlockObject(e.Text)
		formElem.URL = e.URL
		formElem.Value = e.Value
		formElem.Style = string(e.Style)

	case *slack.ImageBlockElement:
		if e.ImageURL != nil {
			formElem.ImageURL = *e.ImageURL
		}
		formElem.AltText = e.AltText

	case *slack.OverflowBlockElement:
		formElem.ActionID = e.ActionID
		formElem.Options = convertOptions(e.Options)
	}

	return formElem
}

// convertMixedElement converts a Slack MixedElement to FormElement
func convertMixedElement(elem slack.MixedElement) *FormElement {
	if elem == nil {
		return nil
	}

	switch e := elem.(type) {
	case *slack.TextBlockObject:
		return &FormElement{
			Type: e.Type,
			Text: convertTextBlockObject(e),
		}
	case *slack.ImageBlockElement:
		var imageURL string
		if e.ImageURL != nil {
			imageURL = *e.ImageURL
		}
		return &FormElement{
			Type:     string(e.Type),
			ImageURL: imageURL,
			AltText:  e.AltText,
		}
	}

	return nil
}

// convertAccessory converts a Slack Accessory to FormElement
func convertAccessory(acc *slack.Accessory) *FormElement {
	if acc == nil {
		return nil
	}

	if acc.ButtonElement != nil {
		return convertBlockElement(acc.ButtonElement)
	}
	if acc.ImageElement != nil {
		return convertBlockElement(acc.ImageElement)
	}
	if acc.SelectElement != nil {
		return convertBlockElement(acc.SelectElement)
	}
	if acc.MultiSelectElement != nil {
		return convertBlockElement(acc.MultiSelectElement)
	}
	if acc.DatePickerElement != nil {
		return convertBlockElement(acc.DatePickerElement)
	}
	if acc.TimePickerElement != nil {
		return convertBlockElement(acc.TimePickerElement)
	}
	if acc.CheckboxGroupsBlockElement != nil {
		return convertBlockElement(acc.CheckboxGroupsBlockElement)
	}
	if acc.RadioButtonsElement != nil {
		return convertBlockElement(acc.RadioButtonsElement)
	}
	if acc.OverflowElement != nil {
		return convertBlockElement(acc.OverflowElement)
	}

	return nil
}

// convertOptions converts Slack OptionBlockObjects to FormOptionObjects
func convertOptions(options []*slack.OptionBlockObject) []*FormOptionObject {
	if options == nil {
		return nil
	}

	result := make([]*FormOptionObject, len(options))
	for i, opt := range options {
		result[i] = convertOption(opt)
	}
	return result
}

// convertOption converts a single Slack OptionBlockObject to FormOptionObject
func convertOption(opt *slack.OptionBlockObject) *FormOptionObject {
	if opt == nil {
		return nil
	}
	return &FormOptionObject{
		Text:        convertTextBlockObject(opt.Text),
		Value:       opt.Value,
		Description: convertTextBlockObject(opt.Description),
	}
}

// convertOptionGroups converts Slack OptionGroupBlockObjects to FormOptionGroups
func convertOptionGroups(groups []*slack.OptionGroupBlockObject) []*FormOptionGroup {
	if groups == nil {
		return nil
	}

	result := make([]*FormOptionGroup, len(groups))
	for i, grp := range groups {
		result[i] = &FormOptionGroup{
			Label:   convertTextBlockObject(grp.Label),
			Options: convertOptions(grp.Options),
		}
	}
	return result
}

// validateFormSubmission validates the submitted form data against the form configuration
func (s *Server) validateFormSubmission(formData *FormPageData, submission *FormSubmission) []FormValidationError {
	var errors []FormValidationError

	// Validate that the submitted task name matches the current active task
	if submission.TaskName != formData.TaskName {
		errors = append(errors, FormValidationError{
			Field:   "task_name",
			Message: fmt.Sprintf("Invalid task name: expected '%s', got '%s'", formData.TaskName, submission.TaskName),
		})
		// Return early - if task name doesn't match, other validation is meaningless
		return errors
	}

	// Build a map of required fields and their validation rules
	for _, block := range formData.Blocks {
		if block.Type != "input" {
			continue
		}

		if block.Element == nil {
			continue
		}

		actionID := block.Element.ActionID
		if len(actionID) == 0 {
			continue
		}

		value, exists := submission.Values[actionID]

		// Check required fields
		if !block.Optional && (!exists || len(value) == 0) {
			label := "Field"
			if block.Label != nil {
				label = block.Label.Text
			}
			errors = append(errors, FormValidationError{
				Field:   actionID,
				Message: fmt.Sprintf("%s is required", label),
			})
			continue
		}

		// Skip further validation if field is empty and optional
		if !exists || len(value) == 0 {
			continue
		}

		// Validate based on element type
		switch block.Element.Type {
		case "plain_text_input":
			if block.Element.MinLength > 0 && len(value) < block.Element.MinLength {
				errors = append(errors, FormValidationError{
					Field:   actionID,
					Message: fmt.Sprintf("Minimum length is %d characters", block.Element.MinLength),
				})
			}
			if block.Element.MaxLength > 0 && len(value) > block.Element.MaxLength {
				errors = append(errors, FormValidationError{
					Field:   actionID,
					Message: fmt.Sprintf("Maximum length is %d characters", block.Element.MaxLength),
				})
			}

		case "email_text_input":
			if !common.IsValidEmail(value) {
				errors = append(errors, FormValidationError{
					Field:   actionID,
					Message: "Please enter a valid email address",
				})
			}

		case "url_text_input":
			if !common.IsValidURL(value) {
				errors = append(errors, FormValidationError{
					Field:   actionID,
					Message: "Please enter a valid URL",
				})
			}

		case "number_input":
			if !common.IsValidNumber(value, block.Element.IsDecimalAllowed) {
				errors = append(errors, FormValidationError{
					Field:   actionID,
					Message: "Please enter a valid number",
				})
			} else if rangeErr := common.ValidateNumberRange(value, block.Element.MinValue, block.Element.MaxValue); len(rangeErr) != 0 {
				errors = append(errors, FormValidationError{
					Field:   actionID,
					Message: rangeErr,
				})
			}
		}
	}

	return errors
}
