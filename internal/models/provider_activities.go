package models

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

// RegisterProviderActivities is the single entry point for activity registration.
// It gates generic Synchronize* activities on provider capability, then delegates
//
//	func (b *myProvider) RegisterActivities(c models.TemporalImpl) error {
//	    return models.RegisterProviderActivities(c, b)
//	}
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

func (a *ProviderActivities) SynchronizeTenants(
	ctx context.Context,
	req *SynchronizeTenantsRequest,
) (*SynchronizeTenantsResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizeTenants activity")

	result, err := handleNotImplementedError(a.provider.SynchronizeTenants(ctx, req))

	if err == nil {
		a.provider.AddTenants(result.Tenants...)
	}

	return result, err
}

func (a *ProviderActivities) SynchronizeIdentities(
	ctx context.Context,
	req *SynchronizeIdentitiesRequest,
) (*SynchronizeIdentitiesResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizeIdentities activity")

	result, err := handleNotImplementedError(a.provider.SynchronizeIdentities(ctx, req))

	if err == nil {
		a.provider.AddIdentities(result.Identities...)
	}

	return result, err
}

func (a *ProviderActivities) SynchronizeResources(
	ctx context.Context,
	req *SynchronizeResourcesRequest,
) (*SynchronizeResourcesResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizeResources activity")

	result, err := handleNotImplementedError(a.provider.SynchronizeResources(ctx, req))

	if err == nil {
		a.provider.AddResources(result.Resources...)
	}

	return result, err
}

// SynchronizeUsers fetches users from the provider and adds them to the provider's identity store
// This must be called as a local activity to ensure that the provider's identity store is updated
// within the same process context
func (a *ProviderActivities) SynchronizeUsers(
	ctx context.Context,
	req *SynchronizeUsersRequest,
) (*SynchronizeUsersResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizeUsers activity")

	result, err := handleNotImplementedError(a.provider.SynchronizeUsers(ctx, req))

	if err == nil {
		a.provider.AddIdentities(result.Identities...)
	}

	return result, err

}

// SynchronizeGroups fetches groups from the provider and adds them to the provider's identity store
// This must be called as a local activity to ensure that the provider's identity store is updated
// within the same process context
func (a *ProviderActivities) SynchronizeGroups(
	ctx context.Context,
	req *SynchronizeGroupsRequest,
) (*SynchronizeGroupsResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizeGroups activity")

	result, err := handleNotImplementedError(a.provider.SynchronizeGroups(ctx, req))

	if err == nil {
		a.provider.AddIdentities(result.Identities...)
	}

	return result, err
}

// SynchronizePermissions fetches permissions from the provider and adds them to the provider's permission store
// This must be called as a local activity to ensure that the provider's permission store is updated
// within the same process context
func (a *ProviderActivities) SynchronizePermissions(
	ctx context.Context,
	req *SynchronizePermissionsRequest,
) (*SynchronizePermissionsResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizePermissions activity")

	result, err := handleNotImplementedError(a.provider.SynchronizePermissions(ctx, req))

	if err == nil {
		a.provider.AddPermissions(result.Permissions...)
	}

	return result, err
}

// SynchronizeRoles fetches roles from the provider and adds them to the provider's role store
// This must be called as a local activity to ensure that the provider's role store is updated
// within the same process context
func (a *ProviderActivities) SynchronizeRoles(
	ctx context.Context,
	req *SynchronizeRolesRequest,
) (*SynchronizeRolesResponse, error) {

	logrus.WithFields(logrus.Fields{
		"pagination": req.Pagination,
	}).Infoln("Starting SynchronizeRoles activity")

	result, err := handleNotImplementedError(a.provider.SynchronizeRoles(ctx, req))

	if err == nil {
		a.provider.AddRoles(result.Roles...)
	}

	return result, err
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
