package models

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// RegisterProviderActivities is the single entry point for activity registration.
// It gates generic Synchronize* activities on provider capability and is called
// automatically by config/providers.go — implementers do not call it directly.
//
// To expose additional, provider-specific activities, override RegisterActivities
// on your provider struct to return a populated activities struct (or nil to skip):
//
//	func (p *myProvider) RegisterActivities() any {
//	    return &myProviderActivities{provider: p}
//	}
//
// The returned value is passed to models.RegisterActivities, which registers it
// with the Temporal worker under the provider's identifier namespace.
func RegisterProviderActivities(temporalClient TemporalImpl, provider Provider) error {
	if temporalClient == nil {
		return ErrNotImplemented
	}
	worker := temporalClient.GetWorker()
	if worker == nil {
		return ErrNotImplemented
	}

	identifier := provider.GetIdentifier()
	pa := NewProviderActivities(provider)

	type capabilityActivity struct {
		capability ProviderCapability
		fn         any
		name       string
	}
	for _, ca := range []capabilityActivity{
		{ProviderCapabilityTenants, pa.SynchronizeTenants, "SynchronizeTenants"},
		{ProviderCapabilityIdentities, pa.SynchronizeIdentities, "SynchronizeIdentities"},
		{ProviderCapabilityResources, pa.SynchronizeResources, "SynchronizeResources"},
		{ProviderCapabilityUsers, pa.SynchronizeUsers, "SynchronizeUsers"},
		{ProviderCapabilityGroups, pa.SynchronizeGroups, "SynchronizeGroups"},
		{ProviderCapabilityPermissions, pa.SynchronizePermissions, "SynchronizePermissions"},
		{ProviderCapabilityRoles, pa.SynchronizeRoles, "SynchronizeRoles"},
	} {
		if !provider.HasCapability(ca.capability) {
			logrus.Debugf("Skipping activity %s for provider %s (capability %s not enabled)", ca.name, identifier, ca.capability)
			continue
		}
		activityName := CreateTemporalProviderWorkflowName(identifier, ca.name)
		worker.RegisterActivityWithOptions(ca.fn, activity.RegisterOptions{Name: activityName})
		logrus.Debugf("Registered activity: %s for provider: %s", activityName, identifier)
	}

	return nil
}

// RegisterActivities — BaseProvider default; returns ErrNotImplemented.
func (b *BaseProvider) RegisterActivities() any {
	return nil
}

type ProviderActivities struct {
	provider Provider
}

func NewProviderActivities(provider Provider) *ProviderActivities {
	return &ProviderActivities{
		provider: provider,
	}
}

// runProviderActivity is the generic Temporal activity helper. It calls the
// provider's sync method and wraps ErrNotImplemented into a non-retryable
// Temporal error. The caller (paginatedSync, via runSyncLoop) is responsible
// for calling resp.AddToProvider.
func runProviderActivity[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx context.Context,
	provider Provider,
	activityName string,
	req Req,
	syncFunc func(context.Context, Req) (Resp, error),
) (Resp, error) {

	log := activity.GetLogger(ctx)

	log.Info("Starting activity for provider: "+provider.GetIdentifier(),
		"activity", activityName,
		"pagination", req)

	result, err := handleNotImplementedError(syncFunc(ctx, req))
	if err != nil {
		log.Error("Error in activity for provider: "+provider.GetIdentifier(),
			"activity", activityName,
			"error", err)
		return result, err
	}

	return result, nil
}

func (a *ProviderActivities) SynchronizeTenants(
	ctx context.Context,
	req *SynchronizeTenantsRequest,
) (*SynchronizeTenantsResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizeTenants), req, a.provider.SynchronizeTenants)
}

func (a *ProviderActivities) SynchronizeIdentities(
	ctx context.Context,
	req *SynchronizeIdentitiesRequest,
) (*SynchronizeIdentitiesResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizeIdentities), req, a.provider.SynchronizeIdentities)
}

func (a *ProviderActivities) SynchronizeResources(
	ctx context.Context,
	req *SynchronizeResourcesRequest,
) (*SynchronizeResourcesResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizeResources), req, a.provider.SynchronizeResources)
}

func (a *ProviderActivities) SynchronizeUsers(
	ctx context.Context,
	req *SynchronizeUsersRequest,
) (*SynchronizeUsersResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizeUsers), req, a.provider.SynchronizeUsers)
}

func (a *ProviderActivities) SynchronizeGroups(
	ctx context.Context,
	req *SynchronizeGroupsRequest,
) (*SynchronizeGroupsResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizeGroups), req, a.provider.SynchronizeGroups)
}

func (a *ProviderActivities) SynchronizePermissions(
	ctx context.Context,
	req *SynchronizePermissionsRequest,
) (*SynchronizePermissionsResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizePermissions), req, a.provider.SynchronizePermissions)
}

func (a *ProviderActivities) SynchronizeRoles(
	ctx context.Context,
	req *SynchronizeRolesRequest,
) (*SynchronizeRolesResponse, error) {
	return runProviderActivity(ctx, a.provider, string(SynchronizeRoles), req, a.provider.SynchronizeRoles)
}

func handleNotImplementedError[T any](res T, err error) (T, error) {
	if err != nil {
		if errors.Is(err, ErrNotImplemented) {
			return res, temporal.NewNonRetryableApplicationError(
				"activity not implemented for this provider",
				"NotImplementedError",
				err,
			)
		}
	}
	return res, err
}
