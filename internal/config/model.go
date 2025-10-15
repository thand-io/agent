package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/services"
	"github.com/thand-io/agent/internal/models"
)

type Mode string

const (

	// Runs in cloud environment as a login server
	// allows agents to sync roles and policies and get tasking
	ModeServer Mode = "server"

	// Runs as a background agent to store session data and
	// exec platform specific elevations
	ModeAgent Mode = "agent"

	// Just the CLI mode - used to connect to login-servers
	ModeClient Mode = "client"
)

// Config represents the application configuration structure
type Config struct {

	// Environment configuration and core services
	Environment models.EnvironmentConfig `mapstructure:"environment"`

	// External services / non-core services
	Services models.ServicesConfig `mapstructure:"services"`

	// System configuration
	Login   LoginConfig   `mapstructure:"login"`
	Server  ServerConfig  `mapstructure:"server"`
	Logging LoggingConfig `mapstructure:"logging"`
	API     APIConfig     `mapstructure:"api"`
	Secret  string        `mapstructure:"secret"` // Secret used for signing cookies and tokens

	// Workflow engine config
	Roles     RoleConfig     `mapstructure:"roles"`
	Workflows WorkflowConfig `mapstructure:"workflows"` // These are workflows to run for role associated workflows
	Providers ProviderConfig `mapstructure:"providers"` // These are integration providers like AWS, GCP, etc.

	// Internal mode of operation
	mode   Mode
	logger thandLogger

	// Cached services client
	initializeServiceClientOnce sync.Once
	servicesClient              models.ServicesClientImpl
}

func (c *Config) GetSecret() string {
	return c.Secret
}

func (c *Config) GetMode() Mode {
	return c.mode
}

func (c *Config) SetMode(mode Mode) {
	c.mode = mode
}

func (c *Config) IsServer() bool {
	return c.mode == ModeServer
}

func (c *Config) IsAgent() bool {
	return c.mode == ModeAgent
}

func (c *Config) IsClient() bool {
	return c.mode == ModeClient
}

func (c *Config) GetRoles() RoleConfig {
	return c.Roles
}

func (c *Config) GetWorkflows() WorkflowConfig {
	return c.Workflows
}

func (c *Config) GetProviders() ProviderConfig {
	return c.Providers
}

func (c *Config) GetServices() models.ServicesClientImpl {

	c.initializeServiceClientOnce.Do(func() {
		newClient := services.NewServicesClient(&c.Environment, &c.Services)
		err := newClient.Initialize()
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to initialize services client: %v", err)
			return
		}
		c.servicesClient = newClient
	})

	return c.servicesClient

}

func (c *Config) GetProviderByName(name string) (*models.Provider, error) {
	if provider, exists := c.Providers.Definitions[name]; exists {
		return &provider, nil
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

func (c *Config) GetProvidersByCapability(capability ...models.ProviderCapability) map[string]models.Provider {
	providers := make(map[string]models.Provider)
	for name, provider := range c.Providers.Definitions {
		// Skip providers that don't have a client initialized
		client := provider.GetClient()
		if client == nil {
			continue
		}

		for _, cap := range capability {
			if slices.Contains(client.GetCapabilities(), cap) {
				providers[name] = provider
			}
		}
	}
	return providers
}

func (c *Config) GetRoleByName(name string) (*models.Role, error) {
	if role, exists := c.Roles.Definitions[name]; exists {
		return &role, nil
	}
	return nil, fmt.Errorf("role not found: %s", name)
}

func (c *Config) GetWorkflowByName(name string) (*models.Workflow, error) {
	if workflow, exists := c.Workflows.Definitions[name]; exists {
		return &workflow, nil
	}
	return nil, fmt.Errorf("workflow not found: %s", name)
}

func (c *Config) GetVault() models.VaultImpl {
	return c.GetServices().GetVault()
}

func (c *Config) HasVault() bool {
	return c.GetServices().HasVault()
}

func (c *Config) GetStorage() models.StorageImpl {
	return c.GetServices().GetStorage()
}

func (c *Config) HasStorage() bool {
	return c.GetServices().HasStorage()
}

func (c *Config) GetScheduler() models.SchedulerImpl {
	return c.GetServices().GetScheduler()
}

func (c *Config) HasScheduler() bool {
	return c.GetServices().HasScheduler()
}

func (c *Config) GetLargeLanguageModel() models.LargeLanguageModelImpl {
	return c.GetServices().GetLargeLanguageModel()
}

func (c *Config) HasLargeLanguageModel() bool {
	return c.GetServices().HasLargeLanguageModel()
}

type ServerConfig struct {
	Host     string             `mapstructure:"host"`
	Port     int                `mapstructure:"port"`
	Limits   ServerLimitsConfig `mapstructure:"limits"`
	Metrics  MetricsConfig      `mapstructure:"metrics"`
	Health   HealthConfig       `mapstructure:"health"`
	Security SecurityConfig     `mapstructure:"security"`
}

type ServerLimitsConfig struct {
	ReadTimeout       time.Duration `mapstructure:"read_timeout"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout"`
	RequestsPerMinute int           `mapstructure:"requests_per_minute"`
	Burst             int           `mapstructure:"burst"`
}

type LoginConfig struct {
	Endpoint string `mapstructure:"endpoint" default:"https://auth.thand.io/"`
	ApiKey   string `mapstructure:"api_key"`          // API key for authenticating with the login server
	Base     string `mapstructure:"base" default:"/"` // Base path for login endpoint e.g. /
}

type LoggingConfig struct {
	Level  string `mapstructure:"level" default:"info"`
	Format string `mapstructure:"format" default:"text"`
	Output string `mapstructure:"output"`
}

type MetricsConfig struct {
	Enabled   bool   `mapstructure:"enabled" default:"true"`
	Path      string `mapstructure:"path" default:"/metrics"`
	Namespace string `mapstructure:"namespace"`
}

type HealthConfig struct {
	Enabled bool   `mapstructure:"enabled" default:"true"`
	Path    string `mapstructure:"path" default:"/healthz"`
}

type SecurityConfig struct {
	CORS CORSConfig `mapstructure:"cors"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
	MaxAge         int      `mapstructure:"max_age"`
}

type APIConfig struct {
	Version   string          `mapstructure:"version" default:"v1"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
}

func (api *APIConfig) GetVersion() string {
	if len(api.Version) > 0 {
		return api.Version
	}
	return "v1"
}

type RateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
	Burst             int `mapstructure:"burst"`
}

type RoleConfig struct {
	Path  string          `mapstructure:"path"`
	URL   *model.Endpoint `mapstructure:"url"`
	Vault string          `mapstructure:"vault"` // vault secret / path to use

	// Store everything in memory
	Definitions map[string]models.Role `mapstructure:",remain"`
}

func (r *RoleConfig) IsExternal() bool {
	return (len(r.Path) > 0 || r.URL != nil || len(r.Vault) > 0)
}

func (r *RoleConfig) GetRoleByName(name string) (*models.Role, error) {
	if role, exists := r.Definitions[name]; exists {
		return &role, nil
	}
	return nil, fmt.Errorf("role not found: %s", name)
}

type WorkflowConfig struct {
	Path  string          `mapstructure:"path"`
	URL   *model.Endpoint `mapstructure:"url"`
	Vault string          `mapstructure:"vault"` // vault secret / path to use

	// Load dynamic plugin registry for custom call tools
	Plugins WorkflowPluginConfig `mapstructure:"plugins"`

	// Store everything in memory
	Definitions map[string]models.Workflow `mapstructure:",remain"`
}

func (r *WorkflowConfig) IsExternal() bool {
	return (len(r.Path) > 0 || r.URL != nil || len(r.Vault) > 0)
}

func (p *WorkflowConfig) GetWorkflowByName(name string) (*models.Workflow, error) {
	if workflow, exists := p.Definitions[name]; exists {
		return &workflow, nil
	}
	return nil, fmt.Errorf("workflow not found: %s", name)
}

func (p *WorkflowConfig) GetDefinitions() map[string]models.Workflow {
	return p.Definitions
}

type WorkflowPluginConfig struct {
	Path string `mapstructure:"path"`
	URL  string `mapstructure:"url"`

	// Store everything in memory
	Definitions map[string]WorkflowPlugin `mapstructure:",remain"`
}

type WorkflowPlugin struct {
}

type ProviderConfig struct {
	Path  string          `mapstructure:"path"`
	URL   *model.Endpoint `mapstructure:"url"`
	Vault string          `mapstructure:"vault"` // vault secret / path to use

	// Load dynamic provider configs
	Plugins ProviderPluginConfig `mapstructure:"plugins"`

	// Load providers directly from config using mapstructure:",remain"
	Definitions map[string]models.Provider `mapstructure:",remain"`
}

func (r *ProviderConfig) IsExternal() bool {
	return (len(r.Path) > 0 || r.URL != nil || len(r.Vault) > 0)
}

func (p *ProviderConfig) GetProviderByName(name string) (*models.Provider, error) {
	if provider, exists := p.Definitions[name]; exists {
		return &provider, nil
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

type ProviderPluginConfig struct {
	Path string `mapstructure:"path"`
	URL  string `mapstructure:"url"`

	// Load providers directly from config using mapstructure:",remain"
	Definitions map[string]ProviderPlugin `mapstructure:",remain"`
}

type ProviderPlugin struct {
}

// GetServerAddress returns the server bind address
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c *Config) GetLocalServerUrl() string {
	hostname := c.Server.Host
	if hostname == "0.0.0.0" {
		hostname = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", hostname, c.Server.Port)
}

func (c *Config) GetLoginServerUrl() string {
	return strings.TrimSuffix(fmt.Sprintf(
		"%s/%s",
		strings.TrimSuffix(c.Login.Endpoint, "/"),
		strings.TrimSuffix(c.Login.Base, "/")),
		"/")
}

func (c *Config) GetLoginServerApiUrl() string {

	// Make request to the login server to get the /health endpoint
	// to get the base param which is our api endpoint using resty

	baseUrl := c.GetLoginServerUrl()

	healthCheckUrl := fmt.Sprintf("%s/health", baseUrl)
	defaultUrl := fmt.Sprintf("%s/api/v1", baseUrl)

	client := resty.New()
	res, err := client.R().
		EnableTrace().
		Get(healthCheckUrl)

	if err != nil {
		return defaultUrl
	}

	if res.StatusCode() != http.StatusOK {
		return defaultUrl
	}

	// Get the path field in the JSON response this is our API path
	var healthCheckResponse struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(res.Body(), &healthCheckResponse); err != nil {
		return defaultUrl
	}

	trimPath := strings.TrimSuffix(strings.TrimPrefix(healthCheckResponse.Path, "/"), "/")

	return fmt.Sprintf("%s/%s", c.GetLoginServerUrl(), trimPath)
}

func (c *Config) GetLoginServerHostname() string {
	hostname, err := url.Parse(c.Login.Endpoint)
	if err != nil {
		return "localhost"
	}
	// Return only the hostname, no port or schema
	return hostname.Hostname()
}

func (c *Config) SetLoginServer(loginServer string) error {
	// parse url
	parsedUrl, err := url.Parse(loginServer)
	if err != nil {
		return fmt.Errorf("invalid login server URL: %w", err)
	}
	c.Login.Endpoint = parsedUrl.String()
	return nil
}

func (c *Config) GetAPIKey() string {
	return c.Login.ApiKey
}

func (c *Config) SetAPIKey(apiKey string) error {
	if len(apiKey) == 0 {
		return fmt.Errorf("API key cannot be empty")
	}
	c.Login.ApiKey = apiKey
	return nil
}

func (c *Config) HasAPIKey() bool {
	return len(c.Login.ApiKey) > 0
}

func (c *Config) GetApiBasePath() string {
	return strings.TrimSuffix(fmt.Sprintf("/api/%s", c.API.GetVersion()), "/")
}

func (c *Config) GetAuthCallbackUrl(providerName string) string {
	return fmt.Sprintf(
		"%s/%s/auth/callback/%s",
		c.GetLoginServerUrl(),
		strings.TrimPrefix(c.GetApiBasePath(), "/"),
		url.PathEscape(providerName),
	)
}

func (c *Config) GetResumeCallbackUrl(workflowTask *models.WorkflowTask) string {

	queryParams := url.Values{
		"state": {workflowTask.GetEncodedTask(
			c.servicesClient.GetEncryption(),
		)},
		"taskName": {workflowTask.GetTaskName()},
	}

	return fmt.Sprintf(
		"%s/%s/elevate/resume?%s",
		c.GetLoginServerUrl(),
		strings.TrimPrefix(c.GetApiBasePath(), "/"),
		queryParams.Encode(),
	)
}

/*
GetSecret retrieves a secret from the provider configuration
If the provider is not found, it returns an empty string.

A jq expression must be provided to extract the secret from the provider's configuration.

Node is the incoming yaml/json node that contains the jq expression.
*/
func (c *Config) TraverseAndEvaluateProviderSecrets(providerName string, node map[string]any) (any, error) {

	if len(c.Providers.Definitions) == 0 {
		return nil, fmt.Errorf("no providers defined in configuration")
	}

	// Check if the provider exists
	/*
		if provider, exists := c.Providers.Definitions[providerName]; exists {

			variables := map[string]any{
				fmt.Sprintf("$%s", providerName): provider.GetConfig(),
			}
			input := map[string]any{}

			// Create interpolator and use it for string interpolation
			interpolator := configExpr.NewStringInterpolator()
			result, err := interpolator.InterpolateAndEvaluate(node, input, variables, context.TODO())

			return result, err
		}
	*/

	return nil, fmt.Errorf("provider '%s' not found", providerName)
}

func (c *Config) GetEventsWithFilter(filter LogFilter) []*models.LogEntry {
	return c.logger.GetEventsWithFilter(filter)
}

func (r *Config) GetWorkflowFromElevationRequest(elevationRequest *models.ElevateRequest) (*models.Workflow, error) {

	if elevationRequest == nil {
		return nil, fmt.Errorf("elevation request is nil")
	}

	if elevationRequest.Role == nil {
		return nil, fmt.Errorf("role is nil")
	}

	if len(elevationRequest.Providers) == 0 {
		return nil, fmt.Errorf("providers are empty")
	}

	primaryProvider := strings.ToLower(elevationRequest.Providers[0])

	roleName := strings.ToLower(elevationRequest.Role.Name)
	providerName := strings.ToLower(primaryProvider)
	workflowName := strings.ToLower(elevationRequest.Workflow)

	role := elevationRequest.Role

	if len(workflowName) == 0 {
		// If no workflow is specified, use the first workflow associated with the role
		if len(role.Workflows) == 0 {
			return nil, fmt.Errorf("no workflow specified and role has no associated workflows")
		}

		workflowName = role.Workflows[0]
	}

	if !slices.Contains(role.Providers, providerName) {
		return nil, fmt.Errorf("provider '%s' not allowed for role '%s', roles: %v", providerName, roleName, role.Providers)
	}

	if !slices.Contains(role.Workflows, workflowName) {
		return nil, fmt.Errorf("workflow '%s' not allowed for role '%s', workflows: %v", workflowName, roleName, role.Workflows)
	}

	workflow, foundWorkflow := r.Workflows.Definitions[workflowName]

	if !foundWorkflow {
		return nil, fmt.Errorf("workflow '%s' not found for role '%s'", workflowName, roleName)
	}

	return &workflow, nil

}

// TemplateData represents data passed to HTML templates
type TemplateData struct {
	Config      *Config
	ServiceName string
	User        *models.User
	Version     string
	Status      string
}

type RegistrationRequest struct {
	Environment *models.EnvironmentConfig `json:"environment,omitempty"`
}

type RegistrationResponse struct {
	Success   bool                   `json:"success" required:"true"`
	Services  *models.ServicesConfig `json:"services,omitempty"`
	Roles     *RoleConfig            `json:"roles,omitempty"`
	Providers *ProviderConfig        `json:"providers,omitempty"`
	Workflows *WorkflowConfig        `json:"workflows,omitempty"`
}
