package daemon

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

type ExecutionsPageData struct {
	config.TemplateData
	Executions []*models.WorkflowExecutionInfo `json:"executions"`
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
		s.getErrorPage(c, http.StatusBadRequest, "Workflow listing is only available in server mode")
		return
	}

	_, foundUser, err := s.getUser(c)

	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user for list of available workflows", err)
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

	response := ExecutionsPageData{
		TemplateData: s.GetTemplateData(c),
		Executions:   runningWorkflows,
	}

	if s.canAcceptHtml(c) {

		s.renderHtml(c, "executions.html", response)

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

	if !s.Config.IsServer() {
		// In non-server mode we can assume a default user
		// TODO: Proxy request to server
		s.getErrorPage(c, http.StatusBadRequest, "Workflow listing is only available in server mode")
		return
	}

	_, _, err := s.getUser(c)

	if err != nil {
		s.getErrorPage(c, http.StatusUnauthorized, "Unauthorized: unable to get user to get workflow information", err)
		return
	}

	temporalClient := temporal.GetClient()

	// If it timesout then get the workflow information without the task details
	wkflw, err := temporalClient.DescribeWorkflowExecution(ctx, workflowID, models.TemporalEmptyRunId)

	if err != nil {
		s.getErrorPage(c, http.StatusInternalServerError, "Failed to get workflow state", err)
		return
	}

	workflowExecInfo := workflowExecutionInfo(wkflw.GetWorkflowExecutionInfo())

	var workflowTask models.WorkflowTask

	// If workflow hasn't completed the query for the current state
	if workflowExecInfo.CloseTime == nil {

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

		if err == nil {

			err = queryResponse.QueryResult.Get(&workflowTask)

			if err != nil {
				s.getErrorPage(c, http.StatusInternalServerError, "Failed to get workflow state", err)
				return
			}

			workflowName := workflowTask.WorkflowName

			if len(workflowName) == 0 {

				elevationReq, err := workflowTask.GetContextAsElevationRequest()

				if err == nil && elevationReq != nil {
					workflowName = elevationReq.Workflow
				}

			}

			// Get the workflow template name if available
			foundWorkflow, err := s.GetConfig().GetWorkflowByName(workflowName)

			if err != nil {
				logrus.WithError(err).Warn("Failed to get workflow definition")
			} else {
				workflowTask.Workflow = foundWorkflow.GetWorkflow()
			}

			// Copy over task status phases to the response
			phases := []string{}
			for phase := range workflowTask.TasksStatusPhase {
				phases = append(phases, phase)
			}
			workflowExecInfo.History = phases

			workflowExecInfo.Input = workflowTask.Input
			workflowExecInfo.Output = workflowTask.Output
			workflowExecInfo.Context = workflowTask.Context

		}
	} else {

		// Otherwise if the workflow has completed then get the last output

		fut := temporalClient.GetWorkflow(
			ctx, workflowID, models.TemporalEmptyRunId)

		err := fut.Get(ctx, &workflowTask)

		if err != nil {
			logrus.WithError(err).Error("Failed to get workflow output")
			workflowTask.Output = err.Error()
		}

	}

	data := ExecutionStatePageData{
		TemplateData: s.GetTemplateData(c),
		Execution:    workflowExecInfo,
		Workflow:     workflowTask.Workflow,
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

func workflowExecutionInfo(
	workflowInfo *workflow.WorkflowExecutionInfo,
) *models.WorkflowExecutionInfo {

	exec := workflowInfo.GetExecution()

	searchAttributes := workflowInfo.GetSearchAttributes().GetIndexedFields()

	response := models.WorkflowExecutionInfo{
		WorkflowID: exec.GetWorkflowId(),
		RunID:      exec.GetRunId(),
		StartTime:  workflowInfo.GetStartTime().AsTime(),
		Status:     strings.ToUpper(workflowInfo.GetStatus().String()),
		Identities: []string{},
		History:    []string{},
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

	if approvedAttr, exists := searchAttributes[models.VarsContextApproved]; exists && approvedAttr != nil {
		var approvedValue bool
		if err := dataConverter.FromPayload(approvedAttr, &approvedValue); err == nil {
			response.Approved = &approvedValue
		}
	}

	if workflowAttr, exists := searchAttributes[models.VarsContextWorkflow]; exists && workflowAttr != nil {
		var workflowValue string
		if err := dataConverter.FromPayload(workflowAttr, &workflowValue); err == nil {
			response.Workflow = workflowValue
		}
	}

	if taskAttr, exists := searchAttributes["task"]; exists && taskAttr != nil {
		var taskValue string
		if err := dataConverter.FromPayload(taskAttr, &taskValue); err == nil {
			response.Task = taskValue
		}
	}

	if reasonAttr, exists := searchAttributes["reason"]; exists && reasonAttr != nil {
		var reasonValue string
		if err := dataConverter.FromPayload(reasonAttr, &reasonValue); err == nil {
			response.Reason = reasonValue
		}
	}

	if durationAttr, exists := searchAttributes["duration"]; exists && durationAttr != nil {
		var durationValue int64
		if err := dataConverter.FromPayload(durationAttr, &durationValue); err == nil {
			response.Duration = durationValue
		}
	}

	if identitiesAttr, exists := searchAttributes["identities"]; exists && identitiesAttr != nil {
		var identitiesValue []string
		if err := dataConverter.FromPayload(identitiesAttr, &identitiesValue); err == nil {
			response.Identities = identitiesValue
		}
	}

	return &response

}
