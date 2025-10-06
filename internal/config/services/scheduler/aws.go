package scheduler

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

type awsScheduler struct {
	config *models.BasicConfig
}

func NewAwsSchedulerFromConfig(config *models.BasicConfig) *awsScheduler {
	return &awsScheduler{
		config: config,
	}
}

func (a *awsScheduler) Initialize() error {
	// Initialize AWS Scheduler here
	return fmt.Errorf("AWS Scheduler not yet implemented")
}

func (a *awsScheduler) Shutdown() error {
	// Shutdown AWS Scheduler here
	return fmt.Errorf("AWS Scheduler not yet implemented")
}

func (a *awsScheduler) AddJob(job models.JobImpl) error {
	// Add job to AWS Scheduler here
	return fmt.Errorf("AWS Scheduler not yet implemented")
}

func (a *awsScheduler) RemoveJob(job models.JobImpl) error {
	// Remove job from AWS Scheduler here
	return fmt.Errorf("AWS Scheduler not yet implemented")
}
