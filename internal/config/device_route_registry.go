package config

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

const deviceRouteRegistryTickInterval = 30 * time.Second

func deviceRouteRegistryWorkflow(ctx workflow.Context) error {
	routes := map[string]models.DeviceConnectionState{}

	if err := workflow.SetQueryHandler(ctx, models.TemporalGetDeviceRouteQueryName, func(deviceID string) (*models.DeviceConnectionState, error) {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			return nil, fmt.Errorf("device id is required")
		}

		route, ok := routes[deviceID]
		if !ok {
			return nil, fmt.Errorf("%w: device %q is not connected", ErrDeviceRouteUnavailable, deviceID)
		}

		route.Connected = route.TaskQueue != "" && !route.LastSeenAt.IsZero() && workflow.Now(ctx).Sub(route.LastSeenAt) <= models.DeviceRouteFreshnessTTL
		if !route.Connected {
			return nil, fmt.Errorf("%w: device %q is not connected", ErrDeviceRouteUnavailable, deviceID)
		}

		routeCopy := route
		return &routeCopy, nil
	}); err != nil {
		return err
	}

	signalCh := workflow.GetSignalChannel(ctx, models.TemporalDeviceRouteUpsertSignalName)
	for {
		cancelled := false
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			var route models.DeviceConnectionState
			c.Receive(ctx, &route)
			route.DeviceID = strings.TrimSpace(route.DeviceID)
			if route.DeviceID == "" {
				return
			}
			route.TaskQueue = strings.TrimSpace(route.TaskQueue)
			if route.LastSeenAt.IsZero() {
				route.LastSeenAt = workflow.Now(ctx)
			}
			route.Connected = route.TaskQueue != "" && workflow.Now(ctx).Sub(route.LastSeenAt) <= models.DeviceRouteFreshnessTTL
			routes[route.DeviceID] = route
		})
		selector.AddReceive(ctx.Done(), func(workflow.ReceiveChannel, bool) {
			cancelled = true
		})
		selector.AddFuture(workflow.NewTimer(ctx, deviceRouteRegistryTickInterval), func(workflow.Future) {})
		selector.Select(ctx)
		if cancelled {
			return ctx.Err()
		}
	}
}

func (c *Config) PublishDeviceConnectionState(ctx context.Context, state models.DeviceConnectionState) error {
	c.SetDeviceConnectionState(state)

	services := c.GetServices()
	if services == nil || !services.HasTemporal() {
		return nil
	}

	temporalService := services.GetTemporal()
	if temporalService == nil || !temporalService.HasClient() {
		return nil
	}

	state.DeviceID = strings.TrimSpace(state.DeviceID)
	state.TaskQueue = strings.TrimSpace(state.TaskQueue)
	if state.DeviceID == "" || state.TaskQueue == "" {
		return nil
	}
	if state.LastSeenAt.IsZero() {
		state.LastSeenAt = time.Now().UTC()
	}
	state.Connected = true

	_, err := temporalService.GetClient().SignalWithStartWorkflow(
		ctx,
		models.TemporalDeviceRouteRegistryWorkflowID,
		models.TemporalDeviceRouteUpsertSignalName,
		state,
		deviceRegistryStartWorkflowOptions(models.TemporalDeviceRouteRegistryWorkflowID),
		models.TemporalDeviceRouteRegistryWorkflowName,
	)
	return err
}
