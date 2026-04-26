package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
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
	SynchronizeTenants     SynchronizeCapability = "SynchronizeTenants"
)

type SynchronizeRequest struct {
	ProviderIdentifier string                  `json:"provider"` // Provider name
	Requests           []SynchronizeCapability `json:"requests,omitempty"`
}

type SynchronizeResponse struct {
	// Everything will be updated using local activities,
	// so we can just return an empty response for now
}

// BaseProvider provides a base implementation of the ProviderImpl interface
func (b *BaseProvider) RegisterWorkflows() any {
	return nil
}

func CreateTemporalWorkflowIdentifier(workflowName string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s", common.GetClientIdentifier(), workflowName))
}

// CreateChildWorkflowID generates a unique child workflow ID by hashing a composite
// identifier built from provider, role, identity, tenant, and parent workflow ID.
// This ensures uniqueness across different identities/tenants requesting the same role.
// Format: parentWorkflowID_operation_hash
func CreateChildWorkflowID(parentWorkflowID, operation, provider string, req *WorkflowRoleRequest) string {
	// Build composite identifier similar to CompositeRoleWorkflowIdentifier
	// but using the data available in WorkflowRoleRequest
	parts := []string{
		parentWorkflowID,
		provider,
	}

	if req.Role != nil {
		parts = append(parts, req.Role.Identifier)
		parts = append(parts, req.Role.GetVersionString())
		parts = append(parts, req.Role.Name)
	}

	parts = append(parts, req.Identity)

	if len(req.Tenant) > 0 {
		parts = append(parts, req.Tenant)
	}

	// Create composite string
	composite := strings.Join(parts, ":")

	// Hash to get a stable, short identifier
	hash := sha256.Sum256([]byte(composite))
	hashStr := hex.EncodeToString(hash[:])[:12] // Use first 12 chars (48 bits)

	return fmt.Sprintf("%s_%s_%s", parentWorkflowID, operation, hashStr)
}

// runSyncLoop runs a single synchronization capability inside a Temporal workflow
// goroutine. It delegates to paginatedSync, providing an executor that calls the
// registered local activity and an onPage hook that buffers patch payloads.
// Provider store mutations (AddToProvider) happen inside the local activity
// (runProviderActivity), not here, to keep the workflow deterministic.
func runSyncLoop[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx workflow.Context,
	provider Provider,
	providerID string,
	activityMethod SynchronizeCapability,
	req Req,
) error {

	log := workflow.GetLogger(ctx)
	log.Debug("Starting synchronization loop", "provider", providerID, "activity", activityMethod)

	ao := workflow.LocalActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    1 * time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    100 * time.Second,
			MaximumAttempts:    10,
		},
	}

	activityName := CreateTemporalProviderWorkflowName(providerID, string(activityMethod))
	actx := workflow.WithLocalActivityOptions(ctx, ao)

	// Buffer page responses during pagination so that no patch activities are
	// scheduled until every page has been fetched.  This guarantees the local
	// activity worker slots are fully dedicated to page-fetching activities
	// even when thousands of pages are produced.
	var patchPayloads []Resp

	err := paginatedSync(log, provider, activityMethod, req,
		// executePage: run the local activity and return the deserialized response.
		func(r Req) (Resp, error) {
			var resp Resp
			err := workflow.ExecuteLocalActivity(actx, activityName, r).Get(ctx, &resp)
			if err != nil {
				// Normalise Temporal's non-retryable wrapper back to ErrNotImplemented
				// so paginatedSync can handle it uniformly.
				var appErr *temporal.ApplicationError
				if errors.As(err, &appErr) && appErr.Type() == "NotImplementedError" {
					return resp, ErrNotImplemented
				}
			}
			return resp, err
		},
		// onPage: stash the response; patch activities are deferred until all
		// pages have been fetched so they never compete with executePage.
		func(resp Resp) {
			patchPayloads = append(patchPayloads, resp)
		},
	)
	if err != nil {
		log.Error("Error executing synchronization", "provider", providerID, "activity", activityMethod, "error", err)
		return err
	}

	// Schedule and drain all patch activities in the background so
	// runSyncLoop returns immediately without blocking the caller.
	for _, payload := range patchPayloads {
		if err := workflow.ExecuteLocalActivity(
			actx,
			TemporalPatchProviderUpstreamActivityName,
			activityMethod,
			providerID,
			payload,
		).Get(actx, nil); err != nil {
			log.Warn("Error patching synchronization results upstream",
				"provider", providerID, "error", err)
		}
	}

	log.Debug("Completed synchronization", "provider", providerID)

	return nil
}

// CreateProviderSynchronizeWorkflow returns a workflow function that captures the
// live provider instance via closure. Provider store mutations (AddToProvider)
// happen inside the local activities (runProviderActivity), not in the workflow
// goroutine, which keeps execution deterministic during Temporal replay.
func CreateProviderSynchronizeWorkflow(provider Provider) func(workflow.Context, SynchronizeRequest) (*SynchronizeResponse, error) {
	return func(ctx workflow.Context, syncReq SynchronizeRequest) (*SynchronizeResponse, error) {

		if len(syncReq.ProviderIdentifier) == 0 {
			return nil, fmt.Errorf("provider identifier is required")
		}

		log := workflow.GetLogger(ctx)
		log.Info("Starting synchronization workflow", "provider", syncReq.ProviderIdentifier)

		errChan := workflow.NewChannel(ctx)
		syncCount := 0

		shouldSync := func(cap SynchronizeCapability) bool {
			if len(syncReq.Requests) == 0 {
				return true
			}
			return slices.Contains(syncReq.Requests, cap)
		}

		if shouldSync(SynchronizeTenants) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizeTenantsRequest, SynchronizeTenantsResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizeTenants, &SynchronizeTenantsRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		if shouldSync(SynchronizeIdentities) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizeIdentitiesRequest, SynchronizeIdentitiesResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizeIdentities, &SynchronizeIdentitiesRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		if shouldSync(SynchronizeUsers) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizeUsersRequest, SynchronizeUsersResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizeUsers, &SynchronizeUsersRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		if shouldSync(SynchronizeGroups) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizeGroupsRequest, SynchronizeGroupsResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizeGroups, &SynchronizeGroupsRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		if shouldSync(SynchronizeResources) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizeResourcesRequest, SynchronizeResourcesResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizeResources, &SynchronizeResourcesRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		if shouldSync(SynchronizeRoles) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizeRolesRequest, SynchronizeRolesResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizeRoles, &SynchronizeRolesRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		if shouldSync(SynchronizePermissions) {
			syncCount++
			workflow.Go(ctx, func(syncCtx workflow.Context) {
				err := runSyncLoop[*SynchronizePermissionsRequest, SynchronizePermissionsResponse](
					syncCtx, provider, syncReq.ProviderIdentifier,
					SynchronizePermissions, &SynchronizePermissionsRequest{},
				)
				errChan.Send(syncCtx, err)
			})
		}

		log.Info("Launched synchronization activities", "provider", syncReq.ProviderIdentifier, "activity_count", syncCount)

		var errs []error
		for range syncCount {
			var err error
			errChan.Receive(ctx, &err)
			if err != nil {
				errs = append(errs, err)
			}
		}

		if len(errs) > 0 {
			log.Error("Synchronization workflow encountered errors", "errors", errs)
			return nil, fmt.Errorf("synchronization failed: %v", errs)
		}

		log.Info("Completed synchronization workflow", "provider", syncReq.ProviderIdentifier)

		return &SynchronizeResponse{}, nil
	}
}

type WorkflowRoleRequest struct {
	WorkflowID string         `json:"workflow_id"`      // ID of the workflow for which the role is being authorized
	Tenant     string         `json:"tenant,omitempty"` // Optional tenant ID for multi-account providers
	Identity   string         `json:"identity"`         // User or group identifier
	Role       *Role          `json:"role"`
	Duration   *time.Duration `json:"duration,omitempty"` // Optional duration for temporary access
}

// IsValid checks if any of the fields are nil
// if they are then it returns false
func (r *WorkflowRoleRequest) IsValid() bool {
	return r.WorkflowID != "" && r.Identity != "" && r.Role != nil
}

func (r *WorkflowRoleRequest) GetWorkflowID() string {
	return r.WorkflowID
}

func (r *WorkflowRoleRequest) GetIdentity() string {
	return r.Identity
}

func (r *WorkflowRoleRequest) GetRole() *Role {
	return r.Role
}

func (r *WorkflowRoleRequest) GetDuration() *time.Duration {
	return r.Duration
}

// CreateProviderAuthorizeRoleWorkflow returns a workflow function that captures the
// live provider instance via closure. The child workflow receives the Temporal
// workflow.Context, dispatches a local activity to resolve the AuthorizeRoleRequest
// (config/identity/tenant lookups and composite-role construction), and then
// delegates to provider.AuthorizeRole — allowing the provider to dispatch activities,
// use workflow.Go, and manage state just as it does in the primary workflow.
func CreateProviderAuthorizeRoleWorkflow(provider Provider) func(workflow.Context, WorkflowRoleRequest) (*AuthorizeRoleResponse, error) {
	return func(ctx workflow.Context, req WorkflowRoleRequest) (*AuthorizeRoleResponse, error) {

		log := workflow.GetLogger(ctx)
		log.Info("Starting authorize role workflow", "provider", provider.GetIdentifier())

		// Resolve config/provider state via a registered local activity rather than
		// workflow.SideEffect. This keeps mutable config reads outside workflow code,
		// records the resolved request in history with a stable activity type name,
		// and enables retry on transient failure.
		activityName := CreateTemporalProviderWorkflowName(
			provider.GetIdentifier(),
			TemporalBuildAuthorizeRoleRequestActivityName,
		)
		lao := workflow.LocalActivityOptions{
			StartToCloseTimeout: 30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				InitialInterval:    1 * time.Second,
				BackoffCoefficient: 2.0,
				MaximumInterval:    30 * time.Second,
				MaximumAttempts:    5,
			},
		}
		lctx := workflow.WithLocalActivityOptions(ctx, lao)

		var authReq AuthorizeRoleRequest
		if err := workflow.ExecuteLocalActivity(lctx, activityName, &req).Get(ctx, &authReq); err != nil {
			log.Error("Failed to build authorize role request", "error", err)
			return nil, err
		}

		log.Debug("Constructed authorize role request, invoking provider",
			"provider", provider.GetIdentifier(),
			"authorizeReq", authReq,
		)

		return provider.AuthorizeRole(ctx, &authReq)
	}
}

type WorkflowRevokeRoleRequest struct {
	RevokeRoleRequest     *WorkflowRoleRequest
	AuthorizeRoleResponse *AuthorizeRoleResponse
}

// CreateProviderRevokeRoleWorkflow returns a workflow function that captures the
// live provider instance via closure for revocation operations.
func CreateProviderRevokeRoleWorkflow(provider Provider) func(workflow.Context, WorkflowRevokeRoleRequest) (*RevokeRoleResponse, error) {
	return func(ctx workflow.Context, req WorkflowRevokeRoleRequest) (*RevokeRoleResponse, error) {

		log := workflow.GetLogger(ctx)
		log.Info("Starting revoke role workflow", "provider", provider.GetIdentifier())

		var authReq *AuthorizeRoleRequest
		if req.RevokeRoleRequest != nil {
			// Resolve config/provider state via a registered local activity. See
			// CreateProviderAuthorizeRoleWorkflow for the full rationale.
			activityName := CreateTemporalProviderWorkflowName(
				provider.GetIdentifier(),
				TemporalBuildAuthorizeRoleRequestActivityName,
			)
			lao := workflow.LocalActivityOptions{
				StartToCloseTimeout: 30 * time.Second,
				RetryPolicy: &temporal.RetryPolicy{
					InitialInterval:    1 * time.Second,
					BackoffCoefficient: 2.0,
					MaximumInterval:    30 * time.Second,
					MaximumAttempts:    5,
				},
			}
			lctx := workflow.WithLocalActivityOptions(ctx, lao)

			var result AuthorizeRoleRequest
			if err := workflow.ExecuteLocalActivity(lctx, activityName, req.RevokeRoleRequest).Get(ctx, &result); err != nil {
				log.Error("Failed to build authorize role request for revocation", "error", err)
				return nil, err
			}
			authReq = &result
		}

		revokeReq := &RevokeRoleRequest{
			AuthorizeRoleRequest:  authReq,
			AuthorizeRoleResponse: req.AuthorizeRoleResponse,
		}

		log.Debug("Constructed revoke role request, invoking provider",
			"provider", provider.GetIdentifier(),
			"revokeReq", revokeReq,
		)

		return provider.RevokeRole(ctx, revokeReq)
	}
}

// CreateAuthorizeRoleRequest builds an AuthorizeRoleRequest from a WorkflowRoleRequest by resolving identity and tenant information, validating the requested role, and materializing any composite role configuration for the given provider.
// Careful: This helper wraps errors from identity, tenant, and role resolution with additional context to make diagnosing workflow failures easier.
func CreateAuthorizeRoleRequest(
	cfg ConfigImpl,
	provider Provider,
	req *WorkflowRoleRequest,
) (*AuthorizeRoleRequest, error) {

	// Get the user identity from the request
	identity, err := cfg.GetIdentity(req.Identity)
	if err != nil {
		identity = &Identity{
			ID: req.Identity,
			User: &User{
				Email: req.Identity,
			},
		}
	}

	var tenant *ProviderTenant
	if len(req.Tenant) != 0 {
		tenant, err = cfg.GetTenant(req.Tenant)
		if err != nil {
			tenant = &ProviderTenant{
				ID: req.Tenant,
			}
		}
	}

	// Validate the role
	_, err = validateRoleAndBuildOutput(provider, ElevateRequestInternal{
		User:         identity.User,
		AuthorizedAt: &time.Time{},
		ElevateRequest: ElevateRequest{
			Role: req.GetRole(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("role validation failed: %w", err)
	}

	compositeRole, err := cfg.GetCompositeRoleForWorkflow(
		identity,
		req.GetRole(),
		req.GetWorkflowID(),
		provider,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get composite role for identity %s: %w", req.Identity, err)
	}

	return &AuthorizeRoleRequest{
		Identity: identity,
		Tenant:   tenant,
		Role:     compositeRole,
		Duration: req.Duration,
	}, nil
}

// validateRoleAndBuildOutput validates the role and builds the initial model output
// Careful: This function is called within a workflow.SideEffect, so it must be deterministic and cannot perform any Temporal operations (activities, child workflows, timers) or access non-deterministic data (current time, random numbers). It can only use the data passed in the parameters and perform pure computations.
func validateRoleAndBuildOutput(
	provider Provider,
	elevateRequest ElevateRequestInternal,
) (map[string]any, error) {

	modelOutput := map[string]any{}

	validateOut, err := ValidateRole(provider, elevateRequest)
	if err != nil {
		return nil, err
	}

	if len(validateOut) > 0 {
		maps.Copy(modelOutput, validateOut)
	}

	return modelOutput, nil
}
