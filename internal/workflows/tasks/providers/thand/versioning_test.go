package thand

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func TestChildWorkflowOptionsForTaskQueueInheritsBuildAcrossQueues(t *testing.T) {
	t.Parallel()

	opts := childWorkflowOptionsForTaskQueue(
		"thand_local_server_alpha",
		"thand_local_workstation_alpha",
		workflow.ChildWorkflowOptions{TaskQueue: "thand_local_workstation_alpha"},
	)

	assert.Equal(t, temporal.VersioningIntentInheritBuildID, opts.VersioningIntent)
}

func TestChildWorkflowOptionsForTaskQueueLeavesSameQueueUnspecified(t *testing.T) {
	t.Parallel()

	opts := childWorkflowOptionsForTaskQueue(
		"thand_local_server_alpha",
		"thand_local_server_alpha",
		workflow.ChildWorkflowOptions{TaskQueue: "thand_local_server_alpha"},
	)

	assert.Equal(t, temporal.VersioningIntentUnspecified, opts.VersioningIntent)
}

func TestChildWorkflowOptionsForTaskQueueLeavesUnknownQueueUnspecified(t *testing.T) {
	t.Parallel()

	opts := childWorkflowOptionsForTaskQueue(
		"",
		"thand_local_workstation_alpha",
		workflow.ChildWorkflowOptions{TaskQueue: "thand_local_workstation_alpha"},
	)

	assert.Equal(t, temporal.VersioningIntentUnspecified, opts.VersioningIntent)
}
