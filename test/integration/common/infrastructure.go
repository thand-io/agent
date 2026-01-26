package common

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	// TemporalTestNamespace is the namespace used for Temporal integration tests
	TemporalTestNamespace = "thand-test"
	// TemporalDefaultPort is the default gRPC port for Temporal server
	TemporalDefaultPort = "7233"
)

// TestInfrastructure holds all shared test containers and clients
type TestInfrastructure struct {
	T   *testing.T
	Ctx context.Context

	// LocalStack (AWS)
	localstackContainer testcontainers.Container
	LocalStackEndpoint  string

	// MailHog (SMTP testing)
	mailhogContainer testcontainers.Container
	MailHogSMTP      string // SMTP endpoint for sending (host:port)
	MailHogAPI       string // HTTP API endpoint for reading emails

	// PostgreSQL (for Temporal)
	postgresContainer testcontainers.Container

	// Temporal
	temporalContainer testcontainers.Container
	TemporalEndpoint  string
	TemporalClient    client.Client

	// Cleanup callbacks - called before container teardown to gracefully shutdown workers
	cleanupCallbacks []func()
}

// SetupInfrastructure creates and starts shared test containers
func SetupInfrastructure(t *testing.T, ctx context.Context) *TestInfrastructure {
	t.Helper()

	infra := &TestInfrastructure{
		T:   t,
		Ctx: ctx,
	}

	// Start LocalStack (AWS mock)
	infra.StartLocalStack(ctx)

	// Start MailHog (SMTP testing)
	infra.StartMailHog(ctx)

	// Start Temporal
	infra.StartTemporal(ctx)

	return infra
}

// StartLocalStack starts the LocalStack container
func (infra *TestInfrastructure) StartLocalStack(ctx context.Context) {
	infra.T.Log("Starting LocalStack container...")

	container, err := localstack.Run(ctx,
		"localstack/localstack:3.0",
		testcontainers.WithEnv(map[string]string{
			"SERVICES": "iam,sts,ses",
			"DEBUG":    "1",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/health").
				WithPort("4566/tcp").
				WithStartupTimeout(60*time.Second).
				WithPollInterval(2*time.Second),
		),
	)
	require.NoError(infra.T, err, "Failed to start LocalStack container")
	infra.localstackContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.T, err, "Failed to get LocalStack host")
	mappedPort, err := container.MappedPort(ctx, "4566/tcp")
	require.NoError(infra.T, err, "Failed to get LocalStack port")

	infra.LocalStackEndpoint = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	infra.T.Logf("LocalStack started at %s", infra.LocalStackEndpoint)
}

// StartMailHog starts the MailHog container for SMTP testing
func (infra *TestInfrastructure) StartMailHog(ctx context.Context) {
	infra.T.Log("Starting MailHog container...")

	req := testcontainers.ContainerRequest{
		Image:        "mailhog/mailhog:v1.0.1",
		ExposedPorts: []string{"1025/tcp", "8025/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("1025/tcp").WithStartupTimeout(30*time.Second),
			wait.ForListeningPort("8025/tcp").WithStartupTimeout(30*time.Second),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(infra.T, err, "Failed to start MailHog container")
	infra.mailhogContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.T, err, "Failed to get MailHog host")
	smtpPort, err := container.MappedPort(ctx, "1025/tcp")
	require.NoError(infra.T, err, "Failed to get MailHog SMTP port")
	apiPort, err := container.MappedPort(ctx, "8025/tcp")
	require.NoError(infra.T, err, "Failed to get MailHog API port")

	infra.MailHogSMTP = net.JoinHostPort(host, smtpPort.Port())
	infra.MailHogAPI = fmt.Sprintf("http://%s:%s", host, apiPort.Port())
	infra.T.Logf("MailHog started - SMTP: %s, API: %s", infra.MailHogSMTP, infra.MailHogAPI)
}

// StartTemporal starts the Temporal container with PostgreSQL
func (infra *TestInfrastructure) StartTemporal(ctx context.Context) {
	infra.T.Log("Starting Temporal container...")

	// First start PostgreSQL
	postgresReq := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "temporal",
			"POSTGRES_PASSWORD": "temporal",
			"POSTGRES_DB":       "temporal",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}

	postgresContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: postgresReq,
		Started:          true,
	})
	require.NoError(infra.T, err, "Failed to start PostgreSQL container")

	postgresHost, err := postgresContainer.Host(ctx)
	require.NoError(infra.T, err, "Failed to get PostgreSQL host")
	postgresPort, err := postgresContainer.MappedPort(ctx, "5432/tcp")
	require.NoError(infra.T, err, "Failed to get PostgreSQL port")
	infra.T.Logf("PostgreSQL started at %s:%s", postgresHost, postgresPort.Port())

	// Get internal container IP for Temporal to connect to PostgreSQL
	postgresInspect, err := postgresContainer.Inspect(ctx)
	require.NoError(infra.T, err, "Failed to inspect PostgreSQL container")
	bridgeNet, exists := postgresInspect.NetworkSettings.Networks["bridge"]
	if !exists {
		infra.T.Fatal("Bridge network not found for PostgreSQL container")
	}
	postgresIP := bridgeNet.IPAddress
	infra.T.Logf("PostgreSQL internal IP: %s", postgresIP)

	// Now start Temporal auto-setup
	temporalReq := testcontainers.ContainerRequest{
		Image:        "temporalio/auto-setup:1.27.2",
		ExposedPorts: []string{TemporalDefaultPort + "/tcp"},
		Env: map[string]string{
			"DB":                       "postgres12",
			"DB_PORT":                  "5432",
			"POSTGRES_USER":            "temporal",
			"POSTGRES_PWD":             "temporal",
			"POSTGRES_SEEDS":           postgresIP,
			"DYNAMIC_CONFIG_FILE_PATH": "/etc/temporal/dynamic_config.yaml",
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader: strings.NewReader(`
frontend.workerVersioningDataAPIs:
  - value: true
frontend.workerVersioningWorkflowAPIs:
  - value: true
frontend.enableDeployments:
  - value: true
system.forceSearchAttributesCacheRefreshOnRead:
  - value: true
`),
				ContainerFilePath: "/etc/temporal/dynamic_config.yaml",
				FileMode:          0644,
			},
		},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(TemporalDefaultPort+"/tcp").WithStartupTimeout(180*time.Second),
			wait.ForLog("Temporal server started").WithStartupTimeout(180*time.Second),
		).WithDeadline(240 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: temporalReq,
		Started:          true,
	})
	require.NoError(infra.T, err, "Failed to start Temporal container")
	infra.temporalContainer = container
	infra.postgresContainer = postgresContainer

	host, err := container.Host(ctx)
	require.NoError(infra.T, err, "Failed to get Temporal host")
	mappedPort, err := container.MappedPort(ctx, TemporalDefaultPort+"/tcp")
	require.NoError(infra.T, err, "Failed to get Temporal port")

	infra.TemporalEndpoint = net.JoinHostPort(host, mappedPort.Port())
	infra.T.Logf("Temporal started at %s", infra.TemporalEndpoint)

	// Register the test namespace with custom search attributes
	infra.registerNamespaceWithSearchAttributes(ctx)

	// Create Temporal client connected to our custom namespace
	c, err := client.Dial(client.Options{
		HostPort:  infra.TemporalEndpoint,
		Namespace: TemporalTestNamespace,
	})
	require.NoError(infra.T, err, "Failed to create Temporal client")
	infra.TemporalClient = c
	infra.T.Log("Temporal client connected")
}

// registerNamespaceWithSearchAttributes creates the test namespace and registers custom search attributes
func (infra *TestInfrastructure) registerNamespaceWithSearchAttributes(ctx context.Context) {
	infra.T.Log("Registering test namespace with custom search attributes...")

	conn, err := grpc.NewClient(
		infra.TemporalEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(infra.T, err, "Failed to create gRPC connection")
	defer conn.Close()

	workflowClient := workflowservice.NewWorkflowServiceClient(conn)
	operatorClient := operatorservice.NewOperatorServiceClient(conn)

	// Register the namespace
	_, err = workflowClient.RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace:                        TemporalTestNamespace,
		Description:                      "Integration test namespace for thand agent",
		WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
	})
	require.NoError(infra.T, err, "Failed to register namespace")
	infra.T.Logf("Namespace '%s' registered", TemporalTestNamespace)

	// Build the search attributes map
	searchAttributeTypes := []interface {
		GetName() string
		GetValueType() enums.IndexedValueType
	}{
		models.TypedSearchAttributeStatus,
		models.TypedSearchAttributeTask,
		models.TypedSearchAttributeUser,
		models.TypedSearchAttributeRole,
		models.TypedSearchAttributeWorkflow,
		models.TypedSearchAttributeProviders,
		models.TypedSearchAttributeReason,
		models.TypedSearchAttributeDuration,
		models.TypedSearchAttributeIdentities,
		models.TypedSearchAttributeApproved,
	}

	searchAttributes := make(map[string]enums.IndexedValueType, len(searchAttributeTypes))
	for _, attr := range searchAttributeTypes {
		searchAttributes[attr.GetName()] = attr.GetValueType()
	}

	_, err = operatorClient.AddSearchAttributes(ctx, &operatorservice.AddSearchAttributesRequest{
		Namespace:        TemporalTestNamespace,
		SearchAttributes: searchAttributes,
	})
	require.NoError(infra.T, err, "Failed to add search attributes to namespace")
	infra.T.Logf("Registered %d custom search attributes", len(searchAttributes))
}

// GetEmails retrieves all emails from MailHog
func (infra *TestInfrastructure) GetEmails() ([]MailHogMessage, error) {
	resp, err := http.Get(infra.MailHogAPI + "/api/v2/messages")
	if err != nil {
		return nil, fmt.Errorf("failed to get emails from MailHog: %w", err)
	}
	defer resp.Body.Close()

	var messages MailHogMessages
	if err := json.NewDecoder(resp.Body).Decode(&messages); err != nil {
		return nil, fmt.Errorf("failed to parse MailHog response: %w", err)
	}

	return messages.Items, nil
}

// ClearEmails deletes all emails from MailHog
func (infra *TestInfrastructure) ClearEmails() error {
	req, err := http.NewRequestWithContext(infra.Ctx, http.MethodDelete, infra.MailHogAPI+"/api/v1/messages", nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code when clearing emails: %d", resp.StatusCode)
	}

	return nil
}

// RegisterCleanup adds a cleanup callback
func (infra *TestInfrastructure) RegisterCleanup(cleanup func()) {
	infra.cleanupCallbacks = append(infra.cleanupCallbacks, cleanup)
}

// Teardown stops and removes all containers
func (infra *TestInfrastructure) Teardown() {
	infra.T.Log("Tearing down test infrastructure...")

	// Run cleanup callbacks first to gracefully shutdown workers
	for _, cleanup := range infra.cleanupCallbacks {
		cleanup()
	}

	terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if infra.TemporalClient != nil {
		infra.TemporalClient.Close()
	}

	containers := []testcontainers.Container{
		infra.temporalContainer,
		infra.postgresContainer,
		infra.mailhogContainer,
		infra.localstackContainer,
	}

	for _, container := range containers {
		if container != nil {
			if err := container.Terminate(terminateCtx); err != nil {
				infra.T.Logf("Warning: Failed to terminate container: %v", err)
			}
		}
	}

	infra.T.Log("Test infrastructure teardown complete")
}

// MailHogMessage represents an email captured by MailHog
type MailHogMessage struct {
	ID   string `json:"ID"`
	From struct {
		Mailbox string `json:"Mailbox"`
		Domain  string `json:"Domain"`
	} `json:"From"`
	To []struct {
		Mailbox string `json:"Mailbox"`
		Domain  string `json:"Domain"`
	} `json:"To"`
	Content struct {
		Headers map[string][]string `json:"Headers"`
		Body    string              `json:"Body"`
	} `json:"Content"`
	Created time.Time `json:"Created"`
}

// MailHogMessages represents the response from MailHog API
type MailHogMessages struct {
	Total int              `json:"total"`
	Count int              `json:"count"`
	Start int              `json:"start"`
	Items []MailHogMessage `json:"items"`
}
