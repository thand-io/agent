package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

type thandActivities struct {
	config *Config
}

func (t *thandActivities) SignalWorkflow(ctx context.Context,
	workflowId string,
	runId string,
	signalName string,
	signalInput any,
) error {

	services := t.config.GetServices()

	if !services.HasTemporal() {
		return temporal.NewNonRetryableApplicationError(
			"Temporal service is not configured",
			"TemporalServiceNotConfigured",
			nil,
		)
	}

	if !services.GetTemporal().HasClient() {
		return temporal.NewNonRetryableApplicationError(
			"Temporal client is not configured",
			"TemporalClientNotConfigured",
			nil,
		)
	}

	log := activity.GetLogger(ctx)

	log.Info("Signaling workflow")

	err := services.GetTemporal().GetClient().SignalWorkflow(
		ctx,
		workflowId,
		runId,
		signalName,
		signalInput,
	)

	if err != nil {
		log.Error("Failed to signal workflow")
		return err
	}

	return nil

}

// This queries both system and agent workflows to get the task queue
// identifier - used in order to figure out where the workflow op should
// run
func (t *thandActivities) LookupSystemIdentifier(
	ctx context.Context,
	identifier string,
) (string, error) {

	if len(identifier) == 0 {
		return "", temporal.NewNonRetryableApplicationError(
			"Identifier cannot be empty",
			"InvalidIdentifier",
			nil,
		)
	}

	c := t.config

	log := activity.GetLogger(ctx)

	log.Info("Looking up system identifier in temporal workflows", "identifier", identifier)

	// First we'll query the system workflows to see if there is a match
	// if there is a match, we'll return the workflow id which is the same as the system id

	if !c.GetServices().HasTemporal() {

		log.Warn("Thand service is not configured; skipping PatchProviderUpstream activity")

		return "", temporal.NewNonRetryableApplicationError(
			"Thand service is not configured",
			"ThandServiceNotConfigured",
			nil,
		)
	}

	temporalService := c.GetServices().GetTemporal()

	if !temporalService.HasClient() {
		log.Warn("Thand service is not configured; skipping PatchProviderUpstream activity")

		return "", temporal.NewNonRetryableApplicationError(
			"Thand service is not configured",
			"ThandServiceNotConfigured",
			nil,
		)
	}

	// Now lookup

	temporalClient := temporalService.GetClient()

	listResponse, err := temporalClient.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: temporalService.GetNamespace(),
		Query: fmt.Sprintf(
			"`identities` in(\"%s\") AND (`WorkflowType`=\"server-workflow\" OR `WorkflowType`=\"agent-workflow\") AND `ExecutionStatus`=\"Running\"",
			identifier,
		),
	})

	if err != nil {
		log.Error("failed to list workflows", "error", err)
		return "", err
	}

	log.Info("Found workflows for system identifier", "count", len(listResponse.GetExecutions()))

	for _, workflowExec := range listResponse.GetExecutions() {

		// Send a quick query to see if there is a worker avaliable
		// we'll keep going until we find an alive system

		// We'll signal the workflow to see if its alive

		_, err := temporalClient.QueryWorkflow(
			ctx,
			workflowExec.Execution.GetWorkflowId(),
			workflowExec.Execution.GetRunId(),
			models.TemporalSystemPingQueryName,
			nil, // empty ping
		)

		if err != nil {
			log.Info("failed to query workflow",
				"workflowId", workflowExec.Execution.GetWorkflowId())
			continue
		}

		// Device is alive - we'll query this one
		return workflowExec.Execution.WorkflowId, nil
	}

	// Re-try the query after a short delay - this is to handle the case where the workflow has just been started and is not yet available in the list query
	return "", fmt.Errorf("no active workflows found for identifier: %s", identifier)

}

// PatchProviderUpstreamDummy is a no-op activity for thand server/agents that are not
// configured to use the Thand service
func (t *thandActivities) PatchProviderUpstreamDummy(
	ctx context.Context,
	activityMethod models.SynchronizeCapability,
	providerIdentifier string,
	resp any,
) error {
	return nil
}

// PatchProviderUpstream patches the provider's upstream endpoint in the Thand server
// This sends updates for users, groups, roles, permissions, resources, etc.
// So that Thand can paginate through the data when the provider is synchronized
func (t *thandActivities) PatchProviderUpstream(
	ctx context.Context,
	activityMethod models.SynchronizeCapability,
	providerIdentifier string,
	resp any,
) error {

	c := t.config

	log := activity.GetLogger(ctx)

	if !c.HasThandService() {

		log.Warn("Thand service is not configured; skipping PatchProviderUpstream activity")

		return temporal.NewNonRetryableApplicationError(
			"Thand service is not configured",
			"ThandServiceNotConfigured",
			nil,
		)
	}

	baseUrl := c.DiscoverThandServerApiUrl()
	providerSyncUrl := fmt.Sprintf("%s/providers/%s/sync",
		strings.TrimSuffix(baseUrl, "/"),
		strings.ToLower(providerIdentifier),
	)

	upstream := &model.Endpoint{
		EndpointConfig: &model.EndpointConfiguration{
			URI: &model.LiteralUri{Value: providerSyncUrl},
			Authentication: &model.ReferenceableAuthenticationPolicy{
				AuthenticationPolicy: &model.AuthenticationPolicy{
					Bearer: &model.BearerAuthenticationPolicy{
						Token: c.Thand.ApiKey,
					},
				},
			},
		},
	}

	// Make patch request
	err := models.PatchProviderUpstream(
		activityMethod,
		upstream,
		resp,
	)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to send pagination patch to server")
	}

	return err

}
