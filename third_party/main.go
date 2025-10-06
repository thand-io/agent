package third_party

import (
	_ "embed"
)

//go:embed iam-dataset/aws/docs.json
var ec2docs []byte

func GetEc2Docs() []byte {
	return ec2docs
}

//go:embed iam-dataset/aws/managed_policies.json
var ec2roles []byte

func GetEc2Roles() []byte {
	return ec2roles
}

//go:embed iam-dataset/azure/built-in-roles.json
var azureRoles []byte

func GetAzureRoles() []byte {
	return azureRoles
}

//go:embed iam-dataset/azure/provider-operations.json
var azurePermissions []byte

func GetAzurePermissions() []byte {
	return azurePermissions
}

//go:embed iam-dataset/gcp/role_permissions.json
var gcpPermissions []byte

func GetGcpPermissions() []byte {
	return gcpPermissions
}

//go:embed iam-dataset/gcp/predefined_roles.json
var gcpRoles []byte

func GetGcpRoles() []byte {
	return gcpRoles
}
