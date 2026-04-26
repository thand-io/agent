package models

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/workflow"
)

const TemporalSynchronizeWorkflowName = "synchronize"
const TemporalAuthorizeRoleWorkflowName = "authorize-role"
const TemporalRevokeRoleWorkflowName = "revoke-role"
const TemporalPatchProviderUpstreamActivityName = "patch-provider-upstream"
const TemporalBuildAuthorizeRoleRequestActivityName = "build-authorize-role-request"

func CreateTemporalProviderWorkflowIdentifier(identifier, base string) string {
	return CreateTemporalWorkflowIdentifier(fmt.Sprintf("%s-%s", identifier, base))
}

func CreateTemporalProviderWorkflowName(identifier, base string) string {
	return strings.ToLower(fmt.Sprintf("%s-%s", identifier, base))
}

// RegisterActivitiesForStruct registers all public methods of an arbitrary struct as Temporal
// activities using the naming pattern "<identifier>-<MethodName>". This enables provider-specific
// activity structs (e.g. awsProviderActivities) to be registered alongside the generic
// ProviderActivities without duplicating the reflection loop.
func RegisterActivities(temporalClient TemporalImpl, identifier string, s any) error {
	if temporalClient == nil {
		return ErrNotImplemented
	}

	worker := temporalClient.GetWorker()
	if worker == nil {
		return ErrNotImplemented
	}

	if len(identifier) == 0 {
		return fmt.Errorf("identifier cannot be empty")
	}

	if s == nil {
		return fmt.Errorf("cannot register activities for a nil struct")
	}

	structValue := reflect.ValueOf(s)
	structType := structValue.Type()
	count := 0

	for i := 0; i < structValue.NumMethod(); i++ {
		methodValue := structValue.Method(i)
		method := structType.Method(i)

		// skip unexported methods
		if len(method.PkgPath) != 0 {
			continue
		}

		name := method.Name

		if err := validateFnFormat(method.Type, false, false); err != nil {
			logrus.WithError(err).Errorf(
				"Failed to register method %s of struct %s for provider %s due to invalid function signature: %v", name, structType.Name(), identifier, err)
			return fmt.Errorf("method %s of %s: %w", name, structType.Name(), err)
		}

		activityName := CreateTemporalProviderWorkflowName(identifier, name)

		worker.RegisterActivityWithOptions(
			methodValue.Interface(),
			activity.RegisterOptions{
				Name: activityName,
			},
		)

		logrus.Debugf("Registered activity: %s for provider: %s", activityName, identifier)
		count++
	}

	if count == 0 {
		logrus.Debugf("No public methods found to register as activities for struct %s with provider identifier %s", structType.Name(), identifier)
		return fmt.Errorf("no activities (public methods) found at %v structure", structType.Name())
	}

	return nil
}

// Validate function parameters.
func validateFnFormat(fnType reflect.Type, isWorkflow, isDynamic bool) error {
	if fnType.Kind() != reflect.Func {
		return fmt.Errorf("expected a func as input but was %s", fnType.Kind())
	}
	if isWorkflow {
		if fnType.NumIn() < 1 {
			return fmt.Errorf(
				"expected at least one argument of type workflow.Context in function, found %d input arguments",
				fnType.NumIn(),
			)
		}
		if !isWorkflowContext(fnType.In(0)) {
			return fmt.Errorf("expected first argument to be workflow.Context but found %s", fnType.In(0))
		}
	} else {
		// For activities, check that workflow context is not accidentally provided
		// Activities registered with structs will have their receiver as the first argument so confirm it is not
		// in the first two arguments
		for i := 0; i < fnType.NumIn() && i < 2; i++ {
			if isWorkflowContext(fnType.In(i)) {
				return fmt.Errorf("unexpected use of workflow context for an activity")
			}
		}
	}

	if isDynamic {
		if fnType.NumIn() != 2 {
			return fmt.Errorf(
				"expected function to have two arguments, first being workflow.Context and second being an EncodedValues type, found %d arguments", fnType.NumIn(),
			)
		}
		if fnType.In(1) != reflect.TypeOf((*converter.EncodedValues)(nil)).Elem() {
			return fmt.Errorf("expected function to EncodedValues as second argument, got %s", fnType.In(1).Elem())
		}
	}

	// Return values
	// We expect either
	// 	<result>, error
	//	(or) just error
	if fnType.NumOut() < 1 || fnType.NumOut() > 2 {
		return fmt.Errorf(
			"expected function to return result, error or just error, but found %d return values", fnType.NumOut(),
		)
	}
	if fnType.NumOut() > 1 && !isValidResultType(fnType.Out(0)) {
		return fmt.Errorf(
			"expected function first return value to return valid type but found: %v", fnType.Out(0).Kind(),
		)
	}
	if !isError(fnType.Out(fnType.NumOut() - 1)) {
		return fmt.Errorf(
			"expected function second return value to return error but found %v", fnType.Out(fnType.NumOut()-1).Kind(),
		)
	}
	return nil
}

func isValidResultType(inType reflect.Type) bool {
	// https://golang.org/pkg/reflect/#Kind
	switch inType.Kind() {
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return false
	}

	return true
}

func isWorkflowContext(inType reflect.Type) bool {
	// NOTE: We don't expect any one to derive from workflow context.
	return inType == reflect.TypeOf((*workflow.Context)(nil)).Elem()
}

func isError(inType reflect.Type) bool {
	errorElem := reflect.TypeOf((*error)(nil)).Elem()
	return inType != nil && inType.Implements(errorElem)
}
