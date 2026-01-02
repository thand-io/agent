package models

import internal "github.com/thand-io/agent/internal/models"

// ProviderResource represents a resource within a provider that can have permissions
// or policies applied to it. Resources are the targets of access control - the specific
// entities, objects, or services that users need access to.
//
// Examples across different providers:
//   - AWS: S3 buckets, EC2 instances, RDS databases, IAM policies
//     e.g., "arn:aws:s3:::my-bucket", "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0"
//   - GCP: Compute instances, Cloud Storage buckets, BigQuery datasets
//     e.g., "projects/my-project/buckets/my-bucket", "projects/my-project/zones/us-central1-a/instances/my-vm"
//   - Azure: Virtual machines, storage accounts, resource groups
//     e.g., "/subscriptions/{subscription-id}/resourceGroups/{resource-group}"
//   - Kubernetes: Namespaces, pods, deployments, services, secrets
//     e.g., "namespace/production", "deployment/web-app"
//   - Okta: Applications, groups, users as resources for policy assignment
//
// “
// Resources are used to:
//   - Define what permissions can be applied to (resource-based access control)
//   - Scope roles and policies to specific entities
//   - Display available resources during access request workflows
//   - Track and audit which resources users have access to
//   - Support resource-level provisioning and deprovisioning
//
// The Type field categorizes the resource (e.g., "bucket", "instance", "database"),
// while Metadata stores provider-specific attributes like tags, regions, or ownership.
// The Resource field holds the original provider-specific resource object for advanced use cases.
type ProviderResource = internal.ProviderResource
