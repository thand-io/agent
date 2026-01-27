package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/activity"
)

func (c *Config) setupTemporalServices() error {

	if !c.IsServer() {
		return fmt.Errorf("temporal services can only be set up in server mode")
	}

	// Register workflows
	err := c.registerTemporalWorkflows()
	if err != nil {
		return fmt.Errorf("registering temporal workflows: %w", err)
	}

	// Register activities
	err = c.registerTemporalActivities()
	if err != nil {
		return fmt.Errorf("registering temporal activities: %w", err)
	}

	return nil
}

// Register temporal workflows and activities
func (c *Config) registerTemporalWorkflows() error {

	if c.GetServices() == nil || c.GetServices().GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalWorker := c.servicesClient.GetTemporal().GetWorker()

	if temporalWorker == nil {
		return fmt.Errorf("temporal worker is not initialized")
	}

	return nil

}

func (c *Config) registerTemporalActivities() error {

	if c.GetServices() == nil || c.GetServices().GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalWorker := c.servicesClient.GetTemporal().GetWorker()

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

	return nil

}
