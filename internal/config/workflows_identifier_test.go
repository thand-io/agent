package config

import (
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestApplyWorkflows_KeyBecomesIdentifier verifies that the YAML map key used
// in a WorkflowDefinitions entry is assigned as the Identifier on the
// resulting Workflow and is not modified.
func TestApplyWorkflows_KeyBecomesIdentifier(t *testing.T) {
	cfg := &Config{mode: ModeServer}

	yamlKey := "onboarding-flow"
	defs := []*models.WorkflowDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0.0")),
			Workflows: map[string]models.Workflow{
				yamlKey: {
					Name:    "Onboarding Flow",
					Enabled: true,
					Workflow: &model.Workflow{
						Document: model.Document{
							DSL:  "1.0.0",
							Name: "onboarding-flow",
						},
						Do: &model.TaskList{},
					},
				},
			},
		},
	}

	result, err := cfg.ApplyWorkflows(defs)
	require.NoError(t, err)
	require.Len(t, result, 1, "expected exactly one workflow")

	wf, exists := result[yamlKey]
	require.True(t, exists, "expected the map key to be %q", yamlKey)
	assert.Equal(t, yamlKey, wf.Identifier, "Workflow.Identifier must equal the YAML key")
}
