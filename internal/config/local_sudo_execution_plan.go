package config

import "github.com/thand-io/agent/internal/models"

// localSudoExecutionPlanDecorator is a no-op until local sudo request shaping
// lands. Keeping the hook in the execution-plan layer lets later commits add
// the feature without reshaping this baseline.
type localSudoExecutionPlanDecorator struct{}

func (localSudoExecutionPlanDecorator) Applies(*models.ElevateRequestInternal) bool {
	return false
}

func (localSudoExecutionPlanDecorator) Decorate(
	models.ConfigImpl,
	*models.WorkflowRoleRequest,
	*models.ElevateRequestInternal,
	executionPlanBuildOptions,
) error {
	return nil
}

func (localSudoExecutionPlanDecorator) Finalize(
	*models.WorkflowRoleRequest,
	*models.ElevateRequestInternal,
	string,
) error {
	return nil
}
