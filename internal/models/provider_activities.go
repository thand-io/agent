package models

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
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
//	func (p *myProvider) RegisterActivities(runtime sdkConstants.Mode) any {
//	    return &myProviderActivities{provider: p}
//	}
//
// The returned value is passed to models.RegisterActivities, which registers it
// with the Temporal worker under the provider's identifier namespace.
func RegisterProviderActivities(temporalClient TemporalImpl, provider Provider, cfg ConfigImpl) error {
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

	if provider.HasCapability(ProviderCapabilityProvisioning) && cfg != nil {
		prov := NewProvisioningActivities(cfg, provider)
		buildActivityName := CreateTemporalProviderWorkflowName(identifier, TemporalBuildAuthorizeRoleRequestActivityName)
		worker.RegisterActivityWithOptions(prov.BuildAuthorizeRoleRequest, activity.RegisterOptions{Name: buildActivityName})
		logrus.Debugf("Registered provisioning activity: %s for provider: %s", buildActivityName, identifier)
	}

	return nil
}

// RegisterActivities — BaseProvider default; returns ErrNotImplemented.
func (b *BaseProvider) RegisterActivities(runtime sdkConstants.Mode) any {
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
// provider's sync method, applies results to the in-memory provider stores via
// AddToProvider, and wraps ErrNotImplemented into a non-retryable Temporal error.
//
// Performing the AddToProvider mutation here — inside a local activity — keeps
// the provider store updates out of the Temporal workflow goroutine, avoiding
// non-deterministic operations (sync.Mutex, background goroutines for index
// building) that would break workflow replay.
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
		"pagination", req.GetPagination())

	result, err := handleNotImplementedError(syncFunc(ctx, req))
	if err != nil {
		log.Error("Error in activity for provider: "+provider.GetIdentifier(),
			"activity", activityName,
			"error", err)
		return result, err
	}

	// Apply results to the provider's in-memory stores inside the activity
	// context where sync.Mutex and background index goroutines are safe.
	result.AddToProvider(provider)

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

// ProvisioningActivities holds the dependencies required to resolve a
// WorkflowRoleRequest into a fully-formed AuthorizeRoleRequest. The struct is
// instantiated once per provider at worker registration time and its method
// name is stable across deployments, satisfying Temporal's replay requirements.
type ProvisioningActivities struct {
	cfg      ConfigImpl
	provider Provider
}

func NewProvisioningActivities(cfg ConfigImpl, provider Provider) *ProvisioningActivities {
	return &ProvisioningActivities{cfg: cfg, provider: provider}
}

// BuildAuthorizeRoleRequest is a Temporal local activity that resolves
// config/provider state (identity lookup, tenant lookup, composite-role
// construction including UUID derivation) into a fully-formed
// AuthorizeRoleRequest. Running this as an activity rather than
// workflow.SideEffect keeps mutable config reads outside workflow code,
// records the resolved request in history with an explicit activity type,
// and enables retry on transient failure.
func (a *ProvisioningActivities) BuildAuthorizeRoleRequest(
	ctx context.Context,
	req *WorkflowRoleRequest,
) (*AuthorizeRoleRequest, error) {
	return CreateAuthorizeRoleRequest(a.cfg, a.provider, req)
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
