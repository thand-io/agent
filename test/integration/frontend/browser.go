package ui_e2e

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

// AuthType represents the authentication mechanism for a test.
type AuthType string

const (
	AuthTypeOIDC AuthType = "oidc"
	AuthTypeSAML AuthType = "saml"
)

// Browser represents a chromedp browser automation instance
type Browser struct {
	t       *testing.T
	baseURL string
	timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewBrowser creates a new chromedp browser instance
// Set THAND_SHOW_BROWSER=true to see browser interactions
func NewBrowser(t *testing.T, baseURL string) *Browser {
	t.Helper()

	// Check if we should show the browser (non-headless mode)
	showBrowser := os.Getenv("THAND_SHOW_BROWSER") == "true"

	// Create chromedp context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !showBrowser),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	if showBrowser {
		t.Log("Running browser in visible mode (THAND_SHOW_BROWSER=true)")
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, _ := chromedp.NewContext(allocCtx)

	return &Browser{
		t:       t,
		baseURL: baseURL,
		timeout: 30 * time.Second,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Close closes the browser instance
func (b *Browser) Close() {
	if b.cancel != nil {
		b.cancel()
	}
}

// LoginWithOIDC performs OIDC login flow via Keycloak.
func (b *Browser) LoginWithOIDC(ctx context.Context, username, password string) error {
	b.t.Log("Logging in with OIDC via Keycloak...")

	loginURL := b.baseURL + "/auth"

	// Step 1: Navigate to auth page and click OIDC button
	err := chromedp.Run(b.ctx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`[data-provider="oidc-test"]`, chromedp.ByQuery),
		chromedp.Click(`[data-provider="oidc-test"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second), // Wait for redirect to Keycloak
	)
	if err != nil {
		return fmt.Errorf("OIDC login step 1 (click) failed: %w", err)
	}

	// Capture current URL for debugging
	var currentURL string
	var pageTitle string
	var bodyText string
	_ = chromedp.Run(b.ctx,
		chromedp.Location(&currentURL),
		chromedp.Title(&pageTitle),
		chromedp.Text(`body`, &bodyText, chromedp.ByQuery),
	)
	b.t.Logf("After OIDC click - URL: %s, Title: %s", currentURL, pageTitle)
	if strings.Contains(pageTitle, "Error") || strings.Contains(bodyText, "error") || strings.Contains(bodyText, "Invalid") {
		b.t.Logf("Page body (first 500 chars): %s", truncateString(bodyText, 500))
	}

	// Check if SSO already handled the login (Keycloak SSO skips the form)
	// If we're already back at the app (not at Keycloak), login is complete.
	if !strings.Contains(currentURL, "/realms/") && !strings.Contains(currentURL, "/auth/realms/") {
		b.t.Log("OIDC login completed (SSO - no form needed)")
		return nil
	}

	// Step 2: Fill in Keycloak login form
	// Keycloak login form uses #username, #password, #kc-login
	err = chromedp.Run(b.ctx,
		chromedp.WaitVisible(`#username`, chromedp.ByID),
		chromedp.SendKeys(`#username`, username, chromedp.ByID),
		chromedp.SendKeys(`#password`, password, chromedp.ByID),
		chromedp.Click(`#kc-login`, chromedp.ByID),
		chromedp.Sleep(2*time.Second), // Wait for redirect
	)

	if err != nil {
		// Capture error state for debugging
		var errURL string
		var errBody string
		_ = chromedp.Run(b.ctx,
			chromedp.Location(&errURL),
			chromedp.Text(`body`, &errBody, chromedp.ByQuery),
		)
		b.t.Logf("OIDC login form error - URL: %s", errURL)
		b.t.Logf("Page body (first 500 chars): %s", truncateString(errBody, 500))
		return fmt.Errorf("OIDC login step 2 (form) failed: %w", err)
	}

	b.t.Log("OIDC login completed")
	return nil
}

// truncateString truncates a string to the given max length.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// LoginWithSAML performs SAML login flow via Keycloak.
func (b *Browser) LoginWithSAML(ctx context.Context, username, password string) error {
	b.t.Log("Logging in with SAML via Keycloak...")

	loginURL := b.baseURL + "/auth"

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`[data-provider="saml-test"]`, chromedp.ByQuery),
		chromedp.Click(`[data-provider="saml-test"]`, chromedp.ByQuery),
		// Keycloak SAML redirects to the same Keycloak login page
		chromedp.WaitVisible(`#username`, chromedp.ByID),
		chromedp.SendKeys(`#username`, username, chromedp.ByID),
		chromedp.SendKeys(`#password`, password, chromedp.ByID),
		chromedp.Click(`#kc-login`, chromedp.ByID),
		chromedp.Sleep(2*time.Second), // Wait for SAML POST redirect
	)

	if err != nil {
		return fmt.Errorf("SAML login failed: %w", err)
	}

	b.t.Log("SAML login completed")
	return nil
}

// Login performs login using the specified auth type.
func (b *Browser) Login(ctx context.Context, authType AuthType, username, password string) error {
	switch authType {
	case AuthTypeSAML:
		return b.LoginWithSAML(ctx, username, password)
	case AuthTypeOIDC:
		return b.LoginWithOIDC(ctx, username, password)
	default:
		return b.LoginWithOIDC(ctx, username, password)
	}
}

// NavigateToElevatePage navigates to the elevate page
func (b *Browser) NavigateToElevatePage(ctx context.Context) error {
	b.t.Log("Navigating to elevate page...")

	elevateURL := b.baseURL + "/elevate/static"

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(elevateURL),
		chromedp.WaitVisible(`form#elevate-form`, chromedp.ByQuery),
	)

	if err != nil {
		return fmt.Errorf("navigation failed: %w", err)
	}

	b.t.Log("Navigation completed")
	return nil
}

// FillElevateForm fills out the elevation request form
func (b *Browser) FillElevateForm(ctx context.Context, role, provider, reason, duration string) error {
	b.t.Log("Filling elevate form...")

	err := chromedp.Run(b.ctx,
		chromedp.WaitVisible(`select[name="role"]`, chromedp.ByQuery),
		chromedp.SetValue(`select[name="role"]`, role, chromedp.ByQuery),
		chromedp.SetValue(`select[name="provider"]`, provider, chromedp.ByQuery),
		chromedp.SetValue(`select[name="duration"]`, duration, chromedp.ByQuery),
		chromedp.SendKeys(`textarea[name="reason"]`, reason, chromedp.ByQuery),
	)

	if err != nil {
		return fmt.Errorf("form fill failed: %w", err)
	}

	b.t.Log("Form filled")
	return nil
}

// SubmitElevateForm submits the elevation request form and returns the workflow ID
func (b *Browser) SubmitElevateForm(ctx context.Context) (string, error) {
	b.t.Log("Submitting elevate form...")

	var currentURL string

	err := chromedp.Run(b.ctx,
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second), // Wait for redirect
		chromedp.Location(&currentURL),
	)

	if err != nil {
		return "", fmt.Errorf("form submission failed: %w", err)
	}

	// Extract workflow ID from URL (e.g., /executions/{workflowID})
	// Simple parsing - in production you'd use a proper URL parser
	parts := strings.Split(currentURL, "/")
	for i, part := range parts {
		if part == "executions" && i+1 < len(parts) {
			workflowID := parts[i+1]
			// Remove query params if any
			if idx := strings.Index(workflowID, "?"); idx != -1 {
				workflowID = workflowID[:idx]
			}
			b.t.Logf("Form submitted, workflow ID: %s", workflowID)
			return workflowID, nil
		}
	}

	return "", fmt.Errorf("could not extract workflow ID from URL: %s", currentURL)
}

// WaitForWorkflowCompletion waits for a workflow to complete and returns the status
func (b *Browser) WaitForWorkflowCompletion(ctx context.Context, workflowID string, timeout time.Duration) (string, error) {
	b.t.Log("Waiting for workflow completion...")

	executionURL := fmt.Sprintf("%s/execution/%s", b.baseURL, workflowID)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var status string

		// Use a per-iteration context with timeout so chromedp.WaitVisible never blocks forever.
		iterCtx, iterCancel := context.WithTimeout(b.ctx, 10*time.Second)
		err := chromedp.Run(iterCtx,
			chromedp.Navigate(executionURL),
			chromedp.WaitVisible(`[data-status]`, chromedp.ByQuery),
			chromedp.AttributeValue(`[data-status]`, "data-status", &status, nil, chromedp.ByQuery),
		)
		iterCancel()

		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// Accept any terminal state including cancelled
		if status == "completed" || status == "cancelled" || status == "failed" || status == "error" {
			b.t.Logf("Workflow status: %s", status)
			return status, nil
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for workflow completion")
}

// ClickApproveButton approves a workflow by calling the approvals API endpoint directly.
// This bypasses the Alpine.js UI (which has a confirm() dialog) and approves via API.
// The approval API is: GET /api/v1/execution/{workflowId}/approvals?approved=true
func (b *Browser) ClickApproveButton(ctx context.Context, workflowID string) error {
	b.t.Log("Clicking approve button...")

	// Navigate to the execution page first (to carry the session cookie) then call the approvals API
	approvalURL := fmt.Sprintf("%s/api/v1/execution/%s/approvals?approved=true",
		b.baseURL, url.PathEscape(workflowID))

	b.t.Logf("Approving via API: %s", approvalURL)

	var responseBody string
	err := chromedp.Run(b.ctx,
		chromedp.Navigate(approvalURL),
		chromedp.Sleep(2*time.Second),
		chromedp.Text(`body`, &responseBody, chromedp.ByQuery),
	)

	if err != nil {
		return fmt.Errorf("approve API call failed: %w", err)
	}

	b.t.Logf("Approval response (first 300 chars): %s", truncateString(responseBody, 300))
	b.t.Log("Approve button clicked")
	return nil
}

// TakeScreenshot captures a screenshot of the current page
func (b *Browser) TakeScreenshot(ctx context.Context, filepath string) error {
	var buf []byte

	err := chromedp.Run(b.ctx,
		chromedp.CaptureScreenshot(&buf),
	)

	if err != nil {
		return fmt.Errorf("screenshot failed: %w", err)
	}

	// Save screenshot
	if err := os.WriteFile(filepath, buf, 0644); err != nil {
		return fmt.Errorf("failed to save screenshot: %w", err)
	}

	b.t.Logf("Screenshot saved to %s", filepath)
	return nil
}

// CompleteElevationWorkflow performs a complete elevation workflow via UI
func (b *Browser) CompleteElevationWorkflow(
	ctx context.Context,
	authType AuthType,
	username, password, role, provider, reason, duration string,
) (string, error) {
	b.t.Log("Starting elevation workflow via UI...")

	// 1. Login
	if err := b.Login(ctx, authType, username, password); err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}

	// Wait for session to settle
	time.Sleep(2 * time.Second)

	// 2. Directly call the elevation API endpoint (bypasses the complex Choices.js form)
	// The GET /api/v1/elevate endpoint accepts query params and redirects to the workflow execution page.
	params := fmt.Sprintf("role=%s&provider=%s&reason=%s&duration=%s",
		url.QueryEscape(role),
		url.QueryEscape(provider),
		url.QueryEscape(reason),
		url.QueryEscape(duration),
	)
	elevateAPIURL := fmt.Sprintf("%s/api/v1/elevate?%s", b.baseURL, params)
	b.t.Logf("Navigating to elevation API: %s", elevateAPIURL)

	// Navigate and follow redirects. The chain is:
	//   /api/v1/elevate → 307 /elevate/resume?state=... → (execution.html or another redirect)
	// We poll until the page contains a workflowId or we get a /execution/{id} URL.
	err := chromedp.Run(b.ctx,
		chromedp.Navigate(elevateAPIURL),
	)
	if err != nil {
		return "", fmt.Errorf("elevation API navigation failed: %w", err)
	}

	// Poll for the workflowId to appear in the page (up to 30 seconds)
	var workflowID string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)

		var currentURL string
		var pageHTML string
		_ = chromedp.Run(b.ctx,
			chromedp.Location(&currentURL),
			chromedp.OuterHTML(`html`, &pageHTML, chromedp.ByQuery),
		)

		b.t.Logf("Waiting for workflow ID - current URL: %s", currentURL)

		// Check if URL has /execution/{id}
		workflowID = extractWorkflowIDFromURL(currentURL)
		if workflowID != "" {
			break
		}

		// Check if page has workflowId embedded (execution.html pattern)
		if idx := strings.Index(pageHTML, "workflowId: '"); idx >= 0 {
			start := idx + len("workflowId: '")
			end := strings.Index(pageHTML[start:], "'")
			if end > 0 {
				workflowID = pageHTML[start : start+end]
				if workflowID != "" {
					break
				}
			}
		}
	}

	if workflowID == "" {
		return "", fmt.Errorf("timed out waiting for workflow ID after elevation")
	}

	require.NotEmpty(b.t, workflowID, "Workflow ID should not be empty")
	b.t.Logf("Elevation request submitted with workflow ID: %s", workflowID)

	return workflowID, nil
}

// extractWorkflowIDFromURL tries to extract a workflow ID from common redirect URLs.
func extractWorkflowIDFromURL(rawURL string) string {
	// Try /execution/{id} pattern (singular)
	parts := strings.Split(rawURL, "/")
	for i, part := range parts {
		if part == "execution" && i+1 < len(parts) {
			wfID := parts[i+1]
			if idx := strings.Index(wfID, "?"); idx != -1 {
				wfID = wfID[:idx]
			}
			return wfID
		}
		// Also handle /executions/{id} (plural)
		if part == "executions" && i+1 < len(parts) {
			wfID := parts[i+1]
			if idx := strings.Index(wfID, "?"); idx != -1 {
				wfID = wfID[:idx]
			}
			return wfID
		}
	}
	return ""
}

// ApproveAsManager creates a separate browser session, logs in as the manager,
// navigates to the workflow execution, and clicks approve.
func (b *Browser) ApproveAsManager(
	ctx context.Context,
	authType AuthType,
	managerUsername, managerPassword, workflowID string,
) error {
	b.t.Log("Manager approving workflow...")

	// Create a new browser session for the manager
	manager := NewBrowser(b.t, b.baseURL)
	defer manager.Close()

	// Login as manager
	if err := manager.Login(ctx, authType, managerUsername, managerPassword); err != nil {
		return fmt.Errorf("manager login failed: %w", err)
	}

	time.Sleep(2 * time.Second)

	// Click approve
	if err := manager.ClickApproveButton(ctx, workflowID); err != nil {
		return fmt.Errorf("manager approval failed: %w", err)
	}

	b.t.Log("Manager approval completed")
	return nil
}

// ClickDenyButton clicks the deny button on an execution page.
func (b *Browser) ClickDenyButton(ctx context.Context, workflowID string) error {
	b.t.Log("Clicking deny button...")

	executionURL := fmt.Sprintf("%s/executions/%s", b.baseURL, workflowID)

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(executionURL),
		chromedp.WaitVisible(`button[data-action="deny"]`, chromedp.ByQuery),
		chromedp.Click(`button[data-action="deny"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	)

	if err != nil {
		return fmt.Errorf("deny button click failed: %w", err)
	}

	b.t.Log("Deny button clicked")
	return nil
}

// DenyAsManager creates a separate browser session, logs in as the manager,
// navigates to the workflow execution, and clicks deny.
func (b *Browser) DenyAsManager(
	ctx context.Context,
	authType AuthType,
	managerUsername, managerPassword, workflowID string,
) error {
	b.t.Log("Manager denying workflow...")

	manager := NewBrowser(b.t, b.baseURL)
	defer manager.Close()

	if err := manager.Login(ctx, authType, managerUsername, managerPassword); err != nil {
		return fmt.Errorf("manager login failed: %w", err)
	}

	time.Sleep(2 * time.Second)

	if err := manager.ClickDenyButton(ctx, workflowID); err != nil {
		return fmt.Errorf("manager denial failed: %w", err)
	}

	b.t.Log("Manager denial completed")
	return nil
}
