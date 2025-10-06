package models

// WorkflowAction defines a workflow action
type WorkflowAction struct {
	Call string         `json:"call" yaml:"call"`
	With map[string]any `json:"with,omitempty" yaml:"with,omitempty"`
}

// Enum for health status
const (
	HealthStatusHealthy   HealthState = "healthy"
	HealthStatusDegraded  HealthState = "degraded"
	HealthStatusUnhealthy HealthState = "unhealthy"
)

type HealthState string

// HealthResponse represents the response for health check
type HealthResponse struct {
	Status      HealthState            `json:"status"`
	ApiBasePath string                 `json:"path"`
	Timestamp   string                 `json:"timestamp"`
	Version     string                 `json:"version"`
	Services    map[string]HealthState `json:"services,omitempty"`
}

// MetricsInfo represents basic metrics information
type MetricsInfo struct {
	Uptime          string `json:"uptime"`
	TotalRequests   int64  `json:"total_requests"`
	RolesCount      int    `json:"roles_count"`
	WorkflowsCount  int    `json:"workflows_count"`
	ProvidersCount  int    `json:"providers_count"`
	ElevateRequests int64  `json:"elevate_requests"`
}
