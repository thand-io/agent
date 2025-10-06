package scheduler

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

type gcpScheduler struct {
	config *models.BasicConfig
}

func NewGcpSchedulerFromConfig(config *models.BasicConfig) *gcpScheduler {
	return &gcpScheduler{
		config: config,
	}
}

func (g *gcpScheduler) Initialize() error {
	// Initialize GCP Scheduler here
	return fmt.Errorf("GCP Scheduler not yet implemented")
}

func (g *gcpScheduler) Shutdown() error {
	// Shutdown GCP Scheduler here
	return fmt.Errorf("GCP Scheduler not yet implemented")
}

func (g *gcpScheduler) AddJob(job models.JobImpl) error {
	// Add job to GCP Scheduler here
	return fmt.Errorf("GCP Scheduler not yet implemented")
}

func (g *gcpScheduler) RemoveJob(job models.JobImpl) error {
	// Remove job from GCP Scheduler here
	return fmt.Errorf("GCP Scheduler not yet implemented")
}
