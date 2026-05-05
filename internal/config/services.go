package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/services"
	"github.com/thand-io/agent/internal/models"
)

func (c *Config) GetServices() models.ServicesClientImpl {

	c.initializeServiceClientOnce.Do(func() {
		newClient := services.NewServicesClient(c)
		err := newClient.Initialize()
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to initialize services client: %v", err)
			return
		}
		c.servicesClient = newClient
	})

	return c.servicesClient

}

func (c *Config) SetupTemporal() error {
	if c.servicesClient != nil && c.servicesClient.GetTemporal() != nil {

		logrus.Infoln("Setting up temporal workflows and activities...")

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

	return nil

}

func (c *Config) StartTemporalWorkers() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return nil
	}

	if err := c.servicesClient.GetTemporal().StartWorkers(); err != nil {
		return fmt.Errorf("starting temporal workers: %w", err)
	}

	// Start the long-running per-system server/agent workflow only after
	// workers are running. When worker versioning is enabled, the pinned
	// deployment version is only registered with the Temporal server once
	// the worker has polled the task queue, so submitting a pinned workflow
	// before that point fails with "Pinned version ... is not present in
	// task queue". GetClient() on the temporal service blocks until version
	// registration completes, making this safe.
	if err := c.StartSystemWorkflow(); err != nil {
		return fmt.Errorf("starting system workflow: %w", err)
	}

	return nil
}
