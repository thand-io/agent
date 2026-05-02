package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
)

const deviceRegistryQueryTimeout = 30 * time.Second

type deviceRegistryTemporalClient interface {
	DescribeWorkflowExecution(ctx context.Context, workflowID, runID string) (*workflowservice.DescribeWorkflowExecutionResponse, error)
	QueryWorkflowWithOptions(ctx context.Context, request *client.QueryWorkflowWithOptionsRequest) (*client.QueryWorkflowWithOptionsResponse, error)
	SignalWithStartWorkflow(
		ctx context.Context,
		workflowID string,
		signalName string,
		signalArg interface{},
		options client.StartWorkflowOptions,
		workflow interface{},
		args ...interface{},
	) (client.WorkflowRun, error)
	TerminateWorkflow(ctx context.Context, workflowID, runID, reason string, details ...interface{}) error
}

func deviceRegistryStartWorkflowOptions(workflowID string) client.StartWorkflowOptions {
	// These internal singleton workflows always run on the shared
	// device-registry queue, which is intentionally unversioned even when the
	// operational server/agent queues use worker deployments.
	return client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: models.TemporalDeviceRegistryTaskQueue,
	}
}

func registryWorkflowUsesVersioning(description *workflowservice.DescribeWorkflowExecutionResponse) bool {
	if description == nil {
		return false
	}

	info := description.GetWorkflowExecutionInfo()
	if info == nil {
		return false
	}

	if strings.TrimSpace(info.GetAssignedBuildId()) != "" || strings.TrimSpace(info.GetInheritedBuildId()) != "" {
		return true
	}

	versioningInfo := info.GetVersioningInfo()
	if versioningInfo == nil {
		return false
	}

	if versioningInfo.GetBehavior() != enums.VERSIONING_BEHAVIOR_UNSPECIFIED {
		return true
	}

	return versioningInfo.GetVersioningOverride() != nil
}

func normalizeDeviceDefinition(device models.Device) models.Device {
	device.ID = strings.TrimSpace(device.ID)
	device.Name = strings.TrimSpace(device.Name)
	device.Description = strings.TrimSpace(device.Description)
	device.Platform = strings.TrimSpace(device.Platform)
	if device.LocalElevation != nil {
		policy := *device.LocalElevation
		policy.AllowedModes = slices.Clone(policy.AllowedModes)
		policy.DeniedUsernames = slices.Clone(policy.DeniedUsernames)
		policy.AllowedUIDRanges = slices.Clone(policy.AllowedUIDRanges)
		if len(policy.Accounts) > 0 {
			policy.Accounts = append([]models.DeviceLocalElevationAccount(nil), policy.Accounts...)
			for i := range policy.Accounts {
				policy.Accounts[i].Identity = strings.TrimSpace(policy.Accounts[i].Identity)
				policy.Accounts[i].Email = strings.TrimSpace(policy.Accounts[i].Email)
				policy.Accounts[i].Username = strings.TrimSpace(policy.Accounts[i].Username)
				policy.Accounts[i].LocalUsername = strings.TrimSpace(policy.Accounts[i].LocalUsername)
			}
		}
		for i := range policy.AllowedModes {
			policy.AllowedModes[i] = strings.TrimSpace(policy.AllowedModes[i])
		}
		for i := range policy.DeniedUsernames {
			policy.DeniedUsernames[i] = strings.TrimSpace(policy.DeniedUsernames[i])
		}
		for i := range policy.AllowedUIDRanges {
			policy.AllowedUIDRanges[i] = strings.TrimSpace(policy.AllowedUIDRanges[i])
		}
		device.LocalElevation = &policy
	}
	return device
}

func deviceDefinitionsEqual(left, right models.Device) bool {
	l := normalizeDeviceDefinition(left)
	r := normalizeDeviceDefinition(right)

	if l.ID != r.ID ||
		l.Name != r.Name ||
		l.Description != r.Description ||
		l.Platform != r.Platform ||
		l.Enabled != r.Enabled {
		return false
	}

	if (l.LocalElevation == nil) != (r.LocalElevation == nil) {
		return false
	}
	if l.LocalElevation == nil {
		return true
	}

	ll := l.LocalElevation
	rl := r.LocalElevation
	if ll.Enabled != rl.Enabled {
		return false
	}

	if !slices.Equal(ll.AllowedModes, rl.AllowedModes) ||
		!slices.Equal(ll.DeniedUsernames, rl.DeniedUsernames) ||
		!slices.Equal(ll.AllowedUIDRanges, rl.AllowedUIDRanges) {
		return false
	}

	if len(ll.Accounts) != len(rl.Accounts) {
		return false
	}
	for i := range ll.Accounts {
		la := ll.Accounts[i]
		ra := rl.Accounts[i]
		if la.Identity != ra.Identity ||
			la.Email != ra.Email ||
			la.Username != ra.Username ||
			la.LocalUsername != ra.LocalUsername {
			return false
		}
	}

	return true
}

func queryDeviceDefinition(
	ctx context.Context,
	temporalClient deviceRegistryTemporalClient,
	deviceID string,
) (*models.Device, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}
	if temporalClient == nil {
		return nil, fmt.Errorf("shared device registry is unavailable")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, deviceRegistryQueryTimeout)
	defer cancel()

	queryResponse, err := temporalClient.QueryWorkflowWithOptions(timeoutCtx, &client.QueryWorkflowWithOptionsRequest{
		WorkflowID:           models.TemporalDeviceDefinitionRegistryWorkflowID,
		RunID:                "",
		QueryType:            models.TemporalGetDeviceDefinitionQueryName,
		QueryRejectCondition: enums.QUERY_REJECT_CONDITION_NOT_OPEN,
		Args:                 []any{deviceID},
	})
	if err != nil {
		return nil, fmt.Errorf("device %q is not configured", deviceID)
	}
	if queryResponse == nil || queryResponse.QueryResult == nil {
		return nil, fmt.Errorf("device %q is not configured", deviceID)
	}

	var device models.Device
	if err := queryResponse.QueryResult.Get(&device); err != nil {
		return nil, err
	}

	normalized := normalizeDeviceDefinition(device)
	return &normalized, nil
}

func ensureRegistryWorkflowTaskQueue(
	ctx context.Context,
	temporalClient deviceRegistryTemporalClient,
	workflowID string,
) error {
	if temporalClient == nil {
		return fmt.Errorf("temporal client is required to manage registry workflow %q", workflowID)
	}

	description, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		var notFound *serviceerror.NotFound
		if errors.As(err, &notFound) {
			return nil
		}
		return err
	}

	taskQueue := ""
	if description != nil && description.ExecutionConfig != nil && description.ExecutionConfig.TaskQueue != nil {
		taskQueue = strings.TrimSpace(description.ExecutionConfig.TaskQueue.Name)
	}
	if taskQueue == "" || taskQueue == models.TemporalDeviceRegistryTaskQueue {
		if !registryWorkflowUsesVersioning(description) {
			return nil
		}

		logrus.WithFields(logrus.Fields{
			"workflow_id": workflowID,
			"task_queue":  models.TemporalDeviceRegistryTaskQueue,
		}).Warn("Recreating versioned device registry workflow on the canonical unversioned device registry queue")

		return temporalClient.TerminateWorkflow(ctx, workflowID, "", "migrating device registry workflow to canonical unversioned queue")
	}

	logrus.WithFields(logrus.Fields{
		"workflow_id": workflowID,
		"task_queue":  taskQueue,
		"expected":    models.TemporalDeviceRegistryTaskQueue,
	}).Warn("Recreating device registry workflow on the canonical device registry queue")

	return temporalClient.TerminateWorkflow(ctx, workflowID, "", "migrating device registry workflow to canonical task queue")
}

func publishDeviceDefinition(
	ctx context.Context,
	temporalClient deviceRegistryTemporalClient,
	device models.Device,
) error {
	if temporalClient == nil {
		return fmt.Errorf("shared device registry is unavailable")
	}

	device = normalizeDeviceDefinition(device)
	if device.ID == "" {
		return nil
	}

	_, err := temporalClient.SignalWithStartWorkflow(
		ctx,
		models.TemporalDeviceDefinitionRegistryWorkflowID,
		models.TemporalDeviceDefinitionUpsertSignalName,
		device,
		deviceRegistryStartWorkflowOptions(models.TemporalDeviceDefinitionRegistryWorkflowID),
		models.TemporalDeviceDefinitionRegistryWorkflowName,
	)
	return err
}

func (c *Config) querySharedDeviceDefinition(ctx context.Context, deviceID string) (*models.Device, error) {
	services := c.GetServices()
	if services == nil || !services.HasTemporal() {
		return nil, fmt.Errorf("shared device registry is unavailable")
	}

	temporalService := services.GetTemporal()
	if temporalService == nil || !temporalService.HasClient() {
		return nil, fmt.Errorf("shared device registry is unavailable")
	}

	return queryDeviceDefinition(ctx, temporalService.GetClient(), deviceID)
}

func (c *Config) EnsureDeviceRegistryWorkflows(ctx context.Context) error {
	if !c.IsServer() {
		return nil
	}

	services := c.GetServices()
	if services == nil || !services.HasTemporal() {
		return fmt.Errorf("temporal service is required to manage device registries")
	}

	temporalService := services.GetTemporal()
	if temporalService == nil || !temporalService.HasClient() {
		return fmt.Errorf("temporal client is required to manage device registries")
	}

	for _, workflowID := range []string{
		models.TemporalDeviceRouteRegistryWorkflowID,
		models.TemporalDeviceDefinitionRegistryWorkflowID,
	} {
		if err := ensureRegistryWorkflowTaskQueue(ctx, temporalService.GetClient(), workflowID); err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) PublishConfiguredDeviceDefinitions(ctx context.Context) error {
	if !c.IsServer() {
		return nil
	}

	services := c.GetServices()
	if services == nil || !services.HasTemporal() {
		return fmt.Errorf("temporal service is required to publish device definitions")
	}

	temporalService := services.GetTemporal()
	if temporalService == nil || !temporalService.HasClient() {
		return fmt.Errorf("temporal client is required to publish device definitions")
	}

	deviceIDs := make([]string, 0, len(c.Devices.Definitions))
	for _, device := range c.Devices.Definitions {
		deviceID := strings.TrimSpace(device.ID)
		if deviceID == "" {
			continue
		}
		deviceIDs = append(deviceIDs, deviceID)
	}
	slices.Sort(deviceIDs)

	for _, deviceID := range deviceIDs {
		device, err := c.GetDevice(deviceID)
		if err != nil {
			return err
		}
		if device == nil {
			continue
		}
		if err := publishDeviceDefinition(ctx, temporalService.GetClient(), *device); err != nil {
			return err
		}
	}

	return nil
}
