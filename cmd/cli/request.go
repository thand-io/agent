package cli

import (
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
	PreRunE: preRunServerE,
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

		loginServer := strings.TrimSuffix(cfg.GetLoginServerApiUrl(), "/")
		evaluateReason := fmt.Sprintf("%s/elevate/llm", loginServer)

		client := resty.New()

		loginSessions, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())

		if err != nil {
			return
		}

		sesh, err := loginSessions.GetFirstActiveSession()

		if err != nil {
			return
		}

		res, err := client.R().
			EnableTrace().
			SetAuthToken(sesh.GetEncodedLocalSession()).
			SetBody(&models.ElevateLLMRequest{
				Reason: reason,
			}).
			Post(evaluateReason)

		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"endpoint": evaluateReason,
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
					"endpoint": evaluateReason,
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
		if len(request.Role.Workflows) == 0 {
			return fmt.Errorf("no workflow specified and role has no associated workflows")
		}

		request.Workflow = request.Role.Workflows[0]
	}

	if err := ensureValidSession(request); err != nil {
		return err
	}

	response, err := sendElevationRequest(request)
	if err != nil {
		return err
	}

	return handleElevationResponse(response, request)
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
	session, err := sessionManager.GetSession(
		cfg.GetLoginServerHostname(), request.Authenticator)
	request.Session = session

	if err != nil || isSessionExpired(session) {
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

		if err := sessionManager.AwaitProviderRefresh(
			cfg.GetLoginServerHostname(), request.Authenticator); err != nil {
			return fmt.Errorf("failed to await provider refresh: %w", err)
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
			cfg.GetLoginServerHostname())

		session, err := sessionHandler.GetFirstActiveSession()

		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}

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

func handleElevationResponse(res *resty.Response, request *models.ElevateRequest) error {
	if res.StatusCode() == http.StatusOK {
		return handleSuccessResponse(res)
	}
	return handleErrorResponse(res, request)
}

func handleSuccessResponse(res *resty.Response) error {
	var elevateResponse models.ElevateResponse
	if err := json.Unmarshal(res.Body(), &elevateResponse); err != nil {
		logrus.Errorf("failed to unmarshal elevation response: %v", err)
		return err
	}

	fmt.Println()
	displayStatusMessage(elevateResponse.Status)
	fmt.Println()
	return nil
}

func displayStatusMessage(status ctx.StatusPhase) {
	switch status {
	case ctx.CompletedStatus:
		fmt.Println(successStyle.Render("Elevation Complete!"))
	case ctx.WaitingStatus:
		fmt.Println(warningStyle.Render("⏳ Elevation Pending... Waiting for user action"))
	case ctx.FaultedStatus:
		fmt.Println(warningStyle.Render("Elevation Failed"))
	case ctx.CancelledStatus:
		fmt.Println(warningStyle.Render("Elevation Cancelled"))
	case ctx.RunningStatus:
		fmt.Println(warningStyle.Render("⏳ Elevation In Progress..."))
	case ctx.SuspendedStatus:
		fmt.Println(warningStyle.Render("⏸️ Elevation Suspended"))
	case ctx.PendingStatus:
		fmt.Println(warningStyle.Render("⏳ Elevation Pending..."))
	default:
		fmt.Println(warningStyle.Render(fmt.Sprintf("Unknown Status: %s", status)))
	}
}

func handleErrorResponse(res *resty.Response, request *models.ElevateRequest) error {
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
		// Parse the URL to get the next task name
		nextTaskName := req.URL.Query().Get("task")

		if len(nextTaskName) == 0 {
			nextTaskName = "unknown"
		}

		fmt.Printf("redirecting .. %s\n", nextTaskName)

		return nil
	})
}

func init() {

	// Add subcommands
	rootCmd.AddCommand(requestCmd) // Request without access uses the LLM to figure out the role

}
