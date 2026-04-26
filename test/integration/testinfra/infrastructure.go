// Package testinfra provides shared container infrastructure for integration tests.
// It manages Temporal (with PostgreSQL), LocalStack (AWS mock), and MailHog (SMTP mock)
// containers via testcontainers-go.
package testinfra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/localstack"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/thand-io/agent/internal/common"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// TemporalTestNamespace is the namespace used for Temporal integration tests.
	TemporalTestNamespace = "thand-test"
	// TemporalDefaultPort is the default gRPC port for Temporal server.
	TemporalDefaultPort = "7233"
)

// TestInfrastructure holds all test containers and clients.
type TestInfrastructure struct {
	t   *testing.T
	ctx context.Context

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

	// Keycloak (Identity Provider - optional)
	keycloakContainer     testcontainers.Container
	KeycloakEndpoint      string
	KeycloakAdvertisedURL string
	allocatedKeycloakPort int

	// Cleanup callbacks - called before container teardown to gracefully shutdown workers
	cleanupCallbacks []func()
}

// SetupOption configures optional infrastructure components.
type SetupOption func(*setupConfig)

type setupConfig struct {
	keycloakRealmPath string
}

// WithKeycloak enables the Keycloak container with the given realm JSON file path.
func WithKeycloak(realmFilePath string) SetupOption {
	return func(cfg *setupConfig) {
		cfg.keycloakRealmPath = realmFilePath
	}
}

// SetupTestInfrastructure creates and starts all containers (Temporal, LocalStack, MailHog).
// Pass optional SetupOption values to enable additional containers (e.g. Keycloak).
func SetupTestInfrastructure(t *testing.T, ctx context.Context, opts ...SetupOption) *TestInfrastructure {
	t.Helper()

	scfg := &setupConfig{}
	for _, opt := range opts {
		opt(scfg)
	}

	infra := &TestInfrastructure{
		t:   t,
		ctx: ctx,
	}

	// Start LocalStack (AWS mock)
	infra.startLocalStack(ctx)

	// Start MailHog (SMTP testing)
	infra.startMailHog(ctx)

	// Start Temporal
	infra.startTemporal(ctx)

	// Optionally start Keycloak
	if scfg.keycloakRealmPath != "" {
		infra.startKeycloak(ctx, scfg.keycloakRealmPath)
	}

	return infra
}

// SetupTemporalInfrastructure creates and starts only Temporal (+ PostgreSQL).
// Use this when tests don't need LocalStack or MailHog.
func SetupTemporalInfrastructure(t *testing.T, ctx context.Context) *TestInfrastructure {
	t.Helper()

	infra := &TestInfrastructure{
		t:   t,
		ctx: ctx,
	}

	infra.startTemporal(ctx)

	return infra
}

// startLocalStack starts the LocalStack container.
func (infra *TestInfrastructure) startLocalStack(ctx context.Context) {
	infra.t.Log("Starting LocalStack container...")

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
	require.NoError(infra.t, err, "Failed to start LocalStack container")

	infra.localstackContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.t, err, "Failed to get LocalStack host")

	mappedPort, err := container.MappedPort(ctx, "4566/tcp")
	require.NoError(infra.t, err, "Failed to get LocalStack port")

	infra.LocalStackEndpoint = fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
	infra.t.Logf("LocalStack started at %s", infra.LocalStackEndpoint)
}

// startMailHog starts the MailHog container for SMTP testing.
// MailHog captures all outgoing emails and provides an API to read them.
func (infra *TestInfrastructure) startMailHog(ctx context.Context) {
	infra.t.Log("Starting MailHog container...")

	req := testcontainers.ContainerRequest{
		Image:        "mailhog/mailhog:v1.0.1",
		ExposedPorts: []string{"1025/tcp", "8025/tcp"}, // SMTP on 1025, HTTP API on 8025
		WaitingFor: wait.ForAll(
			wait.ForListeningPort("1025/tcp").WithStartupTimeout(30*time.Second),
			wait.ForListeningPort("8025/tcp").WithStartupTimeout(30*time.Second),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(infra.t, err, "Failed to start MailHog container")

	infra.mailhogContainer = container

	host, err := container.Host(ctx)
	require.NoError(infra.t, err, "Failed to get MailHog host")

	smtpPort, err := container.MappedPort(ctx, "1025/tcp")
	require.NoError(infra.t, err, "Failed to get MailHog SMTP port")

	apiPort, err := container.MappedPort(ctx, "8025/tcp")
	require.NoError(infra.t, err, "Failed to get MailHog API port")

	infra.MailHogSMTP = net.JoinHostPort(host, smtpPort.Port())
	infra.MailHogAPI = fmt.Sprintf("http://%s:%s", host, apiPort.Port())

	infra.t.Logf("MailHog started - SMTP: %s, API: %s", infra.MailHogSMTP, infra.MailHogAPI)
}

// MailHogMessage represents an email captured by MailHog.
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
	Raw     struct {
		From string   `json:"From"`
		To   []string `json:"To"`
		Data string   `json:"Data"`
	} `json:"Raw"`
}

// MailHogMessages represents the response from the MailHog API.
type MailHogMessages struct {
	Total int              `json:"total"`
	Count int              `json:"count"`
	Start int              `json:"start"`
	Items []MailHogMessage `json:"items"`
}

// GetEmails retrieves all emails from MailHog.
func (infra *TestInfrastructure) GetEmails() ([]MailHogMessage, error) {
	url := infra.MailHogAPI + "/api/v2/messages"
	resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method:   http.MethodGet,
		Endpoint: model.NewEndpoint(url),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get emails from MailHog: %w", err)
	}

	var messages MailHogMessages
	if err := json.Unmarshal(resp.Body(), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse MailHog response: %w", err)
	}

	return messages.Items, nil
}

// GetEmailsForRecipient retrieves emails sent to a specific address.
func (infra *TestInfrastructure) GetEmailsForRecipient(email string) ([]MailHogMessage, error) {
	allEmails, err := infra.GetEmails()
	if err != nil {
		return nil, err
	}

	var filtered []MailHogMessage
	for _, msg := range allEmails {
		for _, to := range msg.To {
			if fmt.Sprintf("%s@%s", to.Mailbox, to.Domain) == email {
				filtered = append(filtered, msg)
				break
			}
		}
	}
	return filtered, nil
}

// WaitForEmail waits for an email to arrive for a specific recipient.
func (infra *TestInfrastructure) WaitForEmail(recipient string, timeout time.Duration) (*MailHogMessage, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		emails, err := infra.GetEmailsForRecipient(recipient)
		if err != nil {
			return nil, err
		}
		if len(emails) > 0 {
			return &emails[0], nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for email to %s", recipient)
}

// ExtractLinksFromEmail extracts all URLs from an email body.
func (infra *TestInfrastructure) ExtractLinksFromEmail(msg *MailHogMessage) []string {
	urlRegex := regexp.MustCompile(`https?://[^\s<>"]+`)
	return urlRegex.FindAllString(msg.Content.Body, -1)
}

// ClearEmails deletes all emails from MailHog.
func (infra *TestInfrastructure) ClearEmails() error {
	req, err := http.NewRequestWithContext(infra.ctx, http.MethodDelete, infra.MailHogAPI+"/api/v1/messages", nil)
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

// startTemporal starts the Temporal container.
// Uses temporalio/auto-setup which includes a development server setup.
func (infra *TestInfrastructure) startTemporal(ctx context.Context) {
	infra.t.Log("Starting Temporal container...")

	// Use auto-setup with a PostgreSQL sidecar for persistence
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
	require.NoError(infra.t, err, "Failed to start PostgreSQL container")

	postgresHost, err := postgresContainer.Host(ctx)
	require.NoError(infra.t, err, "Failed to get PostgreSQL host")

	postgresPort, err := postgresContainer.MappedPort(ctx, "5432/tcp")
	require.NoError(infra.t, err, "Failed to get PostgreSQL port")

	infra.t.Logf("PostgreSQL started at %s:%s", postgresHost, postgresPort.Port())

	// Get internal container IP for Temporal to connect to PostgreSQL
	postgresInspect, err := postgresContainer.Inspect(ctx)
	require.NoError(infra.t, err, "Failed to inspect PostgreSQL container")

	bridgeNet, exists := postgresInspect.NetworkSettings.Networks["bridge"]
	if !exists {
		infra.t.Fatal("Bridge network not found for PostgreSQL container")
	}
	postgresIP := bridgeNet.IPAddress
	infra.t.Logf("PostgreSQL internal IP: %s", postgresIP)

	dynamicConfig := `frontend.workerVersioningDataAPIs:
  - value: true
frontend.workerVersioningWorkflowAPIs:
  - value: true
frontend.enableDeployments:
  - value: true
system.forceSearchAttributesCacheRefreshOnRead:
  - value: true
`

	// Now start Temporal auto-setup
	temporalReq := testcontainers.ContainerRequest{
		Image:        "temporalio/auto-setup:1.28.0",
		ExposedPorts: []string{TemporalDefaultPort + "/tcp"},
		Env: map[string]string{
			"DB":                       "postgres12",
			"DB_PORT":                  "5432",
			"BIND_ON_IP":               "0.0.0.0",
			"POSTGRES_USER":            "temporal",
			"POSTGRES_PWD":             "temporal",
			"POSTGRES_SEEDS":           postgresIP,
			"DYNAMIC_CONFIG_FILE_PATH": "/etc/temporal/dynamic_config.yaml",
		},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(dynamicConfig),
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
	require.NoError(infra.t, err, "Failed to start Temporal container")

	infra.temporalContainer = container
	// Store postgres container for cleanup
	infra.postgresContainer = postgresContainer

	host, err := container.Host(ctx)
	require.NoError(infra.t, err, "Failed to get Temporal host")

	mappedPort, err := container.MappedPort(ctx, TemporalDefaultPort+"/tcp")
	require.NoError(infra.t, err, "Failed to get Temporal port")

	infra.TemporalEndpoint = net.JoinHostPort(host, mappedPort.Port())
	infra.t.Logf("Temporal started at %s", infra.TemporalEndpoint)

	// Register the test namespace with custom search attributes
	infra.registerNamespaceWithSearchAttributes(ctx)

	// Create Temporal client connected to our custom namespace
	c, err := client.Dial(client.Options{
		HostPort:  infra.TemporalEndpoint,
		Namespace: TemporalTestNamespace,
	})
	require.NoError(infra.t, err, "Failed to create Temporal client")

	infra.TemporalClient = c
	infra.t.Log("Temporal client connected")
}

// SaveWorkflowHistory fetches the complete event history for a completed workflow
// execution and writes it as a protojson file at destPath. The file can later be
// loaded by a WorkflowReplayer replay test to guard against non-determinism.
//
// Typical usage (at the end of an integration test, after the workflow has finished):
//
//	infra.SaveWorkflowHistory(t, ctx, workflowID, "", "testdata/my-case/history.json")
func (infra *TestInfrastructure) SaveWorkflowHistory(t *testing.T, ctx context.Context, workflowID, runID, destPath string) {
	t.Helper()

	iter := infra.TemporalClient.GetWorkflowHistory(ctx, workflowID, runID, false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)

	var events []*historypb.HistoryEvent
	for iter.HasNext() {
		ev, err := iter.Next()
		if err != nil {
			t.Logf("Warning: error reading history event for %s: %v; skipping history save", workflowID, err)
			return
		}
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Logf("Warning: no history events found for workflow %s — skipping history save", workflowID)
		return
	}

	history := &historypb.History{Events: events}
	jsonBytes, err := protojson.MarshalOptions{Multiline: true}.Marshal(history)
	if err != nil {
		t.Logf("Warning: failed to marshal history for %s: %v", workflowID, err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		t.Logf("Warning: failed to create history directory %s: %v", filepath.Dir(destPath), err)
		return
	}

	if err := os.WriteFile(destPath, jsonBytes, 0o600); err != nil {
		t.Logf("Warning: failed to write history file %s: %v", destPath, err)
		return
	}

	t.Logf("Saved workflow history (%d events) → %s", len(events), destPath)
}

// RegisterCleanup adds a cleanup callback that will be called before container teardown.
// Use this to gracefully shutdown Temporal workers and other resources.
func (infra *TestInfrastructure) RegisterCleanup(cleanup func()) {
	infra.cleanupCallbacks = append(infra.cleanupCallbacks, cleanup)
}

// Teardown stops and removes all containers.
func (infra *TestInfrastructure) Teardown() {
	infra.t.Log("Tearing down test infrastructure...")

	// Run cleanup callbacks first to gracefully shutdown workers
	for _, cleanup := range infra.cleanupCallbacks {
		cleanup()
	}

	terminateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if infra.TemporalClient != nil {
		infra.TemporalClient.Close()
	}

	if infra.temporalContainer != nil {
		if err := infra.temporalContainer.Terminate(terminateCtx); err != nil {
			infra.t.Logf("Warning: Failed to terminate Temporal container: %v", err)
		}
	}

	if infra.postgresContainer != nil {
		if err := infra.postgresContainer.Terminate(terminateCtx); err != nil {
			infra.t.Logf("Warning: Failed to terminate PostgreSQL container: %v", err)
		}
	}

	if infra.mailhogContainer != nil {
		if err := infra.mailhogContainer.Terminate(terminateCtx); err != nil {
			infra.t.Logf("Warning: Failed to terminate MailHog container: %v", err)
		}
	}

	if infra.localstackContainer != nil {
		if err := infra.localstackContainer.Terminate(terminateCtx); err != nil {
			infra.t.Logf("Warning: Failed to terminate LocalStack container: %v", err)
		}
	}

	if infra.keycloakContainer != nil {
		if err := infra.keycloakContainer.Terminate(terminateCtx); err != nil {
			infra.t.Logf("Warning: Failed to terminate Keycloak container: %v", err)
		}
	}

	infra.t.Log("Test infrastructure teardown complete")
}

// registerNamespaceWithSearchAttributes creates the test namespace and registers
// custom search attributes in a single operation.
func (infra *TestInfrastructure) registerNamespaceWithSearchAttributes(ctx context.Context) {
	infra.t.Log("Registering test namespace with custom search attributes...")

	conn, err := grpc.NewClient(
		infra.TemporalEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(infra.t, err, "Failed to create gRPC connection")
	defer conn.Close()

	workflowClient := workflowservice.NewWorkflowServiceClient(conn)
	operatorClient := operatorservice.NewOperatorServiceClient(conn)

	// Register the namespace directly. Unexpected bootstrap failures should fail fast.
	_, err = workflowClient.RegisterNamespace(ctx, &workflowservice.RegisterNamespaceRequest{
		Namespace:                        TemporalTestNamespace,
		Description:                      "Integration test namespace for thand agent",
		WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
	})
	require.NoError(infra.t, err, "Failed to register namespace")
	infra.t.Logf("Namespace '%s' registered", TemporalTestNamespace)

	// Build the search attributes map from typed search attributes
	searchAttributeTypes := []interface {
		GetName() string
		GetValueType() enums.IndexedValueType
	}{
		sdkConstants.TypedSearchAttributeStatus,
		sdkConstants.TypedSearchAttributeTask,
		sdkConstants.TypedSearchAttributeUser,
		sdkConstants.TypedSearchAttributeRole,
		sdkConstants.TypedSearchAttributeWorkflow,
		sdkConstants.TypedSearchAttributeProviders,
		sdkConstants.TypedSearchAttributeReason,
		sdkConstants.TypedSearchAttributeDuration,
		sdkConstants.TypedSearchAttributeIdentities,
		sdkConstants.TypedSearchAttributeApproved,
	}

	searchAttributes := make(map[string]enums.IndexedValueType, len(searchAttributeTypes))
	for _, attr := range searchAttributeTypes {
		searchAttributes[attr.GetName()] = attr.GetValueType()
	}

	// Temporal can briefly lag namespace visibility after registration, so allow a
	// short retry window for this follow-up call only.
	err = retryAddSearchAttributesUntilNamespaceVisible(ctx, func(callCtx context.Context) error {
		_, err := operatorClient.AddSearchAttributes(callCtx, &operatorservice.AddSearchAttributesRequest{
			Namespace:        TemporalTestNamespace,
			SearchAttributes: searchAttributes,
		})
		return err
	})
	require.NoError(infra.t, err, "Failed to add search attributes to namespace")

	infra.t.Logf("Registered %d custom search attributes for namespace '%s'",
		len(searchAttributes), TemporalTestNamespace)

	// Log the registered attributes
	for name, valueType := range searchAttributes {
		infra.t.Logf("  - %s (%s)", name, valueType.String())
	}
}

// TemporalHostPort splits the Temporal endpoint into host and numeric port.
func (infra *TestInfrastructure) TemporalHostPort() (string, int) {
	host, portStr, _ := net.SplitHostPort(infra.TemporalEndpoint)
	port := 7233
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

func retryAddSearchAttributesUntilNamespaceVisible(ctx context.Context, op func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const (
		pollInterval = 250 * time.Millisecond
		rpcTimeout   = 3 * time.Second
	)

	var lastErr error
	for {
		callCtx, callCancel := context.WithTimeout(ctx, rpcTimeout)
		lastErr = op(callCtx)
		callCancel()
		if lastErr == nil {
			return nil
		}

		if !isNamespaceNotVisibleError(lastErr) {
			return lastErr
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for Temporal namespace visibility: %w", lastErr)
		case <-time.After(pollInterval):
		}
	}
}

func isNamespaceNotVisibleError(err error) bool {
	if err == nil {
		return false
	}

	var namespaceNotFound *serviceerror.NamespaceNotFound
	if errors.As(err, &namespaceNotFound) {
		return true
	}

	convertedErr := serviceerror.FromStatus(serviceerror.ToStatus(err))
	return errors.As(convertedErr, &namespaceNotFound) || status.Code(err) == codes.NotFound
}

// Testing returns the *testing.T associated with this infrastructure.
func (infra *TestInfrastructure) Testing() *testing.T {
	return infra.t
}
