package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type ProviderPatchRequest struct {
	Identities  []Identity           `json:"identities,omitempty"`
	Roles       []ProviderRole       `json:"roles,omitempty"`
	Permissions []ProviderPermission `json:"permissions,omitempty"`
	Resources   []ProviderResource   `json:"resources,omitempty"`
	Tenants     []ProviderTenant     `json:"tenants,omitempty"`
}

func (p *BaseProvider) Synchronize(
	ctx context.Context,
	temporalService TemporalImpl,
	syncRequest *SynchronizeRequest,
) error {

	logrus.Infoln("Using default synchronization strategy for provider: ", p.GetIdentifier())

	return Synchronize(ctx, temporalService, p, syncRequest)
}

// Synchronize performs synchronization of identities, roles, permissions, and resources
// for the given provider. It can use Temporal workflows if a Temporal service
// is provided, otherwise it falls back to a pure Go implementation.
// The SynchronizeRequest can specify which capabilities to synchronize.
// and can be nil to use default behavior.
func Synchronize(
	ctx context.Context,
	temporalService TemporalImpl,
	provider Provider,
	syncRequest *SynchronizeRequest, // can be nil
) error {

	if provider == nil {
		logrus.Error("Provider client is nil. Ensure the provider is initialized")
		return fmt.Errorf("provider client is nil. Ensure the provider is initialized")
	}

	// Check if we have the relevant capabilities for synchronization
	if !provider.HasAnyCapability(
		ProviderCapabilityTenants,
		ProviderCapabilityIdentities,
		ProviderCapabilityUsers,
		ProviderCapabilityGroups,
		ProviderCapabilityResources,
		ProviderCapabilityRoles,
		ProviderCapabilityPermissions,
	) {
		logrus.Infof("Provider %s does not have synchronization capabilities, skipping", provider.GetName())
		return nil
	}

	// Set default values
	if syncRequest == nil {
		syncRequest = &SynchronizeRequest{}
	}

	if len(syncRequest.ProviderIdentifier) == 0 {
		syncRequest.ProviderIdentifier = provider.GetIdentifier()
	}

	if len(syncRequest.Requests) == 0 {

		requests := getSynchronizationRequests(provider)

		if len(requests) == 0 {
			logrus.WithFields(logrus.Fields{
				"provider":   provider.GetProvider(),
				"name":       provider.GetName(),
				"identifier": provider.GetIdentifier(),
			}).Info("Provider does not have overridden synchronization methods, skipping")
			return nil
		}
		syncRequest.Requests = requests

	}

	logrus.WithFields(logrus.Fields{
		"provider": provider.GetName(),
		"requests": syncRequest.Requests,
	}).Info("Starting synchronization for provider: " + provider.GetIdentifier())

	if temporalService != nil && temporalService.HasClient() {

		temporalClient := temporalService.GetClient()

		// Execute the provider workflow synchronize
		workflowOptions := client.StartWorkflowOptions{
			ID: CreateTemporalProviderWorkflowIdentifier(
				provider.GetIdentifier(),
				TemporalSynchronizeWorkflowName,
			),
			TaskQueue: temporalService.GetTaskQueue(),
			// Set a timeout for the workflow execution
			WorkflowExecutionTimeout: 30 * time.Minute,
		}

		// Only add versioning override if versioning is enabled
		if !temporalService.IsVersioningDisabled() {
			workflowOptions.VersioningOverride = &client.PinnedVersioningOverride{
				Version: worker.WorkerDeploymentVersion{
					DeploymentName: sdkConstants.TemporalDeploymentName,
					BuildID:        common.GetBuildIdentifier(),
				},
			}
		}

		syncWorkflowName := CreateTemporalProviderWorkflowName(
			provider.GetIdentifier(),
			TemporalSynchronizeWorkflowName,
		)

		logrus.WithFields(logrus.Fields{
			"workflow":   syncWorkflowName,
			"provider":   provider.GetName(),
			"identifier": provider.GetIdentifier(),
		}).Infoln("Starting provider synchronize workflow with name: " + syncWorkflowName + " for provider: " + provider.GetIdentifier())

		// Before starting the sync workflow, check to see if its currently
		// running and terminate it if so as this is a new instance.
		// Use the workflow ID (not the workflow type name) for describe/terminate.
		running, err := temporalClient.DescribeWorkflowExecution(ctx, workflowOptions.ID, "")

		if err == nil && running.WorkflowExecutionInfo != nil {
			logrus.WithFields(logrus.Fields{
				"workflow_id": running.WorkflowExecutionInfo.Execution.GetWorkflowId(),
				"run_id":      running.WorkflowExecutionInfo.Execution.GetRunId(),
			}).Info("Terminating existing provider synchronize workflow before starting new one")

			err = temporalClient.TerminateWorkflow(ctx, workflowOptions.ID, "", "New synchronization initiated")

			if err != nil {
				logrus.WithError(err).Error("Failed to terminate existing provider synchronize workflow")
			}
		}

		logrus.WithFields(logrus.Fields{
			"workflow": syncWorkflowName,
			"provider": provider.GetName(),
		}).Infoln("Executing provider synchronize workflow for provider: " + provider.GetIdentifier())

		syncWorkflow, err := temporalClient.ExecuteWorkflow(
			ctx,
			workflowOptions,
			syncWorkflowName,
			syncRequest,
		)

		if err != nil {
			logrus.WithError(err).Error("Failed to start provider synchronize workflow")
			return fmt.Errorf("failed to execute synchronize workflow: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"workflow_id": syncWorkflow.GetID(),
			"run_id":      syncWorkflow.GetRunID(),
			"provider":    provider.GetName(),
			"identifier":  provider.GetIdentifier(),
			"requests":    syncRequest.Requests,
		}).Info("Started provider synchronize workflow for: " + provider.GetIdentifier())

		return nil
	}

	// Pure Go implementation
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	if provider.CanSynchronizeTenants() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizeTenants,
			&SynchronizeTenantsRequest{}, provider.SynchronizeTenants)
	}

	if provider.CanSynchronizeIdentities() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizeIdentities,
			&SynchronizeIdentitiesRequest{}, provider.SynchronizeIdentities)
	}

	if provider.CanSynchronizeUsers() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizeUsers,
			&SynchronizeUsersRequest{}, provider.SynchronizeUsers)
	}

	if provider.CanSynchronizeGroups() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizeGroups,
			&SynchronizeGroupsRequest{}, provider.SynchronizeGroups)
	}

	if provider.CanSynchronizeResources() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizeResources,
			&SynchronizeResourcesRequest{}, provider.SynchronizeResources)
	}

	if provider.CanSynchronizeRoles() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizeRoles,
			&SynchronizeRolesRequest{}, provider.SynchronizeRoles)
	}

	if provider.CanSynchronizePermissions() {
		executeSync(ctx, &wg, &mu, &errs, syncRequest, provider, SynchronizePermissions,
			&SynchronizePermissionsRequest{}, provider.SynchronizePermissions)
	}

	logrus.WithFields(logrus.Fields{
		"requests": len(syncRequest.Requests),
	}).Info("Waiting for synchronization tasks to complete")

	wg.Wait()

	if len(errs) > 0 {
		logrus.WithError(errors.Join(errs...)).Error("Synchronization completed with errors")
		return fmt.Errorf("synchronization failed: %v", errs)
	}

	return nil
}

// paginatedSync runs a generic paginated synchronization loop. It calls
// executePage for each page, routes results to the provider via AddToProvider,
// and invokes the optional onPage callback. Both the Temporal workflow path
// (runSyncLoop) and the pure-Go path (executeSync) delegate to this function.
func paginatedSync[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	provider Provider,
	name SynchronizeCapability,
	req Req,
	executePage func(Req) (Resp, error),
	onPage func(Resp),
) error {
	for {
		logrus.Debugf("Making synchronization request: %s", name)

		resp, err := executePage(req)
		if err != nil {
			if errors.Is(err, ErrNotImplemented) {
				logrus.Debugf("Synchronization operation %s is not implemented, skipping", name)
				return nil
			}
			return err
		}

		logrus.WithFields(logrus.Fields{
			"response": provider,
		}).Debugf("Received synchronization response for %s", name)

		resp.AddToProvider(provider)

		if onPage != nil {
			onPage(resp)
		}

		pagination := resp.GetPagination()
		if pagination == nil || len(pagination.Token) == 0 {
			logrus.Debugf("Synchronization operation %s completed with no more pages", name)
			break
		}

		req.SetPagination(pagination)
	}
	return nil
}

// executeSync launches a paginated synchronization goroutine using paginatedSync.
// It calls the provider's sync method directly (pure-Go path, no Temporal).
func executeSync[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx context.Context,
	wg *sync.WaitGroup,
	mu *sync.Mutex,
	errs *[]error,
	syncRequest *SynchronizeRequest,
	provider Provider,
	name SynchronizeCapability,
	req Req,
	syncOp func(context.Context, Req) (Resp, error),
) {
	if !slices.Contains(syncRequest.Requests, name) {
		logrus.Infof("Skipping synchronization for %s as it's not requested", name)
		return
	}

	wg.Go(func() {
		logrus.Infof("Starting synchronization operation: %s", name)

		err := paginatedSync(provider, name, req,
			func(r Req) (Resp, error) { return syncOp(ctx, r) },
			nil, // no post-page hook in the pure-Go path - TODO add patching support
		)
		if err != nil {
			logrus.WithError(err).Errorf("Synchronization operation %s failed", name)

			mu.Lock()
			*errs = append(*errs, fmt.Errorf("%s failed: %w", name, err))
			mu.Unlock()
		}
	})
}

func PatchProviderUpstream(
	name SynchronizeCapability,
	upstream *model.Endpoint,
	payload any,
) error {
	logrus.Debugln("Sending synchronization updates back to server")

	providerReq := ProviderPatchRequest{}

	switch name {
	case SynchronizeTenants:
		tenantsResp, ok := payload.(SynchronizeTenantsResponse)
		if ok {
			providerReq.Tenants = tenantsResp.Tenants
		}
	case SynchronizeIdentities:
		identitiesResp, ok := payload.(SynchronizeIdentitiesResponse)
		if ok {
			providerReq.Identities = identitiesResp.Identities
		}
	case SynchronizeUsers:
		usersResp, ok := payload.(SynchronizeUsersResponse)
		if ok {
			providerReq.Identities = usersResp.Identities
		}
	case SynchronizeGroups:
		groupsResp, ok := payload.(SynchronizeGroupsResponse)
		if ok {
			providerReq.Identities = groupsResp.Identities
		}
	case SynchronizeRoles:
		rolesResp, ok := payload.(SynchronizeRolesResponse)
		if ok {
			providerReq.Roles = rolesResp.Roles
		}
	case SynchronizePermissions:
		permissionsResp, ok := payload.(SynchronizePermissionsResponse)
		if ok {
			providerReq.Permissions = permissionsResp.Permissions
		}
	case SynchronizeResources:
		resourcesResp, ok := payload.(SynchronizeResourcesResponse)
		if ok {
			providerReq.Resources = resourcesResp.Resources
		}
	}

	if len(providerReq.Identities) == 0 && len(providerReq.Roles) == 0 &&
		len(providerReq.Permissions) == 0 && len(providerReq.Resources) == 0 &&
		len(providerReq.Tenants) == 0 {
		logrus.Debugln("No synchronization data to send to upstream")
		return nil
	}

	data, err := json.Marshal(providerReq)

	if err == nil && len(data) > 0 && upstream != nil {

		resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
			Method:   http.MethodPatch,
			Endpoint: upstream,
			Body:     data,
		})

		if err != nil {
			logrus.WithError(err).Errorln("Failed to send synchronization updates to server")
			return err
		}

		if resp.StatusCode() != http.StatusOK {
			logrus.WithField("status_code", resp.StatusCode()).Errorln("Failed to send synchronization updates to server")
		} else {
			logrus.Infoln("Successfully sent synchronization updates to server")
		}
	}

	return nil
}

func getSynchronizationRequests(provider Provider) []SynchronizeCapability {
	requests := make([]SynchronizeCapability, 0)

	// Determine which capabilities to synchronize
	// Check if the underlying provider has been overridden to
	// support identities, roles, permissions, resources

	if provider.CanSynchronizeTenants() {
		requests = append(requests, SynchronizeTenants)
	}

	if provider.CanSynchronizeIdentities() {
		requests = append(requests, SynchronizeIdentities)
	}

	if provider.CanSynchronizeUsers() {
		requests = append(requests, SynchronizeUsers)
	}

	if provider.CanSynchronizeGroups() {
		requests = append(requests, SynchronizeGroups)
	}

	if provider.CanSynchronizeResources() {
		requests = append(requests, SynchronizeResources)
	}

	if provider.CanSynchronizeRoles() {
		requests = append(requests, SynchronizeRoles)
	}

	if provider.CanSynchronizePermissions() {
		requests = append(requests, SynchronizePermissions)
	}

	return requests
}
