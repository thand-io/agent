package localnotification

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/localbroker"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	sdkWorkflowsRunner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	ProviderName                       = "local-notification"
	SendNotificationActivityName       = "SendNotificationActivity"
	localNotificationProviderErrorType = "LocalNotificationProviderError"
	localNotificationBrokerErrorType   = "LocalNotificationBrokerError"
)

type localNotificationProvider struct {
	*models.BaseProvider

	brokerClient localbroker.Client
	goos         func() string
}

func (p *localNotificationProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(identifier, provider, LocalNotificationCapabilities)
	p.goos = func() string { return runtime.GOOS }
	if p.goos() == "darwin" {
		p.brokerClient = localbroker.NewCommandClient(p.GetConfig())
	}
	return nil
}

func (p *localNotificationProvider) SendNotification(
	ctx models.ProviderContext,
	notification models.NotificationRequest,
) error {
	// When invoked from a Temporal workflow coroutine, dispatch the actual
	// broker RPC as a Temporal activity so it benefits from retry, history,
	// and replay determinism (mirrors the email/slack providers).
	if workflowCtx, ok := ctx.(workflow.Context); ok {
		wfCtx := workflow.WithActivityOptions(workflowCtx, workflow.ActivityOptions{
			StartToCloseTimeout: 2 * time.Minute,
			RetryPolicy:         sdkWorkflowsRunner.DefaultRetryPolicy,
		})
		return workflow.ExecuteActivity(
			wfCtx,
			models.CreateTemporalProviderWorkflowName(p.GetIdentifier(), models.SendNotificationActivityName),
			notification,
		).Get(wfCtx, nil)
	}

	goCtx := models.ContextFromProviderContext(ctx)
	var req models.LocalNotificationRequest
	if err := common.ConvertInterfaceToInterface(notification, &req); err != nil {
		return localNotificationNonRetryableError("failed to parse local notification payload", err)
	}
	if err := p.postLocalNotificationDirect(goCtx, req); err != nil {
		return wrapLocalNotificationProviderError(err)
	}
	return nil
}

func (p *localNotificationProvider) postLocalNotificationDirect(
	ctx context.Context,
	req models.LocalNotificationRequest,
) error {
	if strings.TrimSpace(req.Title) == "" {
		return fmt.Errorf("local notification title is required")
	}
	if strings.TrimSpace(req.Body) == "" {
		return fmt.Errorf("local notification body is required")
	}
	if p.goos() != "darwin" {
		return fmt.Errorf("local notifications are only supported on macOS")
	}
	if p.brokerClient == nil {
		return fmt.Errorf("macOS privilege helper is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logrus.WithFields(logrus.Fields{
		"notification_id": strings.TrimSpace(req.NotificationID),
		"title":           strings.TrimSpace(req.Title),
		"thread_id":       strings.TrimSpace(req.ThreadID),
	}).Info("Posting local notification via macOS helper")

	_, err := p.brokerClient.PostLocalNotification(ctx, localbroker.PostLocalNotificationRequest{
		NotificationID: strings.TrimSpace(req.NotificationID),
		Title:          strings.TrimSpace(req.Title),
		Subtitle:       strings.TrimSpace(req.Subtitle),
		Body:           strings.TrimSpace(req.Body),
		ThreadID:       strings.TrimSpace(req.ThreadID),
	})
	if err == nil {
		logrus.WithFields(logrus.Fields{
			"notification_id": strings.TrimSpace(req.NotificationID),
			"thread_id":       strings.TrimSpace(req.ThreadID),
		}).Info("Posted local notification via macOS helper")
	}
	return err
}

func wrapLocalNotificationProviderError(err error) error {
	if err == nil {
		return nil
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		return err
	}

	if localbroker.IsRetryableError(err) {
		return temporal.NewApplicationErrorWithOptions(
			err.Error(),
			localNotificationBrokerErrorType,
			temporal.ApplicationErrorOptions{Cause: err},
		)
	}

	if localbroker.IsNonRetryableError(err) {
		return temporal.NewNonRetryableApplicationError(
			err.Error(),
			localNotificationBrokerErrorType,
			err,
		)
	}

	return localNotificationNonRetryableError(err.Error(), err)
}

func localNotificationNonRetryableError(message string, cause error) error {
	return temporal.NewNonRetryableApplicationError(
		message,
		localNotificationProviderErrorType,
		cause,
	)
}

func init() {
	providers.Register(ProviderName, &localNotificationProvider{}, LocalNotificationCapabilities, &ConfigSchema{})
}
