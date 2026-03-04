package ui_e2e

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
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

// Login performs login via OIDC.
func (b *Browser) Login(ctx context.Context, username, password string) error {
	return b.LoginWithOIDC(ctx, username, password)
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

// selectChoicesOption selects an option in a Choices.js-enhanced select by the original <select> element ID.
// It opens the dropdown, filters by typing the search value, and clicks the matching item.
func (b *Browser) selectChoicesOption(selectID, value string) error {
	// Open the Choices.js dropdown by clicking its inner container.
	openJS := fmt.Sprintf(
		`document.querySelector('#%s').closest('.choices').querySelector('.choices__inner').click()`,
		selectID,
	)
	// Click the matching Choices.js item by data-value (the role identifier), not by label text.
	// Searching by typing would filter on label text and miss identifier-only matches.
	clickJS := fmt.Sprintf(
		`(function() {
			const wrapper = document.querySelector('#%s').closest('.choices');
			const items = wrapper.querySelectorAll('.choices__item--choice:not(.choices__item--disabled)');
			for (const item of items) {
				if (item.dataset.value === %q) {
					item.click();
					return true;
				}
			}
			return false;
		})()`,
		selectID, value,
	)

	// Open the dropdown.
	if err := chromedp.Run(b.ctx,
		chromedp.Evaluate(openJS, nil),
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		return fmt.Errorf("failed to open Choices.js dropdown #%s: %w", selectID, err)
	}

	// Retry clicking the matching item until it appears (API may still be loading).
	var clicked bool
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(clickJS, &clicked)); err == nil && clicked {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !clicked {
		return fmt.Errorf("could not find Choices.js option with value %q in #%s", value, selectID)
	}
	b.t.Logf("Selected Choices.js option %q in #%s", value, selectID)
	return nil
}

// FillElevateForm fills out the elevation request form on /elevate/static.
// It selects the provider (Alpine.js native select), waits for roles to load,
// then selects the role via the Choices.js dropdown, sets duration and reason.
func (b *Browser) FillElevateForm(ctx context.Context, role, provider, reason, duration string) error {
	b.t.Log("Filling elevate form...")

	// 1. Wait for Alpine.js to populate the provider select options, then select the provider.
	//    "option[value!='']" is not valid CSS (jQuery-only), so we poll via JS instead.
	b.t.Logf("Selecting provider: %s", provider)
	waitProviderJS := fmt.Sprintf(
		`(function() {
			const sel = document.querySelector('select[name="provider"]');
			if (!sel) return false;
			for (const opt of sel.options) {
				if (opt.value === %q) return true;
			}
			return false;
		})()`,
		provider,
	)
	var providerReady bool
	providerDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(providerDeadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(waitProviderJS, &providerReady)); err == nil && providerReady {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !providerReady {
		return fmt.Errorf("timed out waiting for provider option %q to appear in select", provider)
	}

	err := chromedp.Run(b.ctx,
		chromedp.SetValue(`select[name="provider"]`, provider, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)
	if err != nil {
		return fmt.Errorf("failed to select provider %q: %w", provider, err)
	}

	// 2. Wait for the role Choices.js dropdown to be populated (API call triggered by provider change).
	//    The original <select id="elevate-roles"> is hidden by Choices.js, so we cannot use
	//    WaitVisible on it. Instead poll via JS until at least one non-disabled choice item appears.
	b.t.Logf("Waiting for roles to load for provider %s...", provider)
	waitRolesJS := `(function() {
		const sel = document.querySelector('#elevate-roles');
		if (!sel) return false;
		const wrapper = sel.closest('.choices');
		if (!wrapper) return false;
		const items = wrapper.querySelectorAll('.choices__item--choice:not(.choices__item--disabled)');
		return items.length > 0;
	})()`
	var rolesReady bool
	rolesDeadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(rolesDeadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(waitRolesJS, &rolesReady)); err == nil && rolesReady {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !rolesReady {
		return fmt.Errorf("timed out waiting for roles to load into Choices.js #elevate-roles")
	}

	// 3. Select role by injecting the value directly into the native <select>.
	//    Choices.js v10's setChoices() builds only the custom UI; it does NOT add <option>
	//    elements back to the native hidden <select> for dynamically-loaded data.
	//    Since we use form.submit() (which reads native controls directly), we inject the
	//    <option selected> into the native element ourselves.
	b.t.Logf("Selecting role: %s", role)
	selectRoleJS := fmt.Sprintf(
		`(function() {
			const sel = document.getElementById('elevate-roles');
			if (!sel) return 'no-select';
			// Remove any existing options and inject only the desired one as selected.
			while (sel.options.length > 0) { sel.remove(0); }
			const opt = document.createElement('option');
			opt.value = %q;
			opt.text = %q;
			opt.selected = true;
			sel.add(opt);
			return sel.value === %q ? 'ok' : 'mismatch:' + sel.value;
		})()`,
		role, role, role,
	)
	var roleResult string
	roleDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(roleDeadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(selectRoleJS, &roleResult)); err == nil && roleResult == "ok" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if roleResult != "ok" {
		return fmt.Errorf("failed to inject role %q into native #elevate-roles select: %q", role, roleResult)
	}
	b.t.Logf("Injected role %q into native select", role)

	// 4. Set duration via Alpine.js native select.
	b.t.Logf("Setting duration: %s", duration)
	err = chromedp.Run(b.ctx,
		chromedp.SetValue(`select[name="duration"]`, duration, chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("failed to set duration %q: %w", duration, err)
	}

	// 5. Fill reason using SetValue (dispatches input+change for Alpine x-model).
	b.t.Logf("Filling reason: %s", reason)
	err = chromedp.Run(b.ctx,
		chromedp.WaitVisible(`textarea[name="reason"]`, chromedp.ByQuery),
		chromedp.SetValue(`textarea[name="reason"]`, reason, chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("failed to fill reason: %w", err)
	}

	// 6. Ensure at least one identity is selected in the native <select name="identities">.
	//    The server pre-fills the current user with selected:true via PREFILLED_IDENTITIES, so it
	//    should already be set.  We verify and, if not pre-selected, select the first non-empty option.
	//    form.submit() reads the native <select>, so Choices.js state is irrelevant here.
	b.t.Log("Ensuring identity is selected...")
	ensureIdentityJS := `(function() {
		const sel = document.getElementById('elevate-identities');
		if (!sel) return 'no-select';
		// Check if any option is already selected.
		const already = Array.from(sel.options).find(o => o.selected && o.value !== '');
		if (already) return 'pre-selected:' + already.value;
		// Otherwise, select the first non-empty option.
		const first = Array.from(sel.options).find(o => o.value !== '');
		if (!first) return 'no-options';
		first.selected = true;
		return 'selected:' + first.value;
	})()`
	var identityResult string
	idDeadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(idDeadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(ensureIdentityJS, &identityResult)); err == nil &&
			identityResult != "" && identityResult != "no-select" && identityResult != "no-options" {
			b.t.Logf("Identity selection: %s", identityResult)
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if identityResult == "no-select" || identityResult == "no-options" {
		b.t.Logf("Warning: could not ensure identity selection (result: %q)", identityResult)
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

// WaitForWorkflowCompletion waits for a workflow to complete and returns the status.
// It polls the REST API via a browser-side fetch so the session cookie is included.
func (b *Browser) WaitForWorkflowCompletion(ctx context.Context, workflowID string, timeout time.Duration) (string, error) {
	b.t.Log("Waiting for workflow completion...")

	apiPath := fmt.Sprintf("/api/v1/execution/%s", url.PathEscape(workflowID))
	deadline := time.Now().Add(timeout)

	// JS snippet: fetch the execution API and return the status string (or "" on error/pending).
	fetchStatusJS := fmt.Sprintf(`(async function() {
		try {
			const r = await fetch(%q);
			if (!r.ok) return 'fetch_error_' + r.status;
			const d = await r.json();
			return (d.execution && d.execution.status) ? d.execution.status : '';
		} catch(e) { return 'js_error'; }
	})()`, apiPath)

	for time.Now().Before(deadline) {
		var rawStatus string

		iterCtx, iterCancel := context.WithTimeout(b.ctx, 12*time.Second)
		err := chromedp.Run(iterCtx, chromedp.Evaluate(fetchStatusJS, &rawStatus, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}))
		iterCancel()

		if err != nil {
			b.t.Logf("WaitForWorkflowCompletion fetch error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		statusUpper := strings.ToUpper(rawStatus)
		b.t.Logf("Workflow status via API: %s", rawStatus)

		// Accept any terminal state
		switch statusUpper {
		case "COMPLETED", "WORKFLOW_EXECUTION_STATUS_COMPLETED", "SUCCESS", "SUCCEEDED",
			"FAILED", "WORKFLOW_EXECUTION_STATUS_FAILED",
			"CANCELED", "CANCELLED", "WORKFLOW_EXECUTION_STATUS_CANCELED",
			"TERMINATED", "WORKFLOW_EXECUTION_STATUS_TERMINATED",
			"ERROR":
			return strings.ToLower(rawStatus), nil
		}

		time.Sleep(2 * time.Second)
	}

	return "", fmt.Errorf("timeout waiting for workflow completion")
}

// ClickApproveButton navigates to the execution page and clicks the Approve button.
// The button triggers a native confirm() dialog which is auto-accepted via a CDP listener.
func (b *Browser) ClickApproveButton(ctx context.Context, workflowID string) error {
	b.t.Log("Clicking approve button in execution UI...")

	executionURL := fmt.Sprintf("%s/execution/%s", b.baseURL, url.PathEscape(workflowID))

	// Navigate to the execution page.
	if err := chromedp.Run(b.ctx, chromedp.Navigate(executionURL)); err != nil {
		return fmt.Errorf("failed to navigate to execution page: %w", err)
	}

	// Register a listener that auto-accepts the native confirm() dialog that
	// signalApproval() shows before calling the approvals API.
	chromedp.ListenTarget(b.ctx, func(ev interface{}) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			b.t.Log("Auto-accepting confirm() dialog for approval")
			go func() {
				if err := chromedp.Run(b.ctx, page.HandleJavaScriptDialog(true)); err != nil {
					b.t.Logf("Warning: failed to handle dialog: %v", err)
				}
			}()
		}
	})

	// Wait for the Alpine.js-rendered Approve button to appear, then click it.
	// The button is inside an x-if block and only shows when the current task is 'approvals'.
	clickApproveJS := `(function() {
		const btns = document.querySelectorAll('button.button-primary');
		for (const btn of btns) {
			if (btn.textContent.trim() === 'Approve') {
				btn.click();
				return true;
			}
		}
		return false;
	})()`

	var clicked bool
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(clickApproveJS, &clicked)); err == nil && clicked {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !clicked {
		return fmt.Errorf("timed out waiting for Approve button on execution page %s", executionURL)
	}

	b.t.Log("Approve button clicked")
	// Wait for the approval signal to be dispatched and processed.
	time.Sleep(2 * time.Second)
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

// CompleteElevationWorkflow performs a complete elevation workflow by driving the
// /elevate/static UI form. It logs in, navigates to the form, selects provider
// and role (including Choices.js search interaction), sets duration and reason,
// submits the form, and returns the resulting workflow ID from the redirect URL.
func (b *Browser) CompleteElevationWorkflow(
	ctx context.Context,

	username, password, role, provider, reason, duration string,
) (string, error) {
	b.t.Log("Starting elevation workflow via UI form...")

	// 1. Login.
	if err := b.Login(ctx, username, password); err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}
	time.Sleep(2 * time.Second)

	// 2. Navigate to the static elevation form and wait for it to be ready.
	b.t.Log("Navigating to /elevate/static...")
	err := chromedp.Run(b.ctx,
		chromedp.Navigate(b.baseURL+"/elevate/static"),
		chromedp.WaitVisible(`form#elevate-form`, chromedp.ByQuery),
	)
	if err != nil {
		return "", fmt.Errorf("failed to load /elevate/static: %w", err)
	}

	// 3. Fill out provider, role (Choices.js), duration, and reason.
	if err := b.FillElevateForm(ctx, role, provider, reason, duration); err != nil {
		return "", fmt.Errorf("form fill failed: %w", err)
	}

	// 4. Construct the submit URL from the native form, then navigate to it directly.
	//    The form is method="GET", so form submission is just a navigation to a URL.
	//    Using chromedp.Navigate() is more reliable than form.submit() because chromedp
	//    waits for the full navigation (including 307 redirect) to complete before returning.
	//    new FormData() picks up our injected native options (role, identity) correctly.
	b.t.Log("Building submit URL from form...")
	var submitURL string
	err = chromedp.Run(b.ctx, chromedp.Evaluate(
		`(function() {
			const form = document.getElementById('elevate-form');
			const params = new URLSearchParams(new FormData(form));
			return form.action + '?' + params.toString();
		})()`,
		&submitURL,
	))
	if err != nil || submitURL == "" {
		return "", fmt.Errorf("failed to build form submit URL: %w", err)
	}
	b.t.Logf("Navigating to submit URL: %s", submitURL)

	var currentURL string
	err = chromedp.Run(b.ctx,
		// chromedp.Navigate follows 307 redirects and waits for the final page to load.
		chromedp.Navigate(submitURL),
		chromedp.Location(&currentURL),
	)
	if err != nil {
		return "", fmt.Errorf("form submission / redirect failed: %w", err)
	}
	b.t.Logf("After submit - URL: %s", currentURL)

	// The server renders execution.html inline at the /api/v1/elevate/resume URL.
	// The Go template embeds the workflow ID in JS as: workflowId: 'thand_xxx'.
	// Extract it directly from the rendered script tag.
	var workflowID string
	extractJS := `(function() {
		const scripts = document.querySelectorAll('script');
		for (const s of scripts) {
			const m = s.textContent.match(/workflowId:\s*'([^']+)'/);
			if (m) return m[1];
		}
		return '';
	})()`

	// Poll briefly in case the page is still rendering.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(extractJS, &workflowID)); err == nil && workflowID != "" {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if workflowID == "" {
		// Fallback: try extracting from the URL (works if the server did a real redirect).
		workflowID = extractWorkflowIDFromURL(currentURL)
	}

	if workflowID == "" {
		return "", fmt.Errorf("could not extract workflow ID from page (URL: %s)", currentURL)
	}

	b.t.Logf("Elevation workflow started with ID: %s", workflowID)
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

	managerUsername, managerPassword, workflowID string,
) error {
	b.t.Log("Manager approving workflow...")

	// Create a new browser session for the manager
	manager := NewBrowser(b.t, b.baseURL)
	defer manager.Close()

	// Login as manager
	if err := manager.Login(ctx, managerUsername, managerPassword); err != nil {
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

// ClickDenyButton navigates to the execution page and clicks the Reject button.
// The button triggers a native confirm() dialog which is auto-accepted via a CDP listener.
func (b *Browser) ClickDenyButton(ctx context.Context, workflowID string) error {
	b.t.Log("Clicking reject button in execution UI...")

	executionURL := fmt.Sprintf("%s/execution/%s", b.baseURL, url.PathEscape(workflowID))

	// Navigate to the execution page.
	if err := chromedp.Run(b.ctx, chromedp.Navigate(executionURL)); err != nil {
		return fmt.Errorf("failed to navigate to execution page: %w", err)
	}

	// Register a listener that auto-accepts the confirm() dialog.
	chromedp.ListenTarget(b.ctx, func(ev interface{}) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			b.t.Log("Auto-accepting confirm() dialog for rejection")
			go func() {
				if err := chromedp.Run(b.ctx, page.HandleJavaScriptDialog(true)); err != nil {
					b.t.Logf("Warning: failed to handle dialog: %v", err)
				}
			}()
		}
	})

	// Poll for the Alpine.js-rendered Reject button (button-destructive, text "Reject").
	clickRejectJS := `(function() {
		const btns = document.querySelectorAll('button.button-destructive');
		for (const btn of btns) {
			if (btn.textContent.trim() === 'Reject') {
				btn.click();
				return true;
			}
		}
		return false;
	})()`

	var clicked bool
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(clickRejectJS, &clicked)); err == nil && clicked {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !clicked {
		return fmt.Errorf("timed out waiting for Reject button on execution page %s", executionURL)
	}

	b.t.Log("Reject button clicked")
	// Wait for the rejection signal to be dispatched and processed.
	time.Sleep(2 * time.Second)
	return nil
}

// CancelWorkflowViaUI navigates to the execution page and clicks the "Cancel Request" button.
// The button triggers a native confirm() dialog which is auto-accepted via a CDP listener.
func (b *Browser) CancelWorkflowViaUI(ctx context.Context, workflowID string) error {
	b.t.Log("Cancelling workflow via UI...")

	executionURL := fmt.Sprintf("%s/execution/%s", b.baseURL, url.PathEscape(workflowID))

	// Navigate to the execution page.
	if err := chromedp.Run(b.ctx, chromedp.Navigate(executionURL)); err != nil {
		return fmt.Errorf("failed to navigate to execution page: %w", err)
	}

	// Register a listener that auto-accepts the confirm() dialog.
	chromedp.ListenTarget(b.ctx, func(ev interface{}) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			b.t.Log("Auto-accepting confirm() dialog for cancel")
			go func() {
				if err := chromedp.Run(b.ctx, page.HandleJavaScriptDialog(true)); err != nil {
					b.t.Logf("Warning: failed to handle dialog: %v", err)
				}
			}()
		}
	})

	// Poll for the enabled "Cancel Request" button.
	clickCancelJS := `(function() {
		const btns = document.querySelectorAll('button');
		for (const btn of btns) {
			if (btn.textContent.trim() === 'Cancel Request' && !btn.disabled) {
				btn.click();
				return true;
			}
		}
		return false;
	})()`

	var clicked bool
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := chromedp.Run(b.ctx, chromedp.Evaluate(clickCancelJS, &clicked)); err == nil && clicked {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !clicked {
		return fmt.Errorf("timed out waiting for Cancel Request button on execution page %s", executionURL)
	}

	b.t.Log("Cancel Request button clicked")
	// Wait for the cancel signal to be dispatched.
	time.Sleep(2 * time.Second)
	return nil
}

// DenyAsManager creates a separate browser session, logs in as the manager,
// navigates to the workflow execution, and clicks deny.
func (b *Browser) DenyAsManager(
	ctx context.Context,

	managerUsername, managerPassword, workflowID string,
) error {
	b.t.Log("Manager denying workflow...")

	manager := NewBrowser(b.t, b.baseURL)
	defer manager.Close()

	if err := manager.Login(ctx, managerUsername, managerPassword); err != nil {
		return fmt.Errorf("manager login failed: %w", err)
	}

	time.Sleep(2 * time.Second)

	if err := manager.ClickDenyButton(ctx, workflowID); err != nil {
		return fmt.Errorf("manager denial failed: %w", err)
	}

	b.t.Log("Manager denial completed")
	return nil
}
