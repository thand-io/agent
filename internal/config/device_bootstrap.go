package config

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

const (
	deviceBootstrapInitialBackoff = 2 * time.Second
	deviceBootstrapMaxBackoff     = 1 * time.Minute
)

func (c *Config) BootstrapDeviceWithLoginServer() error {
	if !c.IsAgent() {
		return fmt.Errorf("device bootstrap is only valid in agent mode")
	}
	if !c.HasLoginServer() {
		return fmt.Errorf("no login server endpoint configured")
	}

	registration, err := c.RegisterWithLoginServer(nil)
	if err != nil {
		return err
	}

	if err := c.applyRegistrationConfiguration(registration); err != nil {
		return err
	}

	environment := c.GetEnvironmentConfig()
	logrus.WithFields(logrus.Fields{
		"device_id":  common.GetDeviceID().String(),
		"name":       environment.Name,
		"hostname":   environment.Hostname,
		"platform":   environment.Platform,
		"has_config": registration != nil,
	}).Info("Bootstrapped agent configuration from login server")

	if err := c.EnsureProviderTemporalBindings(); err != nil {
		return fmt.Errorf("ensuring provider temporal bindings: %w", err)
	}

	if err := c.PublishCurrentAgentRoute(context.Background()); err != nil {
		return fmt.Errorf("publishing current device route: %w", err)
	}

	return nil
}

func (c *Config) RefreshDeviceRegistrationWithLoginServer() error {
	if !c.IsAgent() {
		return fmt.Errorf("device refresh is only valid in agent mode")
	}
	if !c.HasLoginServer() {
		return fmt.Errorf("no login server endpoint configured")
	}

	registration, err := c.RefreshLoginServerRegistration(nil)
	if err != nil {
		return err
	}

	if err := c.applyRegistrationConfiguration(registration); err != nil {
		return err
	}

	environment := c.GetEnvironmentConfig()
	logrus.WithFields(logrus.Fields{
		"device_id": common.GetDeviceID().String(),
		"name":      environment.Name,
		"hostname":  environment.Hostname,
		"platform":  environment.Platform,
	}).Debug("Refreshed device registration with login server")

	if err := c.PublishCurrentAgentRoute(context.Background()); err != nil {
		return fmt.Errorf("publishing current device route: %w", err)
	}

	return nil
}

func (c *Config) RunDeviceBootstrap(ctx context.Context) {
	backoff := deviceBootstrapInitialBackoff
	bootstrapped := false

	for {
		var err error
		if bootstrapped {
			err = c.RefreshDeviceRegistrationWithLoginServer()
		} else {
			err = c.BootstrapDeviceWithLoginServer()
		}
		if err == nil {
			bootstrapped = true
			backoff = deviceBootstrapInitialBackoff

			timer := time.NewTimer(deviceRouteRefreshInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			continue
		}

		logrus.WithError(err).WithField("retry_in", backoff).Warn("device bootstrap failed; retrying")

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		backoff *= 2
		if backoff > deviceBootstrapMaxBackoff {
			backoff = deviceBootstrapMaxBackoff
		}
	}
}

func (c *Config) applyRegistrationConfiguration(registration *RegistrationResponse) error {
	if registration == nil {
		return nil
	}

	if registration.Roles == nil &&
		registration.Workflows == nil &&
		registration.Providers == nil {
		return nil
	}

	beforeGeneration := c.getConfigGeneration()
	if err := c.MergeConfiguration(registration); err != nil {
		return fmt.Errorf("merging registration configuration: %w", err)
	}

	if c.getConfigGeneration() == beforeGeneration {
		return nil
	}

	if err := c.InitializeProviders(); err != nil {
		return fmt.Errorf("initializing providers from registration configuration: %w", err)
	}

	if !c.IsClient() {
		go func() {
			if err := c.ReloadRoleIndexes(); err != nil {
				logrus.WithError(err).Errorln("Failed to reload role indexes after registration configuration update")
			}
		}()
	}

	return nil
}

func (c *Config) getConfigGeneration() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.configGeneration
}

func (c *Config) PublishCurrentAgentRoute(ctx context.Context) error {
	services := c.GetServices()
	if services == nil || !services.HasTemporal() {
		logrus.Debug("Skipping current device route publication because Temporal is unavailable")
		return nil
	}

	temporalService := services.GetTemporal()
	if temporalService == nil || !temporalService.HasClient() {
		logrus.Debug("Skipping current device route publication because the Temporal client is unavailable")
		return nil
	}

	return c.publishCurrentAgentRoute(ctx, c.PublishDeviceConnectionState)
}

func (c *Config) publishCurrentAgentRoute(
	ctx context.Context,
	publish func(context.Context, models.DeviceConnectionState) error,
) error {
	if !c.IsAgent() {
		return fmt.Errorf("current device route publication is only valid in agent mode")
	}
	if publish == nil {
		return fmt.Errorf("device route publisher is required")
	}

	environment := c.GetEnvironmentConfig()
	state := models.DeviceConnectionState{
		DeviceID:  common.GetDeviceID().String(),
		TaskQueue: environment.GetIdentifier(),
		Name:      c.GetEnvironment().Name,
		Hostname:  c.GetEnvironment().Hostname,
		Platform:  string(c.GetEnvironment().Platform),
	}

	return publish(ctx, state)
}
