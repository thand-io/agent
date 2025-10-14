package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/daemon/elevate/llm"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
)

// getElevate handles GET /api/v1/elevate?role=admin&target=server&reason=maintenance
func (s *Server) getElevate(c *gin.Context) {
	var request models.ElevateStaticRequest

	if err := c.ShouldBindQuery(&request); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload", err)
		return
	}

	role, err := s.Config.GetRoleByName(request.Role)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid role", err)
		return
	}

	primaryWorkflow := request.Workflow

	if len(primaryWorkflow) == 0 {
		if len(role.Workflows) == 0 {
			s.getErrorPage(c, http.StatusBadRequest, "No workflow specified and role has no associated workflows", err)
			return
		}
		primaryWorkflow = role.Workflows[0]
	}

	s.elevate(c, models.ElevateRequest{
		Role:       role,
		Providers:  []string{request.Provider},
		Identities: request.Identities,
		Workflow:   primaryWorkflow,
		Reason:     request.Reason,
		Duration:   request.Duration,
		Session:    request.Session,
	})
}

func (s *Server) postElevate(c *gin.Context) {
	// Check content type to determine how to bind the request
	contentType := c.GetHeader("Content-Type")

	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		// Handle form submission (legacy support)
		var dynamicRequest models.ElevateDynamicRequest
		if err := c.ShouldBind(&dynamicRequest); err != nil {
			s.getErrorPage(c, http.StatusBadRequest, "Invalid form data", err)
			return
		}
		s.handleDynamicRequest(c, dynamicRequest)
		return
	} else if strings.Contains(contentType, "application/json") {
		// Handle JSON submission (static/llm request)
		s.postElevateJSON(c)
		return
	} else {
		s.getErrorPage(c, http.StatusBadRequest, "Unsupported Content-Type. Use application/json or application/x-www-form-urlencoded")
		return
	}
}

func (s *Server) postElevateJSON(c *gin.Context) {
	// Read the request body
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to read request body", err)
		return
	}

	// Parse as raw JSON to detect request type
	var rawData map[string]any
	if err := json.Unmarshal(body, &rawData); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	// Check if this is a dynamic request (has providers array, permissions array, etc.)
	if providers, hasProviders := rawData["providers"].([]any); hasProviders && len(providers) > 0 {
		// This is a dynamic request
		var dynamicRequest models.ElevateDynamicRequest
		if err := json.Unmarshal(body, &dynamicRequest); err != nil {
			s.getErrorPage(c, http.StatusBadRequest, "Invalid dynamic request payload", err)
			return
		}
		s.handleDynamicRequest(c, dynamicRequest)
		return
	}

	// This is a standard elevation request
	var request models.ElevateRequest
	if err := json.Unmarshal(body, &request); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid standard request payload", err)
		return
	}

	s.elevate(c, request)
}

func (s *Server) handleDynamicRequest(c *gin.Context, dynamicRequest models.ElevateDynamicRequest) {

	// Validate required fields
	if len(dynamicRequest.Reason) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Reason is required")
		return
	}

	if len(dynamicRequest.Providers) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "At least one provider must be selected")
		return
	}

	// Check that either permissions or inherits is provided
	if len(dynamicRequest.Permissions) == 0 && len(dynamicRequest.Inherits) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Either permissions or role inheritance must be specified")
		return
	}

	// TODO: Convert ElevateDynamicRequest to ElevateRequest
	// For now, let's create a basic ElevateRequest to integrate with existing workflow

	// Create a dynamic role based on the request
	dynamicRole := &models.Role{
		Name:        "dynamic-role-" + time.Now().Format("20060102-150405"),
		Description: "Dynamically created role: " + dynamicRequest.Reason,
		Workflows:   []string{dynamicRequest.Workflow},
		Permissions: models.Permissions{
			Allow: dynamicRequest.Permissions,
		},
		Inherits:  dynamicRequest.Inherits,
		Providers: dynamicRequest.Providers,
		Enabled:   true,
		Resources: models.Resources{
			// TODO: Add resource constraints based on Groups/Users if needed
		},
		Applies: &models.RoleApplies{
			Groups: dynamicRequest.Groups,
			Users:  dynamicRequest.Users,
		},
	}

	// Convert to standard ElevateRequest
	elevateRequest := models.ElevateRequest{
		Role:       dynamicRole,
		Identities: dynamicRequest.Identities,
		Providers:  dynamicRequest.Providers, // Use first provider for now
		Workflow:   dynamicRequest.Workflow,
		Reason:     dynamicRequest.Reason,
		Duration:   dynamicRequest.Duration,
		Session:    nil, // Session will be handled by the workflow if needed
	}

	s.elevate(c, elevateRequest)
}

func (s *Server) elevate(c *gin.Context, request models.ElevateRequest) {

	// Increment elevate requests counter
	atomic.AddInt64(&s.ElevateRequests, 1)

	ctx := context.Background()

	// If we have a web session and one hasn't been set then
	// lets attach a user session to the request.
	if s.Config.IsServer() {

		// Get the auth provider from the workflow if set
		authProvider := []string{}

		if len(request.Workflow) > 0 {
			workflowDef, err := s.Config.GetWorkflowByName(request.Workflow)
			if err != nil {
				s.getErrorPage(c, http.StatusBadRequest, "Invalid workflow specified", err)
				return
			}
			authProvider = []string{workflowDef.GetAuthentication()}
		}

		foundUser, err := s.getUser(c, authProvider...)

		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
			return
		}

		if foundUser != nil {
			request.Session = foundUser.ToLocalSession(s.Config.GetServices().GetEncryption())
		}

	}

	workflowTask, err := s.Workflows.CreateWorkflow(ctx, request)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to execute workflow", err)
		return
	}

	// We now redirect the user to the next workflow step.
	c.Redirect(http.StatusTemporaryRedirect,
		workflowTask.GetRedirectURL(),
	)
}

func (s *Server) getElevateResume(c *gin.Context) {
	// This service is stateless so we need to resume the workflow
	// based on the request payload. We can store the state as 8KB JSON url.

	// get state from the query parameters
	state := c.Query("state")

	if len(state) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "State parameter is required")
		return
	}

	workflow, err := manager.CreateWorkflowFromEncodedTask(
		s.GetConfig().GetServices().GetEncryption(), state)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to create workflow from state", err)
		return
	}

	s.resumeWorkflow(c, workflow)

}

// postElevateResume handles POST /api/v1/elevate/resume
func (s *Server) postElevateResume(c *gin.Context) {

	// If the query param is provided then we are in a redirect
	// and should ignore the local body. The local body should
	// only be used for signals.

	if len(c.Query("state")) > 0 {
		s.getElevateResume(c)
		return
	}

	// Get raw body as string
	body, err := c.Request.GetBody()
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to read request body", err)
		return
	}

	// convert body to string
	encodedTask, err := io.ReadAll(body)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to read request body", err)
		return
	}

	workflow, err := manager.CreateWorkflowFromEncodedTask(
		s.Config.GetServices().GetEncryption(), string(encodedTask))
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to create workflow from state", err)
		return
	}

	s.resumeWorkflow(c, workflow)

}

func (s *Server) getElevateAuthOAuth2(c *gin.Context) {

	// Ok lets grab the state from the query and then
	// call the authority to get the user information.

	ctx := context.Background()

	state := c.Query("state")
	code := c.Query("code")

	if len(state) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "State parameter is required")
		return
	}

	workflowTask, err := manager.CreateWorkflowFromEncodedTask(
		s.GetConfig().GetServices().GetEncryption(), state)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to create workflow from state", err)
		return
	}

	authProvider := workflowTask.GetAuthenticationProvider()

	if len(authProvider) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Authentication provider not found")
		return
	}

	authProviderInstance, err := s.Config.GetProviderByName(authProvider)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to get auth provider", err)
		return
	}

	session, err := authProviderInstance.GetClient().CreateSession(ctx, &models.AuthorizeUser{
		Code:        code,
		State:       state,
		RedirectUri: s.Config.GetAuthCallbackUrl(authProvider),
	})

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to create session for elevation request", err)
		return
	}

	// Get the users identity information and role info.
	fmt.Println("Resuming workflow with state:", state)

	workflowTask.SetUser(session.User)

	localSession := session.ToLocalSession(s.Config.GetServices().GetEncryption())

	if err := s.setAuthCookie(c, authProvider, localSession); err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to set auth cookie", err)
		return
	}

	s.resumeWorkflow(c, workflowTask)

}

func (s *Server) resumeWorkflow(c *gin.Context, workflow *models.WorkflowTask) {

	// Provide no input to resume the workflow as it'll use the saved state
	// inputs are only for signals
	workflowTask, err := s.Workflows.ResumeWorkflow(
		workflow,
	)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to resume workflow", err)
		return
	}

	if workflowTask == nil {
		s.getErrorPage(c, http.StatusNotFound, "Workflow not found or already completed", err)
		return
	}

	logrus.WithFields(logrus.Fields{
		"task_name": workflowTask.GetTaskReference(),
		"state":     workflowTask.GetEncodedTask(s.GetConfig().GetServices().GetEncryption()),
	}).Info("Workflow is still running, redirecting to resume")

	if workflowTask.GetStatus() == ctx.RunningStatus {

		c.Redirect(http.StatusTemporaryRedirect,
			s.Config.GetResumeCallbackUrl(workflowTask),
		)

	} else if s.canAcceptHtml(c) {

		// If this is an API call return the JSON handler
		// otherwise return the html page

		data := ExecutionStatePageData{
			TemplateData: s.GetTemplateData(c),
			Execution: &models.WorkflowExecutionInfo{
				WorkflowID: workflowTask.WorkflowID,
			},
			Workflow: workflowTask.GetWorkflowDef(),
		}

		s.renderHtml(c, "execution.html", data)

	} else {

		c.JSON(http.StatusOK, models.ElevateResponse{
			Status: workflowTask.GetStatus(),
			Output: workflowTask.GetOutputAsMap(),
		})

	}

}

// getElevateLLM handles POST /elevate/llm?reason=I need access to aws
// This function is a handler to take a users reason for an
// elevation and response with a role based on the users request

func (s *Server) getElevateLLM(c *gin.Context) {

	// Get the reason from the query parameters
	reason := c.Query("reason")

	if len(reason) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Reason is required")
		return
	}

	elevateRequest := models.ElevateLLMRequest{
		Reason: reason,
	}

	s.handleLargeLanguageModelRequest(c, elevateRequest)
}

func (s *Server) postElevateLLM(c *gin.Context) {

	var elevateRequest models.ElevateLLMRequest
	if err := c.ShouldBindJSON(&elevateRequest); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload", err)
		return
	}

	s.handleLargeLanguageModelRequest(c, elevateRequest)

}

func (s *Server) handleLargeLanguageModelRequest(c *gin.Context, elevateRequest models.ElevateLLMRequest) {

	if !s.Config.HasLargeLanguageModel() {
		s.getErrorPage(c, http.StatusInternalServerError, "Gemini is not initialized")
		return
	}

	if len(elevateRequest.Reason) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Reason is required")
		return
	}

	providers := s.Config.GetProvidersByCapability(models.ProviderCapabilityRBAC)

	if len(providers) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "No providers with RBAC capability are configured")
		return
	}

	// Get user context
	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusBadRequest, "LLM Elevation is only available in server mode")
		return
	}

	foundUser, err := s.getUser(c)

	if err != nil {
		logrus.WithError(err).Error("failed to get user")
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for elevation", err)
		return
	}

	if foundUser == nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: user not found for elevation")
		return
	}

	workflows := s.Config.GetWorkflows().Definitions

	if len(workflows) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "No workflows are configured")
		return
	}

	elevateResponse, err := llm.GenerateElevateRequestFromReason(
		c.Request.Context(),
		s.Config.GetLargeLanguageModel(),
		providers,
		workflows,
		elevateRequest.Reason,
	)

	if err != nil {
		logrus.WithError(err).Error("failed to generate elevate request")
		s.getErrorPage(c, http.StatusBadRequest, "Failed to generate elevate request", err)
		return
	}

	c.JSON(http.StatusOK, elevateResponse)
}

// getElevatePage handles the request for the elevation page
func (s *Server) getElevatePage(c *gin.Context) {
	data := s.GetTemplateData(c)
	s.renderHtml(c, "elevate.html", data)
}

func (s *Server) getElevateStaticPage(c *gin.Context) {
	data := s.GetTemplateData(c)
	s.renderHtml(c, "elevate_static.html", data)
}

func (s *Server) getElevateDynamicPage(c *gin.Context) {
	data := s.GetTemplateData(c)
	s.renderHtml(c, "elevate_dynamic.html", data)
}

func (s *Server) getElevateLLMPage(c *gin.Context) {
	data := s.GetTemplateData(c)
	s.renderHtml(c, "elevate_llm.html", data)
}
