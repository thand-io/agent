package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	swfCtx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	internalapi "github.com/thand-io/agent/internal/api"
	"github.com/thand-io/agent/internal/daemon/elevate/llm"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/manager"
)

// getElevate handles GET /api/v1/elevate?role=admin&target=server&reason=maintenance
//
//	@Summary		Request role elevation
//	@Description	Request elevation to a specific role with static parameters
//	@Tags			elevate
//	@Accept			json
//	@Produce		json
//	@Param			role		query		string	true	"Role name"
//	@Param			provider	query		string	true	"Provider name"
//	@Param			reason		query		string	true	"Reason for elevation"
//	@Param			duration	query		string	false	"Duration of elevation"
//	@Param			workflow	query		string	false	"Workflow name"
//	@Param			identities	query		string	false	"Identity filter"
//	@Success		200			{object}	map[string]any	"Elevation request submitted"
//	@Failure		400			{object}	map[string]any	"Bad request"
//	@Router			/elevate [get]
func (s *Server) getElevate(c *gin.Context) {
	var request models.ElevateStaticRequest

	if err := c.ShouldBindQuery(&request); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload", err)
		return
	}

	logrus.WithFields(logrus.Fields{
		"provider": request.Provider,
		"role":     request.Role,
		"tenants":  request.Tenants,
	}).Debug("getElevate: Received elevation request")

	role, err := s.Config.GetRoleByName(request.Role)

	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid role", err)
		return
	}

	primaryWorkflow := request.Workflow

	if len(primaryWorkflow) == 0 {
		if len(role.Workflows) == 0 {
			s.getErrorPage(c, http.StatusBadRequest, "No workflow specified and role has no associated workflows")
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
		Tenants:    request.Tenants,
	})
}

// postElevate handles elevation requests with JSON or form data
//
//	@Summary		Submit elevation request
//	@Description	Submit an elevation request with dynamic or static parameters
//	@Tags			elevate
//	@Accept			json,x-www-form-urlencoded,multipart/form-data
//	@Produce		json
//	@Param			request	body		models.ElevateRequest	true	"Elevation request"
//	@Success		200		{object}	map[string]any	"Elevation request submitted"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Router			/elevate [post]
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

		// Manually parse bracket notation for scopes since Gin doesn't support nested form binding
		// Form fields: scopes[groups], scopes[users], scopes[domains]
		if values, ok := c.GetPostFormArray("scopes[groups]"); ok {
			dynamicRequest.Scopes.Groups = values
		}
		if values, ok := c.GetPostFormArray("scopes[users]"); ok {
			dynamicRequest.Scopes.Users = values
		}
		if values, ok := c.GetPostFormArray("scopes[domains]"); ok {
			dynamicRequest.Scopes.Domains = values
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

	// This is a standard elevation request
	var request models.ElevateRequest
	if err := json.Unmarshal(body, &request); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid standard request payload", err)
		return
	}

	if request.IsValid() {
		s.elevate(c, request)
		return
	}

	// Parse as raw JSON to detect request type
	var dynamicRequest models.ElevateDynamicRequest
	if err := json.Unmarshal(body, &dynamicRequest); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid dynamic request payload", err)
		return
	}
	s.handleDynamicRequest(c, dynamicRequest)

}

func (s *Server) handleDynamicRequest(c *gin.Context, dynamicRequest models.ElevateDynamicRequest) {

	// Capability gate: dynamic elevation can be disabled via config
	if !s.Config.IsDynamicElevationEnabled() {
		s.getErrorPage(c, http.StatusForbidden, "Dynamic elevation requests are disabled on this server")
		return
	}

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

	// Convert string permissions to Statements (backwards compatibility)
	allowStatements := make(models.RoleStatements, len(dynamicRequest.Permissions))
	for i, perm := range dynamicRequest.Permissions {
		allowStatements[i] = models.Statement{
			Operations: []string{perm},
			Targets:    []string{},
		}
	}

	// Merge groups and resources into permissions targets
	var allTargets []string
	allTargets = append(allTargets, dynamicRequest.Groups...)
	allTargets = append(allTargets, dynamicRequest.Resources...)

	// Add targets to allowStatements if there are any
	if len(allTargets) > 0 {
		if len(allowStatements) > 0 {
			allowStatements[0].Targets = append(allowStatements[0].Targets, allTargets...)
		} else {
			allowStatements = append(allowStatements, models.Statement{
				Targets: allTargets,
			})
		}
	}

	// Create a dynamic role based on the request
	dynamicRole := &models.Role{
		Name:        "dynamic-role-" + time.Now().Format("20060102-150405"),
		Description: "Dynamically created role: " + dynamicRequest.Reason,
		Workflows:   []string{dynamicRequest.Workflow},
		Permissions: models.RolePermissions{
			Allow: allowStatements,
		},
		Inherits:  dynamicRequest.Inherits,
		Providers: dynamicRequest.Providers,
		Scopes: models.RoleScopes{
			Allow: models.ScopeIdentities{
				Groups:  dynamicRequest.Scopes.Groups,
				Users:   dynamicRequest.Scopes.Users,
				Domains: dynamicRequest.Scopes.Domains,
			},
		},
		Enabled: true,
	}

	// TODO: Convert ElevateDynamicRequest to ElevateRequest
	// For now, let's create a basic ElevateRequest to integrate with existing workflow

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

	authProvider, foundUser, err := s.getUserFromElevationRequest(c, request)

	if err != nil {
		// You will get this error if the user requesting is NOT allowed to make an elevation request for the provided role
		// for the role authorisors to determine if a user is allowed to request elevation, they can check the user's identity against the role's allowed identities and scopes.
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available roles", err)
		return
	}

	input := internalapi.ElevationInput{
		Request:      request,
		User:         foundUser,
		AuthProvider: authProvider,
	}

	result, err := s.API.Elevate(c.Request.Context(), input)
	if err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Failed to execute workflow", err)
		return
	}

	// Submit Analytics event
	if s.Config.HasAnalytics() {

		providers := []string{}

		for _, providerName := range request.Providers {
			foundProvider, err := s.Config.GetProviderByName(providerName)
			if err == nil && foundProvider != nil {
				// Get the underlying provider type
				providers = append(providers, foundProvider.GetProvider())
			}
		}

		properties := map[string]any{
			"providers": providers,
		}
		if foundUser != nil && foundUser.User != nil {
			properties["principal"] = foundUser.User.GetIdentity()
		}
		if err := s.Config.GetAnalytics().Capture(
			"elevate-request", properties,
		); err != nil {
			logrus.WithError(err).Warn("failed to capture elevate-request analytics event")
		}
	}

	// We now redirect the user to the next workflow step.
	c.Redirect(http.StatusTemporaryRedirect,
		result.GetRedirectURL(),
	)
}

// getElevateResume resumes a workflow from a saved state
//
//	@Summary		Resume elevation workflow
//	@Description	Resume a paused or interrupted elevation workflow
//	@Tags			elevate
//	@Accept			json
//	@Produce		json
//	@Param			state	query		string					true	"Workflow state token"
//	@Success		307		"Redirect to next workflow step"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Router			/elevate/resume [get]
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
//
//	@Summary		Resume elevation workflow (POST)
//	@Description	Resume a paused elevation workflow with POST data
//	@Tags			elevate
//	@Accept			json
//	@Produce		json
//	@Param			request	body		map[string]any	true	"Resume request data"
//	@Success		307		"Redirect to next workflow step"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Router			/elevate/resume [post]
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

	session, err := authProviderInstance.CreateSession(ctx, &models.AuthorizeUser{
		Code:        code,
		State:       state,
		RedirectUri: s.Config.GetAuthCallbackUrl(authProvider),
	})

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError,
			"Failed to create session for elevation request", err)
		return
	}

	if session.User == nil {
		s.getErrorPage(c, http.StatusInternalServerError,
			"Failed to get user information from auth provider during elevation")
		return
	}

	// Get the users identity information and role info.
	fmt.Println("Resuming workflow with state:", state)

	workflowTask.SetUser(session.User)

	// Now that we have a user we need to evaluate our composite role
	newRole, err := s.Config.GetCompositeRoleForWorkflow(&models.Identity{
		ID:    session.User.GetIdentity(),
		Label: session.User.GetName(),
		User:  session.User,
	}, workflowTask)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError,
			"Failed to evaluate composite role for elevation request", err)
		return
	}

	workflowTask.SetRole(newRole)

	exportableSession := &models.ExportableSession{
		Session:  session,
		Provider: authProvider,
	}

	localSession := exportableSession.ToLocalSession(
		s.Config.GetServices().GetEncryption())

	if err := s.setAuthCookie(c, authProvider, localSession); err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to set auth cookie", err)
		return
	}

	s.resumeWorkflow(c, workflowTask)

}

func (s *Server) resumeWorkflow(c *gin.Context, workflow *models.ElevateWorkflowTask) {

	// Get user context
	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusBadRequest, "Cannot process elevation request")
		return
	}

	// TODO: Validate the provider that we're using?
	_, foundSession, err := s.getSession(c)

	if err != nil {
		logrus.WithError(err).Error("failed to get user")
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for elevation", err)
		return
	}

	if foundSession == nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: user not found for elevation")
		return
	}

	if foundSession.User == nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: user information is missing for elevation")
		return
	}

	input := internalapi.ResumeInput{
		Workflow: workflow,
		User:     foundSession.User,
	}

	result, err := s.API.Resume(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, internalapi.ErrWorkflowNotFound) {
			s.getErrorPage(c, http.StatusNotFound, "Workflow not found or already completed")
		} else {
			s.getErrorPage(c, http.StatusBadRequest, "Failed to resume workflow", err)
		}
		return
	}

	logrus.WithFields(logrus.Fields{
		"workflow_id": result.GetWorkflowID(),
	}).Info("Workflow resume complete, returning result to caller")

	if result.GetStatus() == swfCtx.RunningStatus {

		c.Redirect(http.StatusTemporaryRedirect,
			s.Config.GetResumeCallbackUrl(result),
		)

	} else if s.canAcceptHtml(c) {

		// If this is an API call return the JSON handler
		// otherwise return the html page

		data := ExecutionStatePageData{
			TemplateData: s.GetTemplateData(c),
			ExecutionStatePageResponse: ExecutionStatePageResponse{
				Execution: &models.WorkflowExecutionInfo{
					WorkflowID: result.GetWorkflowID(),
				},
				Workflow: result.GetWorkflowDef(),
			},
		}

		s.renderHtml(c, "execution.html", data)

	} else {

		c.JSON(http.StatusOK, models.ElevateResponse{
			WorkflowId: result.GetWorkflowID(),
			Status:     result.GetStatus(),
			Output:     result.GetOutputAsMap(),
		})
	}
}

// getElevateLLM handles POST /elevate/llm?reason=I need access to aws
// This function is a handler to take a users reason for an
// elevation and response with a role based on the users request
//
//	@Summary		LLM-based elevation
//	@Description	Request elevation using natural language reasoning with LLM
//	@Tags			elevate
//	@Accept			json
//	@Produce		json
//	@Param			reason	query		string					true	"Natural language reason for elevation"
//	@Success		200		{object}	map[string]any	"LLM response with suggested role"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Router			/elevate/llm [get]
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

// postElevateLLM handles LLM elevation with POST data
//
//	@Summary		LLM-based elevation (POST)
//	@Description	Request elevation using natural language with POST request
//	@Tags			elevate
//	@Accept			json
//	@Produce		json
//	@Param			request	body		models.ElevateLLMRequest	true	"LLM elevation request"
//	@Success		200		{object}	map[string]any		"LLM response with suggested role"
//	@Failure		400		{object}	map[string]any		"Bad request"
//	@Router			/elevate/llm [post]
func (s *Server) postElevateLLM(c *gin.Context) {

	var elevateRequest models.ElevateLLMRequest
	if err := c.ShouldBindJSON(&elevateRequest); err != nil {
		s.getErrorPage(c, http.StatusBadRequest, "Invalid request payload", err)
		return
	}

	s.handleLargeLanguageModelRequest(c, elevateRequest)

}

func (s *Server) handleLargeLanguageModelRequest(c *gin.Context, elevateRequest models.ElevateLLMRequest) {

	if !s.Config.IsLLMElevationEnabled() {
		s.getErrorPage(c, http.StatusForbidden, "LLM elevation requests are disabled on this server")
		return
	}

	if !s.Config.HasLargeLanguageModel() {
		s.getErrorPage(c, http.StatusInternalServerError, "Gemini is not initialized")
		return
	}

	if len(elevateRequest.Reason) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Reason is required")
		return
	}

	// Get user context
	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusBadRequest, "LLM Elevation is only available in server mode")
		return
	}

	authorisedProvider, foundSession, err := s.getSession(c)

	if err != nil {
		logrus.WithError(err).Error("failed to get user")
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for elevation", err)
		return
	}

	if foundSession == nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: user not found for elevation")
		return
	}

	providers := s.Config.GetProvidersByCapabilityWithUser(foundSession.User, models.ProviderCapabilityProvisioning)

	if len(providers) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "No providers with RBAC capability are configured")
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
		authorisedProvider,
		foundSession,
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

type ElevateStaticPageData struct {
	TemplateData
	Identities []models.Identity `json:"identities"`
	Providers  []string          `json:"providers"`
	Roles      []string          `json:"roles"`
	Duration   string            `json:"duration"`
	Reason     string            `json:"reason"`
	Tenants    []string          `json:"tenants"`
}

func (s *Server) getElevationPagePrefill(c *gin.Context) ElevateStaticPageData {
	data := ElevateStaticPageData{
		TemplateData: s.GetTemplateData(c),
	}

	preFilledTenants := c.QueryArray("tenants")
	validTenants := []string{}
	for _, tenantID := range preFilledTenants {
		tenant, err := s.Config.GetTenant(tenantID)
		if err == nil && tenant != nil {
			validTenants = append(validTenants, tenant.ID)
		}
	}

	// Add pre-selected identities from query parameters
	prefilledIdentities := c.QueryArray("identity")

	// Loop over and validate identities
	validIdentities := []models.Identity{}
	for _, identityID := range prefilledIdentities {
		identity, err := s.Config.GetIdentity(identityID)
		if err == nil && identity != nil {
			validIdentities = append(validIdentities, *identity)
		}
	}

	if len(validIdentities) == 0 {
		// Set the current user as the default identity if none are valid
		_, session, err := s.getSession(c)
		if err == nil && session != nil {
			user := session.User
			if user != nil {
				validIdentities = append(validIdentities, models.Identity{
					ID:    user.GetIdentity(),
					Label: user.GetName(),
					User:  user,
				})
			}
		}
	}

	data.Identities = validIdentities

	// Get all the providers from the query parameters
	providers := c.QueryArray("provider")
	if len(providers) > 0 {
		// If providers are specified, use them
		data.Providers = providers
	}

	// Get all the roles from the query parameters
	roles := c.QueryArray("role")
	if len(roles) > 0 {
		// If roles are specified, use them
		data.Roles = roles
	}

	// Get duration from query parameters
	duration := c.Query("duration")
	if len(duration) > 0 {
		data.Duration = duration
	}

	// Get reason from query parameters
	reason := c.Query("reason")
	if len(reason) > 0 {
		data.Reason = reason
	}

	return data
}

func (s *Server) getElevateStaticPage(c *gin.Context) {
	if !s.Config.IsStaticElevationEnabled() {
		s.getErrorPage(c, http.StatusForbidden, "Static elevation requests are disabled on this server")
		return
	}
	s.renderHtml(c, "elevate_static.html", s.getElevationPagePrefill(c))
}

func (s *Server) getElevateDynamicPage(c *gin.Context) {
	if !s.Config.IsDynamicElevationEnabled() {
		s.getErrorPage(c, http.StatusForbidden, "Dynamic elevation requests are disabled on this server")
		return
	}
	s.renderHtml(c, "elevate_dynamic.html", s.getElevationPagePrefill(c))
}

func (s *Server) getElevateLLMPage(c *gin.Context) {
	if !s.Config.IsLLMElevationEnabled() {
		s.getErrorPage(c, http.StatusForbidden, "LLM elevation requests are disabled on this server")
		return
	}
	s.renderHtml(c, "elevate_llm.html", s.getElevationPagePrefill(c))
}
