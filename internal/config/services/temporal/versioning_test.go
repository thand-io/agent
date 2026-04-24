package temporal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestShouldUseVersioningForIdentity(t *testing.T) {
	t.Parallel()

	client := NewTemporalClient(
		&models.TemporalConfig{
			Host:              "localhost",
			Port:              7233,
			Namespace:         "default",
			DisableVersioning: false,
		},
		nil,
		"thand_local_alpha_server01",
		models.TemporalDeviceRegistryTaskQueue,
	)

	assert.True(t, client.shouldUseVersioning("thand_local_alpha_server01"))
	assert.False(t, client.shouldUseVersioning(models.TemporalDeviceRegistryTaskQueue))
}

func TestWorkerOptionsForIdentityKeepsRegistryQueueUnversioned(t *testing.T) {
	t.Parallel()

	client := NewTemporalClient(
		&models.TemporalConfig{
			Host:              "localhost",
			Port:              7233,
			Namespace:         "default",
			DisableVersioning: false,
		},
		nil,
		"thand_local_alpha_server01",
		models.TemporalDeviceRegistryTaskQueue,
	)

	operational := client.workerOptionsForIdentity("thand_local_alpha_server01", "build-123")
	registry := client.workerOptionsForIdentity(models.TemporalDeviceRegistryTaskQueue, "build-123")

	assert.True(t, operational.DeploymentOptions.UseVersioning)
	assert.False(t, registry.DeploymentOptions.UseVersioning)
}
