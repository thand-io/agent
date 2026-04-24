package thand

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thand-io/agent/internal/models"
	localpresence "github.com/thand-io/agent/internal/providers/localpresence"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type localPresenceApprovalResult struct {
	ProviderKey string
	DeviceID    string
	ApprovalKey string
	Response    models.LocalPresenceApprovalResponse
	TimedOut    bool
	Err         error
}

const localPresencePromptTimeoutGrace = 5 * time.Second

func (t *thandTask) startLocalPresenceApprovalChecks(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	approvalsTask *ApprovalsTask,
	elevationRequest *models.ElevateRequestInternal,
	approvals map[string]any,
	deadline time.Time,
) (workflow.ReceiveChannel, int, workflow.CancelFunc, error) {
	if approvalsTask == nil || len(approvalsTask.Notifiers) == 0 {
		return nil, 0, nil, nil
	}

	ctx, cancel := workflow.WithCancel(workflowTask.GetTemporalContext())
	resultCh := workflow.NewChannel(ctx)
	started := 0

	for providerKey, notifier := range approvalsTask.Notifiers {
		if !t.isLocalPresenceProvider(notifier.Provider) {
			continue
		}

		deviceID := resolveLocalPresenceDevice(notifier, elevationRequest)
		if deviceID == "" {
			cancel()
			return nil, 0, nil, fmt.Errorf("local-presence approval requires a target device")
		}

		approvalKey := models.LocalPresenceApprovalKey(deviceID, providerKey)
		if _, exists := approvals[approvalKey]; exists {
			continue
		}

		started++
		presenceConfig := notifier
		key := providerKey
		workflow.Go(ctx, func(ctx workflow.Context) {
			result := t.runLocalPresenceApprovalCheck(
				workflowTask,
				ctx,
				key,
				presenceConfig,
				elevationRequest,
				deadline,
			)
			resultCh.Send(ctx, result)
		})
	}

	if started == 0 {
		cancel()
		return nil, 0, nil, nil
	}
	return resultCh, started, cancel, nil
}

func (t *thandTask) runLocalPresenceApprovalCheck(
	workflowTask *models.ElevateWorkflowTask,
	ctx workflow.Context,
	providerKey string,
	presenceConfig ApprovalNotifierRequest,
	elevationRequest *models.ElevateRequestInternal,
	deadline time.Time,
) localPresenceApprovalResult {
	deviceID := resolveLocalPresenceDevice(presenceConfig, elevationRequest)
	approvalKey := models.LocalPresenceApprovalKey(deviceID, providerKey)
	result := localPresenceApprovalResult{
		ProviderKey: providerKey,
		DeviceID:    deviceID,
		ApprovalKey: approvalKey,
	}

	remaining := deadline.Sub(workflow.Now(ctx))
	if remaining <= 0 {
		result.TimedOut = true
		result.Response = localPresenceTimeoutResponse(workflowTask, deviceID, providerKey, "approval timeout elapsed before local presence could run")
		return result
	}

	route, routeRemaining, err := t.waitForFreshDeviceRoute(ctx, deviceID, remaining)
	if err != nil {
		if errors.Is(err, errDeviceRouteWaitExpired) {
			result.TimedOut = true
			result.Response = localPresenceTimeoutResponse(workflowTask, deviceID, providerKey, err.Error())
			return result
		}
		result.Err = err
		return result
	}

	if routeRemaining <= 0 {
		result.TimedOut = true
		result.Response = localPresenceTimeoutResponse(workflowTask, deviceID, providerKey, "approval timeout elapsed before local presence could run")
		return result
	}

	promptTimeout := routeRemaining
	if promptTimeout > localPresencePromptTimeoutGrace {
		// Let the helper return a structured prompt timeout before the workflow
		// deadline cancels the activity context.
		promptTimeout -= localPresencePromptTimeoutGrace
	}
	request := models.LocalPresenceApprovalRequest{
		ChallengeID: localPresenceChallengeID(workflowTask.GetWorkflowID(), providerKey, deviceID),
		DeviceID:    deviceID,
		WorkflowID:  workflowTask.GetWorkflowID(),
		TaskName:    providerKey,
		Prompt:      localPresencePrompt(presenceConfig),
		Timeout:     promptTimeout,
		RequestedBy: localPresenceRequestedBy(elevationRequest),
		RoleName:    localPresenceRoleName(elevationRequest),
		Reason:      strings.TrimSpace(elevationRequest.Reason),
	}

	ao := workflow.ActivityOptions{
		TaskQueue:              route.TaskQueue,
		StartToCloseTimeout:    routeRemaining,
		ScheduleToCloseTimeout: routeRemaining,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
	actx := workflow.WithActivityOptions(ctx, ao)

	var response models.LocalPresenceApprovalResponse
	err = workflow.ExecuteActivity(
		actx,
		models.CreateTemporalProviderWorkflowName(presenceConfig.Provider, localpresence.CheckLocalPresenceActivityName),
		request,
	).Get(ctx, &response)
	if err != nil {
		if isTemporalTimeoutError(err) {
			result.TimedOut = true
			result.Response = localPresenceTimeoutResponse(workflowTask, deviceID, providerKey, err.Error())
			return result
		}
		result.Err = err
		return result
	}

	if response.Method == "" {
		response.Method = models.LocalPresenceApprovalMethod
	}
	if response.DeviceID == "" {
		response.DeviceID = deviceID
	}
	if response.ChallengeID == "" {
		response.ChallengeID = request.ChallengeID
	}
	result.TimedOut = response.TimedOut
	result.Response = response
	return result
}

func applyLocalPresenceApprovalResult(
	workflowTask *models.ElevateWorkflowTask,
	approvals map[string]any,
	result *localPresenceApprovalResult,
) {
	if result == nil {
		return
	}
	response := result.Response
	if response.Method == "" {
		response.Method = models.LocalPresenceApprovalMethod
	}
	if response.DeviceID == "" {
		response.DeviceID = result.DeviceID
	}
	if response.ChallengeID == "" {
		response.ChallengeID = localPresenceChallengeID(workflowTask.GetWorkflowID(), result.ProviderKey, result.DeviceID)
	}
	approvals[result.ApprovalKey] = response.AsApprovalMap(workflow.Now(workflowTask.GetTemporalContext()))
	if !response.Approved && !response.TimedOut {
		workflowTask.SetContextKeyValue(sdkConstants.VarsContextApproved, false)
	}
}

func (t *thandTask) isLocalPresenceProvider(providerName string) bool {
	providerName = strings.TrimSpace(providerName)
	if providerName == localpresence.ProviderName {
		return true
	}
	if t.config == nil || providerName == "" {
		return false
	}
	provider, err := t.config.GetProviderByName(providerName)
	if err == nil {
		return provider.GetProvider() == localpresence.ProviderName
	}
	if definition, ok := t.config.GetProviderDefinitions()[providerName]; ok {
		return definition.Provider == localpresence.ProviderName
	}
	return false
}

func resolveLocalPresenceDevice(
	presenceConfig ApprovalNotifierRequest,
	elevationRequest *models.ElevateRequestInternal,
) string {
	if deviceID := strings.TrimSpace(presenceConfig.Device); deviceID != "" {
		return deviceID
	}
	if elevationRequest != nil {
		if deviceID := strings.TrimSpace(elevationRequest.Device); deviceID != "" {
			return deviceID
		}
		if elevationRequest.Metadata != nil {
			if raw, exists := elevationRequest.Metadata["device_id"]; exists {
				if deviceID, ok := raw.(string); ok {
					return strings.TrimSpace(deviceID)
				}
			}
		}
	}
	return ""
}

func localPresencePrompt(presenceConfig ApprovalNotifierRequest) string {
	if prompt := strings.TrimSpace(presenceConfig.Prompt); prompt != "" {
		return prompt
	}
	return "Approve this access request on your Mac"
}

func localPresenceTimeoutResponse(
	workflowTask *models.ElevateWorkflowTask,
	deviceID string,
	providerKey string,
	reason string,
) models.LocalPresenceApprovalResponse {
	return models.LocalPresenceApprovalResponse{
		ChallengeID:   localPresenceChallengeID(workflowTask.GetWorkflowID(), providerKey, deviceID),
		DeviceID:      deviceID,
		Approved:      false,
		FailureReason: reason,
		Method:        models.LocalPresenceApprovalMethod,
		TimedOut:      true,
	}
}

func localPresenceRequestedBy(elevationRequest *models.ElevateRequestInternal) string {
	if elevationRequest == nil || elevationRequest.User == nil {
		return ""
	}
	return elevationRequest.User.GetMappableIdentifier()
}

func localPresenceRoleName(elevationRequest *models.ElevateRequestInternal) string {
	if elevationRequest == nil || elevationRequest.Role == nil {
		return ""
	}
	if name := strings.TrimSpace(elevationRequest.Role.GetName()); name != "" {
		return name
	}
	return strings.TrimSpace(elevationRequest.Role.GetIdentifier())
}

func localPresenceChallengeID(workflowID, providerKey, deviceID string) string {
	return strings.Join([]string{
		strings.TrimSpace(workflowID),
		strings.TrimSpace(providerKey),
		strings.TrimSpace(deviceID),
	}, ":")
}
