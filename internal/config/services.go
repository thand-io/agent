package config

import (
	"context"
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

		logrus.Infoln("Setting up temporal services...")

		if !c.IsServer() && !c.IsAgent() {
			return fmt.Errorf("temporal services can only be set up in server or agent mode")
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

		if err := c.EnsureProviderTemporalBindings(); err != nil {
			return fmt.Errorf("registering provider temporal bindings: %w", err)
		}

		return nil

	}

	return nil

}

func (c *Config) StartTemporalWorkers() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return nil
	}

	if !c.IsServer() {
		return nil
	}

	if err := c.servicesClient.GetTemporal().StartWorkers(); err != nil {
		return fmt.Errorf("starting temporal workers: %w", err)
	}

	// Device registry workflow management calls GetClient(), which blocks
	// until workers have started. Run after StartWorkers so the client is
	// ready, instead of during registration in SetupTemporal where it would
	// deadlock.
	if err := c.EnsureDeviceRegistryWorkflows(context.Background()); err != nil {
		return fmt.Errorf("ensuring device registries: %w", err)
	}
	if err := c.PublishConfiguredDeviceDefinitions(context.Background()); err != nil {
		return fmt.Errorf("publishing device definitions: %w", err)
	}

	return nil
}
