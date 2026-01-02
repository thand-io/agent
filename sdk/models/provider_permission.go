package models

import internal "github.com/thand-io/agent/internal/models"

// ProviderPermission represents an individual permission or action that can be granted
// within a provider's access control system. Permissions are the atomic units of access
// that can be combined into roles.
//
// Examples across different providers:
//   - AWS: "s3:GetObject", "ec2:DescribeInstances", "iam:CreateUser"
//   - GCP: "compute.instances.list", "storage.buckets.get", "iam.roles.create"
//   - Azure: "Microsoft.Compute/virtualMachines/read", "Microsoft.Storage/storageAccounts/write"
//   - Okta: "okta.users.read", "okta.apps.manage", "okta.groups.create"
//   - Kubernetes: "pods.get", "deployments.create", "secrets.list"
//
// Permissions are used to:
//   - Define granular access controls in provider configurations
//   - Build custom roles by combining multiple permissions
//   - Validate role definitions against available provider permissions
//   - Display available permissions to users during role creation
//   - Enforce least-privilege access in workflows
//
// The Permission field stores the provider-specific permission object for
// advanced use cases, while the standardized fields (ID, Name, Title, Description)
// provide a consistent interface across all providers.
type ProviderPermission = internal.ProviderPermission
