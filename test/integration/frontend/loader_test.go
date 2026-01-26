package ui_e2e

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hashicorp/go-version"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	_ "github.com/thand-io/agent/internal/workflows/tasks/model" // Register custom thand tasks
	testcommon "github.com/thand-io/agent/test/integration/common"
	"gopkg.in/yaml.v3"
)

// TestCase represents a UI E2E test case loaded from testdata
type TestCase struct {
	Name      string
	Path      string
	Providers map[string]models.ProviderConfig
	Roles     map[string]models.Role
	Workflows map[string]models.Workflow
}

// TestCaseLoader loads test cases from the testdata directory
type TestCaseLoader struct {
	infra    *UITestInfrastructure
	basePath string
}

// NewTestCaseLoader creates a new test case loader
func NewTestCaseLoader(infra *UITestInfrastructure) *TestCaseLoader {
	return &TestCaseLoader{
		infra:    infra,
		basePath: "testdata",
	}
}

// LoadTestCase loads a specific test case by name
func (l *TestCaseLoader) LoadTestCase(name string) (*TestCase, error) {
	testPath := filepath.Join(l.basePath, name)

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("test case not found: %s", name)
	}

	tc := &TestCase{
		Name: name,
		Path: testPath,
	}

	// Load providers
	providers, err := l.loadProviders(testPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load providers: %w", err)
	}
	tc.Providers = providers

	// Load roles
	roles, err := l.loadRoles(testPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load roles: %w", err)
	}
	tc.Roles = roles

	// Load workflows
	workflows, err := l.loadWorkflows(testPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflows: %w", err)
	}
	tc.Workflows = workflows

	return tc, nil
}

// loadProviders loads providers from the test case directory
func (l *TestCaseLoader) loadProviders(testPath string) (map[string]models.ProviderConfig, error) {
	content, err := os.ReadFile(filepath.Join(testPath, "providers.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read providers.yaml: %w", err)
	}

	// Convert YAML to JSON first (required for proper workflow DSL parsing)
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal(content, &yamlData); err != nil {
		return nil, fmt.Errorf("failed to parse providers YAML: %w", err)
	}

	jsonData, err := json.Marshal(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert providers to JSON: %w", err)
	}

	var data struct {
		Providers map[string]models.ProviderConfig `json:"providers"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal providers: %w", err)
	}

	// Interpolate environment variables if the infrastructure is available
	if l.infra != nil {
		for key, provider := range data.Providers {
			// Replace placeholders with actual test infrastructure endpoints
			if provider.Config != nil {
				l.interpolateProviderConfig(&provider, l.infra)
				data.Providers[key] = provider
			}
		}
	}

	return data.Providers, nil
}

// loadRoles loads roles from the test case directory
func (l *TestCaseLoader) loadRoles(testPath string) (map[string]models.Role, error) {
	content, err := os.ReadFile(filepath.Join(testPath, "roles.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read roles.yaml: %w", err)
	}

	// Convert YAML to JSON first
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal(content, &yamlData); err != nil {
		return nil, fmt.Errorf("failed to parse roles YAML: %w", err)
	}

	jsonData, err := json.Marshal(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert roles to JSON: %w", err)
	}

	var data struct {
		Roles map[string]models.Role `json:"roles"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal roles: %w", err)
	}

	return data.Roles, nil
}

// loadWorkflows loads workflows from the test case directory
func (l *TestCaseLoader) loadWorkflows(testPath string) (map[string]models.Workflow, error) {
	content, err := os.ReadFile(filepath.Join(testPath, "workflow.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow.yaml: %w", err)
	}

	// Convert YAML to JSON first
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal(content, &yamlData); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	jsonData, err := json.Marshal(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert workflow to JSON: %w", err)
	}

	var data struct {
		Workflows map[string]models.Workflow `json:"workflows"`
	}

	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflows: %w", err)
	}

	return data.Workflows, nil
}

// interpolateProviderConfig replaces test infrastructure placeholders with actual endpoints
func (l *TestCaseLoader) interpolateProviderConfig(provider *models.ProviderConfig, infra *UITestInfrastructure) {
	if provider.Config == nil {
		return
	}

	// BasicConfig is a map[string]any type
	configMap := *provider.Config

	// Replace LocalStack endpoint
	if _, hasEndpoint := configMap["endpoint"]; hasEndpoint {
		configMap["endpoint"] = infra.LocalStackEndpoint
	}

	// Replace AWS endpoint
	if _, hasAWSEndpoint := configMap["aws_endpoint"]; hasAWSEndpoint {
		configMap["aws_endpoint"] = infra.LocalStackEndpoint
	}

	// Replace SMTP config
	if _, hasSMTP := configMap["smtp"]; hasSMTP {
		smtpConfig, ok := configMap["smtp"].(map[string]interface{})
		if ok {
			smtpConfig["host"] = infra.MailHogSMTP
			smtpConfig["port"] = "1025"
		}
	}

	// Replace OIDC endpoints
	if _, hasIssuer := configMap["issuer_url"]; hasIssuer {
		configMap["issuer_url"] = infra.MockOIDCEndpoint + "/default"
	}

	// provider.Config is already a pointer to the map, changes are applied directly
}

// CreateConfigFromTestCase creates a Config object from a test case
func (l *TestCaseLoader) CreateConfigFromTestCase(tc *TestCase) (*config.Config, error) {
	cfg := config.DefaultConfig()

	// Set mode to Agent so providers are initialized locally (not via proxy)
	cfg.SetMode(config.ModeAgent)

	// Set up roles first (before providers in case providers need them)
	cfg.Roles.Definitions = tc.Roles

	// Apply workflows to set the Identifier field on each workflow
	// The Identifier is critical for workflow hydration - it's used as the workflow name
	// when loading workflow definitions during execution. Without this, workflows will
	// fail with "workflow not found" errors.
	workflows, err := cfg.ApplyWorkflows([]*models.WorkflowDefinitions{
		{
			Version:   version.Must(version.NewVersion("1.0")),
			Workflows: tc.Workflows,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply workflows: %w", err)
	}
	cfg.Workflows.Definitions = workflows

	// Set up providers
	cfg.Providers.Definitions = tc.Providers

	if l.infra != nil {
		// Configure Temporal connection - parse host:port from endpoint
		host, portStr, err := net.SplitHostPort(l.infra.TemporalEndpoint)
		if err != nil {
			// If no port in endpoint, use full endpoint as host with default port
			host = l.infra.TemporalEndpoint
			portStr = testcommon.TemporalDefaultPort
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port in Temporal endpoint: %w", err)
		}

		cfg.Services.Temporal = &models.TemporalConfig{
			Host:              host,
			Port:              port,
			Namespace:         testcommon.TemporalTestNamespace,
			DisableVersioning: true, // Disable versioning for integration tests
		}
	}

	// Initialize providers (this creates the actual provider implementations)
	// This must be done after setting mode so the correct implementation is used
	err = cfg.InitializeProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize providers: %w", err)
	}

	return cfg, nil
}

// ListTestCases returns all available test cases in the testdata directory
func (l *TestCaseLoader) ListTestCases() ([]string, error) {
	entries, err := os.ReadDir(l.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read testdata directory: %w", err)
	}

	var testCases []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it has the required files
			testPath := filepath.Join(l.basePath, entry.Name())
			if l.hasRequiredFiles(testPath) {
				testCases = append(testCases, entry.Name())
			}
		}
	}

	return testCases, nil
}

// hasRequiredFiles checks if a test case directory has all required files
func (l *TestCaseLoader) hasRequiredFiles(testPath string) bool {
	requiredFiles := []string{"providers.yaml", "roles.yaml", "workflow.yaml"}

	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(testPath, file)); os.IsNotExist(err) {
			return false
		}
	}

	return true
}
