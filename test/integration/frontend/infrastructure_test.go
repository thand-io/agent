package ui_e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/thand-io/agent/internal/models"
	testcommon "github.com/thand-io/agent/test/integration/common"
)

const (
	// ThandServerPort is the default HTTP port for Thand server
	ThandServerPort = "8080"
	// MockOIDCPort is the port for mock OIDC provider
	MockOIDCPort = "9090"
)

// UITestInfrastructure extends the common infrastructure with UI-specific containers
type UITestInfrastructure struct {
	*testcommon.TestInfrastructure

	// Mock OIDC Provider
	mockOIDCContainer testcontainers.Container
	MockOIDCEndpoint  string

	// Thand Server
	thandContainer   testcontainers.Container
	ThandEndpoint    string
	ThandAPIEndpoint string

	// Config directory
	configDir string
}

// SetupUITestInfrastructure creates and starts all containers needed for UI E2E tests
func SetupUITestInfrastructure(t *testing.T, ctx context.Context, testCase *TestCase) *UITestInfrastructure {
	t.Helper()

	// Start common infrastructure first
	baseInfra := testcommon.SetupInfrastructure(t, ctx)

	infra := &UITestInfrastructure{
		TestInfrastructure: baseInfra,
	}

	// Start UI-specific containers
	infra.startMockOIDC(ctx)

	// Create temporary config directory with interpolated values
	// Must be done after infrastructure is started so we have endpoints
	infra.createConfigDir(testCase)

	// Start Thand Server (only if test case has actual data)
	if len(testCase.Providers) > 0 || len(testCase.Roles) > 0 || len(testCase.Workflows) > 0 {
		infra.startThandServer(ctx)
	} else {
		t.Log("Skipping Thand server start (empty test case)")
	}

	return infra
}

// createConfigDir creates a temporary directory with all config files
func (infra *UITestInfrastructure) createConfigDir(testCase *TestCase) {
	infra.T.Log("Creating config directory...")
	tempDir, err := os.MkdirTemp("", "thand-ui-test-*")
	require.NoError(infra.T, err, "Failed to create temp config directory")
	infra.configDir = tempDir
	infra.T.Logf("Config directory: %s", infra.configDir)

	// Skip writing if test case is empty (smoke test only)
	if len(testCase.Providers) == 0 && len(testCase.Roles) == 0 && len(testCase.Workflows) == 0 {
		infra.T.Log("Test case is empty, skipping config file writes")
		return
	}

	// Copy YAML files directly from testdata, substituting environment variables
	testdataPath := filepath.Join("testdata", testCase.Name)

	replacements := map[string]string{
		"${LOCALSTACK_ENDPOINT}": infra.LocalStackEndpoint,
		"${MAILHOG_HOST}":        strings.Split(infra.MailHogSMTP, ":")[0],
		"${MAILHOG_PORT}":        strings.Split(infra.MailHogSMTP, ":")[1],
		"${OIDC_ISSUER_URL}":     infra.MockOIDCEndpoint + "/default",
		"${THAND_SERVER_URL}":    infra.ThandEndpoint,
	}

	for _, filenamePair := range []struct{ src, dst string }{
		{"providers.yaml", "providers.yaml"},
		{"roles.yaml", "roles.yaml"},
		{"workflow.yaml", "workflows.yaml"}, // Note: singular in testdata, plural for agent
	} {
		srcPath := filepath.Join(testdataPath, filenamePair.src)
		dstPath := filepath.Join(infra.configDir, filenamePair.dst)

		content, err := os.ReadFile(srcPath)
		if err != nil {
			infra.T.Logf("Warning: Could not read %s: %v", filenamePair.src, err)
			continue
		}

		// Replace all placeholders
		contentStr := string(content)
		for placeholder, value := range replacements {
			contentStr = strings.ReplaceAll(contentStr, placeholder, value)
		}

		err = os.WriteFile(dstPath, []byte(contentStr), 0644)
		require.NoError(infra.T, err, fmt.Sprintf("Failed to write %s", filenamePair.dst))
	}

	infra.T.Logf("Copied and interpolated config files from %s", testdataPath)
}

// startMockOIDC starts a mock OIDC provider container
func (infra *UITestInfrastructure) startMockOIDC(ctx context.Context) {
	infra.T.Log("Starting Mock OIDC provider...")

	// Use a simple mock-oauth2-server for testing
	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/navikt/mock-oauth2-server:2.1.11",
		ExposedPorts: []string{MockOIDCPort + "/tcp"},
		Env: map[string]string{
			"SERVER_PORT": MockOIDCPort,
			"JSON_CONFIG": `{
				"interactiveLogin": false,
				"httpServer": "NettyWrapper",
				"tokenCallbacks": []
			}`,
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(MockOIDCPort+"/tcp").WithStartupTimeout(30*time.Second),
			wait.ForHTTP("/.well-known/openid-configuration").
				WithPort(MockOIDCPort+"/tcp").
				WithStartupTimeout(30*time.Second),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(infra.T, err, "Failed to start Mock OIDC container")
	infra.mockOIDCContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.T, err, "Failed to get Mock OIDC host")
	mappedPort, err := container.MappedPort(ctx, MockOIDCPort+"/tcp")
	require.NoError(infra.T, err, "Failed to get Mock OIDC port")

	infra.MockOIDCEndpoint = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	infra.T.Logf("Mock OIDC provider started at %s", infra.MockOIDCEndpoint)
}

// startThandServer starts the Thand server container
func (infra *UITestInfrastructure) startThandServer(ctx context.Context) {
	infra.T.Log("Starting Thand server container...")

	// Build path to Linux agent binary (needed for Alpine container)
	agentBinaryPath := filepath.Join("..", "..", "..", "bin", "thand-linux-amd64")
	if _, err := os.Stat(agentBinaryPath); os.IsNotExist(err) {
		infra.T.Fatalf("Linux agent binary not found at %s. Run 'make build' first.", agentBinaryPath)
	}

	// Create config.yaml for the server - separate from definitions directory
	// to avoid the agent trying to parse config.yaml as a workflow/role/provider definition
	configDir := filepath.Join(os.TempDir(), fmt.Sprintf("thand-server-config-%d", time.Now().UnixNano()))
	err := os.MkdirAll(configDir, 0755)
	require.NoError(infra.T, err, "Failed to create config directory")
	infra.RegisterCleanup(func() {
		os.RemoveAll(configDir)
	})

	configYAML := fmt.Sprintf(`
mode: server
login:
  endpoint: http://localhost:%s
  base: /
server:
  address: 0.0.0.0:%s
  tls:
    enabled: false
temporal:
  endpoint: %s
  namespace: %s
providers:
  path: /app/definitions
roles:
  path: /app/definitions
workflows:
  path: /app/definitions
`, ThandServerPort, ThandServerPort, infra.TemporalEndpoint, testcommon.TemporalTestNamespace)

	err = os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0644)
	require.NoError(infra.T, err, "Failed to write config.yaml")

	req := testcontainers.ContainerRequest{
		Image: "alpine:latest",
		Cmd: []string{
			"/app/agent", "server", "--config", "/app/config.yaml",
		},
		ExposedPorts: []string{ThandServerPort + "/tcp"},
		Env: map[string]string{
			"THAND_MODE":               "server",
			"THAND_TEMPORAL_ENDPOINT":  infra.TemporalEndpoint,
			"THAND_TEMPORAL_NAMESPACE": testcommon.TemporalTestNamespace,
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      agentBinaryPath,
				ContainerFilePath: "/app/agent",
				FileMode:          0755,
			},
			{
				HostFilePath:      filepath.Join(configDir, "config.yaml"),
				ContainerFilePath: "/app/config.yaml",
				FileMode:          0644,
			},
		},
		Mounts: testcontainers.Mounts(testcontainers.BindMount(infra.configDir, "/app/definitions")),
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(ThandServerPort+"/tcp").WithStartupTimeout(60*time.Second),
			wait.ForHTTP("/api/v1/health").
				WithPort(ThandServerPort+"/tcp").
				WithStartupTimeout(60*time.Second),
		).WithDeadline(90 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(infra.T, err, "Failed to start Thand server container")
	infra.thandContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.T, err, "Failed to get Thand server host")
	mappedPort, err := container.MappedPort(ctx, ThandServerPort+"/tcp")
	require.NoError(infra.T, err, "Failed to get Thand server port")

	infra.ThandEndpoint = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	infra.ThandAPIEndpoint = infra.ThandEndpoint + "/api/v1"
	infra.T.Logf("Thand server started at %s", infra.ThandEndpoint)
}

// Teardown stops and removes all containers
func (infra *UITestInfrastructure) Teardown() {
	infra.T.Log("Tearing down UI test infrastructure...")

	terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Terminate UI-specific containers first
	uiContainers := []testcontainers.Container{
		infra.thandContainer,
		infra.mockOIDCContainer,
	}

	for _, container := range uiContainers {
		if container != nil {
			if err := container.Terminate(terminateCtx); err != nil {
				infra.T.Logf("Warning: Failed to terminate UI container: %v", err)
			}
		}
	}

	// Clean up config directory
	if infra.configDir != "" {
		os.RemoveAll(infra.configDir)
	}

	// Call base infrastructure teardown
	infra.TestInfrastructure.Teardown()

	infra.T.Log("UI test infrastructure teardown complete")
}

// TestUIInfrastructureSetup verifies that all containers start correctly
func TestUIInfrastructureSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UI integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create a minimal test case
	testCase := &TestCase{
		Name:      "infrastructure-test",
		Providers: make(map[string]models.ProviderConfig),
		Roles:     make(map[string]models.Role),
		Workflows: make(map[string]models.Workflow),
	}

	infra := SetupUITestInfrastructure(t, ctx, testCase)
	defer infra.Teardown()

	t.Run("All services are accessible", func(t *testing.T) {
		require.NotEmpty(t, infra.LocalStackEndpoint)
		require.NotEmpty(t, infra.MailHogAPI)
		require.NotEmpty(t, infra.TemporalEndpoint)
		require.NotEmpty(t, infra.MockOIDCEndpoint)

		t.Logf("LocalStack: %s", infra.LocalStackEndpoint)
		t.Logf("MailHog API: %s", infra.MailHogAPI)
		t.Logf("Temporal: %s", infra.TemporalEndpoint)
		t.Logf("Mock OIDC: %s", infra.MockOIDCEndpoint)
		// Note: Thand Server is not started in smoke test (empty test case)
	})
}
