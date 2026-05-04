package thand

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/workflow"
)

const ThandNotifyTask = "notify"
const ThandApprovalEventType = "com.thand.approval"
const ThandFormEventType = "com.thand.form"

// notifyResult holds the result of a notification operation
type notifyResult struct {
	Recipient string
	Error     error
}

// notifyTask represents a notification task with all necessary context
type notifyTask struct {
	ProviderName string
	Recipient    string
	CallFunc     model.CallFunction
	Payload      models.NotificationRequest
}

// temporalNotifyResult represents the result of a notification operation for temporal communication
type temporalNotifyResult struct {
	Index     int
	Recipient string
	Err       error
}

func (t *thandTask) executeNotifyTask(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
) (any, error) {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	var notifyReq thandFunction.NotifierRequest
	err := common.ConvertInterfaceToInterface(call.With, &notifyReq)

	if err != nil {
		return nil, fmt.Errorf("failed to parse notification request: %w", err)
	}

	if !notifyReq.IsValid() {
		return nil, errors.New("invalid notification request")
	}

	notifyImpl := NewDefaultNotifierImpl(notifyReq)

	return t.executeNotify(workflowTask, taskName, notifyImpl)

}

func (t *thandTask) executeNotify(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	notify NotifierImpl,
) (any, error) {

	log := workflowTask.GetLogger()

	// Caller with to: will either be a []string
	recipients := notify.GetRecipients()

	if len(recipients) == 0 {
		return nil, errors.New("notifier 'to' field cannot be empty")
	}

	log.WithFields(logrus.Fields{
		"recipients": recipients,
		"count":      len(recipients),
	}).Info("Preparing to send notifications")

	// Build notification tasks for each recipient
	var notifyTasks []notifyTask
	for _, recipientId := range recipients {

		recipientIdentity := t.resolveIdentity(recipientId)

		if recipientIdentity == nil {
			log.WithField("recipient", recipientId).
				Error("Failed to resolve recipient identity")
			continue
		}

		recipientIdentity.ID = recipientId
		recipientPayload := notify.GetPayload(recipientIdentity)

		notifyTasks = append(notifyTasks, notifyTask{
			Recipient:    recipientId,
			CallFunc:     notify.GetCallFunction(recipientIdentity),
			Payload:      recipientPayload,
			ProviderName: notify.GetProviderName(),
		})

		log.WithFields(logrus.Fields{
			"recipient": recipientId,
			"provider":  notify.GetProviderName(),
		}).Debug("Prepared notification task")
	}

	// Execute notifications in parallel
	var notifyResults []notifyResult
	var err error

	if workflowTask.HasTemporalContext() {
		notifyResults, err = t.executeNotifyTemporalParallel(workflowTask, taskName, notifyTasks)
	} else {
		notifyResults, err = t.executeNotifyGoParallel(workflowTask, notifyTasks)
	}

	if err != nil {
		log.WithError(err).Error("Failed to execute notification tasks")
		return nil, err
	}

	// Process results
	hasErrors := false
	successCount := 0

	for _, result := range notifyResults {
		if result.Error != nil {
			log.WithError(result.Error).
				WithField("recipient", result.Recipient).
				Error("Notification failed")
			hasErrors = true
		} else {
			successCount++
			log.WithField("recipient", result.Recipient).
				Info("Notification sent successfully")
		}
	}

	if hasErrors && successCount == 0 {
		return nil, fmt.Errorf("all notification requests failed")
	}

	if hasErrors {
		log.WithFields(logrus.Fields{
			"success": successCount,
			"total":   len(notifyResults),
		}).Warn("Some notifications failed")
	}

	return nil, nil
}

func hasMatchingProvider(notificationReq thandFunction.NotifierRequest, notifierProviders map[string]models.Provider) bool {

	// filter out providers to see if the name matches
	for _, provider := range notifierProviders {
		if strings.Compare(provider.GetName(), notificationReq.Provider) == 0 {
			return true
		} else if strings.Compare(provider.GetProvider(), notificationReq.Provider) == 0 {
			return true
		} else if strings.Compare(provider.GetIdentifier(), notificationReq.Provider) == 0 {
			return true
		}
	}

	return false
}

// When a Temporal context is available, it dispatches a child workflow using
// the parent workflow's task queue (typically the agent identity), assuming
// the provider is registered on that worker. Otherwise it falls back to local
// provider execution.
func (t *thandTask) runNotifyTask(
	ctx workflow.Context,
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	task notifyTask,
) notifyResult {

	// CRITICAL: YOU CANNOT CALL PROVIDERS directly from
	// temporal.  You must dispatch a child workflow to
	// directly to servers that have the provider registered.  This is because the workflow may be running on a different worker

	// from their we can figure out what task queues to execute on

	// Temporal path: dispatch a child workflow to the agent with this provider
	if workflowTask.HasTemporalContext() {
		wfName := models.CreateTemporalProviderWorkflowName(
			task.ProviderName, models.TemporalNotifyWorkflowName)

		// Create unique child workflow ID using hash of composite identifier
		// (provider + role + identity + tenant) to ensure uniqueness across
		// different identities/tenants requesting the same role
		childOpts := workflow.ChildWorkflowOptions{
			WorkflowID: models.CreateChildWorkflowID(
				workflowTask.GetWorkflowID(),
				models.TemporalNotifyWorkflowName,
				task.ProviderName,
				task.Recipient,
			),
			TaskQueue: workflowTask.GetTaskQueue(),
		}
		ctx = workflow.WithChildOptions(ctx, childOpts)

		req := models.WorkflowNotifyRequest{
			Recipient: task.Recipient,
			Payload:   task.Payload,
		}
		err := workflow.ExecuteChildWorkflow(ctx, wfName, req).Get(ctx, nil)
		if err != nil {
			return notifyResult{
				Recipient: task.Recipient,
				Error:     err,
			}
		}
		return notifyResult{
			Recipient: task.Recipient,
			Error:     nil,
		}
	}

	// Non-Temporal fallback: execute locally
	providerCall, err := t.config.GetProviderByName(task.ProviderName)
	if err != nil {
		return notifyResult{
			Recipient: task.Recipient,
			Error:     fmt.Errorf("failed to get provider: %w", err),
		}
	}

	err = providerCall.SendNotification(
		workflowTask.GetContext(),
		task.Payload,
	)
	return notifyResult{
		Recipient: task.Recipient,
		Error:     err,
	}
}

// executeNotifyTemporalParallel executes notification tasks in parallel using Temporal
func (t *thandTask) executeNotifyTemporalParallel(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	notifyTasks []notifyTask,
) ([]notifyResult, error) {

	logrus.WithFields(logrus.Fields{
		"taskName":  taskName,
		"taskCount": len(notifyTasks),
	}).Info("Starting executeNotifyTemporalParallel")

	temporalContext := workflowTask.GetTemporalContext()

	// Create channel and results slice
	results := make([]notifyResult, len(notifyTasks))
	resultCh := workflow.NewChannel(temporalContext)

	// Start all tasks in parallel using workflow.Go
	for i, task := range notifyTasks {
		taskIndex := i
		taskForGoroutine := task

		logrus.WithFields(logrus.Fields{
			"taskIndex": taskIndex,
			"recipient": taskForGoroutine.Recipient,
			"provider":  taskForGoroutine.ProviderName,
		}).Info("Scheduling notify activity via workflow.Go")

		workflow.Go(temporalContext, func(ctx workflow.Context) {

			log := workflow.GetLogger(ctx)

			log.Info("Inside workflow.Go - about to execute activity",
				"recipient", taskForGoroutine.Recipient,
				"activityName", thandFunction.ThandNotifyFunction,
			)

			notifyResult := t.runNotifyTask(ctx, workflowTask, taskForGoroutine)

			// Send result through channel
			resultCh.Send(ctx, temporalNotifyResult{
				Index:     taskIndex,
				Recipient: notifyResult.Recipient,
				Err:       notifyResult.Error,
			})
		})
	}

	// Collect all results
	for range notifyTasks {
		var result temporalNotifyResult
		resultCh.Receive(temporalContext, &result)
		results[result.Index] = notifyResult{
			Recipient: result.Recipient,
			Error:     result.Err,
		}
	}

	// Don't return errors they're just notifications.
	return results, nil
}

// executeNotifyGoParallel executes notification tasks in parallel using Go routines and WaitGroup
func (t *thandTask) executeNotifyGoParallel(
	workflowTask *models.ElevateWorkflowTask,
	notifyTasks []notifyTask,
) ([]notifyResult, error) {

	results := make([]notifyResult, len(notifyTasks))
	var wg sync.WaitGroup

	for i, task := range notifyTasks {
		wg.Add(1)
		go func(index int, notifyTask notifyTask) {
			defer wg.Done()

			// Get provider config
			provider, err := t.config.GetProviderByName(notifyTask.ProviderName)
			if err != nil {
				results[index] = notifyResult{
					Recipient: notifyTask.Recipient,
					Error:     fmt.Errorf("failed to get provider: %w", err),
				}
				return
			}

			// Send notification
			err = provider.SendNotification(
				workflowTask.GetContext(),
				notifyTask.Payload,
			)

			results[index] = notifyResult{
				Recipient: notifyTask.Recipient,
				Error:     err,
			}
		}(i, task)
	}

	wg.Wait()

	return results, nil
}

// processNotificationResults processes notification results and logs errors/successes.
// Returns an error if all notifications failed, otherwise logs warnings for partial failures.
func processNotificationResults(results []notifyResult, notificationType string) error {
	hasErrors := false
	successCount := 0

	for _, result := range results {
		if result.Error != nil {
			logrus.WithError(result.Error).
				WithField("recipient", result.Recipient).
				Error(fmt.Sprintf("%s failed", notificationType))
			hasErrors = true
		} else {
			successCount++
			logrus.WithField("recipient", result.Recipient).
				Info(fmt.Sprintf("%s sent successfully", notificationType))
		}
	}

	if hasErrors && successCount == 0 {
		return fmt.Errorf("all %s requests failed", notificationType)
	}

	if hasErrors {
		logrus.WithFields(logrus.Fields{
			"success": successCount,
			"total":   len(results),
		}).Warn(fmt.Sprintf("Some %s requests failed", notificationType))
	}

	return nil
}
