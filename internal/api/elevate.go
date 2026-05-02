// Package api provides a transport-agnostic elevation API that can be consumed
// by the HTTP daemon, gRPC handlers, the CLI, or any other future caller.
package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	swfCtx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

// ErrWorkflowNotFound is returned by Resume when the workflow could not be
// located or has already completed.
var ErrWorkflowNotFound = errors.New("workflow not found or already completed")

// ElevationInput is the transport-agnostic input for Elevate.
// Auth resolution (session look-up, provider) is the caller's responsibility;
// the resolved session is passed in via User / AuthProvider.
type ElevationInput struct {
	// Request contains the role, workflow, providers, reason, etc.
	Request models.ElevateRequest

	// User is the authenticated session resolved by the caller from the
	// transport layer (HTTP cookie, bearer token, …). May be nil when no
	// session is available.
	User *models.Session

	// AuthProvider is the name of the provider that authenticated User.
	// Empty when User is nil.
	AuthProvider string
}

// ResumeInput is the transport-agnostic input for Resume.
type ResumeInput struct {
	// Workflow is the partially-executed task to be resumed.
	Workflow *models.ElevateWorkflowTask

	// User is the authenticated user resolved by the caller from the transport
	// layer. Used to stamp the CloudEvent extension when a signal is pending.
	User *models.User
}

// Elevate starts an elevation workflow.
// All auth resolution must be performed by the caller prior to this call.
func (s *Service) Elevate(ctx context.Context, input ElevationInput) (*models.WorkflowRequest, error) {
	if !s.cfg.IsServer() {
		return nil, errors.New("cannot process elevation request: not in server mode")
	}

	if len(input.Request.Workflow) == 0 {
		return nil, errors.New("no workflow specified for elevation request")
	}

	request := input.Request

	if err := models.NormalizeLocalSudoRequest(&request, s.cfg.GetProviderDefinitions()); err != nil {
		return nil, fmt.Errorf("failed to normalize elevation request: %w", err)
	}

	if input.User != nil {
		exportableSession := &models.ExportableSession{
			Session:  input.User,
			Provider: input.AuthProvider,
		}
		request.Session = exportableSession.ToLocalSession(
			s.cfg.GetServices().GetEncryption())

		// Self-elevate: default identities to the authenticated user's e-mail
		// when the caller did not supply explicit identity targets.
		if len(request.Identities) == 0 &&
			input.User.User != nil &&
			len(input.User.User.Email) > 0 {
			request.Identities = []string{input.User.User.Email}
		}
	}

	workflowTask, err := s.workflows.CreateElevationWorkflow(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to execute workflow: %w", err)
	}

	return workflowTask, nil
}

// Resume continues a previously started elevation workflow.
// The caller is responsible for resolving the active session before calling
// Resume; the resolved user is passed via ResumeInput.User.
func (s *Service) Resume(ctx context.Context, input ResumeInput) (*models.ElevateWorkflowTask, error) {
	if !s.cfg.IsServer() {
		return nil, errors.New("cannot process elevation request: not in server mode")
	}

	workflow := input.Workflow

	if workflow == nil {
		return nil, errors.New("resume: workflow task must not be nil")
	}

	// Attach the authenticated user identity to any pending CloudEvent signal.
	if input.User != nil {
		event := workflow.GetInputAsCloudEvent()
		if event != nil {
			identity := input.User.AsIdentity()
			event.SetExtension(sdkConstants.VarsContextUser,
				identity.EncodeBase64())

			if len(event.FieldErrors) > 0 {
				logrus.WithField("errors", event.FieldErrors).
					Error("failed to set user extension on cloudevent")
				return nil, errors.New("failed to set user extension on cloudevent")
			}

			workflow.SetInput(event)
		}
	}

	workflowTask, err := s.workflows.ResumeWorkflow(workflow)
	if err != nil {
		if isAlreadyCompletedResumeError(err) {
			logrus.WithFields(logrus.Fields{
				"workflow_id": workflow.GetWorkflowID(),
			}).Debug("elevation resume: workflow already completed")
			workflow.Status = swfCtx.CompletedStatus
			return workflow, nil
		}
		return nil, fmt.Errorf("failed to resume workflow: %w", err)
	}

	if workflowTask == nil {
		return nil, ErrWorkflowNotFound
	}

	logrus.WithFields(logrus.Fields{
		"task_name": workflowTask.GetTaskReference(),
	}).Info("elevation resume: returning result to caller")

	return workflowTask, nil
}

func isAlreadyCompletedResumeError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "workflow execution already completed")
}
