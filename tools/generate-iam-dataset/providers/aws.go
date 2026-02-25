package providers

import (
	"encoding/json"
	"os"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/thand-io/agent/internal/data/iam-dataset/generated/aws"
)

func GenerateAWSFlatBuffers() error {
	// Generate AWS Permissions FlatBuffer
	if err := generateAWSPermissions(); err != nil {
		return err
	}

	// Generate AWS Roles FlatBuffer
	if err := generateAWSRoles(); err != nil {
		return err
	}

	return nil
}

func generateAWSPermissions() error {
	// Read and parse JSON
	docsData, err := os.ReadFile("third_party/iam-dataset/aws/docs.json")
	if err != nil {
		return err
	}

	var docs map[string]string
	if err := json.Unmarshal(docsData, &docs); err != nil {
		return err
	}

	// Create HTML to Markdown converter
	converter := md.NewConverter("", true, nil)

	// Create FlatBuffer
	builder := flatbuffers.NewBuilder(1024)

	// Create permissions
	var permissions []flatbuffers.UOffsetT
	for rawName, description := range docs {
		// Convert EC2.AllocateAddress → ec2:AllocateAddress (lowercase service prefix, colon separator)
		colonIdx := strings.Index(rawName, ".")
		var name string
		if colonIdx >= 0 {
			name = strings.ToLower(rawName[:colonIdx]) + ":" + rawName[colonIdx+1:]
		} else {
			name = rawName
		}

		// Convert HTML description to markdown
		markdownDesc, err := converter.ConvertString(description)
		if err != nil {
			// If conversion fails, use original description
			markdownDesc = description
		}

		nameOffset := builder.CreateString(name)
		descOffset := builder.CreateString(markdownDesc)

		aws.PermissionStart(builder)
		aws.PermissionAddName(builder, nameOffset)
		aws.PermissionAddDescription(builder, descOffset)
		permission := aws.PermissionEnd(builder)
		permissions = append(permissions, permission)
	}

	// Create permissions vector
	aws.PermissionsListStartPermissionsVector(builder, len(permissions))
	for i := len(permissions) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(permissions[i])
	}
	permissionsVector := builder.EndVector(len(permissions))

	// Create root table
	aws.PermissionsListStart(builder)
	aws.PermissionsListAddPermissions(builder, permissionsVector)
	permissionsList := aws.PermissionsListEnd(builder)

	// Finish and write
	builder.Finish(permissionsList)
	return os.WriteFile("internal/data/iam-dataset/aws/docs.fb", builder.FinishedBytes(), 0644)
}

func generateAWSRoles() error {
	// Read and parse JSON
	rolesData, err := os.ReadFile("third_party/iam-dataset/aws/managed_policies.json")
	if err != nil {
		return err
	}

	type AwsManagedPolicies struct {
		Policies []struct {
			Name string `json:"name"`
		} `json:"policies"`
	}

	var awsRoles AwsManagedPolicies
	if err := json.Unmarshal(rolesData, &awsRoles); err != nil {
		return err
	}

	// Create FlatBuffer
	builder := flatbuffers.NewBuilder(1024)

	// Create policies
	var policies []flatbuffers.UOffsetT
	for _, policy := range awsRoles.Policies {
		nameOffset := builder.CreateString(policy.Name)

		aws.ManagedPolicyStart(builder)
		aws.ManagedPolicyAddName(builder, nameOffset)
		managedPolicy := aws.ManagedPolicyEnd(builder)
		policies = append(policies, managedPolicy)
	}

	// Create policies vector
	aws.ManagedPoliciesListStartPoliciesVector(builder, len(policies))
	for i := len(policies) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(policies[i])
	}
	policiesVector := builder.EndVector(len(policies))

	// Create root table
	aws.ManagedPoliciesListStart(builder)
	aws.ManagedPoliciesListAddPolicies(builder, policiesVector)
	managedPoliciesList := aws.ManagedPoliciesListEnd(builder)

	// Finish and write
	builder.Finish(managedPoliciesList)
	return os.WriteFile("internal/data/iam-dataset/aws/managed_policies.fb", builder.FinishedBytes(), 0644)
}
