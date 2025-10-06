package services

import (
	scheduler "github.com/thand-io/agent/internal/config/services/scheduler"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureScheduler() models.SchedulerImpl {

	provider := "local"

	schedulerConfig := e.GetServicesConfig().GetSchedulerConfig()

	if e.config.Scheduler != nil && len(e.config.Scheduler.Provider) > 0 {
		provider = schedulerConfig.GetProvider()
	} else if e.environment != nil && len(e.environment.Platform) > 0 {
		provider = string(e.environment.Platform)
	}

	// This allows us to pass in any config values defined in the environment
	configValues := e.config.GetSchedulerConfigWithDefaults(e.GetEnvironmentConfig().Config)

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
