package models

import internal "github.com/thand-io/agent/internal/models"

// ServicesConfig defines the configuration for external services
// that the agent integrates with for notifications, storage, and other features.
type ServicesConfig = internal.ServicesConfig

// ServicesClientImpl is the runtime service registry. Implementations provide
// access to optional services such as encryption, Temporal, vault, scheduler
// and analytics. External callers implement this interface as part of ConfigImpl.
type ServicesClientImpl = internal.ServicesClientImpl

// Analytics is the interface for the analytics service.
type Analytics = internal.Analytics
