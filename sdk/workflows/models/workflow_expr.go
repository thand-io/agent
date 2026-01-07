// Copyright 2025 The Serverless Workflow Specification Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"maps"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/interpolate"
)

func (t *WorkflowTask) TraverseAndEvaluateWithVars(node any, input any, variables map[string]any) (any, error) {
	if err := t.mergeContextInVars(variables); err != nil {
		return nil, err
	}
	return interpolate.NewTraverse(node, input, variables)
}

// TraverseAndEvaluate recursively processes and evaluates all expressions in a JSON-like structure
func (t *WorkflowTask) TraverseAndEvaluate(node any, input any) (any, error) {
	return t.TraverseAndEvaluateWithVars(node, input, map[string]any{})
}

func (t *WorkflowTask) mergeContextInVars(variables map[string]any) error {
	if variables == nil {
		variables = make(map[string]any)
	}
	// merge
	maps.Copy(variables, t.GetVars())

	return nil
}

func (t *WorkflowTask) TraverseAndEvaluateObj(runtimeExpr *model.ObjectOrRuntimeExpr, input any, taskName string) (output any, err error) {
	if runtimeExpr == nil {
		return input, nil
	}
	output, err = t.TraverseAndEvaluate(runtimeExpr.AsStringOrMap(), input)
	if err != nil {
		return nil, model.NewErrExpression(err, taskName)
	}
	return output, nil
}

func (t *WorkflowTask) TraverseAndEvaluateBool(runtimeExpr string, input any) (bool, error) {
	if len(runtimeExpr) == 0 {
		return false, nil
	}
	output, err := t.TraverseAndEvaluate(runtimeExpr, input)
	if err != nil {
		return false, nil
	}
	if result, ok := output.(bool); ok {
		return result, nil
	}
	return false, nil
}
