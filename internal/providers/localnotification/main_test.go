package localnotification

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLocalNotificationProviderAdvertisesNotifierCapabilityOnly(t *testing.T) {
	if !LocalNotificationCapabilities.IsCapabilityEnabled(models.ProviderCapabilityNotifier) {
		t.Fatal("expected notifier capability")
	}
	if LocalNotificationCapabilities.IsCapabilityEnabled(models.ProviderCapabilityAuthorizer) {
		t.Fatal("did not expect authorizer capability")
	}
}

func TestLocalNotificationProviderRegistersActivities(t *testing.T) {
	provider := newTestProvider(t, "darwin")
	require.NotNil(t, provider.RegisterActivities())
}

func TestSendNotificationCallsBrokerHelperOnDarwin(t *testing.T) {
	provider := newTestProvider(t, "darwin")
	client := &fakeBrokerClient{}
	provider.brokerClient = client

	err := provider.SendNotification(context.Background(), models.NotificationRequest{
		"title":     "Access approved",
		"body":      "Your sudo access is ready",
		"thread_id": "workflow-1",
	})
	if err != nil {
		t.Fatalf("SendNotification returned error: %v", err)
	}

	if len(client.notificationRequests) != 1 {
		t.Fatalf("notification request count = %d, want 1", len(client.notificationRequests))
	}
	if got, want := client.notificationRequests[0].Title, "Access approved"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if got, want := client.notificationRequests[0].Body, "Your sudo access is ready"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if got, want := client.notificationRequests[0].ThreadID, "workflow-1"; got != want {
		t.Fatalf("thread_id = %q, want %q", got, want)
	}
}

func TestSendNotificationPermissionDeniedIsNonRetryable(t *testing.T) {
	provider := newTestProvider(t, "darwin")
	provider.brokerClient = &fakeBrokerClient{
		notificationErr: status.Error(codes.PermissionDenied, "notification permission denied"),
	}

	err := provider.SendNotification(context.Background(), models.NotificationRequest{
		"title": "Access approved",
		"body":  "Your sudo access is ready",
	})
	if err == nil {
		t.Fatal("SendNotification returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatalf("error = %v, want nonretryable", err)
	}
}

func TestSendNotificationUnavailableBrokerErrorStaysRetryable(t *testing.T) {
	provider := newTestProvider(t, "darwin")
	provider.brokerClient = &fakeBrokerClient{
		notificationErr: status.Error(codes.Unavailable, "helper unavailable"),
	}

	err := provider.SendNotification(context.Background(), models.NotificationRequest{
		"title": "Access approved",
		"body":  "Your sudo access is ready",
	})
	if err == nil {
		t.Fatal("SendNotification returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want *temporal.ApplicationError", err)
	}
	if appErr.NonRetryable() {
		t.Fatalf("error = %v, want retryable application error", err)
	}
}

func TestSendNotificationNonDarwinUnsupportedIsNonRetryable(t *testing.T) {
	provider := newTestProvider(t, "linux")

	err := provider.SendNotification(context.Background(), models.NotificationRequest{
		"title": "Access approved",
		"body":  "Your sudo access is ready",
	})
	if err == nil {
		t.Fatal("SendNotification returned nil error")
	}

	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %T, want *temporal.ApplicationError", err)
	}
	if !appErr.NonRetryable() {
		t.Fatalf("error = %v, want nonretryable", err)
	}
}

func TestSendNotificationRequiresTitleAndBody(t *testing.T) {
	provider := newTestProvider(t, "darwin")
	provider.brokerClient = &fakeBrokerClient{}

	err := provider.SendNotification(context.Background(), models.NotificationRequest{
		"title": "Access approved",
	})
	require.Error(t, err)

	var appErr *temporal.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.True(t, appErr.NonRetryable())
}

func newTestProvider(t *testing.T, goos string) *localNotificationProvider {
	t.Helper()

	provider := &localNotificationProvider{}
	if err := provider.Initialize("local-notification", models.ProviderConfig{
		Name:     "Local Notification",
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
	notificationRequests []localbroker.PostLocalNotificationRequest
	notificationErr      error
}

func (f *fakeBrokerClient) GrantTimedSudoers(context.Context, localbroker.TimedSudoersGrantRequest) (*localbroker.TimedSudoersGrantResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeBrokerClient) RevokeTimedGrant(context.Context, string) (*localbroker.RevokeTimedGrantResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeBrokerClient) CheckLocalPresence(context.Context, localbroker.CheckLocalPresenceRequest) (*localbroker.CheckLocalPresenceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (f *fakeBrokerClient) PostLocalNotification(_ context.Context, req localbroker.PostLocalNotificationRequest) (*localbroker.PostLocalNotificationResponse, error) {
	f.notificationRequests = append(f.notificationRequests, req)
	if f.notificationErr != nil {
		return nil, f.notificationErr
	}
	return &localbroker.PostLocalNotificationResponse{Posted: true}, nil
}
