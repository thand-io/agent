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

	if temporalService != nil {

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

		_, err := temporalClient.ExecuteWorkflow(
			ctx,
			workflowOptions,
			CreateTemporalProviderWorkflowName(
				provider.GetIdentifier(),
				TemporalSynchronizeWorkflowName,
			),
			syncRequest,
		)

		if err != nil {
			logrus.WithError(err).Error("Failed to start provider synchronize workflow")
			return fmt.Errorf("failed to execute synchronize workflow: %w", err)
		}

		return nil
	}

	// Pure Go implementation
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	if provider.CanSynchronizeTenants() {
		// Synchronize Tenants
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizeTenants, &SynchronizeTenantsRequest{},
			func(ctx context.Context, req *SynchronizeTenantsRequest) (*SynchronizeTenantsResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).Debug("Starting tenant synchronization")
				return provider.SynchronizeTenants(ctx, req)
			},
			func(resp *SynchronizeTenantsResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Tenants),
				}).Info("Synchronized tenants")
				provider.AddTenants(resp.Tenants...)
			})
	}

	if provider.CanSynchronizeIdentities() {
		// Synchronize Identities
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizeIdentities, &SynchronizeIdentitiesRequest{},
			func(ctx context.Context, req *SynchronizeIdentitiesRequest) (*SynchronizeIdentitiesResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).Debug("Starting identity synchronization")
				return provider.SynchronizeIdentities(ctx, req)
			},
			func(resp *SynchronizeIdentitiesResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Identities),
				}).Info("Synchronized identities")
				provider.AddIdentities(resp.Identities...)
			})
	}

	if provider.CanSynchronizeUsers() {
		// Synchronize Users
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizeUsers, &SynchronizeUsersRequest{},
			func(ctx context.Context, req *SynchronizeUsersRequest) (*SynchronizeUsersResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).Debug("Starting user synchronization")
				return provider.SynchronizeUsers(ctx, req)
			},
			func(resp *SynchronizeUsersResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Identities),
				}).Debug("Synchronized users")
				provider.AddIdentities(resp.Identities...)
			})
	}

	if provider.CanSynchronizeGroups() {
		// Synchronize Groups
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizeGroups, &SynchronizeGroupsRequest{},
			func(ctx context.Context, req *SynchronizeGroupsRequest) (*SynchronizeGroupsResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).Debug("Starting group synchronization")
				return provider.SynchronizeGroups(ctx, req)
			},
			func(resp *SynchronizeGroupsResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Identities),
				}).Debug("Synchronized groups")
				provider.AddIdentities(resp.Identities...)
			})
	}

	if provider.CanSynchronizeResources() {
		// Synchronize Resources
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizeResources, &SynchronizeResourcesRequest{},
			func(ctx context.Context, req *SynchronizeResourcesRequest) (*SynchronizeResourcesResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).Debug("Starting resource synchronization")
				return provider.SynchronizeResources(ctx, req)
			},
			func(resp *SynchronizeResourcesResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Resources),
				}).Debug("Synchronized resources")
				provider.AddResources(resp.Resources...)
			})
	}

	if provider.CanSynchronizeRoles() {
		// Synchronize Roles
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizeRoles, &SynchronizeRolesRequest{},
			func(ctx context.Context, req *SynchronizeRolesRequest) (*SynchronizeRolesResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).Debug("Starting role synchronization")
				return provider.SynchronizeRoles(ctx, req)
			},
			func(resp *SynchronizeRolesResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Roles),
				}).Debug("Synchronized roles")
				provider.AddRoles(resp.Roles...)
			})
	}

	if provider.CanSynchronizePermissions() {
		// Synchronize Permissions
		executeSync(ctx, &wg, &mu, &errs, syncRequest, SynchronizePermissions, &SynchronizePermissionsRequest{},
			func(ctx context.Context, req *SynchronizePermissionsRequest) (*SynchronizePermissionsResponse, error) {
				logrus.WithFields(logrus.Fields{
					"provider":   provider.GetIdentifier(),
					"pagination": req.Pagination,
				}).
					Debug("Starting permission synchronization")
				return provider.SynchronizePermissions(ctx, req)
			},
			func(resp *SynchronizePermissionsResponse) {
				logrus.WithFields(logrus.Fields{
					"provider": provider.GetIdentifier(),
					"count":    len(resp.Permissions),
				}).Debug("Synchronized permissions")
				provider.AddPermissions(resp.Permissions...)
			})
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

func executeSync[Req SynchronizeRequestImpl, Resp SynchronizeResponseImpl](
	ctx context.Context,
	wg *sync.WaitGroup,
	mu *sync.Mutex,
	errs *[]error,
	syncRequest *SynchronizeRequest,
	name SynchronizeCapability,
	req Req,
	syncOp func(context.Context, Req) (Resp, error),
	processOp func(Resp),
) {
	if !slices.Contains(syncRequest.Requests, name) {
		logrus.Infof("Skipping synchronization for %s as it's not requested", name)
		return
	}

	wg.Go(func() {

		logrus.Infof("Starting synchronization operation: %s", name)

		for {

			logrus.Debugf("Making synchronization request: %s", name)

			resp, err := syncOp(ctx, req)

			if err != nil {
				// Ignore not implemented errors
				if errors.Is(err, ErrNotImplemented) {
					return
				}
				mu.Lock()
				*errs = append(*errs, fmt.Errorf("%s failed: %w", name, err))
				mu.Unlock()
				return
			}

			processOp(resp)

			pagination := resp.GetPagination()

			if pagination == nil || len(pagination.Token) == 0 {
				break
			}

			req.SetPagination(pagination)

			/*
				// Disable this for now for non-thand instances.
				// If there is no temporal provided by thand.io
				// then don't attempt to patch upstream.
					go func() {
						if syncRequest.Upstream != nil {
							PatchProviderUpstream(
								name,
								syncRequest.Upstream,
								resp,
							)
						}
					}()
			*/
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
