package thand

import (
	"errors"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflowModels "github.com/thand-io/agent/sdk/workflows/models"
	runner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

var ThandApprovalsTask = "approvals"

const approvalDeadlinesContextKey = "approval_deadlines"

type ApprovalsTask struct {
	Approvals   int                                      `json:"approvals" default:"1"`
	SelfApprove bool                                     `json:"selfApprove" default:"false"`
	Timeout     string                                   `json:"timeout,omitempty"`
	Notifiers   map[string]thandFunction.NotifierRequest `json:"notifiers"`
}

func (n *ApprovalsTask) IsValid() bool {
	return n.Approvals != 0
}

func (t *ApprovalsTask) HasNotifiers() bool {
	return len(t.Notifiers) > 0
}

func (t *ApprovalsTask) TimeoutDuration() (time.Duration, error) {
	timeout := strings.TrimSpace(t.Timeout)
	if timeout == "" {
		return 0, nil
	}
	duration, err := time.ParseDuration(timeout)
	if err != nil {
		return 0, fmt.Errorf("invalid approval timeout %q: %w", t.Timeout, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("approval timeout must be positive")
	}
	return duration, nil
}

func (n *ApprovalsTask) AsMap() map[string]any {
	response, err := common.ConvertInterfaceToMap(n)
	if err != nil {
		panic(fmt.Sprintf("failed to convert ApprovalsTask to map: %v", err))
	}
	return response
}

func (t *thandTask) executeApprovalsTask(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	input any) (any, error) {

	elevationRequest, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to get elevation request from context")

		return nil, err
	}

	var approvalsTask ApprovalsTask
	err = common.ConvertInterfaceToInterface(call.With, &approvalsTask)

	// Debug logging for integration test
	logrus.WithFields(logrus.Fields{
		"taskName":       taskName,
		"callWith":       fmt.Sprintf("%+v", call.With),
		"approvalsTask":  fmt.Sprintf("%+v", approvalsTask),
		"hasNotifiers":   approvalsTask.HasNotifiers(),
		"notifiersCount": len(approvalsTask.Notifiers),
	}).Info("Parsed approvals task")

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to parse notification request")

		return nil, err
	}

	if !approvalsTask.IsValid() {
		return nil, errors.New("invalid notification request")
	}

	approvedState, foundApprovedState := call.On.GetString("approved")
	deniedState, foundDeniedState := call.On.GetString("denied")
	timeoutState, foundTimeoutState := call.On.GetString("timeout")

	if !foundApprovedState || !foundDeniedState {
		return nil, errors.New("both approved and denied states must be specified in the on block")
	}

	if err := validateApprovalTimeoutPair(&approvalsTask, foundTimeoutState); err != nil {
		return nil, err
	}

	availableIdentities := elevationRequest.ResolveIdentities(
		workflowTask.GetContext(),
		t.config.GetProvidersByCapability(
			models.IdentityCapabilities...,
		),
	)

	if common.IsNilOrZero(input) {

		logrus.Infof("Starting Thand approvals task: %s", taskName)

		newConfig := &models.BasicConfig{}
		newConfig.Update(approvalsTask.AsMap())

		call.With = newConfig

		if _, _, err := t.ensureApprovalDeadline(workflowTask, taskName, &approvalsTask); err != nil {
			return nil, err
		}

		if approvalsTask.HasNotifiers() {

			err = t.makeApprovalNotifications(
				workflowTask,
				taskName,
				&approvalsTask,
				elevationRequest,
			)

			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"taskName": taskName,
				}).Error("Failed to create approval notifications")

				return nil, err
			}
		}

	} else {
		logrus.Infof("Resuming Thand approvals task: %s", taskName)
	}

	logrus.Infof("Executing Thand monitor task: %s", taskName)

	approval, timedOut, err := t.listenForApprovalEvent(workflowTask, taskName, &approvalsTask, input)

	if err != nil {

		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to listen for approval event")

		return nil, err
	}

	if timedOut {
		logrus.WithFields(logrus.Fields{
			"taskName": taskName,
		}).Info("Approval task timed out")
		return &model.FlowDirective{Value: timeoutState}, nil
	}

	defaultFlowState := model.FlowDirective{
		Value: taskName, // loop back to await more approvals
	}

	// Set the context to hold all the approvals
	/*
		output:
			# Simply convert the output to a list of approvals
			as: '${ { "approvals": [{"approved": .data.approved}] } }'
		export:
			# Next we need to map the existing approvals to the new
			# list of approvals in the context as export handles
			# context access
			as: '${ $context + { "approvals": ($context.approvals // []) + .approvals } }'
		then: check_approval
	*/

	workflowContext := workflowTask.GetContextAsMap()

	approvals, ok := workflowContext["approvals"].(map[string]any)

	if !ok {
		approvals = map[string]any{}
	}

	var approvalData map[string]any

	if approvalEvent, ok := approval.(*cloudevents.Event); ok {

		/*
			{
			"specversion": "1.0",
			"id": "123e4567-e89b-12d3-a456-426614174000",
			"type": "com.thand.approval",
			"source": "urn:thand:test",
			"data": {
				"approved": true
			},
			"user": "approver1@thand.io",
			"signature": "abc123signature"
			}
		*/

		approvalEvent.DataAs(&approvalData)
		extensions := approvalEvent.Extensions()

		approverIdentity, userExists := extensions[sdkConstants.VarsContextUser].(string)

		if !userExists {
			logrus.Warn("Approval event missing user extension")
			return &defaultFlowState, nil
		}

		logrus.WithFields(logrus.Fields{
			"taskName":         taskName,
			"approverIdentity": approverIdentity,
			"approvalData":     approvalData,
		}).Info("Received approval event")

		// Convert approverIdentity to models.Identity
		approverIdentityObj, err := models.NewIdentityFromBase64(approverIdentity)

		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"taskName": taskName,
			}).Warn("Failed to convert approver identity to models.Identity")
			return &defaultFlowState, nil
		}

		approverUser := approverIdentityObj.GetUser()

		if approverUser == nil {
			logrus.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Warn("Approver identity is not a user; cannot process approval")
			return &defaultFlowState, nil
		}

		// Check if self-approval is disabled and the approver is the requester or one of the elevated identities
		if !approvalsTask.SelfApprove {

			logrus.WithFields(logrus.Fields{
				"taskName":     taskName,
				"approverUser": approverUser.String(),
			}).Info("Self-approval is disabled; checking if approver is requester or identity being elevated")

			// Get requester identity
			requestingUser := elevationRequest.User

			if requestingUser == nil {
				logrus.WithFields(logrus.Fields{
					"taskName":     taskName,
					"approverUser": approverUser.String(),
				}).Warn("Elevation request has no requesting user; cannot check for self-approval")

				// Return to the default flow state to await more approvals
				return &defaultFlowState, nil
			}

			// Check if approver is the requester
			if approverUser.Equals(requestingUser) {
				logrus.WithFields(logrus.Fields{
					"taskName":      taskName,
					"approverUser":  approverUser.String(),
					"requesterUser": requestingUser.String(),
				}).Warn("Self-approval is disabled; ignoring approval from requester")

				// Notify the approver that their action was rejected
				t.notifyApprovalRejection(
					workflowTask, taskName,
					approverUser, &approvalsTask,
					"Self-approval is disabled. As the requester you cannot approve your own elevation request.")

				// Return to the default flow state to await more approvals
				return &defaultFlowState, nil
			}

			// Loop through the identities being elevated
			for _, requestedIdentityID := range elevationRequest.Identities {

				requestedIdentity, foundRequestedIdentity := availableIdentities[requestedIdentityID]

				if !foundRequestedIdentity {
					continue
				}

				requestedUser := requestedIdentity.GetUser()

				if requestedUser == nil {
					continue
				}

				// Check if approver is the identity being elevated
				if approverUser.Equals(requestedUser) {
					logrus.WithFields(logrus.Fields{
						"taskName":      taskName,
						"requestedUser": requestedUser.String(),
						"approverUser":  approverUser.String(),
					}).Warn("Self-approval is disabled; ignoring approval from identity being elevated")

					// Notify the approver that their action was rejected
					t.notifyApprovalRejection(
						workflowTask, taskName,
						approverUser, &approvalsTask,
						"Self-approval is disabled. As the user to be elevated you cannot approve this elevation request.")

					// Return to the default flow state to await more approvals
					return &defaultFlowState, nil
				}
			}
		}

		approvedVal, exists := approvalData["approved"]

		if exists {

			approved, ok := approvedVal.(bool)

			if !ok {
				logrus.WithFields(logrus.Fields{
					"taskName":     taskName,
					"approverUser": approverUser.String(),
				}).Warn("Approval value is not a boolean; ignoring this approval")
				return &defaultFlowState, nil
			}

			approvals[approverIdentityObj.GetMappableIdentifier()] = map[string]any{
				"approved":  approved,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}

			// If the approval was denied then mark the approval as denied
			if !approved {

				logrus.WithFields(logrus.Fields{
					"taskName":     taskName,
					"approverUser": approverUser.String(),
				}).Info("Approval denied by user")

				workflowTask.SetContextKeyValue(sdkConstants.VarsContextApproved, false)
			}
		}
	}

	workflowTask.SetContextKeyValue("approvals", approvals)

	/*
		# If anyone rejects then reject the entire request
		# otherwise if the required number of approvals is met then authorize
		# Approvals are stored as a map[identity]approval_data structure
		- case1:
			when: any($context.approvals | to_entries[]; .value.approved == false)
			then: denied
		- case2:
			when: '[$context.approvals | to_entries[] | select(.value.approved == true)] | length >= N'
			then: authorize
		- default:
			then: loop back to task to await more approvals
	*/

	// Create the switch task to handle approval or rejection
	flowDirective, err := t.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		approvalsTask.Approvals,
		approvedState,
		deniedState,
	)

	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to execute switch task for approval logic")

		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"taskName":      taskName,
		"flowDirective": flowDirective.Value,
	}).Info("Completed Thand approvals task")

	return flowDirective, nil
}

func validateApprovalTimeoutPair(approvalsTask *ApprovalsTask, hasTimeoutBranch bool) error {
	hasTimeout := strings.TrimSpace(approvalsTask.Timeout) != ""
	if hasTimeout != hasTimeoutBranch {
		return errors.New("approvals with.timeout and on.timeout must be configured together")
	}
	if hasTimeout {
		if _, err := approvalsTask.TimeoutDuration(); err != nil {
			return err
		}
	}
	return nil
}

func approvalListenTask() *model.ListenTask {
	return &model.ListenTask{
		Listen: model.ListenTaskConfiguration{
			To: &model.EventConsumptionStrategy{
				Any: []*model.EventFilter{{
					With: &model.EventProperties{
						Type: ThandApprovalEventType,
					},
				}},
			},
		},
	}
}

func (t *thandTask) listenForApprovalEvent(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	approvalsTask *ApprovalsTask,
	input any,
) (any, bool, error) {
	deadline, hasDeadline, err := t.ensureApprovalDeadline(workflowTask, taskName, approvalsTask)
	if err != nil {
		return nil, false, err
	}

	listenTask := approvalListenTask()
	listenTaskName := fmt.Sprintf("%s.listen", taskName)

	if !hasDeadline || !workflowTask.HasTemporalContext() || !common.IsNilOrZero(input) {
		approval, err := runner.ListenTaskHandler(workflowTask, listenTaskName, listenTask, input)
		return approval, false, err
	}

	remaining := deadline.Sub(approvalNow(workflowTask))
	if remaining <= 0 {
		return nil, true, nil
	}

	return listenForApprovalEventWithTimeout(workflowTask, listenTaskName, remaining)
}

func listenForApprovalEventWithTimeout(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	remaining time.Duration,
) (any, bool, error) {
	ctx := workflowTask.GetTemporalContext()
	log := workflowTask.GetLogger()

	resumeChan := workflow.GetSignalChannel(ctx, sdkWorkflowModels.TemporalResumeSignalName)
	signalChan := workflow.GetSignalChannel(ctx, sdkWorkflowModels.TemporalEventSignalName)
	timer := workflow.NewTimer(ctx, remaining)

	for {
		var input any
		timedOut := false
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(resumeChan, func(c workflow.ReceiveChannel, more bool) {
			var resumableWorkflow sdkWorkflowModels.WorkflowTask
			c.Receive(ctx, &resumableWorkflow)
			input = resumableWorkflow.GetInput()
		})
		selector.AddReceive(signalChan, func(c workflow.ReceiveChannel, more bool) {
			var signalEvent cloudevents.Event
			c.Receive(ctx, &signalEvent)
			input = &signalEvent
		})
		selector.AddFuture(timer, func(f workflow.Future) {
			timedOut = true
		})

		selector.Select(ctx)
		if timedOut {
			return nil, true, nil
		}

		approvalEvent, ok, err := decodeApprovalEvent(input)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Ignoring non-approval signal while waiting for approval")
			continue
		}
		return approvalEvent, false, nil
	}
}

func decodeApprovalEvent(input any) (*cloudevents.Event, bool, error) {
	if common.IsNilOrZero(input) {
		return nil, false, nil
	}

	var event cloudevents.Event
	if err := common.ConvertInterfaceToInterface(input, &event); err != nil {
		return nil, false, fmt.Errorf("failed to convert signal to cloudevent: %w", err)
	}
	if event.Type() != ThandApprovalEventType {
		return nil, false, nil
	}
	return &event, true, nil
}

func (t *thandTask) ensureApprovalDeadline(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	approvalsTask *ApprovalsTask,
) (time.Time, bool, error) {
	timeout, err := approvalsTask.TimeoutDuration()
	if err != nil {
		return time.Time{}, false, err
	}
	if timeout == 0 {
		return time.Time{}, false, nil
	}

	deadlines := approvalDeadlines(workflowTask)
	if rawDeadline, ok := deadlines[taskName]; ok {
		deadline, err := parseApprovalDeadline(rawDeadline)
		if err != nil {
			return time.Time{}, false, err
		}
		return deadline, true, nil
	}

	deadline := approvalNow(workflowTask).Add(timeout).UTC()
	deadlines[taskName] = deadline.Format(time.RFC3339Nano)
	workflowTask.SetContextKeyValue(approvalDeadlinesContextKey, deadlines)
	return deadline, true, nil
}

func approvalDeadlines(workflowTask *models.ElevateWorkflowTask) map[string]any {
	workflowContext := workflowTask.GetContextAsMap()
	if raw, ok := workflowContext[approvalDeadlinesContextKey]; ok {
		if deadlines, ok := raw.(map[string]any); ok {
			return deadlines
		}
		if deadlines, ok := raw.(map[string]string); ok {
			converted := make(map[string]any, len(deadlines))
			for key, value := range deadlines {
				converted[key] = value
			}
			return converted
		}
	}
	return map[string]any{}
}

func parseApprovalDeadline(raw any) (time.Time, error) {
	switch value := raw.(type) {
	case time.Time:
		return value, nil
	case string:
		deadline, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid persisted approval deadline %q: %w", value, err)
		}
		return deadline, nil
	default:
		return time.Time{}, fmt.Errorf("invalid persisted approval deadline type %T", raw)
	}
}

func approvalNow(workflowTask *models.ElevateWorkflowTask) time.Time {
	if workflowTask.HasTemporalContext() {
		return workflow.Now(workflowTask.GetTemporalContext()).UTC()
	}
	return time.Now().UTC()
}

// evaluateApprovalSwitch evaluates the approval logic using a switch task
// to determine if the request should be approved, denied, or loop back for more approvals
func (t *thandTask) evaluateApprovalSwitch(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	approvals map[string]any,
	requiredApprovals int,
	approvedState string,
	deniedState string,
) (*model.FlowDirective, error) {

	return runner.SwitchTaskHandler(
		workflowTask,
		map[string]any{
			"approvals": approvals,
		},
		fmt.Sprintf("%s.switch", taskName),
		&model.SwitchTask{
			Switch: []model.SwitchItem{{
				"case1": model.SwitchCase{
					When: &model.RuntimeExpression{
						Value: "any($context.approvals | to_entries[]; .value.approved == false)",
					},
					Then: &model.FlowDirective{
						Value: deniedState, // go to denied state
					},
				},
			}, {
				"case2": model.SwitchCase{
					When: &model.RuntimeExpression{
						Value: fmt.Sprintf("[$context.approvals | to_entries[] | select(.value.approved == true)] | length >= %d", requiredApprovals),
					},
					Then: &model.FlowDirective{
						Value: approvedState, // proceed to the next state
					},
				},
			}, {
				"default": model.SwitchCase{
					// No When condition = default case (return to await more approvals)
					Then: &model.FlowDirective{
						Value: taskName, // loop back to await more approvals
					},
				},
			}},
		})
}

func (t *thandTask) makeApprovalNotifications(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	approvalsTask *ApprovalsTask,
	elevationRequest *models.ElevateRequestInternal,
) error {

	// In parallel create a notifier for each of the notifiers
	// Build notification tasks for each provider
	var notifyTasks []notifyTask
	for providerKey, notifierRequest := range approvalsTask.Notifiers {
		// Create an ApprovalNotifier for each provider
		approvalNotifier := NewApprovalsNotifier(
			t.config,
			workflowTask,
			elevationRequest,
			&ApprovalNotifier{
				Approvals:   approvalsTask.Approvals,
				SelfApprove: approvalsTask.SelfApprove,
				Notifier:    notifierRequest,
				Entrypoint:  taskName,
			},
		)

		// Get recipients for this notifier
		recipients := approvalNotifier.GetRecipients()

		logrus.WithFields(logrus.Fields{
			"providerKey": providerKey,
			"recipients":  recipients,
		}).Info("Processing approval notifier")

		// Build notification tasks for each recipient
		for _, recipientID := range recipients {

			recipientIdentity := t.resolveIdentity(
				recipientID,
			)

			if recipientIdentity == nil {
				logrus.WithFields(logrus.Fields{
					"recipient":   recipientID,
					"providerKey": providerKey,
				}).Warn("Failed to resolve recipient identity; skipping notification for this recipient")
				continue
			}

			recipientIdentity.ID = recipientID
			recipientPayload := approvalNotifier.GetPayload(recipientIdentity)

			notifyTasks = append(notifyTasks, notifyTask{
				Recipient: recipientID,
				CallFunc:  approvalNotifier.GetCallFunction(recipientIdentity),
				Payload:   recipientPayload,
				Provider:  approvalNotifier.GetProviderName(),
			})

			logrus.WithFields(logrus.Fields{
				"recipient":   recipientID,
				"provider":    approvalNotifier.GetProviderName(),
				"providerKey": providerKey,
			}).Debug("Prepared approval notification task")
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
		}).Error("Failed to execute approval notifications")

		return err
	}

	// Process results using shared function
	if err := processNotificationResults(notifyResults, "Approval notification"); err != nil {

		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to process approval notification results")

		return err
	}

	return nil
}

// notifyApprovalRejection sends a notification to the approver when their approval is rejected
func (t *thandTask) notifyApprovalRejection(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	approverUser *models.User,
	approvalsTask *ApprovalsTask,
	reason string,
) {
	logrus.WithFields(logrus.Fields{
		"taskName":     taskName,
		"approverUser": approverUser.String(),
		"reason":       reason,
	}).Info("Notifying approver of rejection")

	// If there are no notifiers configured, just log the rejection
	if !approvalsTask.HasNotifiers() {
		logrus.WithFields(logrus.Fields{
			"taskName":     taskName,
			"approverUser": approverUser.String(),
			"reason":       reason,
		}).Warn("Approval rejected (no notifiers configured)")
		return
	}

	// Send rejection notification using each configured notifier
	for providerKey, notifierRequest := range approvalsTask.Notifiers {
		// Create a generic notifier for rejection with the approver as recipient
		rejectionRequest := thandFunction.NotifierRequest{
			Provider: notifierRequest.Provider,
			To:       []string{approverUser.Email},
			Message:  reason,
		}

		notifyImpl := NewDefaultNotifierImpl(rejectionRequest)

		logrus.WithFields(logrus.Fields{
			"providerKey": providerKey,
			"reason":      reason,
		}).Info("Executing approval rejection notification")

		_, err := t.executeNotify(
			workflowTask,
			taskName,
			notifyImpl,
		)

		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"taskName":     taskName,
				"approverUser": approverUser.String(),
				"providerKey":  providerKey,
			}).Warn("Failed to execute approval rejection notification")
		}
	}

	logrus.WithFields(logrus.Fields{
		"taskName":     taskName,
		"approverUser": approverUser.String(),
		"reason":       reason,
	}).Info("Approval rejection notification sent")
}
