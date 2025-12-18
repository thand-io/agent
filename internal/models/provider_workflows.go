package models

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/thand-io/agent/internal/common"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type SynchronizeCapability string

const (
	SynchronizeRoles       SynchronizeCapability = "SynchronizeRoles"
	SynchronizePermissions SynchronizeCapability = "SynchronizePermissions"
	SynchronizeResources   SynchronizeCapability = "SynchronizeResources"
	SynchronizeIdentities  SynchronizeCapability = "SynchronizeIdentities"
	SynchronizeUsers       SynchronizeCapability = "SynchronizeUsers"
	SynchronizeGroups      SynchronizeCapability = "SynchronizeGroups"
)

type SynchronizeRequest struct {
	ProviderIdentifier string                  `json:"provider"` // Provider name
	Requests           []SynchronizeCapability `json:"requests,omitempty"`
}

type SynchronizeResponse struct {
	// Everything will be updated using local activities,
	// so we can just return an empty response for now
}

func CreateTemporalWorkflowIdentifier(workflowName string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s", common.GetClientIdentifier(), workflowName))
}

func runSyncLoop[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx workflow.Context,
	providerID string,
	activityMethod SynchronizeCapability,
	req Req,
) error {

	ao := workflow.LocalActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    100 * time.Second,
			MaximumAttempts:    10,
		},
	}

	ctx = workflow.WithLocalActivityOptions(ctx, ao)

	for {

		var resp Resp

		activityName := CreateTemporalProviderWorkflowName(
			providerID,
			string(activityMethod),
		)

		err := workflow.ExecuteLocalActivity(
			ctx,
			activityName,
			req,
		).Get(ctx, &resp)

		if err != nil {
			return err
		}

		err = workflow.ExecuteLocalActivity(
			ctx,
			TemporalPatchProviderUpstreamActivityName,
			activityMethod,
			providerID,
			resp,
		).Get(ctx, nil)

		if err != nil {
			return err
		}

		pagination := resp.GetPagination()

		if pagination == nil || len(pagination.Token) == 0 {
			break
		}

		req.SetPagination(pagination)

	}

	return nil
}

func ProviderSynchronizeWorkflow(ctx workflow.Context, syncReq SynchronizeRequest) (*SynchronizeResponse, error) {

	if len(syncReq.ProviderIdentifier) == 0 {
		return nil, fmt.Errorf("provider identifier is required")
	}

	log := workflow.GetLogger(ctx)
	log.Info("Starting synchronization workflow for provider: ",
		syncReq.ProviderIdentifier)

	errChan := workflow.NewChannel(ctx)
	syncCount := 0

	shouldSync := func(cap SynchronizeCapability) bool {
		if len(syncReq.Requests) == 0 {
			return true
		}
		return slices.Contains(syncReq.Requests, cap)
	}

	// Synchronize Identities
	if shouldSync(SynchronizeIdentities) {
		syncCount++
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizeIdentitiesRequest, SynchronizeIdentitiesResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizeIdentities,
				&SynchronizeIdentitiesRequest{},
			)
			errChan.Send(ctx, err)
		})
	}

	// Synchronize Users
	if shouldSync(SynchronizeUsers) {
		syncCount++
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizeUsersRequest, SynchronizeUsersResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizeUsers,
				&SynchronizeUsersRequest{},
			)
			errChan.Send(ctx, err)
		})
	}

	// Synchronize Groups
	if shouldSync(SynchronizeGroups) {
		syncCount++
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizeGroupsRequest, SynchronizeGroupsResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizeGroups,
				&SynchronizeGroupsRequest{},
			)
			errChan.Send(ctx, err)
		})
	}

	// Synchronize Resources
	if shouldSync(SynchronizeResources) {
		syncCount++
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizeResourcesRequest, SynchronizeResourcesResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizeResources,
				&SynchronizeResourcesRequest{},
			)
			errChan.Send(ctx, err)
		})
	}

	// Synchronize Roles
	if shouldSync(SynchronizeRoles) {
		syncCount++
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizeRolesRequest, SynchronizeRolesResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizeRoles,
				&SynchronizeRolesRequest{},
			)
			errChan.Send(ctx, err)
		})
	}

	// Synchronize Permissions
	if shouldSync(SynchronizePermissions) {
		syncCount++
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizePermissionsRequest, SynchronizePermissionsResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizePermissions,
				&SynchronizePermissionsRequest{},
			)
			errChan.Send(ctx, err)
		})
	}

	var errs []error
	for range syncCount {
		var err error
		errChan.Receive(ctx, &err)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		// Log errors but return what we have
		log.Error("Synchronization workflow encountered errors: ", errs)
		return nil, fmt.Errorf("synchronization failed: %v", errs)
	}

	return &SynchronizeResponse{}, nil
}
