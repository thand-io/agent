package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// SchedulerService provides an interface for scheduling and managing time-based tasks
// and cron jobs within Thand workflows. It enables automated execution of recurring
// tasks, time-delayed operations, and scheduled workflow triggers.
//
// Common use cases:
//   - Periodic access reviews and recertification (e.g., quarterly reviews)
//   - Scheduled provisioning and deprovisioning (e.g., temporary access expiration)
//   - Regular synchronization of provider data (users, roles, permissions)
//   - Automated compliance checks and reporting on a schedule
//   - Time-based workflow triggers (e.g., daily access summaries)
//   - Scheduled notifications and reminders
//   - Cleanup tasks for expired sessions or cached data
//
// The scheduler supports:
//   - Cron-style schedules for recurring tasks (e.g., "0 0 * * *" for daily at midnight)
//   - One-time scheduled execution at a specific time
//   - Job management (add, remove, update scheduled tasks)
//   - Graceful shutdown to complete in-progress jobs
//
// Supported providers:
//   - Local: In-memory scheduler using Go routines and timers
//   - AWS: CloudWatch Events / EventBridge for serverless scheduling
//   - GCP: Cloud Scheduler for managed cron jobs
//   - Azure: Logic Apps or Azure Functions with timer triggers
//
// Configure the scheduler service in your config.yaml under services.scheduler
// with provider-specific settings for your deployment environment.
type SchedulerService = internal.SchedulerImpl
