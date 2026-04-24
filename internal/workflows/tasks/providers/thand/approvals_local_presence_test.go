package thand

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	localpresence "github.com/thand-io/agent/internal/providers/localpresence"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func newLocalPresenceApprovalsTaskDef(notifier map[string]any, approvals int) *taskModel.ThandTask {
	if approvals == 0 {
		approvals = 1
	}
	return &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals": approvals,
			"timeout":   "2m",
			"notifiers": map[string]any{
				"touch_id": notifier,
			},
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
			"timeout":  "timed_out",
		},
	}
}

func newLegacyLocalPresenceApprovalsTaskDef(localPresence map[string]any) *taskModel.ThandTask {
	return &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals":      1,
			"timeout":        "2m",
			"local_presence": localPresence,
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
			"timeout":  "timed_out",
		},
	}
}

func newLocalPresenceTestConfig() *config.Config {
	return &config.Config{
		Providers: config.ProviderDefinitionsConfig{
			Definitions: map[string]models.ProviderConfig{
				"local-presence": {
					Name:     "Local Presence",
					Provider: localpresence.ProviderName,
					Enabled:  true,
				},
			},
		},
	}
}

func newLocalPresenceWorkflowTask(workflowID, deviceID string) *models.ElevateWorkflowTask {
	task := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: workflowID,
		Context: map[string]any{
			"user": map[string]any{
				"id":    "requester@example.com",
				"email": "requester@example.com",
			},
			"role": map[string]any{
				"name":       "Local Sudo",
				"identifier": models.LocalSudoRoleIdentifier,
			},
			"device":    deviceID,
			"reason":    "testing local presence",
			"approvals": map[string]any{},
		},
	})
	task.SetTaskQueue("thand_local_server_alpha")
	return task
}

func TestResolveLocalPresenceDeviceFallsBackToMetadataDeviceID(t *testing.T) {
	t.Parallel()

	deviceID := resolveLocalPresenceDevice(ApprovalNotifierRequest{}, &models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Metadata: map[string]any{
				"device_id": "device-from-metadata",
			},
		},
	})

	assert.Equal(t, "device-from-metadata", deviceID)
}

func TestExecuteApprovalsTaskLocalPresenceRoutesToDeviceQueueAndApproves(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			return models.DeviceConnectionState{
				DeviceID:  "device-alpha",
				TaskQueue: "thand_local_workstation_alpha",
				Connected: true,
			}, nil
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)

	env.RegisterActivityWithOptions(
		func(ctx context.Context, req models.LocalPresenceApprovalRequest) (*models.LocalPresenceApprovalResponse, error) {
			assert.Equal(t, "thand_local_workstation_alpha", activity.GetInfo(ctx).TaskQueue)
			assert.Equal(t, "device-alpha", req.DeviceID)
			assert.Equal(t, "requester@example.com", req.RequestedBy)
			assert.Equal(t, "Local Sudo", req.RoleName)
			assert.Equal(t, "Approve this access request on your Mac", req.Prompt)
			return &models.LocalPresenceApprovalResponse{
				ChallengeID:     req.ChallengeID,
				DeviceID:        req.DeviceID,
				Approved:        true,
				AuthenticatedAt: time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC),
				Method:          models.LocalPresenceApprovalMethod,
			}, nil
		},
		activity.RegisterOptions{Name: models.CreateTemporalProviderWorkflowName("local-presence", localpresence.CheckLocalPresenceActivityName)},
	)

	taskDef := newLocalPresenceApprovalsTaskDef(map[string]any{
		"provider": "local-presence",
		"device":   "device-alpha",
	}, 1)

	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-approve", "")
		workflowTask.WithTemporalContext(ctx)

		result, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			taskDef,
			nil,
		)
		if err != nil {
			return "", err
		}
		directive, ok := result.(*model.FlowDirective)
		if !ok {
			return "", fmt.Errorf("result = %T, want *model.FlowDirective", result)
		}

		approvals := workflowTask.Context.(map[string]any)["approvals"].(map[string]any)
		approval := approvals[models.LocalPresenceApprovalKey("device-alpha", "touch_id")].(map[string]any)
		if approved, ok := approval["approved"].(bool); !ok || !approved {
			return "", fmt.Errorf("local presence approval not recorded as approved: %#v", approval)
		}
		return directive.Value, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var directive string
	require.NoError(t, env.GetWorkflowResult(&directive))
	assert.Equal(t, "authorize", directive)
}

func TestExecuteApprovalsTaskLocalPresenceUsesRequestDeviceFallback(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			return models.DeviceConnectionState{DeviceID: "device-request", TaskQueue: "thand_local_workstation_request", Connected: true}, nil
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)
	env.RegisterActivityWithOptions(
		func(_ context.Context, req models.LocalPresenceApprovalRequest) (*models.LocalPresenceApprovalResponse, error) {
			if req.DeviceID != "device-request" {
				return nil, fmt.Errorf("device_id = %q, want device-request", req.DeviceID)
			}
			return &models.LocalPresenceApprovalResponse{
				ChallengeID:     req.ChallengeID,
				DeviceID:        req.DeviceID,
				Approved:        true,
				AuthenticatedAt: time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC),
				Method:          models.LocalPresenceApprovalMethod,
			}, nil
		},
		activity.RegisterOptions{Name: models.CreateTemporalProviderWorkflowName("local-presence", localpresence.CheckLocalPresenceActivityName)},
	)

	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-device-fallback", "device-request")
		workflowTask.WithTemporalContext(ctx)

		result, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			newLocalPresenceApprovalsTaskDef(map[string]any{"provider": "local-presence"}, 1),
			nil,
		)
		if err != nil {
			return "", err
		}
		return result.(*model.FlowDirective).Value, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var directive string
	require.NoError(t, env.GetWorkflowResult(&directive))
	assert.Equal(t, "authorize", directive)
}

func TestExecuteApprovalsTaskLocalPresenceMissingDeviceFailsClearly(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-missing-device", "")
		workflowTask.WithTemporalContext(ctx)

		_, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			newLocalPresenceApprovalsTaskDef(map[string]any{"provider": "local-presence"}, 1),
			nil,
		)
		return err
	})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	assert.ErrorContains(t, env.GetWorkflowError(), "target device")
}

func TestExecuteApprovalsTaskLegacyTopLevelLocalPresenceFailsClearly(t *testing.T) {
	t.Parallel()

	workflowTask := newLocalPresenceWorkflowTask("wf-presence-legacy", "device-alpha")
	_, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
		workflowTask,
		"presence",
		newLegacyLocalPresenceApprovalsTaskDef(map[string]any{
			"provider": "local-presence",
			"device":   "device-alpha",
		}),
		nil,
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, "with.local_presence")
}

func TestExecuteApprovalsTaskLocalPresenceOfflineDeviceBranchesTimeout(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			return models.DeviceConnectionState{}, temporal.NewNonRetryableApplicationError(
				"device route unavailable",
				"DeviceRouteUnavailable",
				nil,
			)
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)

	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-offline", "")
		workflowTask.WithTemporalContext(ctx)

		taskDef := newLocalPresenceApprovalsTaskDef(map[string]any{
			"provider": "local-presence",
			"device":   "device-alpha",
		}, 1)
		(*taskDef.With)["timeout"] = "1s"

		result, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			taskDef,
			nil,
		)
		if err != nil {
			return "", err
		}
		return result.(*model.FlowDirective).Value, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var directive string
	require.NoError(t, env.GetWorkflowResult(&directive))
	assert.Equal(t, "timed_out", directive)
}

func TestExecuteApprovalsTaskLocalPresenceDoesNotBlockExternalApproval(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			return models.DeviceConnectionState{}, temporal.NewNonRetryableApplicationError(
				"device route unavailable",
				"DeviceRouteUnavailable",
				nil,
			)
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(sdkWorkflowsModel.TemporalEventSignalName, newApprovalSignalEvent("approver@example.com", true))
	}, time.Second)

	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-external-approval", "")
		workflowTask.WithTemporalContext(ctx)

		taskDef := newLocalPresenceApprovalsTaskDef(map[string]any{
			"provider": "local-presence",
			"device":   "device-alpha",
		}, 1)
		(*taskDef.With)["timeout"] = "30s"

		result, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			taskDef,
			nil,
		)
		if err != nil {
			return "", err
		}
		return result.(*model.FlowDirective).Value, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var directive string
	require.NoError(t, env.GetWorkflowResult(&directive))
	assert.Equal(t, "authorize", directive)
}

func TestExecuteApprovalsTaskLocalPresencePromptTimeoutBranchesTimeout(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			return models.DeviceConnectionState{DeviceID: "device-alpha", TaskQueue: "thand_local_workstation_alpha", Connected: true}, nil
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)
	env.RegisterActivityWithOptions(
		func(_ context.Context, req models.LocalPresenceApprovalRequest) (*models.LocalPresenceApprovalResponse, error) {
			return &models.LocalPresenceApprovalResponse{
				ChallengeID:   req.ChallengeID,
				DeviceID:      req.DeviceID,
				Approved:      false,
				FailureReason: "timed out waiting for local presence",
				Method:        models.LocalPresenceApprovalMethod,
				TimedOut:      true,
			}, nil
		},
		activity.RegisterOptions{Name: models.CreateTemporalProviderWorkflowName("local-presence", localpresence.CheckLocalPresenceActivityName)},
	)

	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-prompt-timeout", "")
		workflowTask.WithTemporalContext(ctx)

		result, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			newLocalPresenceApprovalsTaskDef(map[string]any{
				"provider": "local-presence",
				"device":   "device-alpha",
			}, 1),
			nil,
		)
		if err != nil {
			return "", err
		}
		return result.(*model.FlowDirective).Value, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var directive string
	require.NoError(t, env.GetWorkflowResult(&directive))
	assert.Equal(t, "timed_out", directive)
}

func TestExecuteApprovalsTaskLocalPresenceDeniedBranchesDenied(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			return models.DeviceConnectionState{DeviceID: "device-alpha", TaskQueue: "thand_local_workstation_alpha", Connected: true}, nil
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)
	env.RegisterActivityWithOptions(
		func(_ context.Context, req models.LocalPresenceApprovalRequest) (*models.LocalPresenceApprovalResponse, error) {
			return &models.LocalPresenceApprovalResponse{
				ChallengeID:   req.ChallengeID,
				DeviceID:      req.DeviceID,
				Approved:      false,
				FailureReason: "user canceled local presence",
				Method:        models.LocalPresenceApprovalMethod,
			}, nil
		},
		activity.RegisterOptions{Name: models.CreateTemporalProviderWorkflowName("local-presence", localpresence.CheckLocalPresenceActivityName)},
	)

	env.ExecuteWorkflow(func(ctx workflow.Context) (string, error) {
		workflowTask := newLocalPresenceWorkflowTask("wf-presence-denied", "")
		workflowTask.WithTemporalContext(ctx)

		result, err := (&thandTask{config: newLocalPresenceTestConfig()}).executeApprovalsTask(
			workflowTask,
			"presence",
			newLocalPresenceApprovalsTaskDef(map[string]any{
				"provider": "local-presence",
				"device":   "device-alpha",
			}, 1),
			nil,
		)
		if err != nil {
			return "", err
		}
		return result.(*model.FlowDirective).Value, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var directive string
	require.NoError(t, env.GetWorkflowResult(&directive))
	assert.Equal(t, "denied", directive)
}
