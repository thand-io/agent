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
		go func() {
			// Post services setup initialization, we need to do some additional setup for certain services that are dependent on the configuration being fully loaded.
			err = c.SetupTemporal()
			if err != nil {
				logrus.WithError(err).Error("Failed to set up temporal services")
			}
		}()
	})

	return c.servicesClient

}

func (c *Config) SetupTemporal() error {

	if c.GetServices() != nil && c.GetServices().GetTemporal() != nil {

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

		if c.IsServer() {
			if err := c.EnsureDeviceRegistryWorkflows(context.Background()); err != nil {
				return fmt.Errorf("ensuring device registries: %w", err)
			}
			if err := c.PublishConfiguredDeviceDefinitions(context.Background()); err != nil {
				return fmt.Errorf("publishing device definitions: %w", err)
			}
		}

		return nil

	}

	return nil

}
