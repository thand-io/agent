package thand

import (
	"encoding/json"
	"errors"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	runner "github.com/thand-io/agent/internal/workflows/runner"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
)

const ThandFormTask = "form"

// FormTask defines the configuration for a form task
type FormTask struct {
	// Form metadata
	Title       string `json:"title,omitempty"`        // Title of the form
	Description string `json:"description,omitempty"`  // Description or instructions for the form
	SubmitLabel string `json:"submit_label,omitempty"` // Custom label for the submit button

	// Form definition using Slack Block Kit
	slack.Blocks
	// Where to send the form notification
	Notifiers map[string]thandFunction.NotifierRequest `json:"notifiers"` // Notifier configurations
}

// UnmarshalJSON custom unmarshaler to handle blocks array at top level
func (f *FormTask) UnmarshalJSON(data []byte) error {
	// First unmarshal into a temporary struct for the simple fields
	type formTaskAlias struct {
		Title       string                                   `json:"title,omitempty"`
		Description string                                   `json:"description,omitempty"`
		SubmitLabel string                                   `json:"submit_label,omitempty"`
		Blocks      json.RawMessage                          `json:"blocks,omitempty"`
		Notifiers   map[string]thandFunction.NotifierRequest `json:"notifiers"`
	}

	var alias formTaskAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	f.Title = alias.Title
	f.Description = alias.Description
	f.SubmitLabel = alias.SubmitLabel
	f.Notifiers = alias.Notifiers

	// Now unmarshal the blocks using slack.Blocks custom unmarshaler
	if len(alias.Blocks) > 0 {
		var blocks slack.Blocks
		if err := json.Unmarshal(alias.Blocks, &blocks); err != nil {
			return fmt.Errorf("failed to unmarshal blocks: %w", err)
		}
		f.Blocks = blocks
	}

	return nil
}

// MarshalJSON custom marshaler to output blocks array at top level
func (f FormTask) MarshalJSON() ([]byte, error) {
	type formTaskAlias struct {
		Title       string                                   `json:"title,omitempty"`
		Description string                                   `json:"description,omitempty"`
		SubmitLabel string                                   `json:"submit_label,omitempty"`
		Blocks      []slack.Block                            `json:"blocks,omitempty"`
		Notifiers   map[string]thandFunction.NotifierRequest `json:"notifiers,omitempty"`
	}

	alias := formTaskAlias{
		Title:       f.Title,
		Description: f.Description,
		SubmitLabel: f.SubmitLabel,
		Blocks:      f.Blocks.BlockSet,
		Notifiers:   f.Notifiers,
	}

	return json.Marshal(alias)
}

func (f *FormTask) IsValid() bool {
	return len(f.Blocks.BlockSet) > 0
}

func (f *FormTask) HasNotifiers() bool {
	return len(f.Notifiers) > 0
}

func (f *FormTask) AsMap() map[string]any {
	response, err := common.ConvertInterfaceToMap(f)
	if err != nil {
		panic(fmt.Sprintf("failed to convert FormTask to map: %v", err))
	}
	return response
}

/*
Thand Forms are designed to collect structured input from users during workflow executions.
Slack blocks will be used to create the interactive form elements presented to users.

For Slack: The blocks are sent directly as an interactive form
For Email: A link to the HTML form page is sent
*/
func (t *thandTask) executeFormTask(
	workflowTask *models.WorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
) (any, error) {

	log := workflowTask.GetLogger()

	elevationRequest, err := workflowTask.GetContextAsElevationRequest()
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to get elevation request from context")
		return nil, err
	}

	// Parse the form task configuration
	var formTask FormTask
	err = common.ConvertInterfaceToInterface(call.With, &formTask)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to parse form task configuration")
		return nil, err
	}

	if !formTask.IsValid() {
		return nil, errors.New("invalid form configuration: no blocks defined")
	}

	// Get input to check if we're resuming
	input := workflowTask.GetInput()

	if common.IsNilOrZero(input) {
		log.WithField("taskName", taskName).Info("Starting Thand form task")

		// Update call configuration
		newConfig := &models.BasicConfig{}
		newConfig.Update(formTask.AsMap())
		call.With = newConfig

		// Send notifications if configured
		if formTask.HasNotifiers() {
			err = t.makeFormNotifications(
				workflowTask,
				taskName,
				&formTask,
				elevationRequest,
			)
			if err != nil {
				log.WithError(err).WithFields(logrus.Fields{
					"taskName": taskName,
				}).Error("Failed to send form notifications")
				return nil, err
			}
		}
	} else {
		log.WithField("taskName", taskName).Info("Resuming Thand form task")
	}

	log.WithField("taskName", taskName).Info("Waiting for form submission")

	// Listen for form submission event
	formSubmission, err := runner.ListenTaskHandler(
		workflowTask, fmt.Sprintf("%s.listen", taskName), &model.ListenTask{
			Listen: model.ListenTaskConfiguration{
				To: &model.EventConsumptionStrategy{
					Any: []*model.EventFilter{
						{
							With: &model.EventProperties{
								Type: ThandFormEventType,
							},
						},
					},
				},
			},
		}, input)

	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to listen for form submission event")
		return nil, err
	}

	// Process the form submission
	var formData map[string]any

	if formEvent, ok := formSubmission.(*cloudevents.Event); ok {
		formEvent.DataAs(&formData)
		extensions := formEvent.Extensions()

		userIdentity, userExists := extensions[models.VarsContextUser].(string)
		if userExists {
			log.WithFields(logrus.Fields{
				"taskName": taskName,
				"user":     userIdentity,
			}).Info("Form submitted by user")
		}

		// Store form submission data in context
		workflowTask.SetContextKeyValue("form_submission", map[string]any{
			"values":       formData["values"],
			"submitted_by": userIdentity,
			"submitted_at": formData["submitted_at"],
		})
	}

	log.WithFields(logrus.Fields{
		"taskName": taskName,
	}).Info("Completed Thand form task")

	// Return the form data as output
	return formData, nil
}

// makeFormNotifications sends form notifications to all configured notifiers
func (t *thandTask) makeFormNotifications(
	workflowTask *models.WorkflowTask,
	taskName string,
	formTask *FormTask,
	elevationRequest *models.ElevateRequestInternal,
) error {

	var notifyTasks []notifyTask

	for providerKey, notifierRequest := range formTask.Notifiers {
		// Create a FormNotifier for each provider
		formNotifier := NewFormNotifier(
			t.config,
			workflowTask,
			elevationRequest,
			&FormNotifierConfig{
				Title:       formTask.Title,
				Description: formTask.Description,
				SubmitLabel: formTask.SubmitLabel,
				Blocks:      formTask.Blocks.BlockSet,
				Notifier:    notifierRequest,
				Entrypoint:  taskName,
			},
		)

		// Get recipients for this notifier
		recipients := formNotifier.GetRecipients()

		// Build notification tasks for each recipient
		for _, recipientId := range recipients {

			recipientIdentity := t.resolveIdentity(recipientId)

			if recipientIdentity == nil {
				logrus.WithField("recipient", recipientId).Warn("Skipping form notification, identity not found")
				continue
			}

			recipientIdentity.ID = recipientId
			recipientPayload := formNotifier.GetPayload(recipientIdentity)

			notifyTasks = append(notifyTasks, notifyTask{
				Recipient: recipientId,
				CallFunc:  formNotifier.GetCallFunction(recipientIdentity),
				Payload:   recipientPayload,
				Provider:  formNotifier.GetProviderName(),
			})

			logrus.WithFields(logrus.Fields{
				"recipient":   recipientId,
				"provider":    formNotifier.GetProviderName(),
				"providerKey": providerKey,
			}).Debug("Prepared form notification task")
		}
	}

	// Execute all notifications in parallel
	var err error
	var notifyResults []notifyResult

	if workflowTask.HasTemporalContext() {
		notifyResults, err = t.executeNotifyTemporalParallel(workflowTask, fmt.Sprintf("%s.notify", taskName), notifyTasks)
	} else {
		notifyResults, err = t.executeNotifyGoParallel(workflowTask, notifyTasks)
	}

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to execute form notifications")
		return err
	}

	// Process results
	if err := processNotificationResults(notifyResults, "Form notification"); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to process form notification results")
		return err
	}

	return nil
}
