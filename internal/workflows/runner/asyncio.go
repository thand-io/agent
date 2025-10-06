package runner

import (
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

func (r *ResumableWorkflowRunner) executeAsyncFunction(
	taskName string,
	call *model.CallAsyncAPI,
	input any,
) (map[string]any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
		"call": call.Call,
	}).Info("Executing AsyncAPI function call")

	// Execute the function call

	asyncCall := call.With

	// For now, just log the async call details
	logrus.WithFields(logrus.Fields{
		"asyncCall": asyncCall,
	}).Info("AsyncAPI call details")

	return nil, fmt.Errorf("asyncapi call not implemented yet")
}
