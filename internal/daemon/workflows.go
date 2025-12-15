package daemon

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
)

type ExecutionStatePageData struct {
	TemplateData `json:"-"`
	ExecutionStatePageResponse
}

type ExecutionStatePageResponse struct {
	Execution *models.WorkflowExecutionInfo `json:"execution"`
	Workflow  *model.Workflow               `json:"workflow"`
}

type WorkflowPageData struct {
	TemplateData
	Workflow    map[string]any
	Name        string
	Description string
	Enabled     bool
}

// getWorkflows handles GET /api/v1/workflows
//
//	@Summary		List workflows
//	@Description	Get a list of all available workflows
//	@Tags			workflows
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.WorkflowsResponse	"List of workflows"
//	@Failure		401	{object}	map[string]any		"Unauthorized"
//	@Router			/workflows [get]
//	@Security		BearerAuth
func (s *Server) getWorkflows(c *gin.Context) {

	var authenticatedUser *models.Session

	// If we're in server mode then we need to ensure the user is authenticated
	// before we return any roles
	// This is because roles can contain sensitive information
	// and we want to ensure that only authenticated users can access them
	if s.Config.IsServer() {
		_, foundUser, err := s.getUser(c)
		if err != nil {
			s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available workflows", err)
			return
		}
		authenticatedUser = foundUser
	}

	workflows := map[string]models.WorkflowResponse{}

	for name, workflow := range s.Config.Workflows.Definitions {

		if !workflow.Enabled {
			continue
		}

		if authenticatedUser != nil && !workflow.HasPermission(authenticatedUser.User) {
			continue
		}

		workflows[name] = models.WorkflowResponse{
			Name:        name,
			Description: workflow.Description,
			Enabled:     workflow.Enabled,
		}
	}

	response := models.WorkflowsResponse{
		Version:   "1.0",
		Workflows: workflows,
	}

	if s.canAcceptHtml(c) {

		data := struct {
			TemplateData TemplateData
			Response     models.WorkflowsResponse
		}{
			TemplateData: s.GetTemplateData(c),
			Response:     response,
		}
		s.renderHtml(c, "workflows.html", data)

	} else {

		c.JSON(http.StatusOK, response)
	}

}

// getWorkflowByName handles GET /api/v1/workflow/:name
//
//	@Summary		Get workflow by name
//	@Description	Retrieve detailed information about a specific workflow
//	@Tags			workflows
//	@Accept			json
//	@Produce		json
//	@Param			name	path		string					true	"Workflow name"
//	@Success		200		{object}	map[string]any	"Workflow details"
//	@Failure		400		{object}	map[string]any	"Bad request"
//	@Failure		404		{object}	map[string]any	"Workflow not found"
//	@Router			/workflow/{name} [get]
//	@Security		BearerAuth
func (s *Server) getWorkflowByName(c *gin.Context) {

	workflowName := c.Param("name")

	if len(workflowName) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Workflow name is required")
		return
	}

	workflow, err := s.Config.GetWorkflowByName(workflowName)

	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Workflow not found")
		return
	}

	if s.canAcceptHtml(c) {

		workflowMap, err := workflow.GetWorkflow().AsMap()

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to convert workflow to map", err)
			return
		}

		name := workflowName

		if len(workflow.GetName()) > 0 {
			name = workflow.GetName()
		}

		// Lets create a page that shows the workflow details
		// including the steps and their descriptions

		data := WorkflowPageData{
			TemplateData: s.GetTemplateData(c),
			Workflow:     workflowMap,
			Name:         name,
			Description:  workflow.GetDescription(),
			Enabled:      workflow.GetEnabled(),
		}

		s.renderHtml(c, "workflow.html", data)

	} else {

		c.JSON(http.StatusOK, workflow)
	}
}

func (s *Server) getWorkflowsPage(c *gin.Context) {
	s.getWorkflows(c)
}

// terminateRunningWorkflow forcefully terminates a workflow execution
//
//	@Summary		Terminate workflow execution
//	@Description	Forcefully terminate a running workflow execution
//	@Tags			executions
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string					true	"Workflow execution ID"
//	@Success		200	{object}	map[string]any	"Workflow terminated"
//	@Failure		401	{object}	map[string]any	"Unauthorized"
//	@Failure		403	{object}	map[string]any	"Forbidden"
//	@Failure		404	{object}	map[string]any	"Workflow not found"
//	@Failure		500	{object}	map[string]any	"Internal server error"
//	@Router			/execution/{id}/terminate [get]
//	@Security		BearerAuth
func (s *Server) terminateRunningWorkflow(c *gin.Context) {
	// TODO: Implement forceful termination logic
	s.cancelRunningWorkflow(c)
}

// cancelRunningWorkflow gracefully cancels a workflow execution
//
//	@Summary		Cancel workflow execution
//	@Description	Gracefully cancel a running workflow execution
//	@Tags			executions
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string					true	"Workflow execution ID"
//	@Success		200	{object}	map[string]any	"Workflow cancelled"
//	@Failure		401	{object}	map[string]any	"Unauthorized"
//	@Failure		403	{object}	map[string]any	"Forbidden"
//	@Failure		404	{object}	map[string]any	"Workflow not found"
//	@Failure		500	{object}	map[string]any	"Internal server error"
//	@Router			/execution/{id}/cancel [get]
//	@Security		BearerAuth
func (s *Server) cancelRunningWorkflow(c *gin.Context) {

	workflowId := c.Param("id")

	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to cancel workflow", nil)
	}

	_, authenticatedUser, err := s.getUser(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for terminating workflow", err)
		return
	}

	services := s.GetConfig().GetServices()

	if !services.HasTemporal() {
		s.getErrorPage(c, http.StatusInternalServerError, "Temporal service is not configured")
		return
	}

	temporalClient := services.GetTemporal().GetClient()

	workflowRun, err := temporalClient.DescribeWorkflow(c, workflowId, models.TemporalEmptyRunId)

	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Failed to find running workflow", err)
		return
	}

	// Check if the workflow is owned by the user

	ownerEmail, foundUser := workflowRun.TypedSearchAttributes.GetKeyword(models.TypedSearchAttributeUser)

	if !foundUser {
		s.getErrorPage(c, http.StatusForbidden, "Unable to determine owner of workflow", nil)
		return
	}

	if strings.Compare(ownerEmail, authenticatedUser.User.Email) != 0 {
		s.getErrorPage(c, http.StatusForbidden, "You do not have permission to terminate this workflow", nil)
		return
	}

	err = temporalClient.CancelWorkflow(c, workflowId, models.TemporalEmptyRunId)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to signal workflow for termination", err)
		return
	}

	if s.canAcceptHtml(c) {

		// TODO: Maybe add a page for this later
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/execution/%s?canceled=true", workflowId))

	} else {

		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"message": "Workflow termination signal sent",
		})

	}

}
