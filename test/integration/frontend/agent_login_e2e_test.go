package ui_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/test/integration/testinfra"
)

// TestAgentLoginE2E exercises the browser login flow through a self-contained
// server+agent test environment.
//
// Run with Brave by setting CHROME_BIN, for example:
//
//	CHROME_BIN="/Applications/Brave Browser.app/Contents/MacOS/Brave Browser" go test -v -run TestAgentLoginE2E ./integration/frontend/...
func TestAgentLoginE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping UI integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	loader := testinfra.NewTestCaseLoader(nil, "testdata")
	testCase, err := loader.LoadTestCase("agent-login")
	require.NoError(t, err, "Failed to load agent-login test case")

	infra := SetupUITestInfrastructure(t, ctx, testCase, WithAgentContainer(), WithAgentLocalDefinitions())
	defer infra.Teardown()

	require.NotEmpty(t, infra.AgentEndpoint, "Agent endpoint should be configured")

	browser := NewBrowser(t, infra.AgentEndpoint)
	defer browser.Close()

	err = browser.Login(ctx, "testuser@thand.io", "testpass123")
	require.NoError(t, err, "Agent login should succeed")

	err = browser.WaitForAuthCallbackSuccess(ctx, 30*time.Second)
	require.NoError(t, err, "Agent auth callback should register the session")

	sessions := waitForAgentSessions(t, infra.AgentEndpoint, "oidc-test", 30*time.Second)
	require.Contains(t, sessions.Sessions, "oidc-test", "Agent should expose the authenticated session")
	localSession := sessions.Sessions["oidc-test"]
	require.False(t, localSession.IsExpired(), "OIDC session should remain active")
}

func waitForAgentSessions(t *testing.T, agentEndpoint string, provider string, timeout time.Duration) *models.SessionsResponse {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err, "Failed to create cookie jar for agent session polling")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
	}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		sessions, err := fetchAgentSessions(client, agentEndpoint)
		if err == nil {
			if _, found := sessions.Sessions[provider]; found {
				return sessions
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for agent session %q at %s", provider, agentEndpoint)
	return nil
}

func fetchAgentSessions(client *http.Client, agentEndpoint string) (*models.SessionsResponse, error) {
	resp, err := client.Get(agentEndpoint + "/api/v1/sessions")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected sessions status: %s", resp.Status)
	}

	var sessions models.SessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return nil, err
	}

	return &sessions, nil
}
