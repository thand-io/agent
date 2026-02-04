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

// TemporalConfig holds configuration settings for connecting to a Temporal server.
type TemporalConfig = internal.TemporalConfig

// TemporalAuthAPIKey holds API key authentication settings for connecting to Temporal using
// API keys.
type TemporalAuthAPIKey = internal.TemporalAuthAPIKey

// TemporalAuthMTLSFile holds mTLS authentication settings for connecting to Temporal using
// certificate files.
type TemporalAuthMTLSFile = internal.TemporalAuthMTLSFile

// TemporalAuthMTLSInline holds mTLS authentication settings for connecting to Temporal using
// inline certificate data.
type TemporalAuthMTLSInline = internal.TemporalAuthMTLSInline

// TemporalAuthMTLSVault holds mTLS authentication settings for connecting to Temporal using
// certificates stored in a vault.
type TemporalAuthMTLSVault = internal.TemporalAuthMTLSVault
