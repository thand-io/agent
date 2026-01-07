package runner

import (
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

func (d *ResumableWorkflowRunner) executeSwitchTask(
	taskKey string,
	switchTask *model.SwitchTask,
	input any,
) (*model.FlowDirective, error) {

	return SwitchTaskHandler(
		d.GetWorkflowTask(),
		input,
		taskKey,
		switchTask,
	)

}

func SwitchTaskHandler(
	workflowTask *sdkWorkflowsModel.WorkflowTask,
	input any,
	taskKey string,
	switchTask *model.SwitchTask,
) (*model.FlowDirective, error) {

	log := workflowTask.GetLogger()

	log.WithFields(logrus.Fields{
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

			result, err := workflowTask.TraverseAndEvaluateBool(
				model.NormalizeExpr(switchCase.When.String()), input)

			if err != nil {

				log.WithError(err).WithFields(logrus.Fields{
					"taskKey": taskKey,
					"case":    switchCase.When.String(),
					"input":   input,
				}).Error("Failed to evaluate switch case condition")

				return nil, model.NewErrExpression(err, taskKey)
			}
			if !result {

				log.WithFields(logrus.Fields{
					"taskKey": taskKey,
					"case":    switchCase.When.String(),
					"result":  result,
					"input":   input,
				}).Info("Switch case condition NOT matched")

			} else {

				log.WithFields(logrus.Fields{
					"taskKey": taskKey,
					"case":    switchCase.When.String(),
					"result":  result,
					"input":   input,
				}).Info("Switch case condition matched")

				if switchCase.Then == nil {

					log.WithFields(logrus.Fields{
						"taskKey": taskKey,
					}).Error("Missing 'then' directive in matched switch case")

					return nil, model.NewErrExpression(fmt.Errorf("missing 'then' directive in matched switch case"), taskKey)
				}
				return switchCase.Then, nil
			}
		}
	}
	if defaultThen != nil {

		log.WithFields(logrus.Fields{
			"taskKey": taskKey,
		}).Info("No switch cases matched, using default 'then' directive")

		return defaultThen, nil
	}

	log.WithFields(logrus.Fields{
		"taskKey": taskKey,
	}).Info("No switch cases matched and no default 'then' directive defined")

	return nil, model.NewErrExpression(fmt.Errorf("no matching switch case"), taskKey)
}
