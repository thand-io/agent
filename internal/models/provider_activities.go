package models

import (
	"context"
	"errors"

	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/temporal"
)

// RegisterActivities registers provider-specific activities with the Temporal worker
func (b *BaseProvider) RegisterActivities(temporalClient TemporalImpl) error {
	return ErrNotImplemented
}

type ProviderActivities struct {
	provider ProviderImpl
}

func NewProviderActivities(provider ProviderImpl) *ProviderActivities {
	return &ProviderActivities{
		provider: provider,
	}
}

func (a *ProviderActivities) AuthorizeRole(
	ctx context.Context,
	req *AuthorizeRoleRequest,
) (*AuthorizeRoleResponse, error) {

	logrus.Infoln("Starting AuthorizeRole activity")
	return handleNotImplementedError(a.provider.AuthorizeRole(ctx, req))

}

func (a *ProviderActivities) RevokeRole(
	ctx context.Context,
	req *RevokeRoleRequest,
) (*RevokeRoleResponse, error) {

	logrus.Infoln("Starting RevokeRole activity")
	return handleNotImplementedError(a.provider.RevokeRole(ctx, req))

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
