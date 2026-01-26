package workflows_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
	testcommon "github.com/thand-io/agent/test/integration/common"
	"go.temporal.io/api/workflowservice/v1"
)

// TestInfrastructure extends the common test infrastructure with workflow-specific helpers
type TestInfrastructure struct {
	*testcommon.TestInfrastructure
}

// SetupTestInfrastructure creates and starts Temporal and LocalStack containers
func SetupTestInfrastructure(t *testing.T, ctx context.Context) *TestInfrastructure {
	t.Helper()

	baseInfra := testcommon.SetupInfrastructure(t, ctx)

	return &TestInfrastructure{
		TestInfrastructure: baseInfra,
	}
}

// GetEmails retrieves all emails from MailHog using the HTTP invocation method
func (infra *TestInfrastructure) GetEmails() ([]testcommon.MailHogMessage, error) {
	url := infra.MailHogAPI + "/api/v2/messages"
	resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method:   http.MethodGet,
		Endpoint: model.NewEndpoint(url),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get emails from MailHog: %w", err)
	}

	var messages testcommon.MailHogMessages
	if err := json.Unmarshal(resp.Body(), &messages); err != nil {
		return nil, fmt.Errorf("failed to parse MailHog response: %w", err)
	}

	return messages.Items, nil
}

// GetEmailsForRecipient retrieves emails sent to a specific address
func (infra *TestInfrastructure) GetEmailsForRecipient(email string) ([]testcommon.MailHogMessage, error) {
	allEmails, err := infra.GetEmails()
	if err != nil {
		return nil, err
	}

	var filtered []testcommon.MailHogMessage
	for _, msg := range allEmails {
		for _, to := range msg.To {
			if fmt.Sprintf("%s@%s", to.Mailbox, to.Domain) == email {
				filtered = append(filtered, msg)
				break
			}
		}
	}
	return filtered, nil
}

// WaitForEmail waits for an email to arrive for a specific recipient
func (infra *TestInfrastructure) WaitForEmail(recipient string, timeout time.Duration) (*testcommon.MailHogMessage, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		emails, err := infra.GetEmailsForRecipient(recipient)
		if err != nil {
			return nil, err
		}
		if len(emails) > 0 {
			return &emails[0], nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for email to %s", recipient)
}

// ExtractLinksFromEmail extracts all URLs from an email body
func (infra *TestInfrastructure) ExtractLinksFromEmail(msg *testcommon.MailHogMessage) []string {
	// Match URLs in the email body
	urlRegex := regexp.MustCompile(`https?://[^\s<>"]+`)
	return urlRegex.FindAllString(msg.Content.Body, -1)
}

// TestTemporalAndLocalStackSetup verifies that all containers start correctly
func TestTemporalAndLocalStackSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Set a reasonable timeout for container startup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup infrastructure
	infra := SetupTestInfrastructure(t, ctx)
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
			Namespace: testcommon.TemporalTestNamespace,
			PageSize:  1,
		})
		require.NoError(t, err, "Should be able to list workflows")
	})
}
