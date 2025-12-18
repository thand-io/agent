package models

import (
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

type ServerConfig struct {
	Host     string             `json:"host" yaml:"host" mapstructure:"host"`
	Port     int                `json:"port" yaml:"port" mapstructure:"port"`
	Limits   ServerLimitsConfig `json:"limits" yaml:"limits" mapstructure:"limits"`
	Metrics  MetricsConfig      `json:"metrics" yaml:"metrics" mapstructure:"metrics"`
	Health   HealthConfig       `json:"health" yaml:"health" mapstructure:"health"`
	Ready    ReadyConfig        `json:"ready" yaml:"ready" mapstructure:"ready"`
	Security SecurityConfig     `json:"security" yaml:"security" mapstructure:"security"`
}

type ServerLimitsConfig struct {
	ReadTimeout       time.Duration `json:"read_timeout" yaml:"read_timeout" mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `json:"write_timeout" yaml:"write_timeout" mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `json:"idle_timeout" yaml:"idle_timeout" mapstructure:"idle_timeout"`
	RequestsPerMinute int           `json:"requests_per_minute" yaml:"requests_per_minute" mapstructure:"requests_per_minute"`
	Burst             int           `json:"burst" yaml:"burst" mapstructure:"burst"`
}

type LoginConfig struct {
	Endpoint string `json:"endpoint" yaml:"endpoint" mapstructure:"endpoint" default:"https://auth.thand.io/"`
	Base     string `json:"base" yaml:"base" mapstructure:"base" default:"/"` // Base path for login endpoint e.g. /
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
	CORS CORSConfig `json:"cors" yaml:"cors" mapstructure:"cors"`
}

type CORSConfig struct {
	AllowedOrigins   []string `json:"allowed_origins" yaml:"allowed_origins" mapstructure:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods" yaml:"allowed_methods" mapstructure:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers" yaml:"allowed_headers" mapstructure:"allowed_headers"`
	ExposeHeaders    []string `json:"expose_headers" yaml:"expose_headers" mapstructure:"expose_headers"`
	AllowCredentials bool     `json:"allow_credentials" yaml:"allow_credentials" mapstructure:"allow_credentials"`
	MaxAge           int      `json:"max_age" yaml:"max_age" mapstructure:"max_age"`
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
