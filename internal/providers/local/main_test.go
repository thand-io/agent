package local

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/testing/temporaltest"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeProviderContext struct {
	deadline time.Time
	ok       bool
}

func (f fakeProviderContext) Deadline() (time.Time, bool) {
	return f.deadline, f.ok
}

func TestAuthorizeRoleUnixTimedCreatesAndRevokesGrant(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-1",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 30 * time.Minute
	req.Duration = &duration

	response, err := provider.AuthorizeRole(nil, req)
	if err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}

	sudoersPath, _ := response.Metadata["sudoers_path"].(string)
	if len(sudoersPath) == 0 {
		t.Fatal("expected sudoers_path metadata to be set")
	}
	leasePath, _ := response.Metadata["lease_path"].(string)
	if len(leasePath) == 0 {
		t.Fatal("expected lease_path metadata to be set")
	}

	content, err := os.ReadFile(sudoersPath)
	if err != nil {
		t.Fatalf("failed to read sudoers grant: %v", err)
	}
	if !strings.Contains(string(content), "tester ALL=(ALL:ALL) NOPASSWD: ALL") {
		t.Fatalf("unexpected sudoers content: %s", string(content))
	}

	leaseData, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("failed to read lease metadata: %v", err)
	}
	var lease leaseRecord
	if err := json.Unmarshal(leaseData, &lease); err != nil {
		t.Fatalf("failed to decode lease metadata: %v", err)
	}
	if lease.GrantID != "grant-1" || lease.DeviceID != "device-alpha" {
		t.Fatalf("unexpected lease metadata: %#v", lease)
	}

	if _, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{
		AuthorizeRoleResponse: response,
	}); err != nil {
		t.Fatalf("RevokeRole returned error: %v", err)
	}

	if _, err := os.Stat(sudoersPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected lease file to be removed, stat err=%v", err)
	}
}

func TestAuthorizeRoleUnixCommandRunsThroughSudoAndCleansUp(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)

	var calls []string
	provider.runCommand = func(name string, args ...string) (commandResult, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch filepath.Base(name) {
		case "visudo":
			return commandResult{}, nil
		case "sudo":
			return commandResult{Stdout: "root\n"}, nil
		default:
			return commandResult{}, nil
		}
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeCommand,
		Command:       []string{"whoami"},
		GrantID:       "grant-2",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})

	response, err := provider.AuthorizeRole(nil, req)
	if err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}

	if response.Metadata["stdout"] != "root\n" {
		t.Fatalf("stdout = %#v, want %q", response.Metadata["stdout"], "root\n")
	}
	if _, exists := response.Metadata["sudoers_path"]; exists {
		t.Fatalf("sudoers_path metadata should not be returned for immediate command execution")
	}
	if _, exists := response.Metadata["lease_path"]; exists {
		t.Fatalf("lease_path metadata should not be returned for immediate command execution")
	}
	if len(calls) < 2 {
		t.Fatalf("expected visudo and sudo invocations, got %#v", calls)
	}
	if !strings.Contains(calls[len(calls)-1], "/usr/bin/whoami") {
		t.Fatalf("expected sudo invocation to use resolved command path, got %#v", calls[len(calls)-1])
	}
}

func TestAuthorizeRoleDarwinTimedDelegatesToBroker(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	broker := &fakeBrokerClient{
		grantResponse: &localbroker.TimedSudoersGrantResponse{
			BrokerHandle:   "broker-handle-1",
			TargetUsername: "tester",
			ExpiresAt:      time.Now().UTC().Add(30 * time.Minute),
		},
	}
	provider.brokerClient = broker
	provider.lookupUser = func(string) (*user.User, error) {
		t.Fatal("darwin broker path should not resolve local users in the unprivileged client")
		return nil, nil
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:             models.LocalSudoModeTimed,
		GrantID:          "grant-darwin-1",
		DeviceID:         "device-alpha",
		LocalUsername:    "tester",
		DeniedUsernames:  []string{"root"},
		AllowedUIDRanges: []string{"501-60000"},
	})
	duration := 45 * time.Minute
	req.Duration = &duration

	response, err := provider.AuthorizeRole(nil, req)
	if err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}

	if len(broker.grantRequests) != 1 {
		t.Fatalf("grantRequests = %d, want 1", len(broker.grantRequests))
	}
	if got, want := broker.grantRequests[0].GrantID, "grant-darwin-1"; got != want {
		t.Fatalf("grant id = %q, want %q", got, want)
	}
	if got, want := broker.grantRequests[0].TargetUsername, "tester"; got != want {
		t.Fatalf("target username = %q, want %q", got, want)
	}
	if got, want := broker.grantRequests[0].RoleName, "local_sudo"; got != want {
		t.Fatalf("role name = %q, want %q", got, want)
	}
	if got, want := broker.grantRequests[0].Duration, duration; got != want {
		t.Fatalf("duration = %s, want %s", got, want)
	}
	if !reflect.DeepEqual(broker.grantRequests[0].DeniedUsernames, []string{"root"}) {
		t.Fatalf("denied usernames = %#v", broker.grantRequests[0].DeniedUsernames)
	}
	if !reflect.DeepEqual(broker.grantRequests[0].AllowedUIDRanges, []string{"501-60000"}) {
		t.Fatalf("allowed uid ranges = %#v", broker.grantRequests[0].AllowedUIDRanges)
	}
	if !broker.grantRequests[0].RequestExpiresAt.IsZero() {
		t.Fatalf("request_expires_at = %s, want zero value for broker v1", broker.grantRequests[0].RequestExpiresAt)
	}

	if got, want := response.Metadata["broker_handle"], "broker-handle-1"; got != want {
		t.Fatalf("broker_handle = %#v, want %q", got, want)
	}
	if _, exists := response.Metadata["sudoers_path"]; exists {
		t.Fatalf("sudoers_path metadata should not be returned for brokered Darwin grants")
	}
	if _, exists := response.Metadata["lease_path"]; exists {
		t.Fatalf("lease_path metadata should not be returned for brokered Darwin grants")
	}
}

func TestAuthorizeRoleDarwinTimedDoesNotForwardProviderDeadlineToBroker(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	broker := &fakeBrokerClient{
		grantResponse: &localbroker.TimedSudoersGrantResponse{
			BrokerHandle:   "broker-handle-1",
			TargetUsername: "tester",
			ExpiresAt:      time.Now().UTC().Add(30 * time.Minute),
		},
	}
	provider.brokerClient = broker

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-darwin-ctx",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 5 * time.Minute
	req.Duration = &duration

	ctx := fakeProviderContext{
		deadline: time.Now().Add(2 * time.Second),
		ok:       true,
	}

	if _, err := provider.AuthorizeRole(ctx, req); err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}

	if len(broker.grantRequests) != 1 {
		t.Fatalf("grantRequests = %d, want 1", len(broker.grantRequests))
	}
	if !broker.grantRequests[0].RequestExpiresAt.IsZero() {
		t.Fatalf("request_expires_at = %s, want zero value even with provider deadline", broker.grantRequests[0].RequestExpiresAt)
	}
}

func TestAuthorizeRoleDarwinTimedWrapsNonRetryableBrokerErrors(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	provider.brokerClient = &fakeBrokerClient{
		grantErr: status.Error(codes.PermissionDenied, "Peer forbidden (code signing)"),
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-darwin-permission-denied",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 5 * time.Minute
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil {
		t.Fatal("AuthorizeRole returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("AuthorizeRole error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatal("expected broker permission error to be wrapped as non-retryable")
	}
	if got, want := appErr.Type(), "LocalBrokerError"; got != want {
		t.Fatalf("application error type = %q, want %q", got, want)
	}
	if !strings.Contains(appErr.Message(), "Peer forbidden (code signing)") {
		t.Fatalf("application error message = %q, want code-signing text", appErr.Message())
	}
}

func TestAuthorizeRoleDarwinTimedLeavesUnavailableBrokerErrorsRetryable(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	provider.brokerClient = &fakeBrokerClient{
		grantErr: status.Error(codes.Unavailable, "Underlying connection interrupted"),
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-darwin-unavailable",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 5 * time.Minute
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil {
		t.Fatal("AuthorizeRole returned nil error")
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		t.Fatalf("AuthorizeRole error = %T, did not expect non-retryable application wrapper", err)
	}
	if got, want := status.Code(err), codes.Unavailable; got != want {
		t.Fatalf("status code = %v, want %v", got, want)
	}
}

func TestAuthorizeRoleTemporalDarwinCompletesThroughActivity(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	broker := &fakeBrokerClient{
		grantResponse: &localbroker.TimedSudoersGrantResponse{
			BrokerHandle:   "broker-handle-temporal",
			TargetUsername: "tester",
			ExpiresAt:      time.Now().UTC().Add(30 * time.Minute),
		},
	}
	provider.brokerClient = broker

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-darwin-temporal",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 5 * time.Minute
	req.Duration = &duration

	env := newLocalProviderWorkflowEnvironment(provider)
	env.ExecuteWorkflow(func(ctx workflow.Context, req *models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
		return provider.AuthorizeRole(ctx, req)
	}, req)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow returned error: %v", err)
	}

	var resp models.AuthorizeRoleResponse
	if err := env.GetWorkflowResult(&resp); err != nil {
		t.Fatalf("failed to read workflow result: %v", err)
	}
	if got, want := resp.Metadata["broker_handle"], "broker-handle-temporal"; got != want {
		t.Fatalf("broker_handle = %#v, want %q", got, want)
	}
	if len(broker.grantRequests) != 1 {
		t.Fatalf("grantRequests = %d, want 1", len(broker.grantRequests))
	}
}

func TestRevokeRoleTemporalDarwinCompletesThroughActivity(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	broker := &fakeBrokerClient{
		revokeResponse: &localbroker.RevokeTimedGrantResponse{
			Status: localbroker.RevokeTimedGrantStatusRevoked,
		},
	}
	provider.brokerClient = broker

	env := newLocalProviderWorkflowEnvironment(provider)
	env.ExecuteWorkflow(func(ctx workflow.Context, req *models.RevokeRoleRequest) (*models.RevokeRoleResponse, error) {
		return provider.RevokeRole(ctx, req)
	}, &models.RevokeRoleRequest{
		AuthorizeRoleResponse: &models.AuthorizeRoleResponse{
			Metadata: map[string]any{
				"platform":      "darwin",
				"mode":          string(models.LocalSudoModeTimed),
				"broker_handle": "broker-handle-temporal-revoke",
			},
		},
	})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow returned error: %v", err)
	}
	if !reflect.DeepEqual(broker.revokeHandles, []string{"broker-handle-temporal-revoke"}) {
		t.Fatalf("revoke handles = %#v, want [\"broker-handle-temporal-revoke\"]", broker.revokeHandles)
	}
}

func TestAuthorizeRoleTemporalLinuxTimedCompletesThroughActivity(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-linux-temporal",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 5 * time.Minute
	req.Duration = &duration

	env := newLocalProviderWorkflowEnvironment(provider)
	env.ExecuteWorkflow(func(ctx workflow.Context, req *models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
		return provider.AuthorizeRole(ctx, req)
	}, req)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow returned error: %v", err)
	}

	var resp models.AuthorizeRoleResponse
	if err := env.GetWorkflowResult(&resp); err != nil {
		t.Fatalf("failed to read workflow result: %v", err)
	}

	sudoersPath, _ := resp.Metadata["sudoers_path"].(string)
	if sudoersPath == "" {
		t.Fatal("expected sudoers_path metadata to be set")
	}
	if _, err := os.Stat(sudoersPath); err != nil {
		t.Fatalf("expected sudoers file to exist, stat err=%v", err)
	}
}

func TestRevokeRoleTemporalLinuxTimedCompletesThroughActivity(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)
	duration := 5 * time.Minute

	authorizeReq := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-linux-temporal-revoke",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	authorizeReq.Duration = &duration

	authorizeResp, err := provider.AuthorizeRole(nil, authorizeReq)
	if err != nil {
		t.Fatalf("AuthorizeRole returned error: %v", err)
	}
	authorizeResp.Metadata["mode"] = string(models.LocalSudoModeTimed)

	sudoersPath, _ := authorizeResp.Metadata["sudoers_path"].(string)
	leasePath, _ := authorizeResp.Metadata["lease_path"].(string)

	env := newLocalProviderWorkflowEnvironment(provider)
	env.ExecuteWorkflow(func(ctx workflow.Context, req *models.RevokeRoleRequest) (*models.RevokeRoleResponse, error) {
		return provider.RevokeRole(ctx, req)
	}, &models.RevokeRoleRequest{AuthorizeRoleResponse: authorizeResp})

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow returned error: %v", err)
	}
	if _, err := os.Stat(sudoersPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected lease file to be removed, stat err=%v", err)
	}
}

func TestAuthorizeRoleTemporalLinuxCommandCompletesThroughActivity(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)

	var calls []string
	provider.runCommand = func(name string, args ...string) (commandResult, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch filepath.Base(name) {
		case "visudo":
			return commandResult{}, nil
		case "sudo":
			return commandResult{Stdout: "root\n"}, nil
		default:
			return commandResult{}, nil
		}
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeCommand,
		Command:       []string{"whoami"},
		GrantID:       "grant-linux-command-temporal",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})

	env := newLocalProviderWorkflowEnvironment(provider)
	env.ExecuteWorkflow(func(ctx workflow.Context, req *models.AuthorizeRoleRequest) (*models.AuthorizeRoleResponse, error) {
		return provider.AuthorizeRole(ctx, req)
	}, req)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow returned error: %v", err)
	}

	var resp models.AuthorizeRoleResponse
	if err := env.GetWorkflowResult(&resp); err != nil {
		t.Fatalf("failed to read workflow result: %v", err)
	}
	if got, want := resp.Metadata["stdout"], "root\n"; got != want {
		t.Fatalf("stdout = %#v, want %q", got, want)
	}
	if len(calls) < 2 {
		t.Fatalf("expected visudo and sudo invocations, got %#v", calls)
	}
}

func TestRevokeRoleDarwinUsesBrokerHandle(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	broker := &fakeBrokerClient{
		revokeResponse: &localbroker.RevokeTimedGrantResponse{
			Status: localbroker.RevokeTimedGrantStatusRevoked,
		},
	}
	provider.brokerClient = broker
	provider.enforcer = &fakeEnforcer{
		revokeFn: func(meta models.LocalSudoAuthorizationMetadata) error {
			t.Fatalf("darwin broker revoke should not fall back to file-based enforcer: %#v", meta)
			return nil
		},
	}

	_, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{
		AuthorizeRoleResponse: &models.AuthorizeRoleResponse{
			Metadata: map[string]any{
				"platform":      "darwin",
				"mode":          string(models.LocalSudoModeTimed),
				"broker_handle": "broker-handle-2",
			},
		},
	})
	if err != nil {
		t.Fatalf("RevokeRole returned error: %v", err)
	}

	if !reflect.DeepEqual(broker.revokeHandles, []string{"broker-handle-2"}) {
		t.Fatalf("revoke handles = %#v, want [\"broker-handle-2\"]", broker.revokeHandles)
	}
}

func TestRevokeRoleDarwinTreatsMissingBrokerLeaseAsSuccess(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	broker := &fakeBrokerClient{
		revokeResponse: &localbroker.RevokeTimedGrantResponse{
			Status: localbroker.RevokeTimedGrantStatusNotFound,
		},
	}
	provider.brokerClient = broker

	if _, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{
		AuthorizeRoleResponse: &models.AuthorizeRoleResponse{
			Metadata: map[string]any{
				"platform":      "darwin",
				"mode":          string(models.LocalSudoModeTimed),
				"broker_handle": "broker-handle-missing",
			},
		},
	}); err != nil {
		t.Fatalf("RevokeRole returned error: %v", err)
	}

	if !reflect.DeepEqual(broker.revokeHandles, []string{"broker-handle-missing"}) {
		t.Fatalf("revoke handles = %#v, want [\"broker-handle-missing\"]", broker.revokeHandles)
	}
}

func TestRevokeRoleDarwinWrapsNonRetryableBrokerErrors(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	provider.brokerClient = &fakeBrokerClient{
		revokeErr: status.Error(codes.FailedPrecondition, "grant already completed"),
	}

	_, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{
		AuthorizeRoleResponse: &models.AuthorizeRoleResponse{
			Metadata: map[string]any{
				"platform":      "darwin",
				"mode":          string(models.LocalSudoModeTimed),
				"broker_handle": "broker-handle-3",
			},
		},
	})
	if err == nil {
		t.Fatal("RevokeRole returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("RevokeRole error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatal("expected broker completion error to be wrapped as non-retryable")
	}
	if got, want := appErr.Type(), "LocalBrokerError"; got != want {
		t.Fatalf("application error type = %q, want %q", got, want)
	}
}

func TestAuthorizeRoleActivityMarksValidationErrorsNonRetryable(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	activities := &localProviderActivities{provider: provider}

	_, err := activities.AuthorizeRoleActivity(context.Background(), newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-invalid-user",
		DeviceID:      "device-alpha",
		LocalUsername: "invalid user",
	}))
	if err == nil {
		t.Fatal("AuthorizeRoleActivity returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("AuthorizeRoleActivity error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatal("expected validation error to be non-retryable")
	}
	if got, want := appErr.Type(), "LocalProviderActivityError"; got != want {
		t.Fatalf("application error type = %q, want %q", got, want)
	}
}

func TestAuthorizeRoleActivityLeavesUnavailableBrokerErrorsRetryable(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	provider.brokerClient = &fakeBrokerClient{
		grantErr: status.Error(codes.Unavailable, "Underlying connection interrupted"),
	}
	activities := &localProviderActivities{provider: provider}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-darwin-activity-unavailable",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	duration := 5 * time.Minute
	req.Duration = &duration

	_, err := activities.AuthorizeRoleActivity(context.Background(), req)
	if err == nil {
		t.Fatal("AuthorizeRoleActivity returned nil error")
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		t.Fatalf("AuthorizeRoleActivity error = %T, did not expect non-retryable application wrapper", err)
	}
	if got, want := status.Code(err), codes.Unavailable; got != want {
		t.Fatalf("status code = %v, want %v", got, want)
	}
}

func TestRevokeRoleActivityLeavesNonRetryableBrokerErrorsNonRetryable(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())
	provider.brokerClient = &fakeBrokerClient{
		revokeErr: status.Error(codes.FailedPrecondition, "grant already completed"),
	}
	activities := &localProviderActivities{provider: provider}

	_, err := activities.RevokeRoleActivity(context.Background(), &models.RevokeRoleRequest{
		AuthorizeRoleResponse: &models.AuthorizeRoleResponse{
			Metadata: map[string]any{
				"platform":      "darwin",
				"mode":          string(models.LocalSudoModeTimed),
				"broker_handle": "broker-handle-activity-revoke",
			},
		},
	})
	if err == nil {
		t.Fatal("RevokeRoleActivity returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("RevokeRoleActivity error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatal("expected broker completion error to stay non-retryable")
	}
}

func TestAuthorizeRoleDarwinCommandIsUnsupported(t *testing.T) {
	provider := newTestLocalProvider(t, "darwin", t.TempDir())

	_, err := provider.AuthorizeRole(nil, newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeCommand,
		Command:       []string{"whoami"},
		GrantID:       "grant-darwin-command",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	}))
	if err == nil || !strings.Contains(err.Error(), "not supported on macOS") {
		t.Fatalf("expected Darwin command unsupported error, got %v", err)
	}
}

func TestAuthorizeRoleWindowsTimedIsUnsupported(t *testing.T) {
	provider := newTestLocalProvider(t, "windows", t.TempDir())
	duration := 10 * time.Minute

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:     models.LocalSudoModeTimed,
		GrantID:  "grant-3",
		DeviceID: "device-alpha",
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "not supported on Windows") {
		t.Fatalf("expected unsupported timed Windows error, got %v", err)
	}
}

func TestAuthorizeRoleWindowsCommandRequiresWindowsSudo(t *testing.T) {
	provider := newTestLocalProvider(t, "windows", t.TempDir())
	provider.lookPath = func(string) (string, error) {
		return "", errors.New("not found")
	}

	_, err := provider.AuthorizeRole(nil, newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:     models.LocalSudoModeCommand,
		Command:  []string{"netstat", "-ab"},
		GrantID:  "grant-4",
		DeviceID: "device-alpha",
	}))
	if err == nil || !strings.Contains(err.Error(), "Windows Sudo is unavailable") {
		t.Fatalf("expected Windows Sudo availability error, got %v", err)
	}
}

func TestAuthorizeRoleUnixFailsWithoutResolvedLocalUsername(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	duration := 15 * time.Minute

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:     models.LocalSudoModeTimed,
		GrantID:  "grant-missing-user",
		DeviceID: "device-alpha",
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "missing a resolved local username") {
		t.Fatalf("expected missing local username error, got %v", err)
	}
}

func TestAuthorizeRoleUnixFailsForDeniedUsername(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	duration := 15 * time.Minute

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:             models.LocalSudoModeTimed,
		GrantID:          "grant-denied-user",
		DeviceID:         "device-alpha",
		LocalUsername:    "root",
		DeniedUsernames:  []string{"root"},
		AllowedUIDRanges: []string{"0-60000"},
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected denied username error, got %v", err)
	}
}

func TestAuthorizeRoleUnixFailsForUIDOutsideAllowedRange(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	duration := 15 * time.Minute
	provider.lookupUser = func(username string) (*user.User, error) {
		return &user.User{Username: username, Uid: "999"}, nil
	}

	req := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:             models.LocalSudoModeTimed,
		GrantID:          "grant-uid-outside-range",
		DeviceID:         "device-alpha",
		LocalUsername:    "fallbackuser",
		AllowedUIDRanges: []string{"1000-60000"},
	})
	req.Duration = &duration

	_, err := provider.AuthorizeRole(nil, req)
	if err == nil || !strings.Contains(err.Error(), "outside allowed UID ranges") {
		t.Fatalf("expected UID allow-range error, got %v", err)
	}
}

func TestAllowedUIDRangesFromLoginDefs(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	provider.readFile = func(string) ([]byte, error) {
		return []byte(`
# Comment
UID_MIN 1000
UID_MAX 60000
`), nil
	}

	ranges, err := provider.allowedUIDRanges(models.LocalSudoRequestMetadata{})
	if err != nil {
		t.Fatalf("allowedUIDRanges returned error: %v", err)
	}

	expected := []uidRange{{Min: 1000, Max: 60000}}
	if !reflect.DeepEqual(ranges, expected) {
		t.Fatalf("ranges = %#v, want %#v", ranges, expected)
	}
}

func TestAllowedUIDRangesConfigOverridesLoginDefs(t *testing.T) {
	provider := newTestLocalProvider(t, "linux", t.TempDir())
	(*provider.GetConfig())["allowed_uid_ranges"] = []string{"2000-2999"}
	provider.readFile = func(string) ([]byte, error) {
		return []byte("UID_MIN 1000\nUID_MAX 60000\n"), nil
	}

	ranges, err := provider.allowedUIDRanges(models.LocalSudoRequestMetadata{})
	if err != nil {
		t.Fatalf("allowedUIDRanges returned error: %v", err)
	}

	expected := []uidRange{{Min: 2000, Max: 2999}}
	if !reflect.DeepEqual(ranges, expected) {
		t.Fatalf("ranges = %#v, want %#v", ranges, expected)
	}
}

func TestAuthorizeRoleUnixTimedOverlappingGrantsUseDifferentFragments(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)
	duration := 30 * time.Minute

	reqA := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-a",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	reqA.Duration = &duration

	reqB := newAuthorizeRoleRequest(models.LocalSudoRequestMetadata{
		Mode:          models.LocalSudoModeTimed,
		GrantID:       "grant-b",
		DeviceID:      "device-alpha",
		LocalUsername: "tester",
	})
	reqB.Duration = &duration

	respA, err := provider.AuthorizeRole(nil, reqA)
	if err != nil {
		t.Fatalf("AuthorizeRole A returned error: %v", err)
	}
	respB, err := provider.AuthorizeRole(nil, reqB)
	if err != nil {
		t.Fatalf("AuthorizeRole B returned error: %v", err)
	}

	pathA, _ := respA.Metadata["sudoers_path"].(string)
	pathB, _ := respB.Metadata["sudoers_path"].(string)
	if pathA == "" || pathB == "" {
		t.Fatalf("expected both sudoers paths, got %q and %q", pathA, pathB)
	}
	if pathA == pathB {
		t.Fatalf("expected different sudoers paths, both were %q", pathA)
	}

	if _, err := provider.RevokeRole(nil, &models.RevokeRoleRequest{AuthorizeRoleResponse: respA}); err != nil {
		t.Fatalf("RevokeRole A returned error: %v", err)
	}
	if _, err := os.Stat(pathA); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected first sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Fatalf("expected second sudoers file to remain, stat err=%v", err)
	}
}

func TestReconcileRemovesExpiredLease(t *testing.T) {
	tempDir := t.TempDir()
	provider := newTestLocalProvider(t, "linux", tempDir)
	provider.readFile = os.ReadFile

	sudoersPath := filepath.Join(tempDir, "thand-local-sudo-expired")
	if err := os.WriteFile(sudoersPath, []byte("tester ALL=(ALL:ALL) NOPASSWD: ALL\n"), 0440); err != nil {
		t.Fatalf("failed to seed sudoers fragment: %v", err)
	}

	leasePath := filepath.Join(tempDir, "lease-expired.json")
	record := leaseRecord{
		GrantID:     "expired",
		DeviceID:    "device-alpha",
		Username:    "tester",
		SudoersPath: sudoersPath,
		ExpiresAt:   time.Now().UTC().Add(-1 * time.Minute),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("failed to marshal lease record: %v", err)
	}
	if err := os.WriteFile(leasePath, data, 0600); err != nil {
		t.Fatalf("failed to seed lease record: %v", err)
	}

	if err := provider.enforcer.Reconcile(); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if _, err := os.Stat(sudoersPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected expired sudoers file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(leasePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected expired lease file to be removed, stat err=%v", err)
	}
}

func newTestLocalProvider(t *testing.T, goos, tempDir string) *localProvider {
	t.Helper()

	config := models.BasicConfig{
		"sudoers_dir": tempDir,
		"lease_dir":   tempDir,
		"visudo_path": "visudo",
		"sudo_path":   "sudo",
	}

	provider := &localProvider{}
	if err := provider.Initialize("local", models.ProviderConfig{
		Name:     "Local",
		Provider: "local",
		Enabled:  true,
		Config:   &config,
	}); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	provider.goos = func() string { return goos }
	provider.lookupUser = func(username string) (*user.User, error) {
		return &user.User{Username: username, Uid: "1000"}, nil
	}
	provider.readFile = func(string) ([]byte, error) {
		return nil, os.ErrNotExist
	}
	provider.lookPath = func(name string) (string, error) {
		switch name {
		case "visudo":
			return "/usr/sbin/visudo", nil
		case "sudo":
			return "/usr/bin/sudo", nil
		case "whoami":
			return "/usr/bin/whoami", nil
		case "netstat":
			return "C:\\Windows\\System32\\netstat.exe", nil
		default:
			return "", os.ErrNotExist
		}
	}
	provider.runCommand = func(name string, args ...string) (commandResult, error) {
		return commandResult{}, nil
	}

	return provider
}

type fakeBrokerClient struct {
	grantRequests  []localbroker.TimedSudoersGrantRequest
	revokeHandles  []string
	grantResponse  *localbroker.TimedSudoersGrantResponse
	revokeResponse *localbroker.RevokeTimedGrantResponse
	grantErr       error
	revokeErr      error
}

func (f *fakeBrokerClient) GrantTimedSudoers(ctx context.Context, req localbroker.TimedSudoersGrantRequest) (*localbroker.TimedSudoersGrantResponse, error) {
	f.grantRequests = append(f.grantRequests, req)
	if f.grantErr != nil {
		return nil, f.grantErr
	}
	if f.grantResponse == nil {
		return &localbroker.TimedSudoersGrantResponse{
			BrokerHandle:   "broker-handle-default",
			TargetUsername: req.TargetUsername,
		}, nil
	}
	return f.grantResponse, nil
}

func (f *fakeBrokerClient) RevokeTimedGrant(ctx context.Context, handle string) (*localbroker.RevokeTimedGrantResponse, error) {
	f.revokeHandles = append(f.revokeHandles, handle)
	if f.revokeErr != nil {
		return nil, f.revokeErr
	}
	if f.revokeResponse == nil {
		return &localbroker.RevokeTimedGrantResponse{
			Status: localbroker.RevokeTimedGrantStatusRevoked,
		}, nil
	}
	return f.revokeResponse, nil
}

type fakeEnforcer struct {
	revokeFn func(meta models.LocalSudoAuthorizationMetadata) error
}

func (f *fakeEnforcer) GrantTimed(username string, meta models.LocalSudoRequestMetadata, roleName string, duration time.Duration) (*localElevationLease, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeEnforcer) RunCommand(username string, meta models.LocalSudoRequestMetadata, roleName string) (*localCommandExecution, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeEnforcer) Revoke(meta models.LocalSudoAuthorizationMetadata) error {
	if f.revokeFn != nil {
		return f.revokeFn(meta)
	}
	return nil
}

func (f *fakeEnforcer) Reconcile() error {
	return nil
}

func newAuthorizeRoleRequest(metadata models.LocalSudoRequestMetadata) *models.AuthorizeRoleRequest {
	return &models.AuthorizeRoleRequest{
		Identity: &models.Identity{
			User: &models.User{
				Email: "user@example.com",
			},
		},
		Role: &models.CompositeRole{
			Role: models.Role{
				Name:       "Local Sudo",
				Identifier: "local_sudo",
			},
		},
		Metadata: metadata.AsMap(),
	}
}

func newLocalProviderWorkflowEnvironment(provider *localProvider) *testsuite.TestWorkflowEnvironment {
	temporaltest.SeedBinaryChecksum()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	activities := &localProviderActivities{provider: provider}
	env.RegisterActivityWithOptions(
		activities.AuthorizeRoleActivity,
		activity.RegisterOptions{
			Name: models.CreateTemporalProviderWorkflowName(provider.GetIdentifier(), AuthorizeRoleActivityName),
		},
	)
	env.RegisterActivityWithOptions(
		activities.RevokeRoleActivity,
		activity.RegisterOptions{
			Name: models.CreateTemporalProviderWorkflowName(provider.GetIdentifier(), RevokeRoleActivityName),
		},
	)
	return env
}
