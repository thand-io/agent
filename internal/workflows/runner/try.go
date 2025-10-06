package runner

import (
	"fmt"
	"math"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"go.temporal.io/sdk/workflow"
)

// executeTryTask handles try/catch logic with error handling and retry functionality
func (r *ResumableWorkflowRunner) executeTryTask(
	taskName string,
	tryTask *model.TryTask,
	input any,
) (any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
	}).Info("Executing Try task")

	if tryTask.Try == nil {
		return nil, fmt.Errorf("try task requires 'try' block")
	}

	if tryTask.Catch == nil {
		return nil, fmt.Errorf("try task requires 'catch' block")
	}

	workflowTask := r.GetWorkflowTask()

	// Attempt to execute the try block
	tryOutput, tryErr := r.executeTaskList(tryTask.Try, input)

	// If no error occurred, return the successful output
	if tryErr == nil {
		return tryOutput, nil
	}

	logrus.WithFields(logrus.Fields{
		"task":  taskName,
		"error": tryErr,
	}).Debug("Try block failed, evaluating catch conditions")

	// Check if this error should be caught
	shouldCatch, err := r.shouldCatchError(tryErr, tryTask.Catch, input)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate catch conditions: %w", err)
	}

	if !shouldCatch {
		// Error doesn't match catch criteria, re-throw it
		return nil, tryErr
	}

	// Save the error in context if 'as' is specified
	errorVar := "error" // default
	if len(tryTask.Catch.As) > 0 {
		errorVar = tryTask.Catch.As
	}

	// Create error context
	errorContext := r.createErrorContext(tryErr)

	// Set error variable in workflow context
	if workflowTask.Context == nil {
		workflowTask.Context = make(map[string]any)
	}

	contextMap, ok := workflowTask.Context.(map[string]any)
	if !ok {
		contextMap = make(map[string]any)
		workflowTask.Context = contextMap
	}
	contextMap[errorVar] = errorContext

	// Handle retry logic if specified
	if tryTask.Catch.Retry != nil {
		retryOutput, retryErr := r.handleRetryLogic(taskName, tryTask, input, tryErr)
		if retryErr == nil {
			return retryOutput, nil
		}

		// If retry also failed, continue to catch block execution
		logrus.WithFields(logrus.Fields{
			"task":       taskName,
			"retryError": retryErr,
		}).Debug("Retry attempts exhausted, executing catch block")
	}

	// Execute catch block if specified
	if tryTask.Catch.Do != nil {
		logrus.WithFields(logrus.Fields{
			"task": taskName,
		}).Debug("Executing catch block")

		return r.executeTaskList(tryTask.Catch.Do, input)
	}

	// No catch block to execute, return the original error
	return nil, tryErr
}

// shouldCatchError determines if an error should be caught based on catch conditions
func (r *ResumableWorkflowRunner) shouldCatchError(err error, catch *model.TryTaskCatch, input any) (bool, error) {
	workflowTask := r.GetWorkflowTask()

	// Check error filter if specified
	if catch.Errors.With != nil {
		matches := r.errorMatchesFilter(err, catch.Errors.With)
		if !matches {
			return false, nil
		}
	}

	// Evaluate 'when' condition if specified
	if catch.When != nil {
		result, evalErr := workflowTask.TraverseAndEvaluateBool(catch.When.Value, input)
		if evalErr != nil {
			return false, fmt.Errorf("failed to evaluate 'when' condition: %w", evalErr)
		}
		if !result {
			return false, nil
		}
	}

	// Evaluate 'exceptWhen' condition if specified
	if catch.ExceptWhen != nil {
		result, evalErr := workflowTask.TraverseAndEvaluateBool(catch.ExceptWhen.Value, input)
		if evalErr != nil {
			return false, fmt.Errorf("failed to evaluate 'exceptWhen' condition: %w", evalErr)
		}
		if result {
			return false, nil
		}
	}

	return true, nil
}

// errorInfo holds extracted error information for filtering
type errorInfo struct {
	errorType     string
	errorStatus   int
	errorTitle    string
	errorDetails  string
	errorInstance string
}

// extractErrorInfo extracts error information from an error for filtering purposes
func (r *ResumableWorkflowRunner) extractErrorInfo(err error) errorInfo {
	info := errorInfo{}

	// Check if it's a model.Error
	if modelErr, ok := err.(*model.Error); ok {
		if modelErr.Type != nil {
			info.errorType = modelErr.Type.String()
		}
		info.errorStatus = modelErr.Status
		if modelErr.Title != nil {
			info.errorTitle = modelErr.Title.String()
		}
		if modelErr.Detail != nil {
			info.errorDetails = modelErr.Detail.String()
		}
		if modelErr.Instance != nil {
			info.errorInstance = modelErr.Instance.String()
		}
	}

	return info
}

// matchesFilterCriteria checks if error info matches all filter criteria
func (r *ResumableWorkflowRunner) matchesFilterCriteria(info errorInfo, filter *model.ErrorFilter) bool {
	return r.matchesType(info.errorType, filter.Type) &&
		r.matchesStatus(info.errorStatus, filter.Status) &&
		r.matchesTitle(info.errorTitle, filter.Title) &&
		r.matchesDetails(info.errorDetails, filter.Details) &&
		r.matchesInstance(info.errorInstance, filter.Instance)
}

// matchesType checks if error type matches filter type
func (r *ResumableWorkflowRunner) matchesType(errorType, filterType string) bool {
	return len(filterType) == 0 || errorType == filterType
}

// matchesStatus checks if error status matches filter status
func (r *ResumableWorkflowRunner) matchesStatus(errorStatus, filterStatus int) bool {
	return filterStatus == 0 || errorStatus == filterStatus
}

// matchesTitle checks if error title matches filter title
func (r *ResumableWorkflowRunner) matchesTitle(errorTitle, filterTitle string) bool {
	return len(filterTitle) == 0 || errorTitle == filterTitle
}

// matchesDetails checks if error details matches filter details
func (r *ResumableWorkflowRunner) matchesDetails(errorDetails, filterDetails string) bool {
	return len(filterDetails) == 0 || errorDetails == filterDetails
}

// matchesInstance checks if error instance matches filter instance
func (r *ResumableWorkflowRunner) matchesInstance(errorInstance, filterInstance string) bool {
	return len(filterInstance) == 0 || errorInstance == filterInstance
}

// errorMatchesFilter checks if an error matches the specified filter criteria
func (r *ResumableWorkflowRunner) errorMatchesFilter(err error, filter *model.ErrorFilter) bool {
	info := r.extractErrorInfo(err)
	return r.matchesFilterCriteria(info, filter)
}

// createErrorContext creates a context object for the caught error
func (r *ResumableWorkflowRunner) createErrorContext(err error) map[string]any {
	errorContext := make(map[string]any)

	// Check if it's a structured model.Error
	if modelErr, ok := err.(*model.Error); ok {
		if modelErr.Type != nil {
			errorContext["type"] = modelErr.Type.String()
		}
		errorContext["status"] = modelErr.Status
		if modelErr.Title != nil {
			errorContext["title"] = modelErr.Title.String()
		}
		if modelErr.Detail != nil {
			errorContext["detail"] = modelErr.Detail.String()
		}
		if modelErr.Instance != nil {
			errorContext["instance"] = modelErr.Instance.String()
		}
	} else {
		// For regular errors, just include the message
		errorContext["message"] = err.Error()
		errorContext["type"] = "runtime"
	}

	return errorContext
}

// retryConfig holds the configuration for retry attempts
type retryConfig struct {
	maxAttempts int
	maxDuration time.Duration
	startTime   time.Time
}

// validateRetryPolicy validates and extracts retry configuration
func (r *ResumableWorkflowRunner) validateRetryPolicy(taskName string, retryPolicy *model.RetryPolicy) (*retryConfig, error) {
	if retryPolicy == nil {
		return nil, fmt.Errorf("no retry policy")
	}

	// Resolve retry policy reference if needed
	if len(retryPolicy.Ref) > 0 {
		// TODO: Implement reference resolution from workflow.Use.Retries
		logrus.WithFields(logrus.Fields{
			"task": taskName,
			"ref":  retryPolicy.Ref,
		}).Warn("Retry policy references not yet implemented")
		return nil, fmt.Errorf("retry policy references not implemented")
	}

	config := &retryConfig{
		maxAttempts: 3, // default
		startTime:   time.Now(),
	}

	if retryPolicy.Limit.Attempt != nil && retryPolicy.Limit.Attempt.Count > 0 {
		config.maxAttempts = retryPolicy.Limit.Attempt.Count
	}

	if retryPolicy.Limit.Duration != nil {
		if duration, err := common.ValidateDuration(retryPolicy.Limit.Duration.AsExpression()); err == nil {
			config.maxDuration = duration
		}
	}

	return config, nil
}

// checkRetryLimits checks if retry limits have been exceeded
func (r *ResumableWorkflowRunner) checkRetryLimits(taskName string, config *retryConfig, attempt int) bool {
	// Check if we've exceeded the maximum duration
	if config.maxDuration > 0 && time.Since(config.startTime) > config.maxDuration {
		logrus.WithFields(logrus.Fields{
			"task":        taskName,
			"attempt":     attempt,
			"maxDuration": config.maxDuration,
		}).Debug("Retry duration limit exceeded")
		return false
	}
	return true
}

// executeRetryAttempt executes a single retry attempt with delay
func (r *ResumableWorkflowRunner) executeRetryAttempt(
	taskName string,
	tryTask *model.TryTask,
	input any,
	retryPolicy *model.RetryPolicy,
	attempt int,
) (any, error) {
	workflowTask := r.GetWorkflowTask()

	// Calculate delay before retry
	delay := r.calculateRetryDelay(retryPolicy, attempt)

	if delay > 0 {
		logrus.WithFields(logrus.Fields{
			"task":    taskName,
			"attempt": attempt,
			"delay":   delay,
		}).Info("Retrying task after delay")

		// Sleep using appropriate method
		if workflowTask.HasTemporalContext() {
			if err := workflow.Sleep(workflowTask.GetTemporalContext(), delay); err != nil {
				return nil, fmt.Errorf("failed to sleep during retry: %w", err)
			}
		} else {
			time.Sleep(delay)
		}
	}

	// Retry the try block
	logrus.WithFields(logrus.Fields{
		"task":    taskName,
		"attempt": attempt,
	}).Info("Retrying try block")

	return r.executeTaskList(tryTask.Try, input)
}

// handleRetryLogic implements the retry mechanism with backoff strategies
func (r *ResumableWorkflowRunner) handleRetryLogic(taskName string, tryTask *model.TryTask, input any, originalErr error) (any, error) {
	retryPolicy := tryTask.Catch.Retry

	config, err := r.validateRetryPolicy(taskName, retryPolicy)
	if err != nil {
		return nil, originalErr
	}

	for attempt := 1; attempt <= config.maxAttempts; attempt++ {
		if !r.checkRetryLimits(taskName, config, attempt) {
			break
		}

		// Check retry conditions
		shouldRetry, err := r.shouldRetry(retryPolicy, input, originalErr)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"task":  taskName,
				"error": err,
			}).Warn("Failed to evaluate retry conditions")
			break
		}

		if !shouldRetry {
			logrus.WithFields(logrus.Fields{
				"task":    taskName,
				"attempt": attempt,
			}).Debug("Retry conditions not met")
			break
		}

		retryOutput, retryErr := r.executeRetryAttempt(taskName, tryTask, input, retryPolicy, attempt)
		if retryErr == nil {
			logrus.WithFields(logrus.Fields{
				"task":    taskName,
				"attempt": attempt,
			}).Info("Retry succeeded")
			return retryOutput, nil
		}

		// Log retry failure
		logrus.WithFields(logrus.Fields{
			"task":    taskName,
			"attempt": attempt,
			"error":   retryErr,
		}).Debug("Retry attempt failed")

		originalErr = retryErr // Update with latest error
	}

	return nil, originalErr
}

// shouldRetry evaluates retry conditions
func (r *ResumableWorkflowRunner) shouldRetry(retryPolicy *model.RetryPolicy, input any, err error) (bool, error) {
	workflowTask := r.GetWorkflowTask()

	// Evaluate 'when' condition if specified
	if retryPolicy.When != nil {
		result, evalErr := workflowTask.TraverseAndEvaluateBool(retryPolicy.When.Value, input)
		if evalErr != nil {
			return false, fmt.Errorf("failed to evaluate retry 'when' condition: %w", evalErr)
		}
		if !result {
			return false, nil
		}
	}

	// Evaluate 'exceptWhen' condition if specified
	if retryPolicy.ExceptWhen != nil {
		result, evalErr := workflowTask.TraverseAndEvaluateBool(retryPolicy.ExceptWhen.Value, input)
		if evalErr != nil {
			return false, fmt.Errorf("failed to evaluate retry 'exceptWhen' condition: %w", evalErr)
		}
		if result {
			return false, nil
		}
	}

	return true, nil
}

// calculateRetryDelay calculates the delay before the next retry attempt
func (r *ResumableWorkflowRunner) calculateRetryDelay(retryPolicy *model.RetryPolicy, attempt int) time.Duration {
	var baseDelay time.Duration

	// Get base delay
	if retryPolicy.Delay != nil {
		if duration, err := common.ValidateDuration(retryPolicy.Delay.AsExpression()); err == nil {
			baseDelay = duration
		}
	}

	if baseDelay == 0 {
		baseDelay = 1 * time.Second // default delay
	}

	// Apply backoff strategy
	if retryPolicy.Backoff != nil {
		if retryPolicy.Backoff.Exponential != nil {
			// Exponential backoff: delay = baseDelay * (2^(attempt-1))
			multiplier := math.Pow(2, float64(attempt-1))
			baseDelay = time.Duration(float64(baseDelay) * multiplier)
		} else if retryPolicy.Backoff.Linear != nil {
			// Linear backoff: delay = baseDelay * attempt
			baseDelay = time.Duration(int64(baseDelay) * int64(attempt))
		}
		// Constant backoff: delay remains baseDelay (no change needed)
	}

	// Apply jitter if specified
	if retryPolicy.Jitter != nil {
		// TODO: Implement jitter logic
		logrus.Debug("Jitter not yet implemented for retry delays")
	}

	return baseDelay
}
