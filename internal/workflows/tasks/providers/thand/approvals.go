package thand

import (
	"errors"
	"fmt"
	"sort"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	runner "github.com/thand-io/agent/sdk/workflows/runner"
)

var ThandApprovalsTask = "approvals"

type ApprovalsTask struct {
	Approvals   int                                      `json:"approvals" default:"1"`
	SelfApprove bool                                     `json:"selfApprove" default:"false"`
	Notifiers   map[string]thandFunction.NotifierRequest `json:"notifiers"`
}

func (n *ApprovalsTask) IsValid() bool {
	return n.Approvals != 0
}

func (t *ApprovalsTask) HasNotifiers() bool {
	return len(t.Notifiers) > 0
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

	approval, err := runner.ListenTaskHandler(
		workflowTask, fmt.Sprintf("%s.listen", taskName), &model.ListenTask{
			Listen: model.ListenTaskConfiguration{
				To: &model.EventConsumptionStrategy{
					Any: []*model.EventFilter{
						{
							With: &model.EventProperties{
								Type: ThandApprovalEventType,
							},
						},
					},
				},
			},
		}, input)

	if err != nil {

		logrus.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to listen for approval event")

		return nil, err
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

	approvedState, foundApprovedState := call.On.GetString("approved")
	deniedState, foundDeniedState := call.On.GetString("denied")

	if !foundApprovedState || !foundDeniedState {
		return nil, errors.New("both approved and denied states must be specified in the on block")
	}

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
	providerKeys := make([]string, 0, len(approvalsTask.Notifiers))
	for providerKey := range approvalsTask.Notifiers {
		providerKeys = append(providerKeys, providerKey)
	}
	sort.Strings(providerKeys)

	for _, providerKey := range providerKeys {
		notifierRequest := approvalsTask.Notifiers[providerKey]
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
	providerKeys := make([]string, 0, len(approvalsTask.Notifiers))
	for providerKey := range approvalsTask.Notifiers {
		providerKeys = append(providerKeys, providerKey)
	}
	sort.Strings(providerKeys)

	for _, providerKey := range providerKeys {
		notifierRequest := approvalsTask.Notifiers[providerKey]
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
