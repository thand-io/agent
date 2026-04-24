package localpresence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCheckLocalPresenceActivityCallsBrokerHelperOnDarwin(t *testing.T) {
	broker := &fakeBrokerClient{
		presenceResponse: &localbroker.CheckLocalPresenceResponse{
			Approved:        true,
			AuthenticatedAt: time.Date(2026, time.April, 21, 12, 0, 0, 0, time.UTC),
		},
	}
	provider := newTestLocalPresenceProvider(t, "darwin")
	provider.brokerClient = broker
	activities := &localPresenceProviderActivities{provider: provider}

	response, err := activities.CheckLocalPresenceActivity(context.Background(), &models.LocalPresenceApprovalRequest{
		ChallengeID: "challenge-1",
		DeviceID:    "device-alpha",
		WorkflowID:  "workflow-1",
		TaskName:    "presence",
		Prompt:      "Approve this request",
		Timeout:     2 * time.Minute,
		RequestedBy: "requester@example.com",
		RoleName:    "Local Sudo",
		Reason:      "testing",
	})
	if err != nil {
		t.Fatalf("CheckLocalPresenceActivity returned error: %v", err)
	}
	if !response.Approved {
		t.Fatal("expected approved local presence response")
	}
	if got, want := len(broker.presenceRequests), 1; got != want {
		t.Fatalf("presence request count = %d, want %d", got, want)
	}
	if got, want := broker.presenceRequests[0].Prompt, "Approve this request"; got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestCheckLocalPresenceActivityReturnsDeniedResultWithoutRetryForUserCancel(t *testing.T) {
	provider := newTestLocalPresenceProvider(t, "darwin")
	provider.brokerClient = &fakeBrokerClient{
		presenceResponse: &localbroker.CheckLocalPresenceResponse{
			Approved:      false,
			FailureReason: "user canceled local presence",
		},
	}
	activities := &localPresenceProviderActivities{provider: provider}

	response, err := activities.CheckLocalPresenceActivity(context.Background(), &models.LocalPresenceApprovalRequest{
		DeviceID: "device-alpha",
		TaskName: "presence",
		Timeout:  2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CheckLocalPresenceActivity returned error: %v", err)
	}
	if response.Approved {
		t.Fatal("expected denied local presence response")
	}
	if got, want := response.FailureReason, "user canceled local presence"; got != want {
		t.Fatalf("failure reason = %q, want %q", got, want)
	}
	if response.TimedOut {
		t.Fatal("did not expect user cancel to be marked as timeout")
	}
}

func TestCheckLocalPresenceActivityMarksPromptTimeout(t *testing.T) {
	provider := newTestLocalPresenceProvider(t, "darwin")
	provider.brokerClient = &fakeBrokerClient{
		presenceResponse: &localbroker.CheckLocalPresenceResponse{
			Approved:      false,
			FailureReason: "timed out waiting for local presence",
		},
	}
	activities := &localPresenceProviderActivities{provider: provider}

	response, err := activities.CheckLocalPresenceActivity(context.Background(), &models.LocalPresenceApprovalRequest{
		DeviceID: "device-alpha",
		TaskName: "presence",
		Timeout:  2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("CheckLocalPresenceActivity returned error: %v", err)
	}
	if !response.TimedOut {
		t.Fatal("expected local presence timeout marker")
	}
}

func TestCheckLocalPresenceActivityMarksNonDarwinUnsupportedNonRetryable(t *testing.T) {
	provider := newTestLocalPresenceProvider(t, "linux")
	activities := &localPresenceProviderActivities{provider: provider}

	_, err := activities.CheckLocalPresenceActivity(context.Background(), &models.LocalPresenceApprovalRequest{
		DeviceID: "device-alpha",
		TaskName: "presence",
		Timeout:  2 * time.Minute,
	})
	if err == nil {
		t.Fatal("CheckLocalPresenceActivity returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("CheckLocalPresenceActivity error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatal("expected non-Darwin local presence error to be non-retryable")
	}
}

func TestCheckLocalPresenceActivityLeavesUnavailableBrokerErrorsRetryable(t *testing.T) {
	provider := newTestLocalPresenceProvider(t, "darwin")
	provider.brokerClient = &fakeBrokerClient{
		presenceErr: status.Error(codes.Unavailable, "helper unavailable"),
	}
	activities := &localPresenceProviderActivities{provider: provider}

	_, err := activities.CheckLocalPresenceActivity(context.Background(), &models.LocalPresenceApprovalRequest{
		DeviceID: "device-alpha",
		TaskName: "presence",
		Timeout:  2 * time.Minute,
	})
	if err == nil {
		t.Fatal("CheckLocalPresenceActivity returned nil error")
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		t.Fatalf("CheckLocalPresenceActivity error = %T, did not expect non-retryable application wrapper", err)
	}
	if got, want := status.Code(err), codes.Unavailable; got != want {
		t.Fatalf("status code = %v, want %v", got, want)
	}
}

func TestLocalPresenceProviderDoesNotExposeProvisioningCapability(t *testing.T) {
	provider := newTestLocalPresenceProvider(t, "darwin")
	if !provider.HasCapability(models.ProviderCapabilityNotifier) {
		t.Fatal("local-presence provider must advertise notifier capability")
	}
	if provider.HasCapability(models.ProviderCapabilityProvisioning) {
		t.Fatal("local-presence provider must not expose sudo provisioning capability")
	}
}

func newTestLocalPresenceProvider(t *testing.T, goos string) *localPresenceProvider {
	t.Helper()
	provider := &localPresenceProvider{}
	if err := provider.Initialize("local-presence", models.ProviderConfig{
		Name:     "Local Presence",
		Provider: ProviderName,
		Enabled:  true,
	}); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	provider.goos = func() string { return goos }
	if goos != "darwin" {
		provider.brokerClient = nil
	}
	return provider
}

type fakeBrokerClient struct {
	presenceRequests []localbroker.CheckLocalPresenceRequest
	presenceResponse *localbroker.CheckLocalPresenceResponse
	presenceErr      error
}

func (f *fakeBrokerClient) GrantTimedSudoers(context.Context, localbroker.TimedSudoersGrantRequest) (*localbroker.TimedSudoersGrantResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeBrokerClient) RevokeTimedGrant(context.Context, string) (*localbroker.RevokeTimedGrantResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeBrokerClient) CheckLocalPresence(_ context.Context, req localbroker.CheckLocalPresenceRequest) (*localbroker.CheckLocalPresenceResponse, error) {
	f.presenceRequests = append(f.presenceRequests, req)
	if f.presenceErr != nil {
		return nil, f.presenceErr
	}
	if f.presenceResponse == nil {
		return &localbroker.CheckLocalPresenceResponse{Approved: true, AuthenticatedAt: time.Now().UTC()}, nil
	}
	return f.presenceResponse, nil
}

func (f *fakeBrokerClient) PostLocalNotification(context.Context, localbroker.PostLocalNotificationRequest) (*localbroker.PostLocalNotificationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
