package models

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureWorkflowTaskLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	logger := logrus.StandardLogger()
	oldOut := logger.Out
	oldLevel := logger.Level
	oldFormatter := logger.Formatter

	buf := &bytes.Buffer{}
	logger.SetOutput(buf)
	logger.SetLevel(logrus.TraceLevel)

	t.Cleanup(func() {
		logger.SetOutput(oldOut)
		logger.SetLevel(oldLevel)
		logger.SetFormatter(oldFormatter)
	})

	return buf
}

func TestGetInputAsCloudEventIgnoresNonCloudEventInput(t *testing.T) {
	buf := captureWorkflowTaskLogs(t)
	task := &WorkflowTask{
		Input: map[string]any{
			"reason": "sudo",
			"device": "device-alpha",
		},
	}

	event := task.GetInputAsCloudEvent()

	require.Nil(t, event)
	assert.Empty(t, buf.String())
}

func TestGetInputAsCloudEventLogsMalformedCloudEventInput(t *testing.T) {
	buf := captureWorkflowTaskLogs(t)
	task := &WorkflowTask{
		Input: map[string]any{
			"source": "test://source",
			"type":   "com.test.resume",
		},
	}

	event := task.GetInputAsCloudEvent()

	require.Nil(t, event)
	assert.Contains(t, buf.String(), "failed to unmarshal cloudevent from workflow input")
}
