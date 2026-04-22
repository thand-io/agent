package thand

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/testing/temporaltest"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func newDeviceRoutingTestEnv() *testsuite.TestWorkflowEnvironment {
	temporaltest.SeedBinaryChecksum()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	return env
}

func newDeviceRoutingWorkflowTask(workflowID, taskQueue string) *models.ElevateWorkflowTask {
	task := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: workflowID,
		Context:    map[string]any{},
		Input:      map[string]any{},
		Output:     map[string]any{},
	})
	task.SetTaskQueue(taskQueue)
	return task
}

func newDeviceRoutingAuthorizeRequest(duration time.Duration) *models.AuthorizeRoleRequest {
	return &models.AuthorizeRoleRequest{
		Identity: &models.Identity{
			ID: "user@example.com",
			User: &models.User{
				Email: "user@example.com",
			},
		},
		Role: &models.CompositeRole{
			Role:      *newExecutionPlanRole("local_sudo", "Local Sudo"),
			Composite: false,
		},
		Duration: &duration,
	}
}

func TestRunAuthTaskRoutesDeviceWorkflowToFreshRouteQueue(t *testing.T) {
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

	authorizeWorkflowName := models.CreateTemporalProviderWorkflowName("local-elevation", models.TemporalAuthorizeRoleWorkflowName)
	env.RegisterWorkflowWithOptions(
		func(ctx workflow.Context, req models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
			assert.Equal(t, "thand_local_workstation_alpha", workflow.GetInfo(ctx).TaskQueueName)
			require.NotNil(t, req.Identity)
			return &models.AuthorizeRoleResponse{UserId: req.Identity.ID}, nil
		},
		workflow.RegisterOptions{Name: authorizeWorkflowName},
	)

	task := newDeviceRoutingWorkflowTask("wf-auth-route", "thand_local_server_alpha")
	request := newDeviceRoutingAuthorizeRequest(30 * time.Second)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		task.WithTemporalContext(ctx)

		result := (&thandTask{}).runAuthTask(task, authTask{
			EntryID:          "entry-1",
			ProviderName:     "local-elevation",
			Identity:         "user@example.com",
			DeviceID:         "device-alpha",
			AuthorizeRequest: request,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.AuthResponse == nil || result.AuthResponse.UserId != "user@example.com" {
			return fmt.Errorf("authorize child workflow did not return the expected response")
		}
		if got, want := task.GetTaskQueue(), "thand_local_server_alpha"; got != want {
			return fmt.Errorf("parent task queue = %q, want %q", got, want)
		}
		return nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
}

func TestRunAuthTaskFailsWhenDeviceRouteNeverAppears(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()

	var childExecutions atomic.Int32
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

	authorizeWorkflowName := models.CreateTemporalProviderWorkflowName("local-elevation", models.TemporalAuthorizeRoleWorkflowName)
	env.RegisterWorkflowWithOptions(
		func(workflow.Context, models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
			childExecutions.Add(1)
			return &models.AuthorizeRoleResponse{}, nil
		},
		workflow.RegisterOptions{Name: authorizeWorkflowName},
	)

	task := newDeviceRoutingWorkflowTask("wf-auth-no-route", "thand_local_server_alpha")
	request := newDeviceRoutingAuthorizeRequest(time.Second)

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		task.WithTemporalContext(ctx)

		result := (&thandTask{}).runAuthTask(task, authTask{
			EntryID:          "entry-1",
			ProviderName:     "local-elevation",
			Identity:         "user@example.com",
			DeviceID:         "device-alpha",
			AuthorizeRequest: request,
		})
		return result.Error
	})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	assert.ErrorContains(t, env.GetWorkflowError(), "device route wait expired")
	assert.Zero(t, childExecutions.Load(), "device-local authorize child workflow should not run without a live route")
}

func TestRunRevokeTaskWaitsForRouteToReturnAndThenUsesDeviceQueue(t *testing.T) {
	t.Parallel()

	env := newDeviceRoutingTestEnv()

	var routeLookups atomic.Int32
	env.RegisterActivityWithOptions(
		func(context.Context, string) (models.DeviceConnectionState, error) {
			attempt := routeLookups.Add(1)
			if attempt < 3 {
				return models.DeviceConnectionState{}, temporal.NewNonRetryableApplicationError(
					"device route unavailable",
					"DeviceRouteUnavailable",
					nil,
				)
			}
			return models.DeviceConnectionState{
				DeviceID:  "device-alpha",
				TaskQueue: "thand_local_workstation_alpha",
				Connected: true,
			}, nil
		},
		activity.RegisterOptions{Name: models.TemporalResolveFreshDeviceRouteActivityName},
	)

	revokeWorkflowName := models.CreateTemporalProviderWorkflowName("local-elevation", models.TemporalRevokeRoleWorkflowName)
	env.RegisterWorkflowWithOptions(
		func(ctx workflow.Context, req models.WorkflowRevokeRoleRequest) (*models.RevokeRoleResponse, error) {
			assert.Equal(t, "thand_local_workstation_alpha", workflow.GetInfo(ctx).TaskQueueName)
			require.NotNil(t, req.RevokeRoleRequest)
			require.NotNil(t, req.RevokeRoleRequest.AuthorizeRoleRequest)
			return &models.RevokeRoleResponse{}, nil
		},
		workflow.RegisterOptions{Name: revokeWorkflowName},
	)

	task := newDeviceRoutingWorkflowTask("wf-revoke-route-retry", "thand_local_server_alpha")
	duration := 30 * time.Second

	env.ExecuteWorkflow(func(ctx workflow.Context) error {
		task.WithTemporalContext(ctx)

		result := (&thandTask{}).runRevokeTask(task, revokeTask{
			EntryID:      "entry-1",
			ProviderName: "local-elevation",
			Identity:     "user@example.com",
			DeviceID:     "device-alpha",
			RevokeReq: &models.WorkflowRevokeRoleRequest{
				RevokeRoleRequest: &models.RevokeRoleRequest{
					AuthorizeRoleRequest: newDeviceRoutingAuthorizeRequest(duration),
				},
				AuthorizeRoleResponse: &models.AuthorizeRoleResponse{
					UserId: "user@example.com",
				},
			},
		})
		if result.Error != nil {
			return result.Error
		}
		if got, want := task.GetTaskQueue(), "thand_local_server_alpha"; got != want {
			return fmt.Errorf("parent task queue = %q, want %q", got, want)
		}
		return nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	assert.GreaterOrEqual(t, routeLookups.Load(), int32(3), "revoke should keep polling for the device route until it comes back")
}
