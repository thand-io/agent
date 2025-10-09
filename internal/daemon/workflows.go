package daemon

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

type ExecutionStatePageData struct {
	config.TemplateData `json:"-"`
	Execution           *models.WorkflowTask `json:"execution"`
	Workflow            *model.Workflow      `json:"workflow"`
}

type WorkflowPageData struct {
	config.TemplateData
	Workflow       map[string]any
	Name           string
	Description    string
	Authentication string
	Enabled        bool
}

// getWorkflows handles GET /api/v1/workflows
func (s *Server) getWorkflows(c *gin.Context) {

	var authenticatedUser *models.Session

	// If we're in server mode then we need to ensure the user is authenticated
	// before we return any roles
	// This is because roles can contain sensitive information
	// and we want to ensure that only authenticated users can access them
	if s.Config.IsServer() {
		foundUser, err := s.getUser(c)
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
			Name:           name,
			Description:    workflow.Description,
			Authentication: workflow.Authentication,
			Enabled:        workflow.Enabled,
		}
	}

	response := models.WorkflowsResponse{
		Version:   "1.0",
		Workflows: workflows,
	}

	if s.canAcceptHtml(c) {

		data := struct {
			TemplateData config.TemplateData
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
func (s *Server) getWorkflowByName(c *gin.Context) {

	workflowName := c.Param("name")

	if len(workflowName) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Workflow name is required")
		return
	}

	workflow, exists := s.Config.Workflows.Definitions[workflowName]

	if !exists {
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
			TemplateData:   s.GetTemplateData(c),
			Workflow:       workflowMap,
			Name:           name,
			Description:    workflow.GetDescription(),
			Authentication: workflow.GetAuthentication(),
			Enabled:        workflow.GetEnabled(),
		}

		s.renderHtml(c, "workflow.html", data)

	} else {

		c.JSON(http.StatusOK, workflow)
	}
}

func (s *Server) getWorkflowsPage(c *gin.Context) {
	s.getWorkflows(c)
}

func (s *Server) terminateRunningWorkflow(c *gin.Context) {
	// TODO: Implement forceful termination logic
	s.cancelRunningWorkflow(c)
}

func (s *Server) cancelRunningWorkflow(c *gin.Context) {

	workflowId := c.Param("id")

	if !s.Config.IsServer() {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to cancel workflow", nil)
	}

	authenticatedUser, err := s.getUser(c)
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

	ownerEmail, foundUser := workflowRun.TypedSearchAttributes.GetString(models.TypedSearchAttributeUser)

	if !foundUser {
		s.getErrorPage(c, http.StatusForbidden, "Unable to determine owner of workflow", nil)
		return
	}

	if strings.Compare(ownerEmail, authenticatedUser.User.Email) != 0 {
		s.getErrorPage(c, http.StatusForbidden, "You do not have permission to terminate this workflow", nil)
		return
	}

	err = temporalClient.SignalWorkflow(
		c, workflowId, models.TemporalEmptyRunId,
		models.TemporalTerminateSignalName, models.TemporalTerminationRequest{
			Reason:      "Terminated by user " + authenticatedUser.User.Email,
			ScheduledAt: time.Now(),
		},
	)

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
