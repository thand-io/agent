package config

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflows "github.com/thand-io/agent/sdk/workflows/manager"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// This file creates long-running workflows for a given system id.

type ThandSystemStart struct {
	Identities []string
}

type ThandSystemImpl interface {
	GetIdentities() []string
}

type ServerWorkflowStart struct {
	ThandSystemStart
}

func (r *ServerWorkflowStart) GetIdentities() []string {
	return r.Identities
}

type AgentWorkflowStart struct {
	ThandSystemStart
}

func (r *AgentWorkflowStart) GetIdentities() []string {
	return r.Identities
}

type SystemWorkflowShutdown struct {
	Identities []string
	Reason     string
	ShutdownAt time.Time
}

type ServerWorkflowShutdown = SystemWorkflowShutdown
type AgentWorkflowShutdown = SystemWorkflowShutdown

func CreateServerWorkflow(
	configImpl models.ConfigImpl,
	start ServerWorkflowStart,
) func(workflow.Context, ServerWorkflowStart) (*ServerWorkflowShutdown, error) {
	_ = configImpl
	return func(ctx workflow.Context, req ServerWorkflowStart) (*ServerWorkflowShutdown, error) {
		if len(req.Identities) == 0 {
			req = start
		}
		shutdown, err := systemHandler(ctx, &req)
		if shutdown == nil {
			return nil, err
		}
		return (*ServerWorkflowShutdown)(shutdown), err
	}
}

func CreateAgentWorkflow(
	configImpl models.ConfigImpl,
	start AgentWorkflowStart,
) func(workflow.Context, AgentWorkflowStart) (*AgentWorkflowShutdown, error) {
	_ = configImpl
	return func(ctx workflow.Context, req AgentWorkflowStart) (*AgentWorkflowShutdown, error) {
		if len(req.Identities) == 0 {
			req = start
		}
		shutdown, err := systemHandler(ctx, &req)
		if shutdown == nil {
			return nil, err
		}
		return (*AgentWorkflowShutdown)(shutdown), err
	}
}

func systemHandler(
	rootCtx workflow.Context,
	req ThandSystemImpl,
) (outputShutdown *SystemWorkflowShutdown, outputError error) {

	log := workflow.GetLogger(rootCtx)
	log.Info("Primary workflow execution started")

	workflowInfo := workflow.GetInfo(rootCtx)
	log.Info("Primary workflow started.",
		"WorkflowID", workflowInfo.WorkflowExecution.ID,
		"RunID", workflowInfo.WorkflowExecution.RunID,
		"BuildID", workflowInfo.GetCurrentBuildID(),
	)

	cancelCtx, cancelHandler := workflow.WithCancel(rootCtx)

	identities := dedupeStringsStable(req.GetIdentities())
	shutdownReason := ""
	var terminationRequest *models.TemporalTerminationRequest

	if err := upsertIdentitiesSearchAttribute(cancelCtx, identities); err != nil {
		log.Error("Failed to upsert identities search attribute", "Error", err)
		return nil, err
	}

	defer func() {
		if r := recover(); r != nil {
			outputError = fmt.Errorf("workflow failed: %v", r)
			return
		}

		if cancelCtx.Err() != nil && (errors.Is(cancelCtx.Err(), context.Canceled) || temporal.IsCanceledError(cancelCtx.Err())) {
			outputError = nil
		}
		log.Info("Workflow cleanup completed.")
	}()

	// Query support: ping -> pong
	if err := setupSystemQueryHandlers(cancelCtx); err != nil {
		log.Error("Failed to set query handler", "Error", err)
		return nil, err
	}

	// Update handler: update identities (stable de-dupe)
	if err := workflow.SetUpdateHandler(cancelCtx, models.TemporalSystemUpdateIdentitiesUpdateName, func(ctx workflow.Context, newIdentities []string) ([]string, error) {
		log := workflow.GetLogger(ctx)
		identities = dedupeStringsStable(newIdentities)
		if err := upsertIdentitiesSearchAttribute(ctx, identities); err != nil {
			log.Error("Failed to upsert identities search attribute", "Error", err)
			return nil, err
		}
		log.Info("Identities updated", "Count", len(identities))
		return identities, nil
	}); err != nil {
		log.Error("Failed to set update handler", "Error", err, "UpdateName", models.TemporalSystemUpdateIdentitiesUpdateName)
		return nil, err
	}

	// Update handler: request graceful shutdown
	if err := workflow.SetUpdateHandler(cancelCtx, models.TemporalSystemShutdownUpdateName, func(ctx workflow.Context, reason string) error {
		log := workflow.GetLogger(ctx)
		shutdownReason = reason
		log.Info("Shutdown requested", "Reason", reason)
		cancelHandler()
		return nil
	}); err != nil {
		log.Error("Failed to set update handler", "Error", err, "UpdateName", models.TemporalSystemShutdownUpdateName)
		return nil, err
	}

	// Signal support
	heartbeatSignal := workflow.GetSignalChannel(cancelCtx, "heartbeat")
	terminateSignal := workflow.GetSignalChannel(cancelCtx, sdkWorkflowsModel.TemporalTerminateSignalName)

	sdkWorkflows.SetupTerminationHandler(rootCtx, terminateSignal, cancelHandler, &terminationRequest)

	selector := workflow.NewSelector(cancelCtx)
	selector.AddReceive(heartbeatSignal, func(c workflow.ReceiveChannel, more bool) {
		var payload map[string]string
		c.Receive(cancelCtx, &payload)
		log.Info("Heartbeat signal received")
	})

	log.Info("Starting main system workflow loop")
	for {
		if err := waitForSystemSignalOrCancel(cancelCtx, selector); err != nil {
			return nil, err
		}

		if cancelCtx.Err() != nil {
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				log.Info("Workflow context cancelled, exiting")
				break
			}
			log.Error("Error while waiting", "Error", cancelCtx.Err())
			return nil, cancelCtx.Err()
		}

		selector.Select(cancelCtx)
	}

	if terminationRequest != nil && len(shutdownReason) == 0 {
		shutdownReason = terminationRequest.Reason
	}

	return &SystemWorkflowShutdown{
		Identities: identities,
		Reason:     shutdownReason,
		ShutdownAt: workflow.Now(cancelCtx),
	}, nil
}

func setupSystemQueryHandlers(ctx workflow.Context) error {
	return workflow.SetQueryHandler(ctx, models.TemporalSystemPingQueryName, func() (string, error) {
		log := workflow.GetLogger(ctx)
		log.Info("Ping query received")
		return "pong", nil
	})
}

func waitForSystemSignalOrCancel(cancelCtx workflow.Context, selector workflow.Selector) error {
	return workflow.Await(cancelCtx, func() bool {
		if cancelCtx.Err() != nil {
			return true
		}
		return selector.HasPending()
	})
}

func dedupeStringsStable(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if len(v) == 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func upsertIdentitiesSearchAttribute(ctx workflow.Context, identities []string) error {
	if len(identities) == 0 {
		return nil
	}
	return workflow.UpsertTypedSearchAttributes(
		ctx,
		sdkConstants.TypedSearchAttributeIdentities.ValueSet(identities),
	)
}
