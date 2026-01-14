package services

import (
	pki "github.com/thand-io/agent/internal/config/services/pki"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configurePublicKeyInfrastructure() models.PublicKeyInfrastructure {

	provider := "local"

	pkiConfig := e.GetServicesConfig().GetPublicKeyInfrastructureConfig()

	if e.config.PublicKeyInfrastructure != nil && len(e.config.PublicKeyInfrastructure.Provider) > 0 {
		provider = e.config.PublicKeyInfrastructure.Provider
	}

	// Initialise PKI client
	switch provider {
	case "local":
		fallthrough
	default:
		return pki.NewLocalPublicKeyInfrastructure(pkiConfig)
	}

}
