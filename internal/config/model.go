package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
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
	Login   models.LoginConfig   `mapstructure:"login"`
	Server  models.ServerConfig  `mapstructure:"server"`
	Logging models.LoggingConfig `mapstructure:"logging"`
	API     models.APIConfig     `mapstructure:"api"`
	Secret  string               `mapstructure:"secret"` // Secret used for signing cookies and tokens

	// Workflow engine config
	Roles     RoleConfig                `mapstructure:"roles"`
	Workflows WorkflowConfig            `mapstructure:"workflows"` // These are workflows to run for role associated workflows
	Providers ProviderDefinitionsConfig `mapstructure:"providers"` // These are integration providers like AWS, GCP, etc.

	// This is ONLY if the agent is running in server mode
	// and you want to use https://www.thand.io hosted services
	Thand models.ThandConfig `mapstructure:"thand"`

	// Internal mode of operation
	mode   Mode
	logger thandLogger
	mu     sync.RWMutex

	// Cached services client
	initializeServiceClientOnce sync.Once
	servicesClient              models.ServicesClientImpl

	// Provider instances
	providerInstances map[string]models.Provider
}

func (c *Config) GetSecret() string {
	return c.Secret
}

func (c *Config) GetMode() Mode {
	return c.mode
}

func (c *Config) SetMode(mode Mode) {
	logrus.Debugf("Setting mode: %s", mode)
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

type RoleConfig struct {
	Path  string          `mapstructure:"path" json:"path"`
	URL   *model.Endpoint `mapstructure:"url" json:"url"`
	Vault string          `mapstructure:"vault" json:"vault"` // vault secret / path to use

	// Store everything in memory
	Definitions map[string]models.Role `mapstructure:",remain" json:"definitions"`

	// Search indexes
	rolesIndex bleve.Index
}

type WorkflowConfig struct {
	Path  string          `mapstructure:"path" json:"path"`
	URL   *model.Endpoint `mapstructure:"url" json:"url"`
	Vault string          `mapstructure:"vault" json:"vault"` // vault secret / path to use

	// Load dynamic plugin registry for custom call tools
	Plugins WorkflowPluginConfig `mapstructure:"plugins" json:"plugins"`

	// Store everything in memory
	Definitions map[string]models.Workflow `mapstructure:",remain" json:"definitions"`
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

type ProviderDefinitionsConfig struct {
	Path  string          `mapstructure:"path" json:"path"`
	URL   *model.Endpoint `mapstructure:"url" json:"url"`
	Vault string          `mapstructure:"vault" json:"vault"` // vault secret / path to use

	// Load dynamic provider configs
	Plugins ProviderPluginConfig `mapstructure:"plugins" json:"plugins"`

	// Load providers directly from config using mapstructure:",remain"
	Definitions map[string]models.ProviderConfig `mapstructure:",remain" json:"definitions"`
}

func (p *ProviderDefinitionsConfig) GetDefinitions() map[string]models.ProviderConfig {
	return p.Definitions
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

// GetLocalServerUrl returns the local server URL. This is the
// local agent server URL used for agent to server communication.
func (c *Config) GetLocalServerUrl() string {
	hostname := c.GetLocalHostname()
	return fmt.Sprintf("http://%s:%d", hostname, c.Server.Port)
}

func (c *Config) GetLocalHostname() string {
	hostname := c.Server.Host
	if hostname == "0.0.0.0" {
		hostname = "localhost"
	}
	return hostname
}

func (c *Config) GetLoginServerUrl() string {
	return strings.TrimSuffix(fmt.Sprintf(
		"%s/%s",
		strings.TrimSuffix(c.Login.Endpoint.String(), "/"),
		strings.TrimPrefix(strings.TrimSuffix(c.Login.Base, "/"), "/")),
		"/")
}

func (c *Config) GetThandServerUrl() string {
	return strings.TrimSuffix(fmt.Sprintf(
		"%s/%s",
		strings.TrimSuffix(c.Thand.Endpoint, "/"),
		strings.TrimPrefix(strings.TrimSuffix(c.Thand.Base, "/"), "/")),
		"/")
}

func (c *Config) DiscoverThandServerApiUrl() string {
	return c.discoverServerApiUrl(c.Thand.Endpoint, &model.ReferenceableAuthenticationPolicy{
		AuthenticationPolicy: &model.AuthenticationPolicy{
			Bearer: &model.BearerAuthenticationPolicy{
				Token: c.Thand.ApiKey,
			},
		},
	})
}

func (c *Config) DiscoverLoginServerApiUrl(loginServer string) string {
	return c.discoverServerApiUrl(loginServer, nil)
}

func (c *Config) discoverServerApiUrl(
	loginServer string,
	auth *model.ReferenceableAuthenticationPolicy,
) string {

	// Make request to the login server to get the
	// /.well-known/api-configuration endpoint
	// to get the base param which is our api endpoint using resty

	discoveryCheckUrl := fmt.Sprintf("%s/.well-known/api-configuration", loginServer)
	defaultUrl := fmt.Sprintf("%s/api/v1", loginServer)

	resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI:            &model.LiteralUri{Value: discoveryCheckUrl},
				Authentication: auth,
			},
		},
		Method: http.MethodGet,
	})

	if err != nil {
		return defaultUrl
	}

	if resp.StatusCode() != http.StatusOK {
		return defaultUrl
	}

	// Get the path field in the JSON response this is our API path
	var discoveryCheckResponse struct {
		BaseUrl     string `json:"baseUrl"`
		ApiBasePath string `json:"apiBasePath"`
	}

	if err := json.Unmarshal(resp.Body(), &discoveryCheckResponse); err != nil {
		return defaultUrl
	}

	if len(discoveryCheckResponse.BaseUrl) > 0 {
		logrus.Debugf("Discovered login server base URL: %s", discoveryCheckResponse.BaseUrl)
		loginServer = strings.TrimSuffix(discoveryCheckResponse.BaseUrl, "/")
	}

	trimPath := strings.TrimSuffix(strings.TrimPrefix(discoveryCheckResponse.ApiBasePath, "/"), "/")
	return fmt.Sprintf("%s/%s", loginServer, trimPath)
}

func (c *Config) GetLoginServerHostname() string {
	hostname, err := url.Parse(c.Login.Endpoint.String())
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
	c.Login.Endpoint = model.NewEndpoint(parsedUrl.String())
	return nil
}

func (c *Config) GetAPIKey() string {
	return c.Thand.ApiKey
}

func (c *Config) SetAPIKey(apiKey string) error {
	if len(apiKey) == 0 {
		return fmt.Errorf("API key cannot be empty")
	}
	c.Thand.ApiKey = apiKey
	return nil
}

func (c *Config) HasAPIKey() bool {
	return len(c.Thand.ApiKey) > 0
}

func (c *Config) GetApiBasePath() string {
	return strings.TrimSuffix(fmt.Sprintf("/api/%s", c.API.GetVersion()), "/")
}

func (c *Config) GetCallbackUrl(path string) string {
	return fmt.Sprintf(
		"%s/%s/%s",
		c.GetLoginServerUrl(),
		strings.TrimPrefix(strings.TrimSuffix(c.GetApiBasePath(), "/"), "/"),
		strings.TrimPrefix(strings.TrimSuffix(path, "/"), "/"),
	)
}

func (c *Config) GetAuthCallbackUrl(providerName string) string {

	if len(providerName) == 0 {
		logrus.Fatalf("provider name cannot be empty")
	}

	return c.GetCallbackUrl(
		fmt.Sprintf("/auth/callback/%s", url.PathEscape(providerName)),
	)
}

func (c *Config) GetResumeCallbackUrl(workflowTask *models.ElevateWorkflowTask) string {

	queryParams := url.Values{
		"state": {workflowTask.GetEncodedTask(
			c.servicesClient.GetEncryption(),
		)},
		"taskName":   {workflowTask.GetTaskName()},
		"taskStatus": {workflowTask.GetStatus().String()},
	}

	return c.GetCallbackUrl(fmt.Sprintf(
		"/elevate/resume?%s",
		queryParams.Encode(),
	))
}

func (c *Config) GetSignalCallbackUrl(workflowTask *models.ElevateWorkflowTask) string {

	encodedInput := models.EncodingWrapper{
		Type: sdkConstants.ENCODED_WORKFLOW_SIGNAL,
		Data: workflowTask.GetInput(),
	}.EncodeAndEncrypt(c.servicesClient.GetEncryption())

	queryParams := url.Values{
		"input":      {encodedInput},
		"taskName":   {workflowTask.GetTaskName()},
		"taskStatus": {workflowTask.GetStatus().String()},
	}

	return c.GetCallbackUrl(fmt.Sprintf(
		"/execution/%s/signal?%s",
		workflowTask.GetWorkflowID(),
		queryParams.Encode(),
	))
}

func (c *Config) GetEventsWithFilter(filter LogFilter) []*models.LogEntry {
	return c.logger.GetEventsWithFilter(filter)
}

func (r *Config) GetWorkflowFromElevationRequest(
	elevationRequest *models.ElevateRequest) (*models.Workflow, error) {

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

	// We want the original role request. The composite role will be created later
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

	r.mu.RLock()
	workflow, foundWorkflow := r.Workflows.Definitions[workflowName]
	r.mu.RUnlock()

	if !foundWorkflow {
		return nil, fmt.Errorf("workflow '%s' not found for role '%s'", workflowName, roleName)
	}

	return &workflow, nil

}

func (r *Config) HasThandService() bool {
	return len(r.Thand.Endpoint) != 0 && len(r.Thand.ApiKey) != 0
}

type PreflightRequest struct {
	Mode       Mode      `json:"mode,omitempty"`
	Version    string    `json:"version,omitempty"`
	Commit     string    `json:"commit,omitempty"`
	Identifier uuid.UUID `json:"identifier,omitempty"`
}

type PreflightResponse struct {
	Success bool `json:"success" required:"true"`
}

type RegistrationRequest struct {
	Mode        Mode                      `json:"mode,omitempty"`
	Environment *models.EnvironmentConfig `json:"environment,omitempty"`
	Version     string                    `json:"version,omitempty"`
	Commit      string                    `json:"commit,omitempty"`
	Identifier  uuid.UUID                 `json:"identifier,omitempty"`
}

type RegistrationResponse struct {
	Success   bool                       `json:"success" required:"true"`
	Services  *models.ServicesConfig     `json:"services,omitempty"`
	Logging   *models.LoggingConfig      `json:"logging,omitempty"`
	Roles     *RoleConfig                `json:"roles,omitempty"`
	Providers *ProviderDefinitionsConfig `json:"providers,omitempty"`
	Workflows *WorkflowConfig            `json:"workflows,omitempty"`
}

type PostflightRequest struct {
	Mode       Mode      `json:"mode,omitempty"`
	Version    string    `json:"version,omitempty"`
	Commit     string    `json:"commit,omitempty"`
	Identifier uuid.UUID `json:"identifier,omitempty"`
}

type PostflightResponse struct {
	Success bool `json:"success" required:"true"`
}
