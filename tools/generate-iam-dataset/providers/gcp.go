package providers

import (
	"encoding/json"
	"os"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/thand-io/agent/internal/data/iam-dataset/generated/gcp"
)

func GenerateGCPFlatBuffers() error {
	// Read and parse JSON
	rolesData, err := os.ReadFile("third_party/iam-dataset/gcp/predefined_roles.json")
	if err != nil {
		return err
	}

	type GcpPredefinedRole struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Stage       string `json:"stage"`
		// Note: We're intentionally not including Etag as it's not used by the provider
	}

	var roles []GcpPredefinedRole
	if err := json.Unmarshal(rolesData, &roles); err != nil {
		return err
	}

	// Create FlatBuffer
	builder := flatbuffers.NewBuilder(1024)

	// Create roles
	var gcpRoles []flatbuffers.UOffsetT
	for _, role := range roles {
		nameOffset := builder.CreateString(role.Name)
		titleOffset := builder.CreateString(role.Title)
		descOffset := builder.CreateString(role.Description)
		stageOffset := builder.CreateString(role.Stage)

		gcp.PredefinedRoleStart(builder)
		gcp.PredefinedRoleAddName(builder, nameOffset)
		gcp.PredefinedRoleAddTitle(builder, titleOffset)
		gcp.PredefinedRoleAddDescription(builder, descOffset)
		gcp.PredefinedRoleAddStage(builder, stageOffset)
		predefinedRole := gcp.PredefinedRoleEnd(builder)
		gcpRoles = append(gcpRoles, predefinedRole)
	}

	// Create roles vector
	gcp.PredefinedRolesListStartRolesVector(builder, len(gcpRoles))
	for i := len(gcpRoles) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(gcpRoles[i])
	}
	rolesVector := builder.EndVector(len(gcpRoles))

	// Create root table
	gcp.PredefinedRolesListStart(builder)
	gcp.PredefinedRolesListAddRoles(builder, rolesVector)
	predefinedRolesList := gcp.PredefinedRolesListEnd(builder)

	// Finish and write
	builder.Finish(predefinedRolesList)
	return os.WriteFile("internal/data/iam-dataset/gcp/predefined_roles.fb", builder.FinishedBytes(), 0644)
}
