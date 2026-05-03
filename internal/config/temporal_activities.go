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

// This queries both system and agent workflows to get the task queue
// identifier - used in order to figure out where the workflow op should
// run
func (t *thandActivities) LookupSystemIdentifier(
	ctx context.Context,
) (string, error) {

	c := t.config

	log := activity.GetLogger(ctx)

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
		Query:     "status = RUNNING",
	})

	if err != nil {
		return "", err
	}

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

	return "", temporal.NewNonRetryableApplicationError(
		"Thand service is not configured",
		"ThandServiceNotConfigured",
		nil,
	)

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
