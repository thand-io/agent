package daemon

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
)

func (s *Server) getWorkflowStatePage(c *gin.Context, workflowTask *models.WorkflowTask) {
	data := ExecutionStatePageData{
		TemplateData: s.GetTemplateData(c),
		Workflow:     workflowTask,
	}
	s.renderHtml(c, "execution.html", data)
}

func (s *Server) listRunningWorkflows(c *gin.Context) {

	ctx := context.Background()

	temporalService := s.Config.GetServices().GetTemporal()

	if temporalService == nil || !temporalService.HasClient() {
		s.getErrorPage(c, http.StatusBadRequest, "Temporal service is not configured")
		return
	}
	if !s.Config.IsServer() {
		// In non-server mode we can assume a default user
		// TODO: Proxy request to server
	}

	foundUser, err := s.getUser(c)
	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available providers", err)
		return
	}

	if foundUser == nil || foundUser.User == nil || len(foundUser.User.Email) == 0 {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: user information is incomplete", nil)
		return
	}

	temporalClient := temporalService.GetClient()

	resp, err := temporalClient.ListWorkflow(ctx, &workflowservice.ListWorkflowExecutionsRequest{
		Namespace: temporalService.GetNamespace(),
		PageSize:  100,
		Query:     fmt.Sprintf("TaskQueue='%s' AND user='%s'", temporalService.GetTaskQueue(), foundUser.User.Email),
		//NextPageToken: nextPageToken,
	})

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to list workflows", err)
		return
	}

	runningWorkflows := []*models.WorkflowExecutionInfo{}

	for _, exec := range resp.Executions {
		runningWorkflows = append(
			runningWorkflows, workflowExecutionInfo(exec))
	}

	response := struct {
		Workflows []*models.WorkflowExecutionInfo `json:"workflows"`
	}{
		Workflows: runningWorkflows,
	}

	if s.canAcceptHtml(c) {

		data := struct {
			TemplateData config.TemplateData
			Response     struct {
				Workflows []*models.WorkflowExecutionInfo `json:"workflows"`
			}
		}{
			TemplateData: s.GetTemplateData(c),
			Response:     response,
		}
		s.renderHtml(c, "executions.html", data)

	} else {

		c.JSON(http.StatusOK, response)
	}

}

func (s *Server) createWorkflow(c *gin.Context) {
	// TODO: Implement workflow creation logic
}

func (s *Server) getRunningWorkflow(c *gin.Context) {
	workflowID := c.Param("id")

	if len(workflowID) == 0 {
		s.getErrorPage(c, http.StatusBadRequest, "Workflow ID is required")
		return
	}

	ctx := context.Background()

	temporal := s.Config.GetServices().GetTemporal()

	if temporal == nil || !temporal.HasClient() {
		s.getErrorPage(c, http.StatusBadRequest, "Temporal service is not configured")
		return
	}

	temporalClient := temporal.GetClient()

	wkflw, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, "")

	if err != nil {
		s.getErrorPage(c, http.StatusNotFound, "Workflow not found", err)
		return
	}

	workflowInfo := workflowExecutionInfo(wkflw.GetWorkflowExecutionInfo())

	if s.canAcceptHtml(c) {

		data := ExecutionStatePageData{
			TemplateData: s.GetTemplateData(c),
			Workflow: &models.WorkflowTask{
				WorkflowID: workflowInfo.WorkflowID,
				Status:     swctx.StatusPhase(strings.ToLower(workflowInfo.Status)),
				StartedAt:  workflowInfo.StartTime,
			},
		}
		s.renderHtml(c, "execution.html", data)

	} else {

		c.JSON(http.StatusOK, gin.H{
			"workflow": workflowInfo,
		})
	}
}

func (s *Server) getExecutionsPage(c *gin.Context) {
	s.listRunningWorkflows(c)
}

func workflowExecutionInfo(workflowInfo *workflow.WorkflowExecutionInfo) *models.WorkflowExecutionInfo {

	exec := workflowInfo.GetExecution()

	searchAttributes := workflowInfo.GetSearchAttributes().GetIndexedFields()

	response := models.WorkflowExecutionInfo{
		WorkflowID: exec.GetWorkflowId(),
		RunID:      exec.GetRunId(),
		StartTime:  workflowInfo.GetStartTime().AsTime(),
		Status:     strings.ToUpper(workflowInfo.GetStatus().String()),
	}

	if workflowInfo.GetCloseTime() != nil {
		closeTime := workflowInfo.GetCloseTime().AsTime()
		response.CloseTime = &closeTime
	}

	// Safely extract search attributes with proper type conversion
	dataConverter := converter.GetDefaultDataConverter()

	if userAttr, exists := searchAttributes["user"]; exists && userAttr != nil {
		var userValue string
		if err := dataConverter.FromPayload(userAttr, &userValue); err == nil {
			response.User = userValue
		}
	}

	if roleAttr, exists := searchAttributes["role"]; exists && roleAttr != nil {
		var roleValue string
		if err := dataConverter.FromPayload(roleAttr, &roleValue); err == nil {
			response.Role = roleValue
		}
	}

	if workflowInfo.GetStatus() == enums.WORKFLOW_EXECUTION_STATUS_RUNNING {
		if workflowStatusAttr, exists := searchAttributes["status"]; exists && workflowStatusAttr != nil {
			var statusValue string
			if err := dataConverter.FromPayload(workflowStatusAttr, &statusValue); err == nil {
				response.Status = strings.ToUpper(statusValue)
			}
		}
	}

	if approvedAttr, exists := searchAttributes["approved"]; exists && approvedAttr != nil {
		var approvedValue bool
		if err := dataConverter.FromPayload(approvedAttr, &approvedValue); err == nil {
			response.Approved = approvedValue
		}
	}

	return &response

}
