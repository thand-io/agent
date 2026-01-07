package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/sdk/workflows/models"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/workflow"
)

// executeListenTask handles tasks that wait for events
func (r *ResumableWorkflowRunner) executeListenTask(
	taskName string,
	listen *model.ListenTask,
	input any,
) (any, error) {

	return ListenTaskHandler(r.workflowTask, taskName, listen, input)

}

func ListenTaskHandler(
	workflowTask *sdkWorkflowsModel.WorkflowTask,
	taskName string,
	listen *model.ListenTask,
	input any,
) (any, error) {

	log := workflowTask.GetLogger()

	log.WithFields(logrus.Fields{
		"taskName": taskName,
	}).Info("Got signal")

	if workflowTask.HasTemporalContext() {

		cancelCtx := workflowTask.GetTemporalContext()

		resumeChan := workflow.GetSignalChannel(cancelCtx, models.TemporalResumeSignalName)
		signalChan := workflow.GetSignalChannel(cancelCtx, models.TemporalEventSignalName)

		workflowSelector := workflow.NewSelector(cancelCtx)

		workflowSelector.AddReceive(resumeChan, func(c workflow.ReceiveChannel, more bool) {

			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Receiving resume signal...")

			// Otherwise receive the new signal
			var resumeableWorkflow models.WorkflowTask
			c.Receive(cancelCtx, &resumeableWorkflow)

			input = resumeableWorkflow.GetInput()

		})

		workflowSelector.AddReceive(signalChan, func(c workflow.ReceiveChannel, more bool) {

			// Check if its a brand new cloudevent input or a task resumption

			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Receiving event signal...")

			var signalEvent cloudevents.Event
			c.Receive(cancelCtx, &signalEvent)

			input = signalEvent

		})

		// Lets our selector know that the context may be cancelled
		// so we can handle it appropriately
		workflowSelector.AddFuture(workflow.NewTimer(cancelCtx, 0), func(f workflow.Future) {
			// This will be triggered immediately if context is already cancelled
			// or when context gets cancelled

			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Adding timer to detect context cancellation")

			if cancelCtx.Err() != nil {
				log.WithFields(logrus.Fields{
					"taskName": taskName,
				}).Info("Context cancellation detected via timer")
			}
		})

		// This will be triggered immediately by the NewTime above
		workflowSelector.Select(cancelCtx)

		for {

			// Wait for any of the signals
			err := workflow.Await(cancelCtx, func() bool {

				if cancelCtx.Err() != nil {

					if errors.Is(cancelCtx.Err(), context.Canceled) {
						// Context was cancelled
						log.WithFields(logrus.Fields{
							"taskName": taskName,
						}).Info("Context was cancelled")
					}
					// return true to exit the wait loop
					return true
				}

				pending := workflowSelector.HasPending()

				log.WithFields(logrus.Fields{
					"taskName": taskName,
				}).Info("Signal listen pending")

				return pending
			})

			if err != nil {

				log.WithError(err).Error("Error while waiting for signal")
				return nil, err

			} else if cancelCtx.Err() != nil {

				if errors.Is(cancelCtx.Err(), context.Canceled) {
					log.WithFields(logrus.Fields{
						"taskName": taskName,
					}).Info("Workflow context cancelled, exiting main loop")
					break
				}

				log.WithError(cancelCtx.Err()).Error("Error while waiting for signal")
				return nil, cancelCtx.Err()
			}

			// No signal received yet, so we are in waiting state
			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Waiting for signal... for listening task")

			workflowSelector.Select(cancelCtx)

			if input == nil {
				// The signal is empty so lets return

				log.WithFields(logrus.Fields{
					"taskName": taskName,
				}).Info("Empty signal input yet, continuing to listen...")
				continue

			}

			// We now have a signal to process
			out, err := handleListenTask(workflowTask, taskName, listen, input)

			if err != nil {

				log.WithError(err).Error("Failed to handle listen task")
				return nil, err
			}

			if out == nil {

				log.WithFields(logrus.Fields{
					"taskName": taskName,
				}).Info("Still listening for more events...")
				continue
			}

			log.WithFields(logrus.Fields{
				"taskName": taskName,
				"signal":   input,
			}).Info("Received event, exiting listen task")

			return out, nil

		}

		return nil, fmt.Errorf("context cancelled, exiting listen task")

	} else {

		if common.IsNilOrZero(input) {
			// if temporal then wait for signal, otherwise just return

			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Not a Temporal workflow, cannot wait for signal")

			return nil, ErrorAwaitSignal

		}

		// Otherwise we have a signal to process
		out, err := handleListenTask(workflowTask, taskName, listen, input)

		if err != nil {

			log.WithError(err).Error("Failed to handle listen task")

			return nil, err
		}

		if out == nil {

			log.WithFields(logrus.Fields{
				"taskName": taskName,
			}).Info("Still listening for more events... but cannot wait as not a temporal workflow")

			return nil, ErrorAwaitSignal

		}

		return out, nil

	}

}

func handleListenTask(
	workflowTask *sdkWorkflowsModel.WorkflowTask,
	taskName string,
	listen *model.ListenTask,
	input any,
) (*cloudevents.Event, error) {

	if common.IsNilOrZero(input) {
		return nil, fmt.Errorf("no signal input provided")
	}

	log := workflowTask.GetLogger()

	// Right lets validate the signal and covert it to a cloudevent
	var signal cloudevents.Event
	err := common.ConvertInterfaceToInterface(input, &signal)

	if err != nil {

		log.WithError(err).Error("Failed to convert signal to cloudevent")
		return nil, fmt.Errorf("failed to convert signal to cloudevent: %w", err)

	}

	if listen.Listen.To == nil {

		log.Error("To in listener not defined")
		return nil, fmt.Errorf("to in listener not defined")
	}

	oneListener := listen.Listen.To.One
	anyListener := listen.Listen.To.Any
	untilListener := listen.Listen.To.Until
	allListener := listen.Listen.To.All

	if oneListener != nil {

		// Configures the workflow to wait for all defined events before resuming execution.
		// Required if any and one have not been set.
		if evaluateListenFilter(workflowTask, oneListener, signal) {
			return &signal, nil
		}

	} else if anyListener != nil {

		// Configures the workflow to wait for any of the defined events before resuming execution.
		// Required if all and one have not been set.
		// If empty, listens to all incoming events

		if evaluateAnyListener(workflowTask, anyListener, signal) {
			return &signal, nil
		}

	} else if untilListener != nil {

		// Configures the workflow to wait for the defined event before resuming execution.
		// Required if all and any have not been set.

		if evaluateUntilEventFilter(workflowTask, untilListener, signal) {
			return &signal, nil
		}

	} else if allListener != nil {

		// Configures the runtime expression condition or the events that must be consumed to stop listening.
		// Only applies if any has been set, otherwise ignored.
		// If not present, once any event is received, it proceeds to the next task.

		if evaluateAllListener(workflowTask, allListener, signal) {
			return &signal, nil
		}

	} else {
		return nil, fmt.Errorf("no valid listener defined")
	}

	log.WithFields(logrus.Fields{
		"taskName": taskName,
		"signal":   signal,
	}).Info("Listening for more events ...")

	return nil, nil
}

func evaluateUntilEventFilter(
	workflowTask *sdkWorkflowsModel.WorkflowTask,
	listenUntil *model.EventConsumptionUntil,
	signal cloudevents.Event,
) bool {

	log := workflowTask.GetLogger()

	if strings.Compare(listenUntil.Strategy.One.With.Type, signal.Type()) != 0 {

		// Match type now lets check the data

		result, err := workflowTask.TraverseAndEvaluateBool(
			listenUntil.Condition.String(), signal.DataAs(map[string]any{}))

		if err != nil {
			log.WithError(err).Error("Failed to evaluate event filter")
			return false
		}

		return result

	}

	return false
}

func evaluateListenEvent(with *model.EventProperties, signal cloudevents.Event) bool {
	return strings.Compare(with.Type, signal.Type()) == 0
}

func evaluateListenFilter(workflowTask *sdkWorkflowsModel.WorkflowTask, eventFilter *model.EventFilter, signal cloudevents.Event) bool {

	if eventFilter.With != nil {

		return evaluateListenEvent(eventFilter.With, signal)

	}

	return false
}

func evaluateAnyListener(workflowTask *sdkWorkflowsModel.WorkflowTask, anyListener []*model.EventFilter, signal cloudevents.Event) bool {

	for _, eventFilter := range anyListener {

		if evaluateListenFilter(workflowTask, eventFilter, signal) {
			return true
		}

	}

	return false
}

func evaluateAllListener(workflowTask *sdkWorkflowsModel.WorkflowTask, allListener []*model.EventFilter, signal cloudevents.Event) bool {

	for _, eventFilter := range allListener {

		if evaluateListenFilter(workflowTask, eventFilter, signal) {
			return true
		}

	}

	return false
}
