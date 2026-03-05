package ui_e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/test/integration/testinfra"
)

const (
	// ThandServerPort is the default HTTP port for Thand server.
	ThandServerPort = "5225"
)

// UITestInfrastructure extends testinfra with UI-specific containers (Thand server).
type UITestInfrastructure struct {
	*testinfra.TestInfrastructure

	// Thand Server
	thandContainer    testcontainers.Container
	ThandEndpoint     string
	ThandAPIEndpoint  string
	allocatedHostPort int          // pre-allocated host port for deterministic URLs
	portListener      net.Listener // held open until startThandServer closes it just before Docker binds

	// providerEnvVars are injected into the server container so that ResolveConfig
	// can substitute ${ .VARNAME } jq expressions in provider definition YAML files.
	providerEnvVars map[string]string

	// Config directory for Thand server definitions
	configDir string
}

// thandServerLogConsumer forwards Thand server container logs to the test log.
type thandServerLogConsumer struct {
	t *testing.T
}

func (l *thandServerLogConsumer) Accept(log testcontainers.Log) {
	l.t.Logf("[thand-server] %s", string(log.Content))
}

// SetupUITestInfrastructure creates and starts all containers needed for UI E2E tests.
// It starts the base infrastructure (LocalStack, MailHog, Temporal) plus Keycloak and
// optionally the Thand server container.
func SetupUITestInfrastructure(t *testing.T, ctx context.Context, testCase *TestCase) *UITestInfrastructure {
	t.Helper()

	// Resolve Keycloak realm file path
	realmPath, err := filepath.Abs(filepath.Join("keycloak", "thand-test-realm.json"))
	require.NoError(t, err, "Failed to resolve Keycloak realm path")
	require.FileExists(t, realmPath, "Keycloak realm file not found at %s", realmPath)

	// Start base infrastructure with Keycloak enabled
	baseInfra := testinfra.SetupTestInfrastructure(t, ctx,
		testinfra.WithKeycloak(realmPath),
	)

	infra := &UITestInfrastructure{
		TestInfrastructure: baseInfra,
	}

	// Pre-allocate a free host port for the Thand server.
	// This allows us to know the server URL BEFORE starting the container,
	// so provider configs (e.g., redirect_url) can be fully interpolated at config-creation time.
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "Failed to allocate a free port for Thand server")
	infra.allocatedHostPort = listener.Addr().(*net.TCPAddr).Port
	infra.portListener = listener // keep open; closed in startThandServer just before container bind

	// Set endpoint URLs early so createConfigDir can interpolate ${THAND_SERVER_URL}
	infra.ThandEndpoint = fmt.Sprintf("http://localhost:%d", infra.allocatedHostPort)
	infra.ThandAPIEndpoint = infra.ThandEndpoint + "/api/v1"
	t.Logf("Pre-allocated Thand server port: %d → %s", infra.allocatedHostPort, infra.ThandEndpoint)

	// Create temporary config directory with interpolated values.
	// Must be done after infrastructure is started so we have endpoints.
	infra.createConfigDir(t, testCase)

	// Start Thand Server if the test case has definitions
	if len(testCase.Providers) > 0 || len(testCase.Roles) > 0 || len(testCase.Workflows) > 0 {
		infra.startThandServer(t, ctx)
	} else {
		t.Log("Skipping Thand server start (empty test case)")
	}

	return infra
}

// createConfigDir creates a temporary directory with interpolated config YAML files.
func (infra *UITestInfrastructure) createConfigDir(t *testing.T, testCase *TestCase) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "thand-ui-test-*")
	require.NoError(t, err, "Failed to create temp config directory")
	infra.configDir = tempDir

	// Skip writing if test case is empty (smoke test only)
	if len(testCase.Providers) == 0 && len(testCase.Roles) == 0 && len(testCase.Workflows) == 0 {
		return
	}

	testdataPath := filepath.Join("testdata", testCase.Name)

	// Build replacement map from live infrastructure endpoints.
	// Use host.docker.internal so the Thand server container can reach host-mapped ports.
	localStackEndpoint := strings.ReplaceAll(infra.TestInfrastructure.LocalStackEndpoint, "localhost", "host.docker.internal")
	mailhogSMTP := strings.ReplaceAll(infra.TestInfrastructure.MailHogSMTP, "localhost", "host.docker.internal")
	smtpParts := strings.SplitN(mailhogSMTP, ":", 2)
	mailhogHost := smtpParts[0]
	mailhogPort := ""
	if len(smtpParts) == 2 {
		mailhogPort = smtpParts[1]
	}

	// Compute endpoint values and store them for injection into the server container.
	// ResolveConfig reads os.Environ() and resolves ${ .VARNAME } jq expressions in YAML files.
	// host.docker.internal URLs are server-side (inside container).
	// localhost URLs are browser-facing (navigated by the test's chromedp browser).
	temporalEndpointInternal := strings.ReplaceAll(infra.TestInfrastructure.TemporalEndpoint, "localhost", "host.docker.internal")

	infra.providerEnvVars = map[string]string{
		"LOCALSTACK_ENDPOINT": localStackEndpoint,
		"MAILHOG_HOST":        mailhogHost,
		"MAILHOG_PORT":        mailhogPort,
		"TEMPORAL_ENDPOINT":   temporalEndpointInternal,
	}

	if infra.TestInfrastructure.KeycloakEndpoint != "" {
		oidcIssuerBrowser := infra.TestInfrastructure.KeycloakOIDCIssuerURL() // already localhost
		oidcIssuerInternal := strings.ReplaceAll(oidcIssuerBrowser, "localhost", "host.docker.internal")
		samlMetaBrowser := infra.TestInfrastructure.KeycloakSAMLMetadataURL()
		samlMetaInternal := strings.ReplaceAll(samlMetaBrowser, "localhost", "host.docker.internal")
		infra.providerEnvVars["OIDC_ISSUER_URL"] = oidcIssuerBrowser
		infra.providerEnvVars["OIDC_ISSUER_URL_INTERNAL"] = oidcIssuerInternal
		infra.providerEnvVars["SAML_IDP_METADATA_URL"] = samlMetaBrowser
		infra.providerEnvVars["SAML_IDP_METADATA_URL_INTERNAL"] = samlMetaInternal
	}

	// THAND_SERVER_URL uses localhost — the browser navigates here after Keycloak redirect.
	if infra.allocatedHostPort > 0 {
		infra.providerEnvVars["THAND_SERVER_URL"] = fmt.Sprintf("http://localhost:%d", infra.allocatedHostPort)
	}

	// Copy definition files verbatim — no string substitution needed.
	// Values are injected as container env vars; ResolveConfig expands ${ .VARNAME } at startup.
	for _, fp := range []struct{ src, dst string }{
		{"providers.yaml", "providers.yaml"},
		{"roles.yaml", "roles.yaml"},
		{"workflow.yaml", "workflows.yaml"}, // singular in testdata, plural for agent
	} {
		srcPath := filepath.Join(testdataPath, fp.src)
		dstPath := filepath.Join(infra.configDir, fp.dst)

		content, err := os.ReadFile(srcPath)
		if err != nil {
			t.Logf("Warning: Could not read %s: %v", fp.src, err)
			continue
		}

		err = os.WriteFile(dstPath, content, 0644)
		require.NoError(t, err, "Failed to write %s", fp.dst)
	}

	t.Logf("Copied config files from %s", testdataPath)
}

// startThandServer starts the Thand server in an Alpine container.
func (infra *UITestInfrastructure) startThandServer(t *testing.T, ctx context.Context) {
	t.Helper()
	t.Log("Starting Thand server container...")

	// Detect host architecture to select the correct Linux binary.
	// Docker Desktop on macOS M-series runs arm64 containers by default.
	goarch := "amd64"
	if arch := os.Getenv("GOARCH"); arch != "" {
		goarch = arch
	} else {
		// runtime.GOARCH gives the architecture of the test process
		goarch = runtime.GOARCH
	}

	// Build path to Linux agent binary (needed for Alpine container)
	agentBinaryPath := filepath.Join("..", "..", "..", "bin", fmt.Sprintf("thand-linux-%s", goarch))
	if _, err := os.Stat(agentBinaryPath); os.IsNotExist(err) {
		// Fall back to amd64 if native arch binary doesn't exist
		agentBinaryPath = filepath.Join("..", "..", "..", "bin", "thand-linux-amd64")
	}
	if _, err := os.Stat(agentBinaryPath); os.IsNotExist(err) {
		t.Fatalf("Linux agent binary not found at %s. Run 'make build-all' first.", agentBinaryPath)
	}
	t.Logf("Using agent binary: %s (arch: %s)", agentBinaryPath, goarch)

	// Create config.yaml for the server in a separate temp dir
	serverConfigDir := filepath.Join(os.TempDir(), fmt.Sprintf("thand-server-config-%d", time.Now().UnixNano()))
	err := os.MkdirAll(serverConfigDir, 0755)
	require.NoError(t, err, "Failed to create server config directory")
	infra.RegisterCleanup(func() {
		os.RemoveAll(serverConfigDir)
	})

	// Convert localhost endpoints to host.docker.internal for Docker Desktop (macOS/Windows).
	// Inside the container, "localhost" refers to the container itself, not the Docker host.
	temporalEndpoint := strings.ReplaceAll(infra.TestInfrastructure.TemporalEndpoint, "localhost", "host.docker.internal")
	temporalHost, temporalPort, _ := net.SplitHostPort(temporalEndpoint)

	configYAML := fmt.Sprintf(`
mode: server
secret: thand-e2e-test-configured
login:
  endpoint: %s
  base: /
server:
  host: 0.0.0.0
  port: %s
  security:
    cors:
      allowed_origins:
        - "*"
services:
  temporal:
    host: %s
    port: %s
    namespace: %s
    disable_versioning: true
providers:
  path: /app/definitions
roles:
  path: /app/definitions
workflows:
  path: /app/definitions
`, infra.ThandEndpoint, ThandServerPort, temporalHost, temporalPort, testinfra.TemporalTestNamespace)

	err = os.WriteFile(filepath.Join(serverConfigDir, "config.yaml"), []byte(configYAML), 0644)
	require.NoError(t, err, "Failed to write config.yaml")

	// Build ContainerFile list: binary + config + definitions
	containerFiles := []testcontainers.ContainerFile{
		{
			HostFilePath:      agentBinaryPath,
			ContainerFilePath: "/app/agent",
			FileMode:          0755,
		},
		{
			HostFilePath:      filepath.Join(serverConfigDir, "config.yaml"),
			ContainerFilePath: "/app/config.yaml",
			FileMode:          0644,
		},
	}
	// Copy definition files directly (avoids Docker Desktop virtiofs sync delays with bind mounts)
	for _, defFile := range []struct{ hostSrc, containerDst string }{
		{"providers.yaml", "/app/definitions/providers.yaml"},
		{"roles.yaml", "/app/definitions/roles.yaml"},
		{"workflows.yaml", "/app/definitions/workflows.yaml"},
	} {
		hostPath := filepath.Join(infra.configDir, defFile.hostSrc)
		if _, err := os.Stat(hostPath); err == nil {
			containerFiles = append(containerFiles, testcontainers.ContainerFile{
				HostFilePath:      hostPath,
				ContainerFilePath: defFile.containerDst,
				FileMode:          0644,
			})
		}
	}

	// Build container env: Thand config vars + provider interpolation vars.
	// Provider vars are read by ResolveConfig (via os.Environ) to substitute
	// ${ .VARNAME } jq expressions in the definition YAML files.
	containerEnv := map[string]string{
		"THAND_MODE":               "server",
		"THAND_TEMPORAL_ENDPOINT":  temporalEndpoint,
		"THAND_TEMPORAL_NAMESPACE": testinfra.TemporalTestNamespace,
	}
	for k, v := range infra.providerEnvVars {
		containerEnv[k] = v
	}

	req := testcontainers.ContainerRequest{
		Image: "alpine:3.18",
		Cmd: []string{
			"/app/agent", "server", "--config", "/app/config.yaml",
		},
		ExposedPorts: []string{ThandServerPort + "/tcp"},
		Env:          containerEnv,
		Files:        containerFiles,
		// Forward container stdout/stderr to test log for diagnostics
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{
				&thandServerLogConsumer{t: t},
			},
		},
		// Bind to the pre-allocated host port so the URL is deterministic.
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.PortBindings = nat.PortMap{
				nat.Port(ThandServerPort + "/tcp"): []nat.PortBinding{
					{HostIP: "0.0.0.0", HostPort: strconv.Itoa(infra.allocatedHostPort)},
				},
			}
			// Ensure host.docker.internal resolves on Linux Docker (not only Docker Desktop).
			hc.ExtraHosts = []string{"host.docker.internal:host-gateway"}
		},
		WaitingFor: wait.ForListeningPort(ThandServerPort + "/tcp").
			WithStartupTimeout(120 * time.Second),
	}

	// Release the pre-allocated listener as late as possible so that the port
	// stays bound on the host – and therefore unavailable to other processes –
	// right up until Docker's port-proxy claims it.
	if infra.portListener != nil {
		infra.portListener.Close()
		infra.portListener = nil
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Thand server container")
	infra.thandContainer = container

	// Verify the container is bound to our pre-allocated port
	host, err := container.Host(ctx)
	require.NoError(t, err, "Failed to get Thand server host")
	mappedPort, err := container.MappedPort(ctx, ThandServerPort+"/tcp")
	require.NoError(t, err, "Failed to get Thand server port")

	actualEndpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	t.Logf("Thand server started at %s (expected %s)", actualEndpoint, infra.ThandEndpoint)

	if os.Getenv("THAND_E2E_DEBUG") == "true" {
		// Debug: exec into container and cat the provider file to see what the server actually reads
		exitCode, output, err := container.Exec(ctx, []string{"cat", "/app/definitions/providers.yaml"})
		if err == nil {
			buf := make([]byte, 4096)
			n, _ := output.Read(buf)
			t.Logf("DEBUG container /app/definitions/providers.yaml (exit=%d):\n%s", exitCode, string(buf[:n]))
		} else {
			t.Logf("DEBUG: Failed to exec cat in container: %v", err)
		}

		// Debug: cat the server config.yaml
		exitCode2, output2, err2 := container.Exec(ctx, []string{"cat", "/app/config.yaml"})
		if err2 == nil {
			buf := make([]byte, 2048)
			n, _ := output2.Read(buf)
			t.Logf("DEBUG container /app/config.yaml (exit=%d):\n%s", exitCode2, string(buf[:n]))
		}
	}
}

// Teardown stops and removes all UI-specific containers, then delegates to base teardown.
func (infra *UITestInfrastructure) Teardown() {
	terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Release the port listener if startThandServer was skipped (empty test case).
	if infra.portListener != nil {
		infra.portListener.Close()
		infra.portListener = nil
	}

	// Terminate UI-specific containers first
	if infra.thandContainer != nil {
		if err := infra.thandContainer.Terminate(terminateCtx); err != nil {
			// Log warning but don't fail
			_ = err
		}
	}

	// Clean up config directory
	if infra.configDir != "" {
		os.RemoveAll(infra.configDir)
	}

	// Delegate to base infrastructure teardown (handles LocalStack, MailHog, Temporal, Keycloak)
	infra.TestInfrastructure.Teardown()
}

// TestUIInfrastructureSetup verifies that all containers start correctly.
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
		require.NotEmpty(t, infra.KeycloakEndpoint)

		t.Logf("LocalStack: %s", infra.LocalStackEndpoint)
		t.Logf("MailHog API: %s", infra.MailHogAPI)
		t.Logf("Temporal: %s", infra.TemporalEndpoint)
		t.Logf("Keycloak: %s", infra.KeycloakEndpoint)
		t.Logf("OIDC Issuer: %s", infra.KeycloakOIDCIssuerURL())
		t.Logf("SAML Metadata: %s", infra.KeycloakSAMLMetadataURL())
	})
}
