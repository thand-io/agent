package services

import (
	"github.com/sirupsen/logrus"
	analytics "github.com/thand-io/agent/internal/config/services/analytics"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureAnalytics() models.Analytics {

	provider := "posthog"

	analyticsConfig := e.GetServicesConfig().GetAnalyticsConfig()

	if analyticsConfig != nil && len(analyticsConfig.GetProvider()) > 0 {
		provider = analyticsConfig.GetProvider()
	}

	if analyticsConfig != nil && analyticsConfig.Disabled {
		logrus.Infoln("Analytics service is disabled")
		return nil
	}

	// Initialise Analytics client
	switch provider {
	case "posthog":
		fallthrough
	default:
		return analytics.NewPosthogAnalytics(analyticsConfig)
	}

}
