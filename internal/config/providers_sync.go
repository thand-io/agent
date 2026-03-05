package config

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func (c *Config) synchronizeProvider(p models.Provider) {

	if p == nil {
		logrus.Warningln("Provider is nil, cannot synchronize")
		return
	}

	if !c.IsServer() {
		logrus.Debugln("Not a server instance, skipping provider synchronization for provider: ", p.GetIdentifier())
		p.SetReady()
		return
	}

	var temporalClient models.TemporalImpl

	// First check if we have temporal capabilities
	services := c.GetServices()

	if services != nil {
		temporalClient = services.GetTemporal()
	}

	p.SetPending()

	go func() {
		// Always mark the provider as ready when this goroutine exits,
		// even if synchronization fails. This prevents consumers from
		// blocking indefinitely. Roles loaded before the error are still
		// available; roles that failed to load will simply be absent.
		defer p.SetReady()

		syncRequest := models.SynchronizeRequest{
			ProviderIdentifier: p.GetIdentifier(),
		}

		logrus.Infoln("Requesting synchronization for provider: ", p.GetIdentifier())

		err := p.Synchronize(
			context.Background(),
			temporalClient,
			&syncRequest,
		)

		if err != nil {
			logrus.WithError(err).Errorln("Failed to synchronize provider:", p.GetIdentifier())
			return
		}

		logrus.Infoln("Synchronized provider successfully:", p.GetIdentifier())
	}()

}
