package thand

import (
	"strings"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// childWorkflowOptionsForTaskQueue preserves the current workflow build when a
// child is dispatched onto a different task queue. Without this, deployment-
// versioned workers can fail to pick up cross-queue child workflows.
func childWorkflowOptionsForTaskQueue(
	currentQueue string,
	targetQueue string,
	opts workflow.ChildWorkflowOptions,
) workflow.ChildWorkflowOptions {
	currentQueue = strings.TrimSpace(currentQueue)
	targetQueue = strings.TrimSpace(targetQueue)
	if targetQueue != "" && currentQueue != "" && targetQueue != currentQueue {
		opts.VersioningIntent = temporal.VersioningIntentInheritBuildID
	}
	return opts
}
