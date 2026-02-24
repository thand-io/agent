package models

import internal "github.com/thand-io/agent/internal/models"

// ServerConfig defines the main server configuration including
// network settings, timeouts, and general server behavior.
type ServerConfig = internal.ServerConfig

// ServerLimitsConfig defines resource limits for the server,
// such as maximum connections, request sizes, and memory constraints.
type ServerLimitsConfig = internal.ServerLimitsConfig

// LoginConfig defines the authentication configuration for user login,
// including supported methods and session settings.
type LoginConfig = internal.LoginConfig

// LoggingConfig defines the logging configuration for the agent,
// including log levels, output destinations, and formatting options.
type LoggingConfig = internal.LoggingConfig

// OpenTelemetryConfig defines the OpenTelemetry configuration for
// distributed tracing and observability instrumentation.
type OpenTelemetryConfig = internal.OpenTelemetryConfig

// MetricsConfig defines the configuration for metrics collection
// and export, including endpoints and reporting intervals.
type MetricsConfig = internal.MetricsConfig

// HealthConfig defines the health check endpoint configuration,
// including the path and response behavior for liveness probes.
type HealthConfig = internal.HealthConfig

// ReadyConfig defines the readiness check endpoint configuration,
// including the path and conditions for determining service readiness.
type ReadyConfig = internal.ReadyConfig

// SecurityConfig defines security settings for the server,
// including TLS configuration and authentication requirements.
type SecurityConfig = internal.SecurityConfig

// CORSConfig defines Cross-Origin Resource Sharing settings,
// specifying allowed origins, methods, and headers for cross-origin requests.
type CORSConfig = internal.CORSConfig

// APIConfig defines the API configuration including versioning,
// documentation endpoints, and request/response formatting options.
type APIConfig = internal.APIConfig

// RateLimitConfig defines rate limiting settings for API endpoints,
// including request limits, time windows, and throttling behavior.
type RateLimitConfig = internal.RateLimitConfig

// ConfigImpl is the interface that must be implemented to provide the agent SDK
// with environment, service, provider, role, workflow, and URL resolution. External
// callers (e.g. the platform server) implement this interface to drive the
// transport-agnostic API layer (sdk/api).
type ConfigImpl = internal.ConfigImpl
