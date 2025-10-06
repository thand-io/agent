package runner

import (
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

var ErrorEmitUnsupported = fmt.Errorf("emit task is only supported in temporal workflows")

// executeEmitTask handles event emission according to the serverless workflow spec
func (r *ResumableWorkflowRunner) executeEmitTask(
	taskName string,
	emit *model.EmitTask,
	input any,
) (any, error) {

	logrus.Infof("Executing emit task: %s", taskName)

	workflowTask := r.GetWorkflowTask()

	if workflowTask == nil {
		return nil, fmt.Errorf("workflow task is not set")
	}

	if workflowTask.HasTemporalContext() {

		// Create the cloud event based on the emit task specification
		event, err := r.createCloudEventFromEmit(emit, input)
		if err != nil {
			return nil, fmt.Errorf("failed to create cloud event: %w", err)
		}

		// Get temporal context
		ctx := workflowTask.GetTemporalContext()
		if ctx == nil {
			return nil, fmt.Errorf("failed to get temporal context")
		}

		// Send the signal to the current workflow using the event signal channel
		workflow.SignalExternalWorkflow(
			ctx,
			workflowTask.WorkflowID,
			models.TemporalEmptyRunId, // empty run ID means current run
			models.TemporalEventSignalName,
			event,
		)

		logrus.WithFields(logrus.Fields{
			"taskName":    taskName,
			"eventType":   event.Type(),
			"eventSource": event.Source(),
		}).Info("Successfully emitted event")

		// Return the event as output for potential use by next tasks
		return event, nil

	} else {

		logrus.WithFields(logrus.Fields{
			"taskName": taskName,
		}).Info("Not a Temporal workflow, emit task is not supported")

		return nil, ErrorEmitUnsupported
	}
}

// createCloudEventFromEmit creates a CloudEvent from the emit task specification
func (r *ResumableWorkflowRunner) createCloudEventFromEmit(
	emit *model.EmitTask,
	input any,
) (*cloudevents.Event, error) {

	if emit.Emit.Event.With == nil {
		return nil, fmt.Errorf("emit.event.with is required")
	}

	eventProps := emit.Emit.Event.With
	event := cloudevents.NewEvent()

	// Set required fields
	if err := r.setRequiredEventFields(&event, eventProps, input); err != nil {
		return nil, err
	}

	// Set optional fields
	if err := r.setOptionalEventFields(&event, eventProps, input); err != nil {
		return nil, err
	}

	// Set event data
	if err := r.setEventData(&event, eventProps, input); err != nil {
		return nil, err
	}

	// Set additional extensions
	r.setEventExtensions(&event, eventProps)

	return &event, nil
}

// setRequiredEventFields sets the required CloudEvent fields
func (r *ResumableWorkflowRunner) setRequiredEventFields(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
	input any,
) error {
	// Set source
	if err := r.setEventSource(event, eventProps, input); err != nil {
		return err
	}

	// Set type
	if len(eventProps.Type) == 0 {
		return fmt.Errorf("emit.event.with.type is required")
	}
	event.SetType(eventProps.Type)

	return nil
}

// setEventSource sets the event source field
func (r *ResumableWorkflowRunner) setEventSource(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
	input any,
) error {
	if eventProps.Source != nil && eventProps.Source.Value != nil {
		sourceValue, err := r.evaluateRuntimeExpression(eventProps.Source.Value, input)
		if err != nil {
			return fmt.Errorf("failed to evaluate source expression: %w", err)
		}

		sourceStr, ok := sourceValue.(string)
		if !ok {
			return fmt.Errorf("source must evaluate to a string")
		}

		event.SetSource(sourceStr)
	} else {
		return fmt.Errorf("source is required")
	}
	return nil
}

// setOptionalEventFields sets the optional CloudEvent fields
func (r *ResumableWorkflowRunner) setOptionalEventFields(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
	input any,
) error {
	// Set ID if provided
	if len(eventProps.ID) != 0 {
		event.SetID(eventProps.ID)
	}

	// Set subject if provided
	if len(eventProps.Subject) != 0 {
		event.SetSubject(eventProps.Subject)
	}

	// Set time if provided
	if err := r.setEventTime(event, eventProps, input); err != nil {
		return err
	}

	// Set data content type if provided
	if len(eventProps.DataContentType) != 0 {
		event.SetDataContentType(eventProps.DataContentType)
	}

	// Set data schema if provided
	return r.setEventDataSchema(event, eventProps, input)
}

// setEventTime sets the event time field
func (r *ResumableWorkflowRunner) setEventTime(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
	input any,
) error {
	if eventProps.Time == nil || eventProps.Time.Value == nil {
		return nil
	}

	timeValue, err := r.evaluateRuntimeExpression(eventProps.Time.Value, input)
	if err != nil {
		return fmt.Errorf("failed to evaluate time expression: %w", err)
	}

	// Set as extension since CloudEvents library handles time differently
	event.SetExtension("time", timeValue)
	return nil
}

// setEventDataSchema sets the event data schema field
func (r *ResumableWorkflowRunner) setEventDataSchema(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
	input any,
) error {
	if eventProps.DataSchema == nil || eventProps.DataSchema.Value == nil {
		return nil
	}

	dataSchemaValue, err := r.evaluateRuntimeExpression(eventProps.DataSchema.Value, input)
	if err != nil {
		return fmt.Errorf("failed to evaluate dataschema expression: %w", err)
	}

	dataSchemaStr, ok := dataSchemaValue.(string)
	if !ok {
		return fmt.Errorf("dataschema must evaluate to a string")
	}

	event.SetDataSchema(dataSchemaStr)
	return nil
}

// setEventData sets the event data
func (r *ResumableWorkflowRunner) setEventData(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
	input any,
) error {
	// Determine event data source
	eventData := r.getEventData(eventProps, input)

	if eventData == nil {
		return nil
	}

	err := event.SetData(cloudevents.ApplicationJSON, eventData)
	if err != nil {
		return fmt.Errorf("failed to set event data: %w", err)
	}

	return nil
}

// getEventData determines the event data from properties or input
func (r *ResumableWorkflowRunner) getEventData(
	eventProps *model.EventProperties,
	input any,
) any {
	// Check if data is specified in additional properties
	if eventProps.Additional != nil {
		if data, exists := eventProps.Additional["data"]; exists {
			return data
		}
	}

	// Use input as fallback
	return input
}

// setEventExtensions sets additional event extensions
func (r *ResumableWorkflowRunner) setEventExtensions(
	event *cloudevents.Event,
	eventProps *model.EventProperties,
) {
	if eventProps.Additional == nil {
		return
	}

	for key, value := range eventProps.Additional {
		if key != "data" { // Skip data as we handle it separately
			event.SetExtension(key, value)
		}
	}
}

// evaluateRuntimeExpression evaluates runtime expressions
func (r *ResumableWorkflowRunner) evaluateRuntimeExpression(expression any, input any) (any, error) {
	// If it's already a string, return as-is
	if str, ok := expression.(string); ok {
		// Check if it looks like a runtime expression (starts with $)
		str := strings.TrimSpace(str)

		// Check if the string is a runtime expression (e.g., ${ .some.path })
		if model.IsStrictExpr(str) {
			// Use the existing workflow task evaluation method
			return r.workflowTask.TraverseAndEvaluate(str, input)
		}
		return str, nil
	}

	// If it's some other type, try to use it directly
	return expression, nil
}
