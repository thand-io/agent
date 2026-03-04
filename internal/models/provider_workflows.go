package models

import (
	"crypto/sha256"
	"encoding/hex"
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

// RegisterWorkflows registers the provider's workflows with Temporal. Providers can override this method to register custom workflows, but by default it does nothing.
// Note: Providers should not register the primary workflows (synchronize, authorize role, revoke role) here as they are registered globally in the agent. This method is intended for additional custom workflows that a provider may want to define.
// Careful: This method is called during provider initialization and should not perform any operations that require a Temporal workflow context (like starting workflows or activities) or access non-deterministic data. It should only return the workflow function definitions.
func runSyncLoop[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx workflow.Context,
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

	ctx = workflow.WithLocalActivityOptions(ctx, ao)

	// patchFutures collects the upstream-patch activity futures so that patch
	// calls do not block page iteration — we drain them after all pages are
	// fetched.
	var patchFutures []workflow.Future

	for {

		log.Info("Executing synchronization activity", "provider", providerID, "activity", activityMethod)

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
			log.Error("Error executing synchronization activity", "provider", providerID, "error", err)
			return err
		}

		// Fire the patch activity without blocking; collect the future for
		// later resolution so pagination can proceed immediately.
		patchFutures = append(patchFutures, workflow.ExecuteLocalActivity(
			ctx,
			TemporalPatchProviderUpstreamActivityName,
			activityMethod,
			providerID,
			resp,
		))

		pagination := resp.GetPagination()

		if pagination == nil || len(pagination.Token) == 0 {
			break
		}

		req.SetPagination(pagination)

	}

	// Drain all patch futures now that every page has been fetched.
	for _, f := range patchFutures {
		if err := f.Get(ctx, nil); err != nil {
			log.Error("Error patching synchronization results upstream", "provider", providerID, "error", err)
			return err
		}
	}

	log.Info("Completed synchronization", "provider", providerID)

	return nil
}

// SynchronizeTenants is the default implementation for tenant synchronization; it returns ErrNotImplemented. Providers that support tenant synchronization should override this method with their implementation.
// Careful: This method may be called within a Temporal workflow, so it should avoid performing any non-deterministic operations (like generating random numbers or accessing the current time) directly in the method. Any such operations should be performed within activities to ensure correct behavior during workflow replay.
func ProviderSynchronizeWorkflow(
	ctx workflow.Context,
	syncReq SynchronizeRequest,
) (*SynchronizeResponse, error) {

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
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := runSyncLoop[*SynchronizeTenantsRequest, SynchronizeTenantsResponse](
				ctx,
				syncReq.ProviderIdentifier,
				SynchronizeTenants,
				&SynchronizeTenantsRequest{},
			)
			errChan.Send(ctx, err)
		})
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
		// Log errors but return what we have
		log.Error("Synchronization workflow encountered errors", "errors", errs)
		return nil, fmt.Errorf("synchronization failed: %v", errs)
	}

	log.Info("Completed synchronization workflow", "provider", syncReq.ProviderIdentifier)

	return &SynchronizeResponse{}, nil
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
