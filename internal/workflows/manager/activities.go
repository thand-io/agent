package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	models "github.com/thand-io/agent/internal/models"
	runner "github.com/thand-io/agent/internal/workflows/runner"
	"go.temporal.io/sdk/activity"
)

func (m *WorkflowManager) registerActivities() error {

	if !m.config.GetServices().HasTemporal() {
		return fmt.Errorf("temporal service not configured")
	}

	temporalService := m.config.GetServices().GetTemporal()

	if temporalService == nil {
		return fmt.Errorf("temporal service not available")
	}

	if !temporalService.HasWorker() {
		return fmt.Errorf("temporal worker not configured")
	}

	worker := temporalService.GetWorker()

	for _, functionName := range m.functions.GetRegisteredFunctions() {

		logrus.WithField("function", functionName).Infof("Registering activity for function: %s", functionName)

		// Capture the functionName in a local variable to avoid closure issues
		fn := functionName
		worker.RegisterActivityWithOptions(func(
			ctx context.Context,
			workflowTask *models.WorkflowTask,
			taskName string,
			callFunction *model.CallFunction,
			input any,
		) (any, error) {

			err := m.Hydrate(workflowTask)

			if err != nil {
				return nil, fmt.Errorf("failed to hydrate workflow task: %w", err)
			}

			workflowTask.SetState(&models.WorkflowTaskState{
				Definition: callFunction,
				StartedAt:  time.Now().UTC(),
				Name:       taskName,
				Reference:  workflowTask.GetTaskReference(),
				Input:      input,
			})

			caller, foundCaller := m.functions.GetFunction(fn)

			if !foundCaller {
				return nil, fmt.Errorf("function not found: %s", fn)
			}

			output, err := caller.Execute(
				workflowTask, callFunction, input)

			if err != nil {
				logrus.WithError(err).Errorf("failed to execute activity: %s", fn)
			}

			return output, err
		}, activity.RegisterOptions{
			Name: fn,
		})
	}

	/*
		Cleanup Activity
		This activity is responsible for cleaning up resources after a workflow execution.
		It checks for an elevation request in the workflow context and revokes any roles assigned
		during the workflow execution.
	*/
	worker.RegisterActivityWithOptions(func(
		ctx context.Context,
		workflowTask *models.WorkflowTask,
	) (any, error) {

		if workflowTask == nil {
			return nil, fmt.Errorf("workflow task is nil")
		}

		log := activity.GetLogger(ctx)
		log.Info("Executing cleanup activity", "cleanupID", workflowTask.WorkflowID)

		err := m.Hydrate(workflowTask)

		if err != nil {
			return nil, fmt.Errorf("failed to hydrate workflow task: %w", err)
		}

		elevateRequest, err := workflowTask.GetContextAsElevationRequest()

		if err != nil {
			return nil, fmt.Errorf("failed to get elevate request from context: %w", err)
		}

		if elevateRequest == nil || !elevateRequest.IsValid() {
			log.Info("No valid elevate request found, skipping cleanup")
			return nil, fmt.Errorf("no valid elevate request found in workflow context")
		}

		provider := elevateRequest.Provider
		user := elevateRequest.User
		role := elevateRequest.Role

		providerHandler, err := m.config.GetProviderByName(provider)

		if err != nil {
			log.Info("No valid provider found, skipping cleanup")
			return nil, fmt.Errorf("no valid provider found: %w", err)
		}

		output, err := providerHandler.GetClient().RevokeRole(
			ctx, user, role, workflowTask.GetContextAsMap())

		if err != nil {
			return nil, fmt.Errorf("failed to revoke role: %w", err)
		}

		// Perform any necessary cleanup here
		log.Info("Cleanup activity completed successfully", "cleanupID", workflowTask.WorkflowID)

		return output, nil

	}, activity.RegisterOptions{
		Name: models.TemporalCleanupActivityName,
	})

	/*
		HTTP Activity
	*/
	worker.RegisterActivityWithOptions(func(
		ctx context.Context,
		httpCall model.HTTPArguments,
		finalURL string,
	) (any, error) {

		logrus.WithFields(logrus.Fields{
			"activity": models.TemporalHttpActivityName,
			"url":      finalURL,
			"method":   httpCall.Method,
		}).Info("Executing HTTP activity")

		return runner.MakeHttpRequest(httpCall, finalURL)

	}, activity.RegisterOptions{
		Name: models.TemporalHttpActivityName,
	})

	/*
		gRPC Activity
	*/
	worker.RegisterActivityWithOptions(func(
		ctx context.Context,
		grpcCall model.GRPCArguments,
		finalInput map[string]any,
	) (any, error) {

		logrus.WithFields(logrus.Fields{
			"activity": models.TemporalGrpcActivityName,
			"service":  grpcCall.Service.Name,
			"method":   grpcCall.Method,
		}).Info("Executing gRPC activity")

		return runner.MakeGrpcRequest(grpcCall, finalInput)

	}, activity.RegisterOptions{
		Name: models.TemporalGrpcActivityName,
	})

	/*
		AsyncIO Activity
	*/
	worker.RegisterActivityWithOptions(func(
		ctx context.Context,
		asyncIOCall model.AsyncAPIArguments,
		input any,
	) (any, error) {

		logrus.WithFields(logrus.Fields{
			"activity": models.TemporalAsyncionActivityName,
		}).Info("Executing AsyncIO activity")

		return nil, fmt.Errorf("asyncIO activity not implemented yet")

	}, activity.RegisterOptions{
		Name: models.TemporalAsyncionActivityName,
	})

	/*
		OpenApi Activity
	*/
	worker.RegisterActivityWithOptions(func(
		ctx context.Context,
		openAPICall model.OpenAPIArguments,
		input any,
	) (any, error) {

		logrus.WithFields(logrus.Fields{
			"activity": models.TemporalOpenAPIActivityName,
		}).Info("Executing OpenAPI activity")

		return runner.MakeOpenAPIRequest(openAPICall, input)

	}, activity.RegisterOptions{
		Name: models.TemporalOpenAPIActivityName,
	})

	return nil
}
