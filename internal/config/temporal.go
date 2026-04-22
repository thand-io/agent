package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
)

// Register temporal workflows and activities
func (c *Config) registerTemporalWorkflows() error {

	if c.GetServices() == nil || c.GetServices().GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	if !c.IsServer() {
		return nil
	}

	// Registry singletons live on the shared device-registry queue rather than
	// the per-server operational queue.
	registryWorker := c.getDeviceRegistryWorker()
	if registryWorker == nil {
		return fmt.Errorf("device registry worker is not initialized")
	}

	registryWorker.RegisterWorkflowWithOptions(
		deviceRouteRegistryWorkflow,
		workflow.RegisterOptions{
			Name:               models.TemporalDeviceRouteRegistryWorkflowName,
			VersioningBehavior: workflow.VersioningBehaviorAutoUpgrade,
		},
	)
	registryWorker.RegisterWorkflowWithOptions(
		deviceDefinitionRegistryWorkflow,
		workflow.RegisterOptions{
			Name:               models.TemporalDeviceDefinitionRegistryWorkflowName,
			VersioningBehavior: workflow.VersioningBehaviorAutoUpgrade,
		},
	)

	return nil

}

func (c *Config) registerTemporalActivities() error {

	if c.GetServices() == nil || c.GetServices().GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalWorker := c.getOperationalTemporalWorker()

	if temporalWorker == nil {
		return fmt.Errorf("temporal worker is not initialized")
	}

	thandActivities := &thandActivities{
		config: c,
	}

	if c.HasThandService() {

		logrus.Info("Registering upstream patching activities for Thand service")

		temporalWorker.RegisterActivityWithOptions(
			thandActivities.PatchProviderUpstream,
			activity.RegisterOptions{
				Name: models.TemporalPatchProviderUpstreamActivityName,
			},
		)

	} else {

		logrus.Info("Registering dummy upstream patching activities (Thand service not configured)")

		temporalWorker.RegisterActivityWithOptions(
			thandActivities.PatchProviderUpstreamDummy,
			activity.RegisterOptions{
				Name: models.TemporalPatchProviderUpstreamActivityName,
			},
		)
	}

	temporalWorker.RegisterActivityWithOptions(
		thandActivities.ResolveFreshDeviceRoute,
		activity.RegisterOptions{
			Name: models.TemporalResolveFreshDeviceRouteActivityName,
		},
	)
	temporalWorker.RegisterActivityWithOptions(
		thandActivities.BuildExecutionPlan,
		activity.RegisterOptions{
			Name: models.TemporalBuildExecutionPlanActivityName,
		},
	)

	return nil

}
