package runner

import (
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

func (d *ResumableWorkflowRunner) evaluateSwitchTask(input any, taskKey string, switchTask *model.SwitchTask) (*model.FlowDirective, error) {

	logrus.WithFields(logrus.Fields{
		"taskKey": taskKey,
	}).Info("Evaluating switch task")

	if switchTask == nil || len(switchTask.Switch) == 0 {
		return nil, model.NewErrExpression(fmt.Errorf("no switch cases defined"), taskKey)
	}

	var defaultThen *model.FlowDirective
	for _, switchItem := range switchTask.Switch {
		for _, switchCase := range switchItem {

			if switchCase.When == nil {
				defaultThen = switchCase.Then
				continue
			}

			result, err := d.GetWorkflowTask().TraverseAndEvaluateBool(
				model.NormalizeExpr(switchCase.When.String()), input)

			if err != nil {

				logrus.WithError(err).WithFields(logrus.Fields{
					"taskKey": taskKey,
					"case":    switchCase.When.String(),
					"input":   input,
				}).Error("Failed to evaluate switch case condition")

				return nil, model.NewErrExpression(err, taskKey)
			}
			if !result {

				logrus.WithFields(logrus.Fields{
					"taskKey": taskKey,
					"case":    switchCase.When.String(),
					"result":  result,
					"input":   input,
				}).Info("Switch case condition NOT matched")

			} else {

				logrus.WithFields(logrus.Fields{
					"taskKey": taskKey,
					"case":    switchCase.When.String(),
					"result":  result,
					"input":   input,
				}).Info("Switch case condition matched")

				if switchCase.Then == nil {

					logrus.WithFields(logrus.Fields{
						"taskKey": taskKey,
					}).Error("Missing 'then' directive in matched switch case")

					return nil, model.NewErrExpression(fmt.Errorf("missing 'then' directive in matched switch case"), taskKey)
				}
				return switchCase.Then, nil
			}
		}
	}
	if defaultThen != nil {

		logrus.WithFields(logrus.Fields{
			"taskKey": taskKey,
		}).Info("No switch cases matched, using default 'then' directive")

		return defaultThen, nil
	}

	logrus.WithFields(logrus.Fields{
		"taskKey": taskKey,
	}).Info("No switch cases matched and no default 'then' directive defined")

	return nil, model.NewErrExpression(fmt.Errorf("no matching switch case"), taskKey)
}
