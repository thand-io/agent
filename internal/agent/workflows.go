package agent

import (
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/workflows/functions"
	"github.com/thand-io/agent/internal/workflows/manager"
	"github.com/thand-io/agent/internal/workflows/tasks"
)

func createWorkflowService(cfg *config.Config) *manager.WorkflowManager {

	wm := manager.WorkflowManager{
		//config:    cfg,
		functions: functions.NewFunctionRegistry(cfg),
		tasks:     tasks.NewTaskRegistry(cfg),
	}

	// Register all custom tasks
	for _, task := range []tasks.TaskCollection{
		taskThand.NewThandCollection(cfg),
	} {
		task.RegisterTasks(wm.tasks)
	}

	// Register all built-in function providers
	for _, provider := range []functions.FunctionCollection{
		providerThand.NewThandCollection(cfg),
		providerSlack.NewSlackCollection(cfg),
		providerGcp.NewGCPCollection(cfg),
		providerAws.NewAWSCollection(cfg),
	} {
		provider.RegisterFunctions(wm.functions)
	}

	// If we have temporal configured, then we can register
	// all the activities and workflows

	if cfg.GetServices().HasTemporal() {

		// Register our activities
		err := wm.registerActivities()
		if err != nil {
			logrus.WithError(err).Error("Failed to register activities")
		}

		// Register our workflows
		err = wm.registerWorkflows()
		if err != nil {
			logrus.WithError(err).Error("Failed to register workflows")
		}
	}

	return &wm
}
