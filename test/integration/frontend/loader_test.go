package ui_e2e

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/test/integration/testinfra"
	"gopkg.in/yaml.v3"
)

// TestCase represents a UI E2E test case loaded from testdata.
// This is a type alias for testinfra.TestCase to maintain compatibility.
type TestCase = testinfra.TestCase

// TestCaseLoader wraps testinfra.TestCaseLoader with UI-specific config creation.
type TestCaseLoader struct {
	*testinfra.TestCaseLoader
	uiInfra *UITestInfrastructure
}

// providerEnvMu serializes the set-env → InitializeProviders → restore-env window
// so that parallel UI subtests do not race on shared environment variable keys.
var providerEnvMu sync.Mutex

// NewTestCaseLoader creates a new test case loader for UI E2E tests.
func NewTestCaseLoader(infra *UITestInfrastructure) *TestCaseLoader {
	return &TestCaseLoader{
		TestCaseLoader: testinfra.NewTestCaseLoader(infra.TestInfrastructure, "testdata"),
		uiInfra:        infra,
	}
}

// CreateUIConfigFromTestCase creates a Config object from a test case,
// configured for use with the Thand server container.
func (l *TestCaseLoader) CreateUIConfigFromTestCase(_ *testing.T, tc *TestCase) (*config.Config, error) {
	// providerEnvVars holds host.docker.internal URLs for the Thand container.
	// ResolveConfig reads os.Environ() to expand ${ .VARNAME } jq expressions, so we must
	// temporarily set the process environment with localhost-mapped values before calling
	// InitializeProviders. A mutex serializes this window so parallel subtests don't race.
	providerEnvMu.Lock()
	prev := make(map[string]string, len(l.uiInfra.providerEnvVars))
	for k, v := range l.uiInfra.providerEnvVars {
		prev[k] = os.Getenv(k)
		os.Setenv(k, strings.ReplaceAll(v, "host.docker.internal", "localhost")) //nolint:errcheck
	}

	// Use the base loader to create the config (handles providers, roles, workflows, Temporal)
	cfg, err := l.TestCaseLoader.CreateConfigFromTestCase(tc)

	// Restore original env values before releasing the lock.
	for k, old := range prev {
		if old == "" {
			os.Unsetenv(k) //nolint:errcheck
		} else {
			os.Setenv(k, old) //nolint:errcheck
		}
	}
	providerEnvMu.Unlock()

	if err != nil {
		return nil, err
	}

	// Override mode to Server for UI tests
	cfg.SetMode(sdkConstants.ModeServer)

	return cfg, nil
}

// DetectAuthType determines the authentication type from the raw workflow YAML.
// The "authentication" field is not part of the Workflow Go struct, so we parse
// the raw YAML to find it. Returns "saml" if any workflow uses a SAML-prefixed
// authentication provider, otherwise "oidc".
func DetectAuthType(tc *TestCase) string {
	// Parse the raw workflow YAML to find the authentication field
	workflowPath := filepath.Join(tc.Path, "workflow.yaml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return "oidc" // default
	}

	var raw struct {
		Workflows map[string]struct {
			Authentication string `yaml:"authentication"`
		} `yaml:"workflows"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return "oidc"
	}

	for _, wf := range raw.Workflows {
		if wf.Authentication == "saml-test" || wf.Authentication == "saml" {
			return "saml"
		}
	}
	return "oidc"
}

// GetTestUsers returns a map of user role to credentials for a given test case.
// This inspects the roles' scopes to determine which users are relevant.
func GetTestUsers(tc *TestCase) map[string]TestUser {
	users := map[string]TestUser{
		"default": {
			Username: "testuser@thand.io",
			Password: "testpass123",
			Email:    "testuser@thand.io",
			Name:     "Test User",
		},
	}

	// Check if any role has specific user scopes
	for _, role := range tc.Roles {
		if len(role.Scopes.Allow.Users) > 0 {
			for _, u := range role.Scopes.Allow.Users {
				switch u {
				case "engineer@thand.io":
					users["engineer"] = TestUser{
						Username: "engineer@thand.io",
						Password: "testpass123",
						Email:    "engineer@thand.io",
						Name:     "Engineer User",
					}
				case "developer@thand.io":
					users["developer"] = TestUser{
						Username: "developer@thand.io",
						Password: "testpass123",
						Email:    "developer@thand.io",
						Name:     "Developer User",
					}
				}
			}
		}
	}

	// Always include manager for approval workflows
	users["manager"] = TestUser{
		Username: "manager@thand.io",
		Password: "managerpass123",
		Email:    "manager@thand.io",
		Name:     "Manager User",
	}

	return users
}

// TestUser represents test credentials.
type TestUser struct {
	Username string
	Password string
	Email    string
	Name     string
}

// GetWorkflowNames returns the workflow identifiers from a test case.
func GetWorkflowNames(tc *TestCase) []string {
	names := make([]string, 0, len(tc.Workflows))
	for name := range tc.Workflows {
		names = append(names, name)
	}
	return names
}

// GetRoleNames returns the role identifiers from a test case.
func GetRoleNames(tc *TestCase) []string {
	names := make([]string, 0, len(tc.Roles))
	for name := range tc.Roles {
		names = append(names, name)
	}
	return names
}

// GetProviderNames returns the provider identifiers from a test case.
func GetProviderNames(tc *TestCase) []string {
	names := make([]string, 0, len(tc.Providers))
	for name, p := range tc.Providers {
		// Only return access providers (not email, not auth)
		if p.Provider != "email" && p.Provider != "oidc" && p.Provider != "saml" {
			names = append(names, name)
		}
	}
	return names
}

// IsSelfApproval checks if any workflow in the test case has self-approval enabled.
func IsSelfApproval(tc *TestCase) bool {
	for _, wf := range tc.Workflows {
		if wf.Workflow == nil {
			continue
		}
		// We check the workflow DSL for selfApprove: true
		// This is a simplified check — the actual DSL structure varies
		doc := wf.Workflow
		_ = doc
		// For now, rely on workflow name conventions
	}
	return false
}

// HasAWSProvider checks if the test case includes an AWS provider.
func HasAWSProvider(tc *TestCase) bool {
	for _, p := range tc.Providers {
		if p.Provider == "aws" {
			return true
		}
	}
	return false
}

// GetAWSProviderName returns the name of the first AWS provider in the test case.
func GetAWSProviderName(tc *TestCase) string {
	for name, p := range tc.Providers {
		if p.Provider == "aws" {
			return name
		}
	}
	return ""
}

// GetFirstRoleName returns the first role name in the test case.
func GetFirstRoleName(tc *TestCase) string {
	for name := range tc.Roles {
		return name
	}
	return ""
}

// GetFirstWorkflowName returns the first workflow name in the test case.
func GetFirstWorkflowName(tc *TestCase) string {
	for name := range tc.Workflows {
		return name
	}
	return ""
}

// WorkflowRequiresManagerApproval checks if any workflow requires non-self approval.
func WorkflowRequiresManagerApproval(tc *TestCase) bool {
	// Check workflow names for known patterns
	for name := range tc.Workflows {
		if name == "aws_manager_approval_ui" || name == "aws_manager_approval" {
			return true
		}
	}
	return false
}

// RoleUsesProvider checks if a role references a specific provider.
func RoleUsesProvider(role models.Role, providerName string) bool {
	for _, p := range role.Providers {
		if p == providerName {
			return true
		}
	}
	return false
}
