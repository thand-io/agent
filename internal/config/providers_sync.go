package config

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

func (c *Config) synchronizeProvider(p *models.Provider) {

	if !c.IsServer() {
		logrus.Debugln("Not a server instance, skipping provider synchronization")
		return
	}

	if p == nil {
		logrus.Warningln("Provider is nil, cannot synchronize")
		return
	}

	impl := p.GetClient()

	if impl == nil {
		logrus.Warningln("Provider client is nil, cannot synchronize:", p.Name)
		return
	}

	var temporalClient models.TemporalImpl

	// First check if we have temporal capabilities
	services := c.GetServices()

	if services != nil {
		temporalClient = services.GetTemporal()
	}

	go func() {

		syncRequest := models.SynchronizeRequest{
			ProviderIdentifier: impl.GetIdentifier(),
		}

		err := impl.Synchronize(
			context.Background(),
			temporalClient,
			&syncRequest,
		)

		if err != nil {
			logrus.WithError(err).Errorln("Failed to synchronize provider:", p.Name)
			return
		}

		logrus.Infoln("Synchronized provider successfully:", p.Name)

	}()

}
