package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/testsuite"
)

func queryDeviceDefinitionEventually(
	t *testing.T,
	env *testsuite.TestWorkflowEnvironment,
	deviceID string,
	assertDevice func(models.Device),
) {
	t.Helper()

	var poll func()
	poll = func() {
		value, err := env.QueryWorkflow(models.TemporalGetDeviceDefinitionQueryName, deviceID)
		if err != nil && strings.Contains(err.Error(), "unknown queryType") {
			env.RegisterDelayedCallback(poll, time.Millisecond)
			return
		}
		require.NoError(t, err)

		var device models.Device
		require.NoError(t, value.Get(&device))
		assertDevice(device)

		env.CancelWorkflow()
	}

	env.RegisterDelayedCallback(poll, time.Millisecond)
}

func TestDeviceDefinitionRegistryWorkflowReturnsConfiguredDevice(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(models.TemporalDeviceDefinitionUpsertSignalName, models.Device{
			ID:      "device-alpha",
			Name:    "Device Alpha",
			Enabled: true,
		})
	}, 0)

	queryDeviceDefinitionEventually(t, env, "device-alpha", func(device models.Device) {
		assert.Equal(t, "device-alpha", device.ID)
		assert.Equal(t, "Device Alpha", device.Name)
	})

	env.ExecuteWorkflow(deviceDefinitionRegistryWorkflow)
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}

func TestDeviceDefinitionRegistryWorkflowRejectsConflictingUpdates(t *testing.T) {
	t.Parallel()

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(models.TemporalDeviceDefinitionUpsertSignalName, models.Device{
			ID:      "device-alpha",
			Name:    "Device Alpha",
			Enabled: true,
		})
		env.SignalWorkflow(models.TemporalDeviceDefinitionUpsertSignalName, models.Device{
			ID:      "device-alpha",
			Name:    "Conflicting Device Alpha",
			Enabled: true,
		})
	}, 0)

	queryDeviceDefinitionEventually(t, env, "device-alpha", func(device models.Device) {
		assert.Equal(t, "Device Alpha", device.Name)
	})

	env.ExecuteWorkflow(deviceDefinitionRegistryWorkflow)
	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
}
