// Package api re-exports the transport-agnostic elevation API from internal/api
// so that external consumers (CLI plugins, gRPC adapters, future integrations)
// can depend on sdk/api without importing internal packages directly.
package api

import (
	internalapi "github.com/thand-io/agent/internal/api"
)


// NewApiService creates a new Service.
var NewApiService = internalapi.NewApiService
