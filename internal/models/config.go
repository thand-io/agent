package models

import (
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

type ConfigImpl interface {

	// Core
	GetServices() ServicesClientImpl
	GetEnvironment() EnvironmentConfig
	GetSecret() string
	GetLoginServerHostname() string

	// Mode checking
	IsServer() bool
	IsAgent() bool
	IsClient() bool

	// Config structures
	GetServicesConfig() *ServicesConfig
	GetEnvironmentConfig() *EnvironmentConfig

	GetResumeCallbackUrl(workflowTask *ElevateWorkflowTask) string
	GetAuthCallbackUrl(providerName string) string
	GetSignalCallbackUrl(workflowTask *ElevateWorkflowTask) string
	GetLoginServerUrl() string
	GetLocalServerUrl() string

	// Roles
	GetCompositeRole(identity *Identity, baseRole *Role, providers ...Provider) (*CompositeRole, error)
	GetCompositeRoleForWorkflow(identity *Identity, workflow *ElevateWorkflowTask, providers ...Provider) (*CompositeRole, error)

	// Identities
	GetIdentity(byEmail string) (*Identity, error)

	// Tenants
	GetTenant(name string) (*ProviderTenant, error)

	// Workflows
	GetWorkflowByName(name string) (*Workflow, error)
	GetWorkflowFromElevationRequest(elevationRequest *ElevateRequest) (*Workflow, error)

	// Providers
	GetProviderByName(name string) (Provider, error)
	GetProvidersByCapability(capability ...ProviderCapability) map[string]Provider
	GetProvidersByCapabilityWithUser(user *User, capability ...ProviderCapability) map[string]Provider
}

type ServerConfig struct {
	Host         string             `json:"host" yaml:"host" mapstructure:"host"`
	Port         int                `json:"port" yaml:"port" mapstructure:"port"`
	Limits       ServerLimitsConfig `json:"limits" yaml:"limits" mapstructure:"limits"`
	Metrics      MetricsConfig      `json:"metrics" yaml:"metrics" mapstructure:"metrics"`
	Health       HealthConfig       `json:"health" yaml:"health" mapstructure:"health"`
	Ready        ReadyConfig        `json:"ready" yaml:"ready" mapstructure:"ready"`
	Security     SecurityConfig     `json:"security" yaml:"security" mapstructure:"security"`
	Capabilities CapabilitiesConfig `json:"capabilities" yaml:"capabilities" mapstructure:"capabilities"`
}

// CapabilitiesConfig groups server-side feature toggles.
// Add new capability sub-structs here as the feature set grows.
type CapabilitiesConfig struct {
	Elevations ElevationsConfig `json:"elevations" yaml:"elevations" mapstructure:"elevations"`
}

// ElevationsConfig controls which elevation modes are available on this server.
type ElevationsConfig struct {
	Static             StaticElevationsConfig             `json:"static" yaml:"static" mapstructure:"static"`
	Dynamic            DynamicElevationsConfig            `json:"dynamic" yaml:"dynamic" mapstructure:"dynamic"`
	LargeLanguageModel LargeLanguageModelElevationsConfig `json:"llm" yaml:"llm" mapstructure:"llm"`
}

// StaticElevationsConfig controls whether pre-defined role-based elevation requests are permitted.
// When Enabled is false, GET /elevate/static returns 403 and the Static option is hidden in the web UI.
type StaticElevationsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"true"`
}

// DynamicElevationsConfig controls whether runtime dynamic elevation requests are permitted.
// When Enabled is false, POST /elevate with a dynamic payload and GET /elevate/dynamic both return 403
// and the Dynamic option is hidden in the web UI.
type DynamicElevationsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"true"`
}

// LargeLanguageModelElevationsConfig controls whether LLM-assisted elevation requests are permitted.
// When Enabled is false, GET/POST /elevate/llm returns 403 and the AI option is hidden in the web UI.
type LargeLanguageModelElevationsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"true"`
}

type ServerLimitsConfig struct {
	ReadTimeout       time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout" yaml:"write_timeout" mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout" yaml:"idle_timeout" mapstructure:"idle_timeout"`
	RequestsPerMinute int           `json:"requests_per_minute" yaml:"requests_per_minute" mapstructure:"requests_per_minute"`
	Burst             int           `json:"burst" yaml:"burst" mapstructure:"burst"`
}

type LoginConfig struct {
	Endpoint *model.Endpoint `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint" default:"https://auth.thand.io/"`
	Base     string          `json:"base" yaml:"base" mapstructure:"base" default:"/"` // Base path for login endpoint e.g. /
}

type LoggingConfig struct {
	Level  string `json:"level" yaml:"level" mapstructure:"level" default:"info"`
	Format string `json:"format" yaml:"format" mapstructure:"format" default:"text"`
	Output string `json:"output" yaml:"output" mapstructure:"output"`

	OpenTelemetry OpenTelemetryConfig `json:"open_telemetry" yaml:"open_telemetry" mapstructure:"open_telemetry"`
}

type OpenTelemetryConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"false"`
	// Endpoint specifies the OTLP endpoint for remote logging.
	// The structure of model.Endpoint may include fields such as URL, protocol, and authentication.
	// Example YAML:
	//   endpoint:
	//     url: "https://otel-collector.example.com:4317"
	//     protocol: "grpc"
	//     auth:
	//       type: "basic"
	//       username: "user"
	//       password: "pass"
	// Refer to serverlessworkflow/sdk-go/model.Endpoint documentation for full details.
	Endpoint model.Endpoint `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint"` // OTLP endpoint for remote logging
}

type MetricsConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"true"`
	Path      string `json:"path" yaml:"path" mapstructure:"path" default:"/metrics"`
	Namespace string `json:"namespace" yaml:"namespace" mapstructure:"namespace"`
}

type HealthConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"true"`
	// Don't use /healthz as it conflicts with google k8s health checks
	Path string `json:"path" yaml:"path" mapstructure:"path" default:"/health"`
}

type ReadyConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled" mapstructure:"enabled" default:"true"`
	Path    string `json:"path" yaml:"path" mapstructure:"path" default:"/ready"`
}

type SecurityConfig struct {
	CORS     CORSConfig     `json:"cors" yaml:"cors" mapstructure:"cors"`
	Upstream UpstreamConfig `json:"upstream" yaml:"upstream" mapstructure:"upstream"`
}

func (s *SecurityConfig) IsUpstreamAuthEnabled() bool {
	return s.Upstream.Auth.IAP || s.Upstream.Auth.AVA || s.Upstream.Auth.EAP
}

type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins" yaml:"allowed_origins" mapstructure:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods" yaml:"allowed_methods" mapstructure:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers" yaml:"allowed_headers" mapstructure:"allowed_headers"`
	ExposeHeaders    []string `json:"expose_headers" yaml:"expose_headers" mapstructure:"expose_headers"`
	AllowCredentials bool     `json:"allow_credentials" yaml:"allow_credentials" mapstructure:"allow_credentials"`
	MaxAge           int      `json:"max_age" yaml:"max_age" mapstructure:"max_age"`
}

// UpstreamConfig defines upstream proxy settings including authentication and trusted sources
type UpstreamConfig struct {
	Auth UpstreamAuthConfig `json:"auth" yaml:"auth" mapstructure:"auth"`
	// Future: TrustedIPs []string for reverse proxy configuration
}

// UpstreamAuthConfig defines external authentication proxy settings for inbound requests
type UpstreamAuthConfig struct {
	IAP bool `json:"iap" yaml:"iap" mapstructure:"iap" default:"false"` // Google Identity-Aware Proxy
	AVA bool `json:"ava" yaml:"ava" mapstructure:"ava" default:"false"` // Amazon Verified Access
	EAP bool `json:"eap" yaml:"eap" mapstructure:"eap" default:"false"` // Microsoft Entra Application Proxy
}

// WithDefaults returns a CORSConfig with default values applied for any unset fields
func (c CORSConfig) WithDefaults() CORSConfig {
	if len(c.AllowedMethods) == 0 {
		c.AllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	}
	if len(c.AllowedHeaders) == 0 {
		c.AllowedHeaders = []string{
			"Origin",
			"Content-Length",
			"Content-Type",
			"Authorization",
			"Accept",
			"X-Requested-With",
		}
	}
	if c.MaxAge == 0 {
		c.MaxAge = 86400 // 24 hours
	}
	return c
}

// AddOrigins appends additional origins to the allowed list
func (c *CORSConfig) AddOrigins(origins ...string) {
	c.AllowedOrigins = append(c.AllowedOrigins, origins...)
}

type APIConfig struct {
	Version   string          `json:"version" yaml:"version" mapstructure:"version" default:"v1"`
	RateLimit RateLimitConfig `json:"rate_limit" yaml:"rate_limit" mapstructure:"rate_limit"`
}

func (api *APIConfig) GetVersion() string {
	if len(api.Version) > 0 {
		return api.Version
	}
	return "v1"
}

type RateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute" yaml:"requests_per_minute" mapstructure:"requests_per_minute"`
	Burst             int `json:"burst" yaml:"burst" mapstructure:"burst"`
}
