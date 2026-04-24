package thand

import (
	"context"
	"fmt"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	localnotification "github.com/thand-io/agent/internal/providers/localnotification"
	"github.com/thand-io/agent/internal/testing/temporaltest"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestLocalNotificationPayloadUsesDeviceFallbacks(t *testing.T) {
	elevationReq := &models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Device: "device-request",
			Metadata: map[string]any{
				"device_id": "device-metadata",
			},
		},
	}

	payload := localNotificationPayload(
		thandFunction.NotifierRequest{
			Device:  "device-notifier",
			Message: "Custom message",
		},
		elevationReq,
		"Access approved",
		"Fallback body",
		"workflow-1",
	)

	assert.Equal(t, "device-notifier", payload["device_id"])
	assert.Equal(t, "Access approved", payload["title"])
	assert.Equal(t, "Custom message", payload["body"])
	assert.Equal(t, "workflow-1", payload["thread_id"])

	payload = localNotificationPayload(
		thandFunction.NotifierRequest{},
		elevationReq,
		"Access approved",
		"Fallback body",
		"workflow-1",
	)
	assert.Equal(t, "device-request", payload["device_id"])

	elevationReq.Device = ""
	payload = localNotificationPayload(
		thandFunction.NotifierRequest{},
		elevationReq,
		"Access approved",
		"Fallback body",
		"workflow-1",
	)
	assert.Equal(t, "device-metadata", payload["device_id"])
}

func TestAuthorizerNotifierBuildsLocalNotificationPayload(t *testing.T) {
	workflowTask := newLocalNotificationWorkflowTask("wf-authorize", "device-alpha")
	elevationReq := localNotificationElevationRequest(t, workflowTask)
	notifier := NewAuthorizerNotifier(
		nil,
		workflowTask,
		elevationReq,
		&thandFunction.NotifierRequest{
			Provider: "local-notification",
			To:       []string{"user@example.com"},
		},
		"local",
		nil,
		nil,
	)

	payload := notifier.GetPayload(&models.Identity{User: &models.User{Email: "user@example.com"}})

	assert.Equal(t, "device-alpha", payload["device_id"])
	assert.Equal(t, "Access approved: Local Sudo", payload["title"])
	assert.Equal(t, "Your access request for role Local Sudo has been approved", payload["body"])
	assert.Equal(t, "wf-authorize", payload["thread_id"])
}

func TestRevokeNotifierBuildsLocalNotificationPayload(t *testing.T) {
	workflowTask := newLocalNotificationWorkflowTask("wf-revoke", "device-alpha")
	elevationReq := localNotificationElevationRequest(t, workflowTask)
	notifier := NewRevokeNotifier(
		nil,
		workflowTask,
		elevationReq,
		&thandFunction.NotifierRequest{
			Provider: "local-notification",
			To:       []string{"user@example.com"},
		},
		"local",
		nil,
	)

	payload := notifier.GetPayload(&models.Identity{User: &models.User{Email: "user@example.com"}})

	assert.Equal(t, "device-alpha", payload["device_id"])
	assert.Equal(t, "Access ended: Local Sudo", payload["title"])
	assert.Equal(t, "Your access for role Local Sudo has ended", payload["body"])
	assert.Equal(t, "wf-revoke", payload["thread_id"])
}

func TestDefaultNotifierBuildsLocalNotificationPayloadFromRequestDevice(t *testing.T) {
	workflowTask := newLocalNotificationWorkflowTask("wf-notify", "device-alpha")
	elevationReq := localNotificationElevationRequest(t, workflowTask)
	notifier := NewDefaultNotifierImpl(
		thandFunction.NotifierRequest{
			Provider: "local-notification",
			To:       []string{"user@example.com"},
			Message:  "Hello from Thand",
		},
		elevationReq,
	)

	payload := notifier.GetPayload(&models.Identity{User: &models.User{Email: "user@example.com"}})

	assert.Equal(t, "device-alpha", payload["device_id"])
	assert.Equal(t, "Thand notification", payload["title"])
	assert.Equal(t, "Hello from Thand", payload["body"])
}

func TestExecuteNotifyRoutesLocalNotificationToDeviceQueue(t *testing.T) {
	temporaltest.SeedBinaryChecksum()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

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
		func(
			ctx context.Context,
			payload models.NotificationRequest,
		) error {
			assert.Equal(t, "thand_local_workstation_alpha", activity.GetInfo(ctx).TaskQueue)
			assert.Equal(t, "device-alpha", payload["device_id"])
			assert.Equal(t, "Hello from Thand", payload["body"])
			return nil
		},
		activity.RegisterOptions{
			Name: models.CreateTemporalProviderWorkflowName(
				localnotification.ProviderName,
				localnotification.SendNotificationActivityName,
			),
		},
	)

	workflowTask := newLocalNotificationWorkflowTask("wf-notify", "device-alpha")
	workflowTask.SetTaskQueue("thand_local_server_alpha")

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		workflowTask.WithTemporalContext(ctx)
		results, err := (&thandTask{}).executeNotifyTemporalParallel(
			workflowTask,
			"notify",
			[]notifyTask{
				{
					Recipient: "user@example.com",
					Provider:  "local-notification",
					DeviceID:  "device-alpha",
					CallFunc: model.CallFunction{
						Call: thandFunction.ThandNotifyFunction,
					},
					Payload: models.NotificationRequest{
						"device_id": "device-alpha",
						"title":     "Thand notification",
						"body":      "Hello from Thand",
					},
				},
			},
		)
		if err != nil {
			return err
		}
		if len(results) != 1 {
			return fmt.Errorf("result count = %d, want 1", len(results))
		}
		return results[0].Error
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestApprovalNotificationsRouteLocalNotificationToDeviceQueue(t *testing.T) {
	temporaltest.SeedBinaryChecksum()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

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
		func(ctx context.Context, payload models.NotificationRequest) error {
			assert.Equal(t, "thand_local_workstation_alpha", activity.GetInfo(ctx).TaskQueue)
			assert.Equal(t, "device-alpha", payload["device_id"])
			assert.Equal(t, "Access approval required: Local Sudo", payload["title"])
			assert.Equal(t, "Approval is required for Local Sudo", payload["body"])
			return nil
		},
		activity.RegisterOptions{
			Name: models.CreateTemporalProviderWorkflowName(
				localnotification.ProviderName,
				localnotification.SendNotificationActivityName,
			),
		},
	)

	workflowTask := newLocalNotificationWorkflowTask("wf-approval-notify", "device-alpha")
	workflowTask.SetTaskQueue("thand_local_server_alpha")
	elevationReq := localNotificationElevationRequest(t, workflowTask)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		workflowTask.WithTemporalContext(ctx)
		return (&thandTask{config: &config.Config{}}).makeApprovalNotifications(
			workflowTask,
			"presence",
			&ApprovalsTask{
				Approvals: 1,
				Notifiers: map[string]ApprovalNotifierRequest{
					"desktop": {
						Provider: localnotification.ProviderName,
						To:       []string{"user@example.com"},
					},
				},
			},
			elevationReq,
		)
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func newLocalNotificationWorkflowTask(workflowID, deviceID string) *models.ElevateWorkflowTask {
	task := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: workflowID,
		Context: map[string]any{
			"role": &models.Role{
				Identifier: models.LocalSudoRoleIdentifier,
				Name:       "Local Sudo",
			},
			"providers": []string{"local-elevation"},
			"device":    deviceID,
			"reason":    "debug prod",
			"duration":  "1m",
			"metadata": map[string]any{
				"device_id": deviceID,
			},
			"user": &models.User{
				Email: "user@example.com",
			},
		},
		Input:  map[string]any{},
		Output: map[string]any{},
	})
	return task
}

func localNotificationElevationRequest(tb testing.TB, task *models.ElevateWorkflowTask) *models.ElevateRequestInternal {
	tb.Helper()
	req, err := task.GetContextAsElevationRequest()
	require.NoError(tb, err)
	require.NotNil(tb, req)
	return req
}
