package ui_e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
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

// LoginWithOIDC performs OIDC login flow
func (b *Browser) LoginWithOIDC(ctx context.Context, username, password string) error {
	b.t.Log("Logging in with OIDC...")

	loginURL := b.baseURL + "/auth/login"

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`button[data-provider="oidc"]`, chromedp.ByQuery),
		chromedp.Click(`button[data-provider="oidc"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`input[name="username"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="username"]`, username, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="password"]`, password, chromedp.ByQuery),
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second), // Wait for redirect
	)

	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	b.t.Log("Login completed")
	return nil
}

// NavigateToElevatePage navigates to the elevate page
func (b *Browser) NavigateToElevatePage(ctx context.Context) error {
	b.t.Log("Navigating to elevate page...")

	elevateURL := b.baseURL + "/elevate"

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(elevateURL),
		chromedp.WaitVisible(`form`, chromedp.ByQuery),
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
		chromedp.SendKeys(`input[name="reason"]`, reason, chromedp.ByQuery),
		chromedp.SendKeys(`input[name="duration"]`, duration, chromedp.ByQuery),
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

	executionURL := fmt.Sprintf("%s/executions/%s", b.baseURL, workflowID)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		var status string

		err := chromedp.Run(b.ctx,
			chromedp.Navigate(executionURL),
			chromedp.WaitVisible(`[data-status]`, chromedp.ByQuery),
			chromedp.AttributeValue(`[data-status]`, "data-status", &status, nil, chromedp.ByQuery),
		)

		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if status == "completed" || status == "failed" || status == "error" {
			b.t.Logf("Workflow status: %s", status)
			return status, nil
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for workflow completion")
}

// ClickApproveButton clicks the approve button on an approval form
func (b *Browser) ClickApproveButton(ctx context.Context, workflowID string) error {
	b.t.Log("Clicking approve button...")

	executionURL := fmt.Sprintf("%s/executions/%s", b.baseURL, workflowID)

	err := chromedp.Run(b.ctx,
		chromedp.Navigate(executionURL),
		chromedp.WaitVisible(`button[data-action="approve"]`, chromedp.ByQuery),
		chromedp.Click(`button[data-action="approve"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second), // Wait for submission
	)

	if err != nil {
		return fmt.Errorf("approve button click failed: %w", err)
	}

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
	username, password, role, provider, reason, duration string,
) (string, error) {
	b.t.Log("Starting elevation workflow via UI...")

	// 1. Login
	if err := b.LoginWithOIDC(ctx, username, password); err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}

	// Wait for session to settle
	time.Sleep(2 * time.Second)

	// 2. Navigate to elevate page
	if err := b.NavigateToElevatePage(ctx); err != nil {
		return "", fmt.Errorf("navigation failed: %w", err)
	}

	// 3. Fill the form
	if err := b.FillElevateForm(ctx, role, provider, reason, duration); err != nil {
		return "", fmt.Errorf("form fill failed: %w", err)
	}

	// 4. Submit the form
	workflowID, err := b.SubmitElevateForm(ctx)
	if err != nil {
		return "", fmt.Errorf("form submission failed: %w", err)
	}

	require.NotEmpty(b.t, workflowID, "Workflow ID should not be empty")
	b.t.Logf("Elevation request submitted with workflow ID: %s", workflowID)

	return workflowID, nil
}
