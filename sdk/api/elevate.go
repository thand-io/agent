// Package api re-exports the transport-agnostic elevation API from internal/api
// so that external consumers (CLI plugins, gRPC adapters, future integrations)
// can depend on sdk/api without importing internal packages directly.
package api

import (
	internalapi "github.com/thand-io/agent/internal/api"
)

// Type aliases — re-exporting the concrete types so callers can use them
// without a direct dependency on internal/api.
type (
	Service        = internalapi.Service
	ElevationInput = internalapi.ElevationInput
	ResumeInput    = internalapi.ResumeInput
)

// Errors re-exported from internal/api.
var (
	ErrWorkflowNotFound = internalapi.ErrWorkflowNotFound
)
