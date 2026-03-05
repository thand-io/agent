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
// registered local activity and an onPage hook that fires the upstream patch
// activity asynchronously.
func runSyncLoop[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx workflow.Context,
	provider Provider,
	providerID string,
	activityMethod SynchronizeCapability,
	req Req,
) error {

	log := workflow.GetLogger(ctx)
	log.Info("Starting synchronization loop", "provider", providerID, "activity", activityMethod)

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

	err := paginatedSync(provider, activityMethod, req,
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
			log.Error("Error patching synchronization results upstream",
				"provider", providerID, "error", err)
		}
	}

	log.Info("Completed synchronization", "provider", providerID)

	return nil
}

// CreateProviderSynchronizeWorkflow returns a workflow function that captures the
// live provider instance via closure. This allows runSyncLoop — and by extension
// paginatedSync — to call resp.AddToProvider(provider) from within the workflow,
// keeping the in-memory provider stores up-to-date during both normal execution
// and Temporal replay.
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

		log.Info("Determining which synchronization activities to run", "provider", syncReq.ProviderIdentifier, "requested_activities", syncReq.Requests)

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

// authorizeRoleRequestSideEffect is used to carry the result of
// CreateAuthorizeRoleRequest across a workflow.SideEffect boundary so that
// non-deterministic operations (config lookups, UUID generation) are isolated
// from workflow replay.
type authorizeRoleRequestSideEffect struct {
	Request *AuthorizeRoleRequest `json:"request"`
	Err     string                `json:"error"`
}

// CreateProviderAuthorizeRoleWorkflow returns a workflow function that captures the
// live provider instance via closure. The child workflow receives the Temporal
// workflow.Context, constructs a WorkflowTaskSupport with it, and delegates to
// provider.AuthorizeRole — allowing the provider to dispatch activities, use
// workflow.Go, and manage state just as it does in the primary workflow.
// Careful: The workflow function returned by this method will be executed as a Temporal workflow, so it must be deterministic and should not perform any non-deterministic operations (like generating random numbers or accessing the current time) directly in the workflow code. Any such operations should be performed within activities or isolated using workflow.SideEffect to ensure correct behavior during workflow replay.
func CreateProviderAuthorizeRoleWorkflow(cfg ConfigImpl, provider Provider) func(workflow.Context, WorkflowRoleRequest) (*AuthorizeRoleResponse, error) {
	return func(ctx workflow.Context, req WorkflowRoleRequest) (*AuthorizeRoleResponse, error) {

		log := workflow.GetLogger(ctx)
		log.Info("Starting authorize role workflow", "provider", provider.GetIdentifier())

		// Wrap in a SideEffect so that the non-deterministic operations inside
		// CreateAuthorizeRoleRequest (config/identity/tenant lookups, UUID generation
		// for the composite role identifier) are executed only on the first run and
		// their result is recorded in the workflow event history. On replay, Temporal
		// replays the recorded value instead of re-executing the function, keeping
		// workflow execution deterministic.
		//
		// Note: CompositeRole has a custom UnmarshalJSON to ensure UUID, Composite,
		// and Providers fields survive JSON serialization through Temporal's data converter.
		encodedReq := workflow.SideEffect(ctx, func(ctx workflow.Context) any {
			result, err := CreateAuthorizeRoleRequest(cfg, provider, &req)
			if err != nil {
				return authorizeRoleRequestSideEffect{Err: err.Error()}
			}
			return authorizeRoleRequestSideEffect{Request: result}
		})

		var se authorizeRoleRequestSideEffect
		if err := encodedReq.Get(&se); err != nil {
			log.Error("Failed to decode authorize role request side effect", "error", err)
			return nil, err
		}
		if se.Err != "" {
			log.Error("Failed to create authorize role request", "error", se.Err)
			return nil, fmt.Errorf("%s", se.Err)
		}

		log.Info("Constructed authorize role request, invoking provider",
			"provider", provider.GetIdentifier(),
			"authorizeReq", se.Request,
		)

		return provider.AuthorizeRole(ctx, se.Request)
	}
}

type WorkflowRevokeRoleRequest struct {
	RevokeRoleRequest     *WorkflowRoleRequest
	AuthorizeRoleResponse *AuthorizeRoleResponse
}

// CreateProviderRevokeRoleWorkflow returns a workflow function that captures the
// live provider instance via closure for revocation operations.
// Careful: The revoke workflow may need to reconstruct the original AuthorizeRoleRequest
// and must handle it deterministically within a workflow.SideEffect.
func CreateProviderRevokeRoleWorkflow(cfg ConfigImpl, provider Provider) func(workflow.Context, WorkflowRevokeRoleRequest) (*RevokeRoleResponse, error) {
	return func(ctx workflow.Context, req WorkflowRevokeRoleRequest) (*RevokeRoleResponse, error) {

		log := workflow.GetLogger(ctx)
		log.Info("Starting revoke role workflow", "provider", provider.GetIdentifier())

		var authReq *AuthorizeRoleRequest
		if req.RevokeRoleRequest != nil {
			// Same reasoning as in CreateProviderAuthorizeRoleWorkflow: wrap in a
			// SideEffect to isolate non-deterministic config lookups and UUID
			// generation from replay.
			encodedReq := workflow.SideEffect(ctx, func(ctx workflow.Context) any {
				result, err := CreateAuthorizeRoleRequest(cfg, provider, req.RevokeRoleRequest)
				if err != nil {
					return authorizeRoleRequestSideEffect{Err: err.Error()}
				}
				return authorizeRoleRequestSideEffect{Request: result}
			})

			var se authorizeRoleRequestSideEffect
			if err := encodedReq.Get(&se); err != nil {
				log.Error("Failed to decode revoke role request side effect", "error", err)
				return nil, err
			}
			if se.Err != "" {
				log.Error("Failed to create revoke role request", "error", se.Err)
				return nil, fmt.Errorf("%s", se.Err)
			}
			authReq = se.Request
		}

		revokeReq := &RevokeRoleRequest{
			AuthorizeRoleRequest:  authReq,
			AuthorizeRoleResponse: req.AuthorizeRoleResponse,
		}

		log.Info("Constructed revoke role request, invoking provider",
			"provider", provider.GetIdentifier(),
			"revokeReq", revokeReq,
		)

		return provider.RevokeRole(ctx, revokeReq)
	}
}

// CreateTemporalProviderWorkflowName creates a standardized workflow name for provider operations by combining the provider identifier and operation name. This ensures consistent naming across all provider workflows, making them easier to identify and manage in Temporal.
// Careful: The resulting workflow name must be deterministic and should not include any non-deterministic data (like timestamps or random values) to ensure it can be reliably used in workflow execution and querying.
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
