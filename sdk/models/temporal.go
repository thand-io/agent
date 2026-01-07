package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// TemporalService provides an interface to Temporal, a durable workflow orchestration engine
// that powers Thand's access provisioning and approval workflows. It handles workflow execution,
// retries, timeouts, and state management across distributed systems.
//
// Used for access request workflows, just-in-time provisioning, time-based access expiration,
// approval chains, and provider synchronization. Configure in config.yaml under services.temporal.
type TemporalService = internal.TemporalImpl
