package daemon

import "github.com/thand-io/agent/internal/models"

type SimpleServices struct {
	HasTemporal           bool `json:"temporal,omitempty" yaml:"temporal"`
	HasLargeLanguageModel bool `json:"llm,omitempty" yaml:"llm"`
}

type SimpleConfig struct {
	ApiBasePath string
	Server      SimpleServer
	Services    SimpleServices
}

type SimpleServer struct {
	Host string
	Port int

	Health  SimpleHealth
	Metrics SimpleMetrics
}

type SimpleHealth struct {
	Enabled bool
	Path    string
}

type SimpleMetrics struct {
	Enabled bool
	Path    string
}

type SimpleEnvrinment struct {
	Platform string
}

// TemplateData represents data passed to HTML templates
type TemplateData struct {
	Config      SimpleConfig
	ServiceName string
	Environment SimpleEnvrinment
	Provider    string `json:"provider,omitempty" yaml:"provider,omitempty"`
	User        *models.User
	Version     string
	Status      string
}
