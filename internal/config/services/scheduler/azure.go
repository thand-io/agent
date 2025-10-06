package scheduler

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

type azureScheduler struct {
	config *models.BasicConfig
}

func NewAzureSchedulerFromConfig(config *models.BasicConfig) *azureScheduler {
	return &azureScheduler{
		config: config,
	}
}

// Implement the methods for azureScheduler here
// For example:
func (a *azureScheduler) Initialize() error {
	// Initialize Azure Scheduler here
	return fmt.Errorf("Azure Scheduler not yet implemented")
}

func (a *azureScheduler) Shutdown() error {
	// Shutdown Azure Scheduler here
	return fmt.Errorf("Azure Scheduler not yet implemented")
}

func (a *azureScheduler) AddJob(job models.JobImpl) error {
	// Add job to Azure Scheduler here
	return fmt.Errorf("Azure Scheduler not yet implemented")
}

func (a *azureScheduler) RemoveJob(job models.JobImpl) error {
	// Remove job from Azure Scheduler here
	return fmt.Errorf("Azure Scheduler not yet implemented")
}
