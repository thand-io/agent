package workflows_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/test/integration/testinfra"
	"go.temporal.io/api/workflowservice/v1"
)

// TestTemporalAndLocalStackSetup verifies that all containers start correctly
func TestTemporalAndLocalStackSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set a reasonable timeout for container startup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup infrastructure
	infra := testinfra.SetupTestInfrastructure(t, ctx)
	defer infra.Teardown()

	// Verify LocalStack is accessible
	t.Run("LocalStack is accessible", func(t *testing.T) {
		require.NotEmpty(t, infra.LocalStackEndpoint, "LocalStack endpoint should be set")
		t.Logf("LocalStack endpoint: %s", infra.LocalStackEndpoint)
	})

	// Verify MailHog is accessible
	t.Run("MailHog is accessible", func(t *testing.T) {
		require.NotEmpty(t, infra.MailHogSMTP, "MailHog SMTP endpoint should be set")
		require.NotEmpty(t, infra.MailHogAPI, "MailHog API endpoint should be set")
		t.Logf("MailHog SMTP: %s, API: %s", infra.MailHogSMTP, infra.MailHogAPI)

		// Verify we can query the API
		emails, err := infra.GetEmails()
		require.NoError(t, err, "Should be able to query MailHog API")
		t.Logf("MailHog has %d emails", len(emails))
	})

	// Verify Temporal is accessible
	t.Run("Temporal is accessible", func(t *testing.T) {
		require.NotEmpty(t, infra.TemporalEndpoint, "Temporal endpoint should be set")
		require.NotNil(t, infra.TemporalClient, "Temporal client should be connected")
		t.Logf("Temporal endpoint: %s", infra.TemporalEndpoint)

		// Try to list workflows to verify connection
		_, err := infra.TemporalClient.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
			Namespace: testinfra.TemporalTestNamespace,
			PageSize:  1,
		})
		require.NoError(t, err, "Should be able to list workflows")
	})
}
