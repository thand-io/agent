package thand

import (
	"errors"
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

const (
	deviceAuthorizeMaxWait        = 5 * time.Minute
	deviceRouteCheckInterval      = 5 * time.Second
	deviceRouteCheckTimeout       = 10 * time.Second
	deviceRouteRevokeAttemptLimit = 1 * time.Minute
	deviceRouteRevokeInitialRetry = 5 * time.Second
	deviceRouteRevokeMaxRetry     = 1 * time.Minute
)

var errDeviceRouteWaitExpired = errors.New("device route wait expired")

func deviceDispatchBudget(req *models.AuthorizeRoleRequest) time.Duration {
	if req == nil || req.Duration == nil || *req.Duration <= 0 {
		return deviceAuthorizeMaxWait
	}
	if *req.Duration < deviceAuthorizeMaxWait {
		return *req.Duration
	}
	return deviceAuthorizeMaxWait
}

func nextDeviceRouteRetryDelay(current time.Duration) time.Duration {
	if current <= 0 {
		return deviceRouteRevokeInitialRetry
	}
	current *= 2
	if current > deviceRouteRevokeMaxRetry {
		return deviceRouteRevokeMaxRetry
	}
	return current
}

func childWorkflowIDForAttempt(base string, attempt int) string {
	if attempt <= 0 {
		return base
	}
	return fmt.Sprintf("%s_retry_%d", base, attempt)
}

func (t *thandTask) resolveFreshDeviceRoute(
	ctx workflow.Context,
	deviceID string,
) (*models.DeviceConnectionState, error) {
	ao := workflow.LocalActivityOptions{
		StartToCloseTimeout: deviceRouteCheckTimeout,
	}
	actx := workflow.WithLocalActivityOptions(ctx, ao)

	var route models.DeviceConnectionState
	err := workflow.ExecuteLocalActivity(
		actx,
		models.TemporalResolveFreshDeviceRouteActivityName,
		deviceID,
	).Get(ctx, &route)
	if err != nil {
		return nil, err
	}

	return &route, nil
}

func (t *thandTask) waitForFreshDeviceRoute(
	ctx workflow.Context,
	deviceID string,
	timeout time.Duration,
) (*models.DeviceConnectionState, time.Duration, error) {
	if timeout <= 0 {
		return nil, 0, fmt.Errorf("device route wait timeout must be positive")
	}

	deadline := workflow.Now(ctx).Add(timeout)
	for {
		route, err := t.resolveFreshDeviceRoute(ctx, deviceID)
		if err == nil {
			remaining := deadline.Sub(workflow.Now(ctx))
			if remaining <= 0 {
				remaining = time.Second
			}
			return route, remaining, nil
		}
		if !isDeviceRouteUnavailableError(err) {
			return nil, 0, err
		}

		remaining := deadline.Sub(workflow.Now(ctx))
		if remaining <= 0 {
			return nil, 0, fmt.Errorf("%w: device %q did not become available within %s", errDeviceRouteWaitExpired, deviceID, timeout)
		}

		sleepFor := deviceRouteCheckInterval
		if sleepFor > remaining {
			sleepFor = remaining
		}
		if err := workflow.Sleep(ctx, sleepFor); err != nil {
			return nil, 0, err
		}
	}
}
