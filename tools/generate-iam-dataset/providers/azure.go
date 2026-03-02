package providers

import (
	"encoding/json"
	"os"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/thand-io/agent/internal/data/iam-dataset/generated/azure"
)

func GenerateAzureFlatBuffers() error {
	// Generate Azure Roles FlatBuffer
	if err := generateAzureRoles(); err != nil {
		return err
	}

	// Generate Azure Permissions FlatBuffer
	if err := generateAzurePermissions(); err != nil {
		return err
	}

	return nil
}

func generateAzureRoles() error {
	// Read and parse JSON
	rolesData, err := os.ReadFile("third_party/iam-dataset/azure/built-in-roles.json")
	if err != nil {
		return err
	}

	type AzureBuiltInRoles struct {
		Roles []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"roles"`
	}

	var azureRoles AzureBuiltInRoles
	if err := json.Unmarshal(rolesData, &azureRoles); err != nil {
		return err
	}

	// Create FlatBuffer
	builder := flatbuffers.NewBuilder(1024)

	// Create roles
	var roles []flatbuffers.UOffsetT
	for _, role := range azureRoles.Roles {
		nameOffset := builder.CreateString(role.Name)
		descOffset := builder.CreateString(role.Description)

		azure.BuiltInRoleStart(builder)
		azure.BuiltInRoleAddName(builder, nameOffset)
		azure.BuiltInRoleAddDescription(builder, descOffset)
		builtInRole := azure.BuiltInRoleEnd(builder)
		roles = append(roles, builtInRole)
	}

	// Create roles vector
	azure.BuiltInRolesListStartRolesVector(builder, len(roles))
	for i := len(roles) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(roles[i])
	}
	rolesVector := builder.EndVector(len(roles))

	// Create root table
	azure.BuiltInRolesListStart(builder)
	azure.BuiltInRolesListAddRoles(builder, rolesVector)
	builtInRolesList := azure.BuiltInRolesListEnd(builder)

	// Finish and write
	builder.Finish(builtInRolesList)
	return os.WriteFile("internal/data/iam-dataset/azure/built-in-roles.fb", builder.FinishedBytes(), 0644)
}

func generateAzurePermissions() error {
	// Read and parse JSON
	permissionsData, err := os.ReadFile("third_party/iam-dataset/azure/provider-operations.json")
	if err != nil {
		return err
	}

	type Operation struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	type AzureResourceProvider struct {
		Namespace     string      `json:"namespace"`
		Operations    []Operation `json:"operations"`
		ResourceTypes []struct {
			Name       string      `json:"name"`
			Operations []Operation `json:"operations"`
		} `json:"resourceTypes"`
	}

	var providers []AzureResourceProvider
	if err := json.Unmarshal(permissionsData, &providers); err != nil {
		return err
	}

	// Create FlatBuffer
	builder := flatbuffers.NewBuilder(1024)

	// Create providers
	var azureProviders []flatbuffers.UOffsetT
	for _, provider := range providers {
		// Flatten top-level provider operations and all resourceType operations
		// into a single list so callers get the full permission set.
		allOps := make([]Operation, 0, len(provider.Operations))
		allOps = append(allOps, provider.Operations...)
		for _, rt := range provider.ResourceTypes {
			allOps = append(allOps, rt.Operations...)
		}

		// Create operations for this provider
		var operations []flatbuffers.UOffsetT
		for _, op := range allOps {
			nameOffset := builder.CreateString(op.Name)
			descOffset := builder.CreateString(op.Description)

			azure.ResourceProviderOperationStart(builder)
			azure.ResourceProviderOperationAddName(builder, nameOffset)
			azure.ResourceProviderOperationAddDescription(builder, descOffset)
			operation := azure.ResourceProviderOperationEnd(builder)
			operations = append(operations, operation)
		}

		// Create operations vector
		azure.ResourceProviderStartOperationsVector(builder, len(operations))
		for i := len(operations) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(operations[i])
		}
		operationsVector := builder.EndVector(len(operations))

		// Create provider
		namespaceOffset := builder.CreateString(provider.Namespace)
		azure.ResourceProviderStart(builder)
		azure.ResourceProviderAddNamespace(builder, namespaceOffset)
		azure.ResourceProviderAddOperations(builder, operationsVector)
		resourceProvider := azure.ResourceProviderEnd(builder)
		azureProviders = append(azureProviders, resourceProvider)
	}

	// Create providers vector
	azure.ResourceProvidersListStartProvidersVector(builder, len(azureProviders))
	for i := len(azureProviders) - 1; i >= 0; i-- {
		builder.PrependUOffsetT(azureProviders[i])
	}
	providersVector := builder.EndVector(len(azureProviders))

	// Create root table
	azure.ResourceProvidersListStart(builder)
	azure.ResourceProvidersListAddProviders(builder, providersVector)
	resourceProvidersList := azure.ResourceProvidersListEnd(builder)

	// Finish and write
	builder.Finish(resourceProvidersList)
	return os.WriteFile("internal/data/iam-dataset/azure/provider-operations.fb", builder.FinishedBytes(), 0644)
}
