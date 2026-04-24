package localbroker

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	localbrokerv1 "github.com/thand-io/agent/internal/localbroker/proto/localbroker/v1"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type fakeLocalBrokerControlServer struct {
	localbrokerv1.UnimplementedLocalBrokerControlServer

	grantResponse *localbrokerv1.GrantTimedSudoersResponse
	grantError    error
	grantRequest  *localbrokerv1.GrantTimedSudoersRequest

	revokeResponse *localbrokerv1.RevokeTimedGrantResponse
	revokeError    error
	revokeRequest  *localbrokerv1.RevokeTimedGrantRequest
}

func (f *fakeLocalBrokerControlServer) GrantTimedSudoers(
	_ context.Context,
	req *localbrokerv1.GrantTimedSudoersRequest,
) (*localbrokerv1.GrantTimedSudoersResponse, error) {
	f.grantRequest = req
	if f.grantError != nil {
		return nil, f.grantError
	}
	if f.grantResponse != nil {
		return f.grantResponse, nil
	}
	return &localbrokerv1.GrantTimedSudoersResponse{
		BrokerHandle:        "broker-handle",
		TargetUsername:      req.GetTargetUsername(),
		ExpiresAtUnixMillis: time.Date(2026, time.April, 20, 15, 4, 5, 0, time.UTC).UnixMilli(),
	}, nil
}

func (f *fakeLocalBrokerControlServer) RevokeTimedGrant(
	_ context.Context,
	req *localbrokerv1.RevokeTimedGrantRequest,
) (*localbrokerv1.RevokeTimedGrantResponse, error) {
	f.revokeRequest = req
	if f.revokeError != nil {
		return nil, f.revokeError
	}
	if f.revokeResponse != nil {
		return f.revokeResponse, nil
	}
	return &localbrokerv1.RevokeTimedGrantResponse{
		Status: localbrokerv1.RevokeTimedGrantResponse_STATUS_REVOKED,
	}, nil
}

func grpcTestStarter(
	t *testing.T,
	server *fakeLocalBrokerControlServer,
	diagnostics string,
	gotArgs *[]string,
) helperStarter {
	t.Helper()

	return func(_ context.Context, executable string, args []string) (*helperSession, error) {
		if gotArgs != nil {
			*gotArgs = append([]string(nil), args...)
		}
		if got, want := executable, "/test/brokerctl"; got != want {
			t.Fatalf("executable = %q, want %q", got, want)
		}

		listener := bufconn.Listen(1 << 20)
		grpcServer := grpc.NewServer()
		localbrokerv1.RegisterLocalBrokerControlServer(grpcServer, server)

		waitCh := make(chan error, 1)
		go func() {
			err := grpcServer.Serve(listener)
			if err == grpc.ErrServerStopped {
				err = nil
			}
			waitCh <- err
		}()

		clientConn, err := listener.Dial()
		if err != nil {
			t.Fatalf("Dial returned error: %v", err)
		}

		return &helperSession{
			dial: func(context.Context, string) (net.Conn, error) {
				return clientConn, nil
			},
			close: func() error {
				return clientConn.Close()
			},
			wait: func() error {
				_ = clientConn.Close()
				grpcServer.Stop()
				_ = listener.Close()
				return <-waitCh
			},
			diagnostics: func() string {
				return diagnostics
			},
		}, nil
	}
}

func TestGrantTimedSudoersPassesStrictHelperArguments(t *testing.T) {
	server := &fakeLocalBrokerControlServer{}
	var gotArgs []string

	client := &CommandClient{
		executable:   "/test/brokerctl",
		serviceLabel: "io.thand.agent.privilege-broker",
		start:        grpcTestStarter(t, server, "", &gotArgs),
	}

	response, err := client.GrantTimedSudoers(context.Background(), TimedSudoersGrantRequest{
		GrantID:          "grant-1",
		DeviceID:         "device-alpha",
		TargetUsername:   "tester",
		RoleName:         "local_sudo",
		Duration:         5 * time.Minute,
		RequestExpiresAt: time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("GrantTimedSudoers returned error: %v", err)
	}

	if got, want := gotArgs, []string{"serve", "--service-label", "io.thand.agent.privilege-broker"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("args = %#v, want %#v", gotArgs, want)
	}
	if got, want := server.grantRequest.GetGrantId(), "grant-1"; got != want {
		t.Fatalf("grant_id = %q, want %q", got, want)
	}
	if got, want := server.grantRequest.GetDurationMillis(), int64((5 * time.Minute).Milliseconds()); got != want {
		t.Fatalf("duration_millis = %d, want %d", got, want)
	}
	if server.grantRequest.RequestExpiresAtUnixMillis == nil {
		t.Fatal("expected request_expires_at_unix_millis to be set")
	}
	if got, want := response.BrokerHandle, "broker-handle"; got != want {
		t.Fatalf("broker handle = %q, want %q", got, want)
	}
}

func TestGrantTimedSudoersCodeSigningErrorSuggestsStrictLocalSigning(t *testing.T) {
	client := &CommandClient{
		executable:   "/test/brokerctl",
		serviceLabel: "io.thand.agent.privilege-broker",
		start: grpcTestStarter(t, &fakeLocalBrokerControlServer{
			grantError: status.Error(codes.PermissionDenied, "Peer forbidden (code signing)"),
		}, "", nil),
	}

	_, err := client.GrantTimedSudoers(context.Background(), TimedSudoersGrantRequest{
		GrantID:        "grant-1",
		DeviceID:       "device-alpha",
		TargetUsername: "tester",
		RoleName:       "local_sudo",
		Duration:       5 * time.Minute,
	})
	if err == nil {
		t.Fatal("GrantTimedSudoers returned nil error")
	}

	if got, want := status.Code(err), codes.PermissionDenied; got != want {
		t.Fatalf("status code = %v, want %v", got, want)
	}

	errText := err.Error()
	if !strings.Contains(errText, "make build") {
		t.Fatalf("error = %q, want local signing guidance", errText)
	}
	if !strings.Contains(errText, "sudo make install-macos-privilege-services-dev") {
		t.Fatalf("error = %q, want reinstall guidance", errText)
	}
}

func TestGrantTimedSudoersAgentPeerMismatchSuggestsBrokerReinstall(t *testing.T) {
	client := &CommandClient{
		executable:   "/test/brokerctl",
		serviceLabel: "io.thand.agent.privilege-broker",
		start: grpcTestStarter(t, &fakeLocalBrokerControlServer{
			grantError: status.Error(codes.PermissionDenied, "agent peer identity check failed"),
		}, "", nil),
	}

	_, err := client.GrantTimedSudoers(context.Background(), TimedSudoersGrantRequest{
		GrantID:        "grant-1",
		DeviceID:       "device-alpha",
		TargetUsername: "tester",
		RoleName:       "local_sudo",
		Duration:       5 * time.Minute,
	})
	if err == nil {
		t.Fatal("GrantTimedSudoers returned nil error")
	}

	if got, want := status.Code(err), codes.PermissionDenied; got != want {
		t.Fatalf("status code = %v, want %v", got, want)
	}

	errText := err.Error()
	if !strings.Contains(errText, "make install-macos-privilege-services-dev") {
		t.Fatalf("error = %q, want broker reinstall guidance", errText)
	}
}

func TestNewCommandClientHonorsServiceLabelEnvironmentOverride(t *testing.T) {
	previousValue, hadPreviousValue := os.LookupEnv(ServiceLabelEnvVar)
	if err := os.Setenv(ServiceLabelEnvVar, "io.example.privilege-broker"); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	t.Cleanup(func() {
		if hadPreviousValue {
			_ = os.Setenv(ServiceLabelEnvVar, previousValue)
			return
		}
		_ = os.Unsetenv(ServiceLabelEnvVar)
	})

	client := NewCommandClient(nil)
	if got, want := client.serviceLabel, "io.example.privilege-broker"; got != want {
		t.Fatalf("service label = %q, want %q", got, want)
	}
}

func TestNewCommandClientIgnoresProviderConfigOverrides(t *testing.T) {
	config := models.BasicConfig{
		"broker_ctl_path":      "/tmp/override-brokerctl",
		"broker_service_label": "io.example.override",
	}

	client := NewCommandClient(&config)
	if got, want := client.executable, DefaultControlExecutable; got != want {
		t.Fatalf("executable = %q, want %q", got, want)
	}
	if got, want := client.serviceLabel, DefaultMachServiceLabel; got != want {
		t.Fatalf("service label = %q, want %q", got, want)
	}
}

func TestTimedSudoersGrantRequestMarshalsDurationAsSeconds(t *testing.T) {
	payload, err := json.Marshal(TimedSudoersGrantRequest{
		GrantID:        "grant-1",
		DeviceID:       "device-alpha",
		TargetUsername: "tester",
		RoleName:       "local_sudo",
		Duration:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if got, want := decoded["duration"], float64(300); got != want {
		t.Fatalf("duration JSON value = %#v, want %#v", got, want)
	}
	if _, exists := decoded["request_expires_at"]; exists {
		t.Fatalf("unexpected request_expires_at in marshaled payload: %#v", decoded["request_expires_at"])
	}
}

func TestTimedSudoersGrantRequestMarshalsRequestExpiryWhenPresent(t *testing.T) {
	expiresAt := time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC)

	payload, err := json.Marshal(TimedSudoersGrantRequest{
		GrantID:          "grant-1",
		DeviceID:         "device-alpha",
		TargetUsername:   "tester",
		RoleName:         "local_sudo",
		Duration:         5 * time.Minute,
		RequestExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	if got, want := decoded["request_expires_at"], "2026-04-21T12:00:00Z"; got != want {
		t.Fatalf("request_expires_at JSON value = %#v, want %#v", got, want)
	}
}

func TestGrantTimedSudoersIgnoresDiagnosticStderrOnSuccess(t *testing.T) {
	client := &CommandClient{
		executable:   "/test/brokerctl",
		serviceLabel: "io.thand.agent.privilege-broker",
		start:        grpcTestStarter(t, &fakeLocalBrokerControlServer{}, "[thand-privilege-broker] creating broker xpc client session", nil),
	}

	response, err := client.GrantTimedSudoers(context.Background(), TimedSudoersGrantRequest{
		GrantID:        "grant-1",
		DeviceID:       "device-alpha",
		TargetUsername: "tester",
		RoleName:       "local_sudo",
		Duration:       5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GrantTimedSudoers returned error: %v", err)
	}

	if got, want := response.BrokerHandle, "broker-handle"; got != want {
		t.Fatalf("broker handle = %q, want %q", got, want)
	}
}

func TestRevokeTimedGrantReturnsRevokedStatus(t *testing.T) {
	client := &CommandClient{
		executable:   "/test/brokerctl",
		serviceLabel: "io.thand.agent.privilege-broker",
		start: grpcTestStarter(t, &fakeLocalBrokerControlServer{
			revokeResponse: &localbrokerv1.RevokeTimedGrantResponse{
				Status: localbrokerv1.RevokeTimedGrantResponse_STATUS_REVOKED,
			},
		}, "", nil),
	}

	response, err := client.RevokeTimedGrant(context.Background(), "broker-handle")
	if err != nil {
		t.Fatalf("RevokeTimedGrant returned error: %v", err)
	}
	if got, want := response.Status, RevokeTimedGrantStatusRevoked; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestRevokeTimedGrantReturnsNotFoundStatus(t *testing.T) {
	client := &CommandClient{
		executable:   "/test/brokerctl",
		serviceLabel: "io.thand.agent.privilege-broker",
		start: grpcTestStarter(t, &fakeLocalBrokerControlServer{
			revokeResponse: &localbrokerv1.RevokeTimedGrantResponse{
				Status: localbrokerv1.RevokeTimedGrantResponse_STATUS_NOT_FOUND,
			},
		}, "broker session cancelled: Session manually canceled", nil),
	}

	response, err := client.RevokeTimedGrant(context.Background(), "missing-broker-handle")
	if err != nil {
		t.Fatalf("RevokeTimedGrant returned error: %v", err)
	}
	if got, want := response.Status, RevokeTimedGrantStatusNotFound; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestIsRetryableError(t *testing.T) {
	if !IsRetryableError(status.Error(codes.Unavailable, "transport interrupted")) {
		t.Fatal("expected unavailable to be retryable")
	}
	if IsRetryableError(status.Error(codes.PermissionDenied, "peer rejected")) {
		t.Fatal("expected permission denied to be non-retryable")
	}
	if !IsNonRetryableError(status.Error(codes.FailedPrecondition, "grant already completed")) {
		t.Fatal("expected failed precondition to be non-retryable")
	}
}
