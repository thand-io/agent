package models

import (
	"time"

	"github.com/google/uuid"
)

type SchedulerImpl interface {
	Initialize() error
	Shutdown() error

	AddJob(job JobImpl) error
	RemoveJob(job JobImpl) error
}

type JobImpl interface {
	GetId() uuid.UUID
	GetSchedule() string
	GetAt() time.Time
	GetTask() func()
}

type Job struct {
	ID       uuid.UUID  `json:"id"`
	Schedule *string    `json:"schedule"`
	At       *time.Time `json:"at"`
	Task     func()     `json:"-"`
}

func (j *Job) GetId() uuid.UUID {
	return j.ID
}

func (j *Job) GetSchedule() string {
	if j.Schedule != nil {
		return *j.Schedule
	}
	return ""
}

func (j *Job) GetAt() time.Time {
	if j.At != nil {
		return *j.At
	}
	return time.Time{}
}

func (j *Job) GetTask() func() {
	return j.Task
}

func NewAtJob(at time.Time, task func()) *Job {
	return &Job{
		ID:       uuid.New(),
		Schedule: nil,
		At:       &at,
		Task:     task,
	}
}

func NewScheduledJob(schedule string, task func()) *Job {
	return &Job{
		ID:       uuid.New(),
		Schedule: &schedule,
		At:       nil,
		Task:     task,
	}
}
