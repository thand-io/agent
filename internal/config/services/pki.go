package services

import (
	pki "github.com/thand-io/agent/internal/config/services/pki"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configurePublicKeyInfrastructure() models.PublicKeyInfrastructure {

	provider := "local"

	pkiConfig := e.GetServicesConfig().GetPublicKeyInfrastructureConfig()

	if pkiConfig != nil && len(pkiConfig.GetProvider()) > 0 {
		provider = pkiConfig.GetProvider()
	}

	// Initialise PKI client
	switch provider {
	case "local":
		fallthrough
	default:
		return pki.NewLocalPublicKeyInfrastructure(pkiConfig)
	}

}
