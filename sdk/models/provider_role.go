package models

import internal "github.com/thand-io/agent/internal/models"

// ProviderRole represents a role within a provider's identity and access management system.
// Roles are collections of permissions that can be assigned to users, groups, or service
// principals. They provide a way to bundle related permissions together for easier management
// and assignment.
//
// Examples across different providers:
//   - AWS IAM: "AdministratorAccess", "ReadOnlyAccess", "PowerUserAccess"
//     e.g., "arn:aws:iam::aws:policy/AdministratorAccess", custom roles
//   - GCP IAM: "roles/viewer", "roles/editor", "roles/owner", custom roles
//     e.g., "roles/compute.admin", "roles/storage.objectViewer"
//   - Azure RBAC: "Owner", "Contributor", "Reader", custom roles
//     e.g., "Virtual Machine Contributor", "Storage Blob Data Reader"
//   - Okta: "Super Administrator", "Application Administrator", "Group Administrator"
//   - Kubernetes: "cluster-admin", "admin", "edit", "view", custom ClusterRoles and Roles
//   - Salesforce: "System Administrator", "Standard User", "Marketing User"
//
// Roles are used to:
//   - Group related permissions into logical access levels
//   - Simplify access management by assigning pre-defined permission sets
//   - Support just-in-time access provisioning in workflows
//   - Enable role-based access control (RBAC) policies
//   - Display available roles during access request processes
//   - Map organizational roles to provider-specific roles
//
// The Role field stores the provider-specific role object (which may include embedded
// permissions, policies, or other metadata), while the standardized fields (ID, Name,
// Title, Description) provide a consistent interface across all providers.
type ProviderRole = internal.ProviderRole
