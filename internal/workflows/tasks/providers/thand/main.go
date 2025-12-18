package thand

import (
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/tasks"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
)

type thandCollection struct {
	config *config.Config
	tasks.TaskCollection
}

func NewThandCollection(config *config.Config) *thandCollection {
	return &thandCollection{
		config: config,
	}
}

func (c *thandCollection) RegisterTasks(r *tasks.TaskRegistry) {

	// Register tasks
	r.RegisterTasks(
		NewThandTask(c.config),
	)

}

type thandTask struct {
	config *config.Config
}

func NewThandTask(config *config.Config) *thandTask {
	return &thandTask{
		config: config,
	}
}

func (f *thandTask) GetName() string {
	return taskModel.ThandTaskName
}

func (f *thandTask) GetDescription() string {
	return "This task handles approvals in the Thand workflow."
}

// resolveUserFromIdentity looks up the identity from configured providers and returns the user.
// This handles provider-prefixed identities like "aws-prod:username"
// and queries identity providers to get the full user object.
// If the lookup fails, it returns a basic user with the identity as the email.
func (t *thandTask) resolveIdentity(identity string) *models.Identity {

	identityResult, err := t.config.GetIdentity(identity)

	if err != nil {
		logrus.WithError(err).WithField("identity", identity).Warn("Failed to lookup identity, creating fallback identity")
	}

	// Use the looked up user or create a basic one
	if identityResult != nil && identityResult.User != nil {
		return identityResult
	}

	return &models.Identity{
		User: &models.User{
			Email: identity,
		},
	}
}

func (f *thandTask) GetVersion() string {
	return "1.0.0"
}

// Execute executes the Thand approvals task
func (t *thandTask) Execute(
	workflowTask *models.WorkflowTask,
	task *model.TaskItem,
	input any,
) (any, error) {

	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}

	taskName := task.Key
	thandTask, ok := task.Task.(*taskModel.ThandTask)

	if !ok {
		return nil, fmt.Errorf("invalid task type for ServerlessThandTask")
	}

	// Create a copy to preserve the original workflow intent
	interpolatedTask := *thandTask

	if thandTask.With != nil {

		interpolatedWith, err := workflowTask.TraverseAndEvaluate(
			thandTask.With.AsMap(), input)

		if err != nil {
			return nil, fmt.Errorf("failed to interpolate call.with: %w", err)
		}

		withMap, ok := interpolatedWith.(map[string]any)

		if !ok {
			return nil, fmt.Errorf("interpolated call.with is not a map[string]any")
		}

		// Create a new BasicConfig with the interpolated values
		interpolatedConfig := models.BasicConfig(withMap)
		interpolatedTask.With = &interpolatedConfig

	}

	switch interpolatedTask.Thand {
	case ThandApprovalsTask:
		return t.executeApprovalsTask(workflowTask, taskName, &interpolatedTask, input)
	case ThandAuthorizeTask:
		return t.executeAuthorizeTask(workflowTask, taskName, &interpolatedTask)
	case ThandValidateTask:
		return t.executeValidateTask(workflowTask, &interpolatedTask, input)
	case ThandNotifyTask:
		return t.executeNotifyTask(workflowTask, taskName, &interpolatedTask)
	case ThandRevokeTask:
		return t.executeRevokeTask(workflowTask, taskName, &interpolatedTask)
	case ThandMonitorTask:
		return t.executeMonitorTask(workflowTask, taskName, &interpolatedTask, input)
	case ThandFormTask:
		return t.executeFormTask(workflowTask, taskName, &interpolatedTask)
	default:
		return nil, fmt.Errorf("unknown thand task type: %s", interpolatedTask.Thand)
	}

}
