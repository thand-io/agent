package workflows_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"gopkg.in/yaml.v3"
)

// TestCase represents a workflow test case loaded from testdata
type TestCase struct {
	Name      string
	Path      string
	Providers map[string]models.Provider
	Roles     map[string]models.Role
	Workflows map[string]models.Workflow
}

// TestCaseLoader loads test cases from the testdata directory
type TestCaseLoader struct {
	infra    *TestInfrastructure
	basePath string
}

// NewTestCaseLoader creates a new test case loader
func NewTestCaseLoader(infra *TestInfrastructure) *TestCaseLoader {
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
func (l *TestCaseLoader) loadProviders(testPath string) (map[string]models.Provider, error) {
	content, err := os.ReadFile(filepath.Join(testPath, "providers.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read providers.yaml: %w", err)
	}

	// Substitute environment variables from infrastructure
	content = l.substituteVariables(content)

	var data struct {
		Version   string                     `yaml:"version"`
		Providers map[string]models.Provider `yaml:"providers"`
	}

	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse providers.yaml: %w", err)
	}

	return data.Providers, nil
}

// loadRoles loads roles from the test case directory
func (l *TestCaseLoader) loadRoles(testPath string) (map[string]models.Role, error) {
	content, err := os.ReadFile(filepath.Join(testPath, "roles.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read roles.yaml: %w", err)
	}

	var data struct {
		Version string                 `yaml:"version"`
		Roles   map[string]models.Role `yaml:"roles"`
	}

	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse roles.yaml: %w", err)
	}

	return data.Roles, nil
}

// loadWorkflows loads workflows from the test case directory
func (l *TestCaseLoader) loadWorkflows(testPath string) (map[string]models.Workflow, error) {
	content, err := os.ReadFile(filepath.Join(testPath, "workflow.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow.yaml: %w", err)
	}

	// Convert YAML to JSON first (required for proper workflow DSL parsing)
	var yamlData any
	if err := yaml.Unmarshal(content, &yamlData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	jsonData, err := json.Marshal(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	var data models.WorkflowDefinitions
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to parse workflow.yaml: %w", err)
	}

	return data.Workflows, nil
}

// substituteVariables replaces ${VAR} placeholders with actual values from infrastructure
func (l *TestCaseLoader) substituteVariables(content []byte) []byte {
	str := string(content)

	// Parse MailHog host and port
	mailhogHost, mailhogPort, err := net.SplitHostPort(l.infra.MailHogSMTP)
	if err != nil {
		// If no port in SMTP, use full string as host and empty port
		mailhogHost = l.infra.MailHogSMTP
		mailhogPort = ""
	}

	// Define variable substitutions
	substitutions := map[string]string{
		"${LOCALSTACK_ENDPOINT}": l.infra.LocalStackEndpoint,
		"${MAILHOG_HOST}":        mailhogHost,
		"${MAILHOG_PORT}":        mailhogPort,
		"${MAILHOG_SMTP}":        l.infra.MailHogSMTP,
		"${MAILHOG_API}":         l.infra.MailHogAPI,
		"${TEMPORAL_ENDPOINT}":   l.infra.TemporalEndpoint,
	}

	for placeholder, value := range substitutions {
		str = strings.ReplaceAll(str, placeholder, value)
	}

	return []byte(str)
}

// CreateConfigFromTestCase creates a Config object from a test case
func (l *TestCaseLoader) CreateConfigFromTestCase(tc *TestCase) (*config.Config, error) {
	cfg := config.DefaultConfig()

	// Set mode to Agent so providers are initialized locally (not via proxy)
	cfg.SetMode(config.ModeAgent)

	// Set up roles first (before providers in case providers need them)
	cfg.Roles.Definitions = tc.Roles

	// Set up workflows
	cfg.Workflows.Definitions = tc.Workflows

	// Set up providers
	cfg.Providers.Definitions = tc.Providers

	// Configure Temporal connection - parse host:port from endpoint
	host, portStr, err := net.SplitHostPort(l.infra.TemporalEndpoint)
	if err != nil {
		// If no port in endpoint, use full endpoint as host with default port
		host = l.infra.TemporalEndpoint
		portStr = TemporalDefaultPort
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port in Temporal endpoint: %w", err)
	}

	cfg.Services.Temporal = &models.TemporalConfig{
		Host:              host,
		Port:              port,
		Namespace:         TemporalTestNamespace,
		DisableVersioning: true, // Disable versioning for integration tests
	}

	// Initialize providers (this creates the actual provider implementations)
	// This must be done after setting mode so the correct implementation is used
	err = cfg.InitializeProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize providers: %w", err)
	}

	return cfg, nil
}

// ListTestCases returns all available test case names
func (l *TestCaseLoader) ListTestCases() ([]string, error) {
	entries, err := os.ReadDir(l.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read testdata directory: %w", err)
	}

	var testCases []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if it has the required files
			workflowPath := filepath.Join(l.basePath, entry.Name(), "workflow.yaml")
			if _, err := os.Stat(workflowPath); err == nil {
				testCases = append(testCases, entry.Name())
			}
		}
	}

	return testCases, nil
}
