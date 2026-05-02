package config

import (
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// temporalWorkerScope narrows a Temporal service view to a specific worker set.
// In server mode we run both the operational worker queue and the shared device
// registry queue, and this wrapper keeps ordinary workflow/activity/provider
// registrations from accidentally landing on the registry worker.
type temporalWorkerScope struct {
	base      models.TemporalImpl
	workerIDs []string
}

func (t *temporalWorkerScope) Initialize() error {
	return t.base.Initialize()
}

func (t *temporalWorkerScope) Shutdown() error {
	return t.base.Shutdown()
}

func (t *temporalWorkerScope) StartWorkers() error {
	return t.base.StartWorkers()
}

func (t *temporalWorkerScope) GetClient() client.Client {
	return t.base.GetClient()
}

func (t *temporalWorkerScope) HasClient() bool {
	return t.base.HasClient()
}

func (t *temporalWorkerScope) GetWorker(identities ...string) worker.Worker {
	if len(identities) == 0 {
		identities = t.workerIDs
	}
	return t.base.GetWorker(identities...)
}

func (t *temporalWorkerScope) HasWorker() bool {
	return t.base.HasWorker()
}

func (t *temporalWorkerScope) GetHostPort() string {
	return t.base.GetHostPort()
}

func (t *temporalWorkerScope) GetNamespace() string {
	return t.base.GetNamespace()
}

func (t *temporalWorkerScope) GetTaskQueue() string {
	return t.base.GetTaskQueue()
}

func (t *temporalWorkerScope) IsVersioningDisabled() bool {
	return t.base.IsVersioningDisabled()
}

// getOperationalTemporalWorker returns the worker that should own normal server
// workflows and activities. Device-registry singletons are registered on a
// separate shared queue.
func (c *Config) getOperationalTemporalWorker() worker.Worker {
	temporalService := c.servicesClient.GetTemporal()
	if temporalService == nil {
		return nil
	}
	if c.IsServer() {
		return temporalService.GetWorker(temporalService.GetTaskQueue())
	}
	return temporalService.GetWorker()
}

// getOperationalTemporalService scopes provider workflow/activity registration
// away from the shared device-registry queue in server mode.
func (c *Config) getOperationalTemporalService() models.TemporalImpl {
	temporalService := c.servicesClient.GetTemporal()
	if temporalService == nil {
		return nil
	}
	if c.IsServer() {
		return &temporalWorkerScope{
			base:      temporalService,
			workerIDs: []string{temporalService.GetTaskQueue()},
		}
	}
	return temporalService
}

// getDeviceRegistryWorker returns the shared worker that owns device registry
// singleton workflows across servers.
func (c *Config) getDeviceRegistryWorker() worker.Worker {
	temporalService := c.servicesClient.GetTemporal()
	if temporalService == nil {
		return nil
	}
	return temporalService.GetWorker(models.TemporalDeviceRegistryTaskQueue)
}
