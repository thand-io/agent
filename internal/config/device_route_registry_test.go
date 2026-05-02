package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/testsuite"
)

func TestDeviceRouteRegistryWorkflowReturnsFreshRouteByDeviceID(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(models.TemporalDeviceRouteUpsertSignalName, models.DeviceConnectionState{
			DeviceID:  "device-alpha",
			TaskQueue: "thand-local-alpha",
			Name:      "Device Alpha",
			Hostname:  "host-one",
			Platform:  "local",
		})
	}, time.Second)

	env.RegisterDelayedCallback(func() {
		value, err := env.QueryWorkflow(models.TemporalGetDeviceRouteQueryName, "device-alpha")
		require.NoError(t, err)

		var route models.DeviceConnectionState
		require.NoError(t, value.Get(&route))
		assert.Equal(t, "thand-local-alpha", route.TaskQueue)
		assert.Equal(t, "host-one", route.Hostname)

		env.CancelWorkflow()
	}, 2*time.Second)

	env.ExecuteWorkflow(deviceRouteRegistryWorkflow)
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

func TestDeviceRouteRegistryWorkflowUsesHostnameAsMetadataOnly(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(models.TemporalDeviceRouteUpsertSignalName, models.DeviceConnectionState{
			DeviceID:  "device-alpha",
			TaskQueue: "thand-local-alpha",
			Hostname:  "host-one",
		})
		env.SignalWorkflow(models.TemporalDeviceRouteUpsertSignalName, models.DeviceConnectionState{
			DeviceID:  "device-alpha",
			TaskQueue: "thand-local-alpha",
			Hostname:  "host-two",
		})
	}, time.Second)

	env.RegisterDelayedCallback(func() {
		value, err := env.QueryWorkflow(models.TemporalGetDeviceRouteQueryName, "device-alpha")
		require.NoError(t, err)

		var route models.DeviceConnectionState
		require.NoError(t, value.Get(&route))
		assert.Equal(t, "thand-local-alpha", route.TaskQueue)
		assert.Equal(t, "host-two", route.Hostname)

		env.CancelWorkflow()
	}, 2*time.Second)

	env.ExecuteWorkflow(deviceRouteRegistryWorkflow)
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}
