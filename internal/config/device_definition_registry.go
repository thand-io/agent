package config

import (
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

func deviceDefinitionRegistryWorkflow(ctx workflow.Context) error {
	definitions := map[string]models.Device{}

	if err := workflow.SetQueryHandler(ctx, models.TemporalGetDeviceDefinitionQueryName, func(deviceID string) (*models.Device, error) {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			return nil, fmt.Errorf("device id is required")
		}

		device, ok := definitions[deviceID]
		if !ok {
			return nil, fmt.Errorf("device %q is not configured", deviceID)
		}

		deviceCopy := device
		return &deviceCopy, nil
	}); err != nil {
		return err
	}

	signalCh := workflow.GetSignalChannel(ctx, models.TemporalDeviceDefinitionUpsertSignalName)
	for {
		cancelled := false
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, _ bool) {
			var device models.Device
			c.Receive(ctx, &device)

			device = normalizeDeviceDefinition(device)
			if device.ID == "" {
				return
			}

			existing, exists := definitions[device.ID]
			if exists && !deviceDefinitionsEqual(existing, device) {
				workflow.GetLogger(ctx).Warn("Ignoring conflicting device definition update",
					"device_id", device.ID,
				)
				return
			}

			definitions[device.ID] = device
		})
		selector.AddReceive(ctx.Done(), func(workflow.ReceiveChannel, bool) {
			cancelled = true
		})
		selector.Select(ctx)
		if cancelled {
			return ctx.Err()
		}
	}
}
