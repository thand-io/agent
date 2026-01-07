package models

import (
	"github.com/serverlessworkflow/sdk-go/v3/model"
)

type WorkflowTaskSupport interface {
	GetName() string

	HasState() bool
	ClearTaskContext()

	GetWorkflowDef() *model.Workflow
	SetWorkflowDef(workflow *model.Workflow)
}
