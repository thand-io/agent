package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	sdkModels "github.com/thand-io/agent/sdk/models"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	runner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
)

func (m *WorkflowManager) registerActivities() error {

	if !m.HasTemporal() {
		return fmt.Errorf("temporal service not configured")
	}

	temporalService := m.config.GetTemporal()

	if temporalService == nil {
		return fmt.Errorf("temporal service not available")
	}

	if !temporalService.HasWorker() {
		return fmt.Errorf("temporal worker not configured")
	}

	worker := temporalService.GetWorker()

	for _, functionName := range m.GetRegisteredFunctions() {

		logrus.WithField("function", functionName).Infof("Registering activity for function: %s", functionName)

		// Capture the functionName in a local variable to avoid closure issues
		fn := functionName
		worker.RegisterActivityWithOptions(func(
			ctx context.Context,

			/// This needs to be a serialisable object
			workflowTask *sdkWorkflowsModel.WorkflowTask,
			taskName string,
			callFunction *model.CallFunction,
			input any,
		) (any, error) {

			if err := m.config.HydrateWorkflowTask(workflowTask); err != nil {
				logrus.WithError(err).Error("Failed to hydrate workflow task in activity")
				return nil, err
			}

			if workflowTask.HasState() {
				workflowTask.ClearTaskContext()
			}

			workflowTask.SetInternalContext(ctx)
			workflowTask.SetState(&sdkWorkflowsModel.WorkflowTaskState{
				Definition: callFunction,
				StartedAt:  time.Now().UTC(),
				Name:       taskName,
				Reference:  workflowTask.GetTaskReference(),
				Input:      input,
			})

			caller, foundCaller := m.GetFunction(fn)

			if !foundCaller {
				return nil, fmt.Errorf("function not found: %s", fn)
			}

			output, err := caller.Execute(
				workflowTask, callFunction, input)

			// Applications need to handle errors returned from activities
			// by themselves, so we just return the error here
			if err != nil {
				return nil, handleActivityError(fn, err)
			}

			return output, nil
		}, activity.RegisterOptions{
			Name: fn,
		})
	}

	/*
		HTTP Activity
	*/
	worker.RegisterActivityWithOptions(func(
		ctx context.Context,
		httpCall model.HTTPArguments,
		finalURL string,
	) (any, error) {

		logrus.WithFields(logrus.Fields{
			"activity": sdkModels.TemporalHttpActivityName,
			"url":      finalURL,
			"method":   httpCall.Method,
		}).Info("Executing HTTP activity")

		return runner.MakeHttpRequest(httpCall, finalURL)

	}, activity.RegisterOptions{
		Name: sdkModels.TemporalHttpActivityName,
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
			"activity": sdkModels.TemporalGrpcActivityName,
			"service":  grpcCall.Service.Name,
			"method":   grpcCall.Method,
		}).Info("Executing gRPC activity")

		return runner.MakeGrpcRequest(grpcCall, finalInput)

	}, activity.RegisterOptions{
		Name: sdkModels.TemporalGrpcActivityName,
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
			"activity": sdkModels.TemporalAsyncionActivityName,
		}).Info("Executing AsyncIO activity")

		return nil, fmt.Errorf("asyncIO activity not implemented yet")

	}, activity.RegisterOptions{
		Name: sdkModels.TemporalAsyncionActivityName,
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
			"activity": sdkModels.TemporalOpenAPIActivityName,
		}).Info("Executing OpenAPI activity")

		return runner.MakeOpenAPIRequest(openAPICall, input)

	}, activity.RegisterOptions{
		Name: sdkModels.TemporalOpenAPIActivityName,
	})

	return nil
}

func handleActivityError(fn string, err error) error {

	logrus.WithError(err).Errorf("failed to execute activity: %s", fn)

	// Check the error type and wrap if necessary
	if temporal.IsApplicationError(err) {
		return err
	} else if temporal.IsCanceledError(err) {
		return err
	} else if temporal.IsPanicError(err) {
		return err
	} else if temporal.IsTerminatedError(err) {
		return err
	} else {
		return temporal.NewNonRetryableApplicationError(
			fmt.Sprintf("Activity Error: %s", fn),
			"NonRetryableApplicationError",
			err,
		)
	}
}
