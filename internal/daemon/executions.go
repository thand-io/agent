package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

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

	// Create a timeout context for the query
	// to avoid hanging requests
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	queryResponse, err := temporalClient.QueryWorkflowWithOptions(timeoutCtx, &client.QueryWorkflowWithOptionsRequest{
		WorkflowID: workflowID,
		RunID:      models.TemporalEmptyRunId,
		QueryType:  models.TemporalGetWorkflowTaskQueryName,
		Args:       nil,
	})

	var workflowInfo models.WorkflowTask

	if err == nil {

		err = queryResponse.QueryResult.Get(&workflowInfo)

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to get workflow state", err)
			return
		}

		workflowName := workflowInfo.WorkflowName

		if len(workflowName) == 0 {

			elevationReq, err := workflowInfo.GetContextAsElevationRequest()

			if err == nil && elevationReq != nil {
				workflowName = elevationReq.Role.Workflow
			}

		}

		// Get the workflow template name if available
		foundWorkflow, err := s.GetConfig().GetWorkflowByName(workflowName)

		if err != nil {
			logrus.Debug("Unable to find workflow template for execution", "WorkflowName", workflowInfo.WorkflowName, "Error", err)
		} else {
			workflowInfo.Workflow = foundWorkflow.GetWorkflow()
		}

	} else if errors.Is(err, context.DeadlineExceeded) {

		// If it timesout then get the workflow information without the task details
		wkflw, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, models.TemporalEmptyRunId)

		if err != nil {
			s.getErrorPage(c, http.StatusInternalServerError, "Failed to get workflow state", err)
			return
		}

		workflowExecInfo := workflowExecutionInfo(wkflw.GetWorkflowExecutionInfo())

		workflowInfo = models.WorkflowTask{
			WorkflowID: workflowExecInfo.WorkflowID,
			Status:     swctx.StatusPhase(strings.ToLower(workflowExecInfo.Status)),
			StartedAt:  workflowExecInfo.StartTime,
		}

	} else {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to get workflow state", err)
		return
	}

	data := ExecutionStatePageData{
		TemplateData: s.GetTemplateData(c),
		Execution:    &workflowInfo,
		Workflow:     workflowInfo.Workflow,
	}

	if s.canAcceptHtml(c) {

		s.renderHtml(c, "execution.html", data)

	} else {

		c.JSON(http.StatusOK, data)
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

	if userAttr, exists := searchAttributes[models.VarsContextUser]; exists && userAttr != nil {
		var userValue string
		if err := dataConverter.FromPayload(userAttr, &userValue); err == nil {
			response.User = userValue
		}
	}

	if roleAttr, exists := searchAttributes[models.VarsContextRole]; exists && roleAttr != nil {
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
