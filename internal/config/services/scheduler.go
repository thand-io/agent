package services

import (
	scheduler "github.com/thand-io/agent/internal/config/services/scheduler"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureScheduler() models.SchedulerImpl {

	provider := "local"

	schedulerConfig := e.config.GetServicesConfig().GetSchedulerConfig()

	if schedulerConfig != nil && len(schedulerConfig.GetProvider()) > 0 {
		provider = schedulerConfig.GetProvider()
	} else if e.config.GetEnvironmentConfig() != nil && len(e.config.GetEnvironmentConfig().Platform) > 0 {
		provider = string(e.config.GetEnvironmentConfig().Platform)
	}

	// This allows us to pass in any config values defined in the environment
	configValues := e.config.GetServicesConfig().GetSchedulerConfigWithDefaults(e.config.GetEnvironmentConfig().Config)

	switch provider {
	case string(models.AWS):
		// AWS Scheduler - KMS
		awsScheduler := scheduler.NewAwsSchedulerFromConfig(configValues)
		return awsScheduler
	case string(models.GCP):
		// GCP Scheduler - KMS
		gcpScheduler := scheduler.NewGcpSchedulerFromConfig(configValues)
		return gcpScheduler
	case string(models.Azure):
		// Azure Scheduler - KMS
		azureScheduler := scheduler.NewAzureSchedulerFromConfig(configValues)
		return azureScheduler
	case string(models.Local):
		fallthrough
	default:
		localScheduler := scheduler.NewLocalSchedulerFromConfig(configValues)
		return localScheduler
	}

}
