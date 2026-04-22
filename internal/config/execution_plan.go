package config

import (
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
)

type executionPlanBuildOptions struct {
	LookupDeviceDefinition func(deviceID string) (*models.Device, error)
	Decorators             []executionPlanDecorator
}

// executionPlanDecorator lets device-local or provider-specific request shaping
// stay close to the feature that needs it instead of branching inside the
// Temporal activity that drives planning.
type executionPlanDecorator interface {
	Applies(elevateRequest *models.ElevateRequestInternal) bool
	// Decorate runs before EntryID creation and provider request materialization.
	// Use it to populate request metadata and routing fields that should
	// contribute to the stable execution-plan entry identity.
	Decorate(
		cfg models.ConfigImpl,
		req *models.WorkflowRoleRequest,
		elevateRequest *models.ElevateRequestInternal,
		opts executionPlanBuildOptions,
	) error
	// Finalize runs after EntryID creation. Use it for metadata that must depend
	// on the stable entry identity itself, such as broker grant IDs, without
	// feeding that generated value back into the EntryID calculation.
	Finalize(
		req *models.WorkflowRoleRequest,
		elevateRequest *models.ElevateRequestInternal,
		entryID string,
	) error
}

func BuildExecutionPlan(
	cfg models.ConfigImpl,
	workflowID string,
	elevateRequest *models.ElevateRequestInternal,
) (*models.ExecutionPlan, error) {
	return BuildExecutionPlanWithOptions(cfg, workflowID, elevateRequest, executionPlanBuildOptions{})
}

func BuildExecutionPlanWithOptions(
	cfg models.ConfigImpl,
	workflowID string,
	elevateRequest *models.ElevateRequestInternal,
	opts executionPlanBuildOptions,
) (*models.ExecutionPlan, error) {
	if elevateRequest == nil {
		return nil, fmt.Errorf("elevate request is required for execution planning")
	}
	if len(elevateRequest.Providers) == 0 {
		return nil, fmt.Errorf("no providers specified for authorization")
	}
	if len(elevateRequest.Identities) == 0 {
		return nil, fmt.Errorf("no identities specified for authorization")
	}

	opts = opts.withDefaults(cfg)

	duration, err := elevateRequest.AsDuration()
	if err != nil {
		return nil, fmt.Errorf("failed to get duration: %w", err)
	}

	workflowName := strings.TrimSpace(elevateRequest.GetWorkflow())
	if workflowName == "" {
		return nil, fmt.Errorf("workflow name is required for execution planning")
	}

	tenants := elevateRequest.Tenants
	if len(tenants) == 0 {
		tenants = []string{""}
	}

	plan := &models.ExecutionPlan{WorkflowName: workflowName}

	for _, providerName := range elevateRequest.Providers {
		providerName = strings.TrimSpace(providerName)
		if providerName == "" {
			return nil, fmt.Errorf("execution plan entry is missing provider name")
		}

		provider, err := cfg.GetProviderByName(providerName)
		if err != nil {
			return nil, fmt.Errorf("failed to get provider %q: %w", providerName, err)
		}

		for _, identityID := range elevateRequest.Identities {
			resolvedIdentity := resolveIdentitySnapshot(cfg, identityID)

			for _, tenantID := range tenants {
				workflowReq := &models.WorkflowRoleRequest{
					WorkflowID:       workflowID,
					Identity:         identityID,
					ResolvedIdentity: resolvedIdentity,
					Role:             elevateRequest.Role,
					Duration:         &duration,
					Tenant:           tenantID,
				}

				if err := applyExecutionPlanDecorators(cfg, workflowReq, elevateRequest, opts); err != nil {
					return nil, err
				}

				entryID := models.CreateExecutionPlanEntryID(workflowID, providerName, workflowReq)
				if err := finalizeExecutionPlanDecorators(workflowReq, elevateRequest, entryID, opts); err != nil {
					return nil, err
				}

				authorizeRequest, err := models.CreateAuthorizeRoleRequest(cfg, provider, workflowReq)
				if err != nil {
					return nil, fmt.Errorf("failed to create authorize role request for provider %q and identity %q: %w", providerName, identityID, err)
				}

				plan.Entries = append(plan.Entries, models.ExecutionPlanEntry{
					EntryID:          entryID,
					ProviderName:     providerName,
					DeviceID:         workflowReq.DeviceID,
					AuthorizeRequest: authorizeRequest,
				})
			}
		}
	}

	if !plan.IsValid() {
		return nil, fmt.Errorf("execution plan did not contain any entries")
	}

	return plan, nil
}

func (opts executionPlanBuildOptions) withDefaults(cfg models.ConfigImpl) executionPlanBuildOptions {
	if opts.LookupDeviceDefinition == nil {
		opts.LookupDeviceDefinition = cfg.GetDevice
	}
	if opts.Decorators == nil {
		opts.Decorators = []executionPlanDecorator{
			localSudoExecutionPlanDecorator{},
		}
	}
	return opts
}

func applyExecutionPlanDecorators(
	cfg models.ConfigImpl,
	req *models.WorkflowRoleRequest,
	elevateRequest *models.ElevateRequestInternal,
	opts executionPlanBuildOptions,
) error {
	if req == nil {
		return fmt.Errorf("workflow role request is required for execution planning")
	}

	for _, decorator := range opts.Decorators {
		if decorator == nil || !decorator.Applies(elevateRequest) {
			continue
		}
		if err := decorator.Decorate(cfg, req, elevateRequest, opts); err != nil {
			return err
		}
	}

	return nil
}

func finalizeExecutionPlanDecorators(
	req *models.WorkflowRoleRequest,
	elevateRequest *models.ElevateRequestInternal,
	entryID string,
	opts executionPlanBuildOptions,
) error {
	for _, decorator := range opts.Decorators {
		if decorator == nil || !decorator.Applies(elevateRequest) {
			continue
		}
		if err := decorator.Finalize(req, elevateRequest, entryID); err != nil {
			return err
		}
	}

	return nil
}

func resolveIdentitySnapshot(cfg models.ConfigImpl, identityID string) *models.Identity {
	identityResult, err := cfg.GetIdentity(identityID)
	if err != nil || identityResult == nil || identityResult.User == nil {
		return nil
	}
	return identityResult
}
