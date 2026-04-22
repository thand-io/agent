package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

type thandActivities struct {
	config                 *Config
	lookupDeviceDefinition func(ctx context.Context, deviceID string) (*models.Device, error)
}

// PatchProviderUpstreamDummy is a no-op activity for thand server/agents that are not
// configured to use the Thand service
func (t *thandActivities) PatchProviderUpstreamDummy(
	ctx context.Context,
	activityMethod models.SynchronizeCapability,
	providerIdentifier string,
	resp any,
) error {
	return nil
}

// PatchProviderUpstream patches the provider's upstream endpoint in the Thand server
// This sends updates for users, groups, roles, permissions, resources, etc.
// So that Thand can paginate through the data when the provider is synchronized
func (t *thandActivities) PatchProviderUpstream(
	ctx context.Context,
	activityMethod models.SynchronizeCapability,
	providerIdentifier string,
	resp any,
) error {

	c := t.config

	log := activity.GetLogger(ctx)

	if !c.HasThandService() {

		log.Warn("Thand service is not configured; skipping PatchProviderUpstream activity")

		return temporal.NewNonRetryableApplicationError(
			"Thand service is not configured",
			"ThandServiceNotConfigured",
			nil,
		)
	}

	baseUrl := c.DiscoverThandServerApiUrl()
	providerSyncUrl := fmt.Sprintf("%s/providers/%s/sync",
		strings.TrimSuffix(baseUrl, "/"),
		strings.ToLower(providerIdentifier),
	)

	upstream := &model.Endpoint{
		EndpointConfig: &model.EndpointConfiguration{
			URI: &model.LiteralUri{Value: providerSyncUrl},
			Authentication: &model.ReferenceableAuthenticationPolicy{
				AuthenticationPolicy: &model.AuthenticationPolicy{
					Bearer: &model.BearerAuthenticationPolicy{
						Token: c.Thand.ApiKey,
					},
				},
			},
		},
	}

	// Make patch request
	err := models.PatchProviderUpstream(
		activityMethod,
		upstream,
		resp,
	)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to send pagination patch to server")
	}

	return err

}

func (t *thandActivities) ResolveFreshDeviceRoute(
	ctx context.Context,
	deviceID string,
) (*models.DeviceConnectionState, error) {
	route, err := t.queryFreshDeviceRoute(ctx, deviceID)
	if err == nil {
		return route, nil
	}
	if errors.Is(err, ErrDeviceRouteUnavailable) {
		return nil, temporal.NewNonRetryableApplicationError(
			err.Error(),
			"DeviceRouteUnavailable",
			err,
		)
	}
	return nil, err
}

func (t *thandActivities) BuildExecutionPlan(
	ctx context.Context,
	req models.ExecutionPlanRequest,
) (*models.ExecutionPlan, error) {
	if req.ElevateRequest == nil {
		return nil, temporal.NewNonRetryableApplicationError(
			"elevate request is required for execution planning",
			"ExecutionPlanInvalid",
			nil,
		)
	}

	plan, err := BuildExecutionPlanWithOptions(t.config, req.WorkflowID, req.ElevateRequest, executionPlanBuildOptions{
		LookupDeviceDefinition: func(deviceID string) (*models.Device, error) {
			if t.lookupDeviceDefinition != nil {
				return t.lookupDeviceDefinition(ctx, deviceID)
			}
			return t.config.querySharedDeviceDefinition(ctx, deviceID)
		},
	})
	if err == nil {
		return plan, nil
	}

	return nil, temporal.NewNonRetryableApplicationError(
		err.Error(),
		"ExecutionPlanInvalid",
		err,
	)
}

func (t *thandActivities) queryFreshDeviceRoute(
	ctx context.Context,
	deviceID string,
) (*models.DeviceConnectionState, error) {
	services := t.config.GetServices()
	if services == nil || !services.HasTemporal() {
		return t.config.GetFreshDeviceRoute(deviceID)
	}

	temporalService := services.GetTemporal()
	if temporalService == nil || !temporalService.HasClient() {
		return t.config.GetFreshDeviceRoute(deviceID)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, deviceRouteRefreshInterval)
	defer cancel()

	queryResponse, err := temporalService.GetClient().QueryWorkflowWithOptions(timeoutCtx, &client.QueryWorkflowWithOptionsRequest{
		WorkflowID:           models.TemporalDeviceRouteRegistryWorkflowID,
		RunID:                "",
		QueryType:            models.TemporalGetDeviceRouteQueryName,
		QueryRejectCondition: enums.QUERY_REJECT_CONDITION_NOT_OPEN,
		Args:                 []any{deviceID},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: device %q is not connected", ErrDeviceRouteUnavailable, strings.TrimSpace(deviceID))
	}
	if queryResponse == nil || queryResponse.QueryResult == nil {
		return nil, fmt.Errorf("%w: device %q is not connected", ErrDeviceRouteUnavailable, strings.TrimSpace(deviceID))
	}

	var route models.DeviceConnectionState
	if err := queryResponse.QueryResult.Get(&route); err != nil {
		return nil, err
	}
	if !route.Connected || strings.TrimSpace(route.TaskQueue) == "" {
		return nil, fmt.Errorf("%w: device %q is not connected", ErrDeviceRouteUnavailable, strings.TrimSpace(deviceID))
	}

	return &route, nil
}
