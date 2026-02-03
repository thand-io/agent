package config

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/services"
	"github.com/thand-io/agent/internal/models"
)

func (c *Config) GetServices() models.ServicesClientImpl {

	c.initializeServiceClientOnce.Do(func() {
		newClient := services.NewServicesClient(
			&c.Environment,
			&c.Services,
			&c.Secret,
		)
		err := newClient.Initialize()
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to initialize services client: %v", err)
			return
		}
		c.servicesClient = newClient
	})

	return c.servicesClient

}

func (c *Config) ReloadServices() error {

	if c.GetServices() != nil && c.GetServices().GetTemporal() != nil {

		logrus.Infoln("Setting up temporal services...")
		err := c.setupTemporalServices()

		if err != nil {
			return fmt.Errorf("setting up temporal services: %w", err)
		}

	}

	return nil

}