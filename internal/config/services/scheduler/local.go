package scheduler

import (
	"time"

	"github.com/go-co-op/gocron"
	"github.com/google/uuid"
	"github.com/thand-io/agent/internal/models"
)

type CronScheduler struct {
	scheduler *gocron.Scheduler
	jobs      map[uuid.UUID]*gocron.Job
}

func NewLocalSchedulerFromConfig(config *models.BasicConfig) *CronScheduler {
	c := &CronScheduler{}
	c.scheduler = gocron.NewScheduler(time.UTC)
	c.jobs = make(map[uuid.UUID]*gocron.Job)
	return c
}

func (c *CronScheduler) Initialize() error {
	c.scheduler.StartAsync()
	return nil
}

func (c *CronScheduler) Shutdown() error {
	c.scheduler.Stop()
	return nil
}

func (c *CronScheduler) AddJob(job models.JobImpl) error {

	// For one-time execution at a specific time, use StartAt with LimitRunsTo(1)
	cjb, err := c.scheduler.Every(1).Day().StartAt(job.GetAt()).LimitRunsTo(1).Do(job.GetTask())
	if err != nil {
		return err
	}
	c.jobs[job.GetId()] = cjb
	return nil
}

func (c *CronScheduler) RemoveJob(job models.JobImpl) error {
	if foundJob, exists := c.jobs[job.GetId()]; exists {
		c.scheduler.RemoveByID(foundJob)
		delete(c.jobs, job.GetId())
		return nil
	}
	return nil
}
