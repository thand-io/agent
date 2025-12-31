package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

/*
This handles requests to thand.io cloud services. The AI figures out what
access you need and requests it on your behalf.

1. Bounce the user to thand.io login service.
2. After login, the agent gets back its session JWT.
3. The requested workflow workflow is then executed on thand.io
4. The response/status of the workflow workflow is returned to the user in the CLI.
*/
var requestCmd = &cobra.Command{
	Use:     "request",
	Short:   "Request access to resources",
	Long:    `Request just-in-time access to cloud infrastructure or SaaS applications`,
	PreRunE: preRunClientConfigWithSessionE,
	Run: func(cmd *cobra.Command, args []string) {

		reason := strings.TrimSpace(strings.Join(args, " "))

		if len(reason) == 0 {
			fmt.Println(errorStyle.Render("Reason for request is required"))

			// show usage
			cmd.Usage()
			return
		}

		// This is an AI request so lets call the login server to generate our role

		fmt.Println(successStyle.Render("Generating request .."))

		client := resty.New()

		loginSessions, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())

		if err != nil {
			return
		}

		_, session, err := loginSessions.GetFirstActiveSession()

		if err != nil {
			return
		}

		loginServerUrl := cfg.GetLoginServerUrl()

		if len(session.Endpoint) > 0 && !strings.EqualFold(session.Endpoint, cfg.GetLoginServerUrl()) {
			logrus.Infof("Updating login server URL from session endpoint: %s", session.Endpoint)
			loginServerUrl = session.Endpoint
		}

		loginServer := strings.TrimSuffix(cfg.DiscoverLoginServerApiUrl(
			loginServerUrl,
		), "/")

		evaluateLlmUrl := fmt.Sprintf("%s/elevate/llm", loginServer)

		res, err := client.R().
			EnableTrace().
			SetAuthToken(session.GetEncodedLocalSession()).
			SetBody(&models.ElevateLLMRequest{
				Reason: reason,
			}).
			Post(evaluateLlmUrl)

		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"endpoint": evaluateLlmUrl,
			}).Error("failed to send elevation request")
			fmt.Println(errorStyle.Render("Failed to send elevation request"))
			return
		}

		if res.StatusCode() != http.StatusOK {

			// Try and convert the error to a user-friendly message
			var errorResponse models.ErrorResponse
			if err := json.Unmarshal(res.Body(), &errorResponse); err == nil {
				fmt.Println(errorStyle.Render(
					fmt.Sprintf(
						"Failed to elevate access: %s. Reason: %s",
						errorResponse.Title, errorResponse.Message,
					)))
			} else {
				logrus.WithError(err).WithFields(logrus.Fields{
					"endpoint": evaluateLlmUrl,
					"response": res.String(),
				}).Errorf("failed to elevate access")
			}
			return
		}

		// Get json output to models.ElevateRequest

		var elevateRequest models.ElevateRequest
		if err := json.Unmarshal(res.Body(), &elevateRequest); err != nil {
			logrus.Errorf("failed to unmarshal elevation request: %v", err)
			return
		}

		err = MakeElevationRequest(&elevateRequest)

		if err != nil {
			logrus.Errorf("failed to make elevation request: %v", err)
			return
		}
	},
}

func MakeElevationRequest(request *models.ElevateRequest) error {

	if err := validateElevationRequest(request); err != nil {
		return err
	}

	if len(request.Workflow) == 0 {
		// Default to first workflow in role
		if len(request.Role.Workflows) == 0 {
			return fmt.Errorf("no workflow specified and role has no associated workflows")
		}

		request.Workflow = request.Role.Workflows[0]
	}

	if len(request.Authenticator) == 0 {

		// First try to see if we have an authenticator matching the provider
		foundProvider, localSession, err := sessionManager.GetFirstActiveSession(
			cfg.GetLoginServerHostname(),
			request.Role.Authenticators...)

		// If we have a valid session for one of the role's authenticators then use it
		if err == nil && localSession != nil {
			request.Authenticator = foundProvider
			request.Session = localSession
		}
	}

	if err := ensureValidSession(request); err != nil {
		return err
	}

	response, err := sendElevationRequest(request)

	if err != nil {
		return err
	}

	return handleElevationResponse(request, response)
}

func validateElevationRequest(request *models.ElevateRequest) error {
	if request == nil {
		return fmt.Errorf("invalid request: nil")
	}
	if len(request.Reason) == 0 {
		return fmt.Errorf("invalid request: empty reason")
	}
	if request.Role == nil {
		return fmt.Errorf("invalid request: nil role")
	}
	if len(request.Providers) == 0 {
		return fmt.Errorf("invalid request: no providers")
	}
	if _, err := common.ValidateDuration(request.Duration); err != nil {
		return fmt.Errorf("invalid request: duration must be greater than zero")
	}
	return nil
}

func ensureValidSession(request *models.ElevateRequest) error {

	if request.Session == nil || isSessionExpired(request.Session) {
		return authenticateUser(request)
	}

	return nil
}

func isSessionExpired(session *models.LocalSession) bool {
	if session == nil {
		return true
	}
	return time.Now().UTC().After(session.Expiry.UTC())
}

func authenticateUser(request *models.ElevateRequest) error {

	callbackUrl := url.Values{
		"callback": {cfg.GetLocalServerUrl()},
		"code":     {createAuthCode()},
	}

	if len(request.Authenticator) > 0 {
		callbackUrl.Set("provider", request.Authenticator)
	}

	authUrl := fmt.Sprintf("%s/auth?%s", cfg.GetLoginServerUrl(), callbackUrl.Encode())

	fmt.Printf("Opening browser to: %s with callback to: %s\n", authUrl, cfg.GetLocalServerUrl())

	if err := openBrowser(authUrl); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	// If an auth provider is specified then we need to wait for it to be
	// completed before we can get the session
	// This is useful for SSO providers where the user must complete
	// the auth in the browser
	if len(request.Authenticator) > 0 {

		if found := sessionManager.AwaitProviderRefresh(
			context.Background(),
			cfg.GetLoginServerHostname(),
			request.Authenticator,
		); found == nil {
			return fmt.Errorf("failed to await provider refresh. Authentication timed out or failed")
		}

		session, err := sessionManager.GetSession(
			cfg.GetLoginServerHostname(), request.Authenticator)

		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}

		request.Session = session

	} else {

		// If no auth provider is specified then we just wait for any
		// valid session to be created
		sessionHandler := sessionManager.AwaitRefresh(
			context.Background(), cfg.GetLoginServerHostname())

		foundProvider, session, err := sessionHandler.GetFirstActiveSession()

		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}

		request.Authenticator = foundProvider
		request.Session = session

	}
	return nil
}

func sendElevationRequest(request *models.ElevateRequest) (*resty.Response, error) {
	baseUrl := fmt.Sprintf("%s/%s",
		strings.TrimPrefix(cfg.GetLoginServerUrl(), "/"),
		strings.TrimPrefix(cfg.GetApiBasePath(), "/"))
	elevateUrl := fmt.Sprintf("%s/elevate", baseUrl)

	client := resty.New()
	client.SetRedirectPolicy(logRedirectWorkflow())

	res, err := client.R().
		EnableTrace().
		SetAuthToken(request.Session.GetEncodedLocalSession()).
		SetBody(request).
		Post(elevateUrl)

	if err != nil {
		return nil, fmt.Errorf("failed to send elevation request: %w", err)
	}

	return res, nil
}

func handleElevationResponse(request *models.ElevateRequest, res *resty.Response) error {
	if res.StatusCode() == http.StatusOK {
		return handleSuccessResponse(request, res)
	}
	return handleErrorResponse(request, res)
}

func handleSuccessResponse(request *models.ElevateRequest, res *resty.Response) error {
	var elevateResponse models.ElevateResponse
	if err := json.Unmarshal(res.Body(), &elevateResponse); err != nil {
		logrus.Errorf("failed to unmarshal elevation response: %v", err)
		return err
	}

	fmt.Println()
	displayStatusMessage(request, &elevateResponse)
	fmt.Println()
	return nil
}

func displayStatusMessage(request *models.ElevateRequest, response *models.ElevateResponse) {
	switch response.Status {
	case ctx.CompletedStatus:
		fmt.Println(successStyle.Render("Elevation Complete!"))
	case ctx.FaultedStatus:
		fmt.Println(warningStyle.Render("Elevation Failed"))
	case ctx.CancelledStatus:
		fmt.Println(warningStyle.Render("Elevation Cancelled"))
	case ctx.SuspendedStatus:
		fmt.Println(warningStyle.Render("⏸️ Elevation Suspended"))
	case ctx.PendingStatus:
		fallthrough
	case ctx.WaitingStatus:
		fallthrough
	case ctx.RunningStatus:

		// Extract workflow ID from output and try to get live updates
		if err := getElevationStatus(request, response); err != nil {
			// If live updates fail, just show the current status
			fmt.Println(warningStyle.Render(fmt.Sprintf("Status: %s (live updates unavailable)", response.Status)))
		}

	default:
		fmt.Println(warningStyle.Render(fmt.Sprintf("Unknown Status: %s", response.Status)))
	}
}

func handleErrorResponse(request *models.ElevateRequest, res *resty.Response) error {
	var errorResponse models.ErrorResponse
	if err := json.Unmarshal(res.Body(), &errorResponse); err != nil {
		logrus.Errorf("failed to unmarshal error response: %v", err)
		return err
	}

	logrus.WithFields(logrus.Fields{
		"request": request,
		"error":   errorResponse,
	}).Error("Failed to elevate access")

	return fmt.Errorf("failed to elevate access: %s with details: %s", errorResponse.Title, errorResponse.Message)
}

func openBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // linux, freebsd, openbsd, netbsd
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

func logRedirectWorkflow() resty.RedirectPolicy {
	return resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {

		// If the redirect URL does not match the underlying server URL then we need
		// to open the request in the browser
		if req.URL.Host != via[0].URL.Host {

			err := openBrowser(req.URL.String())
			if err != nil {
				return fmt.Errorf("failed to open browser: %w", err)
			}

			return fmt.Errorf("please complete the authentication request in your browser")

		}

		urlQuery := req.URL.Query()

		// Parse the URL to get the next task name
		nextTaskName := urlQuery.Get("taskName")

		if len(nextTaskName) == 0 {
			nextTaskName = "initializing"
		}

		taskStatus := urlQuery.Get("taskStatus")

		if len(taskStatus) == 0 {
			taskStatus = "running"
		}

		fmt.Printf("redirecting .. %s (%s)\n", nextTaskName, taskStatus)

		return nil
	})
}

func getElevationStatus(request *models.ElevateRequest, response *models.ElevateResponse) error {
	// Get server URL and auth token for the API call
	baseUrl := fmt.Sprintf("%s/%s",
		strings.TrimPrefix(cfg.GetLoginServerUrl(), "/"),
		strings.TrimPrefix(cfg.GetApiBasePath(), "/"))

	authToken := request.Session.GetEncodedLocalSession()

	// Try to run the TUI for live status updates
	err := runWorkflowStatusTUI(response.WorkflowId, baseUrl, authToken)
	if err != nil {
		return fmt.Errorf("failed to show live status: %w", err)
	}

	return nil
}

func init() {

	// Add subcommands
	rootCmd.AddCommand(requestCmd) // Request without access uses the LLM to figure out the role

}
