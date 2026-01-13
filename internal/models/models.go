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
	
	RequestsCount   int64  `json:"requests_count"`
	ElevateRequests int64  `json:"elevate_requests_count"`

	RolesCount      int64    `json:"roles_count"`
	WorkflowsCount  int64    `json:"workflows_count"`
	ProvidersCount  int64    `json:"providers_count"`

	IdentitiesCount int64    `json:"identities_count"`
	TenantsCount    int64    `json:"tenants_count"`
}
