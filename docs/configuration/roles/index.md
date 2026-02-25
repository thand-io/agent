---
layout: default
title: Roles
parent: Configuration
nav_order: 9
description: Comprehensive documentation for Thand Agent roles with intelligent inheritance and permission merging
has_children: true
---

# Roles

Roles are the core authorization mechanism in Thand Agent that define what permissions users can request and under what conditions. They act as templates that specify the scope of access, workflows for approval, and inheritance relationships that enable flexible permission management.

## Quick Start

A basic role definition:

```yaml
version: "1.0"
roles:
  aws-developer:
    name: AWS Developer Access
    description: Developer access to AWS resources
    enabled: true
    
    permissions:
      allow:
        - operations:
            - ec2:DescribeInstances
            - s3:GetObject
            - s3:ListBuckets
          targets:
            - "arn:aws:s3:::dev-bucket/*"
    
    scopes:
      allow:
        groups:
          - developers
```

## Core Concepts

### What is a Role?

A Thand role is a configuration template that defines:
- **Permissions**: What actions can be performed and which resources can be targeted (via statements with operations, targets, and optional conditions)
- **Inheritance**: Which other roles this role builds upon
- **Providers**: Which provider instances can be used with this role
- **Scopes**: Who can request this role (with allow/deny rules for users, groups, and domains)
- **Workflows**: How access requests are processed and approved

### Role vs Provider Roles

It's important to distinguish between:
- **Thand Roles**: Defined in your agent configuration (documented here)
- **Provider Roles**: Native roles in external systems (AWS IAM roles, Azure roles, etc.)

Thand roles can **inherit** from other roles and provider roles to leverage existing cloud IAM configurations.

### Composite Roles

When a role inherits from other locally-defined Thand roles, it becomes a **composite role** at runtime. The `composite` field on a role is **system-managed** — you do not set it yourself. During inheritance resolution, Thand automatically:

1. Resolves the full inheritance chain (with cycle detection and depth limits)
2. Filters inherited permissions by the role's configured providers
3. Validates identity scopes at each level of the chain
4. Merges all permissions using intelligent conflict resolution
5. Marks the resulting role as `composite: true`

{: .note}
You will see `composite: true` on roles returned by the API or in debug output. This indicates that the role's permissions were assembled from multiple source roles. You should never set this field manually in your configuration files.

### Intelligent Permission Merging

Thand Agent features intelligent permission merging that:
- **Consolidates condensed actions**: `k8s:pods:get,list` + `k8s:pods:create,update` = `k8s:pods:create,get,list,update`
- **Preserves GCP-style permissions**: Permissions with dots in the action (e.g., `gcp:compute.instances.get`) are treated atomically and not condensed
- **Resolves Allow/Deny conflicts**: Parent permissions take precedence - Parent Allow overrides Child Deny, Parent Deny overrides Child Allow
- **Filters by provider**: Inherited permissions with provider prefixes are automatically filtered to match the role's configured providers, and **matching prefixes are stripped** from the output
- **Handles complex inheritance**: Multi-level role inheritance with proper conflict resolution
- **Supports provider-specific naming**: AWS ARNs, GCP service accounts, Azure resource IDs with complex naming patterns

---

## Table of Contents

1. [Role Structure](#role-structure)
2. [Permissions & Statements](#permissions--statements)
3. [Inheritance](#inheritance)
4. [Scopes & Access Control](#scopes--access-control)
5. [Provider Integration](#provider-integration)
6. [Workflow Integration](#workflow-integration)
7. [Configuration Management](#configuration-management)
8. [Best Practices](#best-practices)
9. [Troubleshooting](#troubleshooting)

---

## Role Structure

### Basic Configuration

```yaml
version: "1.0"
roles:
  role-name:
    name: Human Readable Name
    description: Description of what this role provides
    enabled: true                    # Optional, defaults to true
    
    # Core role definition
    permissions:     # What actions are allowed/denied (using statements)
      allow: []
      deny: []
    inherits: []     # What other roles to inherit from
    providers: []    # Which providers can be used
    
    # Access control
    scopes:          # Who can request this role (allow/deny)
      allow:
        users: []
        groups: []
        domains: []
      deny:
        users: []
        groups: []
        domains: []
    
    # Process control  
    workflows: []         # How requests are processed
    authenticators: []    # Which auth providers are valid
```

### Complete Role Example

```yaml
version: "1.0"
roles:
  aws-developer:
    name: AWS Developer Access
    description: Developer access to AWS resources with approval workflow
    enabled: true
    
    # Inheritance - build upon existing roles
    inherits:
      - aws-basic-user                    # Local role
      - aws-dev:arn:aws:iam::aws:policy/AmazonEC2ReadOnlyAccess  # AWS managed policy
    
    # Explicit permissions using statements
    permissions:
      allow:
        - operations:
            - ec2:DescribeInstances,StartInstances,StopInstances  # Condensed actions
            - s3:GetObject,PutObject          # Multiple S3 actions
            - logs:DescribeLogGroups,DescribeLogStreams
          targets:
            - "arn:aws:ec2:us-east-1:123456789012:instance/*"
            - "arn:aws:s3:::dev-bucket/*"
      deny:
        - operations:
            - ec2:TerminateInstances          # Explicit denial
          targets:
            - "arn:aws:ec2:us-east-1:123456789012:instance/*"
    
    # Provider restrictions
    providers:
      - aws-dev
      - aws-staging
    
    # Who can request this role (allow/deny with users, groups, and domains)
    scopes:
      allow:
        users:
          - developer@example.com
        groups:
          - developers
          - engineering
        domains:
          - example.com
    
    # Approval process
    workflows:
      - manager-approval
      - security-review
    
    # Valid authentication methods
    authenticators:
      - google-oauth
      - saml-sso
```

### Configuration Fields Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Human-readable role name |
| `description` | string | Yes | Description of role purpose |
| `enabled` | boolean | No | Whether role is active (default: true) |
| `permissions` | object | No | Allow/deny permission rules using statements |
| `inherits` | array | No | List of roles to inherit from |
| `providers` | array | No | List of provider instances this role can use |
| `scopes` | object | No | Allow/deny access rules for users, groups, and domains |
| `workflows` | array | No | Approval workflows to execute |
| `authenticators` | array | No | Valid authentication providers |
| `composite` | boolean | No | **System-managed.** Set to `true` when the role is assembled from inherited local roles at runtime. Do not set manually. |

---

## Permissions & Statements

Permissions define **what actions** can be performed and **which resources** can be targeted when a role is activated. Permissions are expressed as **statements** — objects that combine operations, targets, and optional conditions into a single unit.

### Statement Structure

Each permission statement is an object with the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `operations` | array | Yes | Actions that can be performed (provider-specific) |
| `targets` | array | No | Resources the operations apply to (provider-specific) |
| `conditions` | object | No | Provider-specific conditions that must be met |

```yaml
permissions:
  allow:
    - operations:          # What actions to allow
        - action1
        - action2
      targets:             # Which resources these actions apply to
        - resource1
        - resource2
      conditions:          # Optional provider-specific conditions
        ConditionOperator:
          ConditionKey: ConditionValue
  deny:
    - operations:
        - action3
      targets:
        - resource3
```

{: .note}
**Backwards compatibility:** Permissions also accept simple strings (e.g., `- "ec2:DescribeInstances"`) for the `allow` and `deny` lists. These are automatically converted to statements with a single operation and no targets or conditions. See the [migration guide](migration/) for details.

### Condensed Actions

Thand Agent intelligently handles condensed actions where multiple related actions are specified in a single operation string:

```yaml
permissions:
  allow:
    - operations:
        # Condensed format - multiple actions in one string
        - "k8s:pods:get,list,watch,create,update,delete"
        - "s3:GetObject,PutObject,ListBucket"
        - "ec2:DescribeInstances,StartInstances,StopInstances"
      targets:
        - "arn:aws:ec2:us-east-1:*:instance/*"
        - "arn:aws:s3:::dev-bucket/*"
    
    - operations:
        # Individual format - also supported
        - "logs:DescribeLogGroups"
        - "logs:DescribeLogStreams"
```

#### Intelligent Merging

When roles are inherited or merged, condensed actions are intelligently combined:

```yaml
# Base role
base-role:
  permissions:
    allow:
      - operations:
          - "k8s:pods:get,list,watch"
          - "s3:GetObject,ListBucket"

# Child role
child-role:
  inherits: [base-role]
  permissions:
    allow:
      - operations:
          - "k8s:pods:create,update,delete"  # Will merge with base
          - "s3:PutObject,DeleteObject"      # Will merge with base

# Resulting merged permissions:
# - "k8s:pods:create,delete,get,list,update,watch"  (merged and sorted)
# - "s3:DeleteObject,GetObject,ListBucket,PutObject" (merged and sorted)
```

### Cloud Provider Permission Patterns

#### AWS Permissions
```yaml
permissions:
  allow:
    - operations:
        - "ec2:*"                          # All EC2 actions
        - "s3:GetObject,PutObject"         # Specific S3 actions
        - "iam:PassRole"                   # IAM role assumption
        - "logs:DescribeLogGroups,DescribeLogStreams,CreateLogStream"
      targets:
        - "arn:aws:ec2:us-east-1:123456789012:instance/*"
        - "arn:aws:s3:::app-bucket/*"
  deny:
    - operations:
        - "ec2:TerminateInstances"         # Explicit denial
        - "s3:DeleteBucket"                # Protect against deletion
```

#### Azure Permissions
```yaml
permissions:
  allow:
    - operations:
        - "Microsoft.Compute/virtualMachines/read,start,restart"
        - "Microsoft.Storage/storageAccounts/read"
        - "Microsoft.Authorization/roleAssignments/read"
  deny:
    - operations:
        - "Microsoft.Compute/virtualMachines/delete"
        - "Microsoft.Storage/storageAccounts/delete"
```

#### GCP Permissions

{: .important}
**GCP permissions are atomic**: Unlike AWS or Kubernetes permissions, GCP permissions contain dots in the action portion (e.g., `compute.instances.get`). These are detected automatically and treated as atomic - they are never condensed with other permissions.

```yaml
permissions:
  allow:
    # GCP permissions are NOT condensed - each is kept separate
    - operations:
        - "gcp-prod:compute.instances.get"
        - "gcp-prod:compute.instances.list"
        - "gcp-prod:compute.instances.start"
        - "gcp-prod:storage.buckets.list"
        - "gcp-prod:iam.serviceAccounts.get"
  deny:
    - operations:
        - "gcp-prod:compute.instances.delete"
        - "gcp-prod:storage.buckets.delete"
```

{: .note}
The system automatically detects GCP-style permissions by checking if the last segment (after the final colon) contains a dot. If it does, the permission is treated as atomic.

#### Kubernetes Permissions
```yaml
permissions:
  allow:
    - operations:
        - "k8s:pods:get,list,watch,create,update,patch"
        - "k8s:services:get,list,create,update,delete"
        - "k8s:configmaps:get,list,create,update,delete"
        - "k8s:secrets:get,list"  # Read-only for secrets
  deny:
    - operations:
        - "k8s:secrets:create,update,delete"  # No secret modifications
        - "k8s:pods:delete"                   # Cannot delete pods
```

### Targets

Targets define **which resources** the operations in a statement apply to. They use provider-specific resource identifiers such as AWS ARNs, Azure resource IDs, GCP resource paths, or Kubernetes namespace selectors.

#### AWS Targets (ARNs)
```yaml
permissions:
  allow:
    - operations:
        - "ec2:DescribeInstances,StartInstances,StopInstances"
      targets:
        - "arn:aws:ec2:*:*:instance/*"           # All EC2 instances
        - "arn:aws:ec2:us-east-1:123456789012:instance/*"  # Specific region/account
    - operations:
        - "s3:GetObject,PutObject"
      targets:
        - "arn:aws:s3:::dev-bucket/*"            # Specific bucket contents
        - "arn:aws:s3:::app-*/*"                 # Pattern-based bucket access
  deny:
    - operations:
        - "s3:*"
      targets:
        - "arn:aws:s3:::prod-bucket/*"           # Sensitive production data
        - "arn:aws:s3:::audit-*/*"               # Audit data
```

#### Azure Targets
```yaml
permissions:
  allow:
    - operations:
        - "Microsoft.Compute/virtualMachines/read,start,restart"
      targets:
        - "/subscriptions/*/resourceGroups/dev-*/providers/Microsoft.Compute/virtualMachines/*"
        - "/subscriptions/12345/resourceGroups/app-*/providers/Microsoft.Compute/*"
    - operations:
        - "Microsoft.Storage/storageAccounts/read"
      targets:
        - "/subscriptions/*/resourceGroups/*/providers/Microsoft.Storage/storageAccounts/dev*"
```

#### GCP Targets
```yaml
permissions:
  allow:
    - operations:
        - "gcp-prod:compute.instances.get"
        - "gcp-prod:compute.instances.list"
      targets:
        - "projects/dev-project/zones/*/instances/*"
        - "projects/*/zones/us-central1-*/instances/app-*"
    - operations:
        - "gcp-prod:storage.buckets.list"
      targets:
        - "projects/*/global/buckets/dev-*"
        - "projects/my-project/global/buckets/staging-*"
```

#### Kubernetes Targets
```yaml
permissions:
  allow:
    - operations:
        - "k8s:pods:get,list,watch,create,update,patch"
      targets:
        - "namespace:development"
        - "namespace:staging"
        - "namespace:feature-*"
  deny:
    - operations:
        - "k8s:*:*"
      targets:
        - "namespace:production"           # No production namespace
        - "namespace:kube-system"          # No system namespace
```

### Conditions

Conditions add provider-specific constraints to a statement. They are **preserved as-is** through permission merging and inheritance resolution but are **not evaluated by Thand** — they are passed through to the target provider for enforcement.

{: .important}
Currently, only **AWS** maps conditions to IAM policy `Condition` blocks. Other providers (Azure, GCP, Kubernetes) do not use conditions. If you add conditions to non-AWS statements, they will be preserved in the data model but will have no effect at the provider level.

#### AWS Condition Examples

```yaml
permissions:
  allow:
    # Restrict by source IP
    - operations:
        - "s3:GetObject"
        - "s3:PutObject"
      targets:
        - "arn:aws:s3:::sensitive-bucket/*"
      conditions:
        IpAddress:
          "aws:SourceIp": "10.0.0.0/8"
    
    # Require encryption
    - operations:
        - "s3:PutObject"
      targets:
        - "arn:aws:s3:::encrypted-bucket/*"
      conditions:
        StringEquals:
          "s3:x-amz-server-side-encryption": "AES256"
    
    # Restrict by tag
    - operations:
        - "ec2:StartInstances"
        - "ec2:StopInstances"
      targets:
        - "arn:aws:ec2:*:*:instance/*"
      conditions:
        StringEquals:
          "ec2:ResourceTag/Environment": "development"
    
    # MFA required
    - operations:
        - "iam:*"
      conditions:
        Bool:
          "aws:MultiFactorAuthPresent": "true"
```

{: .note}
Conditions follow the same structure as AWS IAM policy conditions. The outer key is the condition operator (e.g., `StringEquals`, `IpAddress`, `Bool`), and the inner key-value pair is the condition key and expected value. Refer to the [AWS IAM Condition documentation](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements_condition.html) for the full list of supported operators and keys.

### Allow/Deny Conflict Resolution

When the same action appears in both `allow` and `deny` lists, the system resolves conflicts using clear precedence rules.

#### Single Role Conflicts
```yaml
# Within a single role, deny takes precedence
role:
  permissions:
    allow:
      - operations:
          - "k8s:pods:get,list,create,update,delete"
    deny:
      - operations:
          - "k8s:pods:delete"  # Removes 'delete' from the allow list

# Resolves to:
# allow: ["k8s:pods:create,get,list,update"]
# deny: []  (deny removed since conflict was resolved)
```

#### Inheritance Conflicts (Parent Wins)

In inheritance chains, the **parent role (the one doing the inheriting) takes precedence** over child roles (the inherited ones):

- **Parent Allow overrides Child Deny**: If parent allows an action that child denied, the action is allowed
- **Parent Deny overrides Child Allow**: If parent denies an action that child allowed, the action is denied

```yaml
# Child role (the inherited role)
child-role:
  permissions:
    allow:
      - operations: ["ec2:StartInstances", "ec2:DescribeInstances"]
    deny:
      - operations: ["ec2:TerminateInstances"]

# Parent role (the role doing the inheriting)
parent-role:
  inherits: [child-role]
  permissions:
    allow:
      - operations: ["ec2:TerminateInstances"]  # Overrides child's deny
    deny:
      - operations: ["ec2:StartInstances"]       # Overrides child's allow

# Final resolved permissions:
# allow: ["ec2:DescribeInstances", "ec2:TerminateInstances"]  
#        (child's allow minus parent's deny, plus parent's allow)
# deny: ["ec2:StartInstances"]  
#       (parent's deny, child's deny removed by parent's allow)
```

{: .important}
This allows you to build restrictive roles that inherit permissive ones, or permissive roles that override restrictions from inherited roles.

### Wildcard Permissions

You can use wildcard patterns (glob syntax using `*`) in role permission definitions for any provider. However, the way wildcards are handled depends on whether the provider's API natively supports wildcard permissions:

| Provider    | Supports Wildcards | Behavior                                                    |
|-------------|:------------------:|-------------------------------------------------------------|
| AWS         | ✅                 | Wildcards kept in condensed form after validation           |
| Azure       | ✅                 | Wildcards kept in condensed form after validation           |
| Kubernetes  | ✅                 | Wildcards kept in condensed form after validation           |
| GCP         | ❌                 | Wildcards expanded to individual permissions automatically  |
| Okta        | ❌                 | Wildcards expanded to individual permissions automatically  |

- **Providers that support wildcards** (AWS, Azure, Kubernetes): Wildcard patterns are validated by expanding them against the provider's known permission set, then re-condensed back to their original wildcard form in the final output. Invalid wildcards that match no known permissions produce a validation error.
- **Providers that do not support wildcards** (GCP, Okta): Wildcard patterns are expanded to the full list of matching individual permissions during validation. The expanded permissions are kept as-is in the final output because the provider's API requires explicit permission names.

```yaml
permissions:
  allow:
    - operations:
        # AWS wildcards (kept as wildcards)
        - "ec2:*"                    # All EC2 actions
        - "s3:*Object*"              # All object-related S3 actions

        # Kubernetes wildcards (kept as wildcards)
        - "k8s:*:*"                  # All Kubernetes actions
        - "k8s:pods:*"               # All pod actions

        # Azure wildcards (kept as wildcards)
        - "Microsoft.Compute/*"      # All compute actions

        # GCP wildcards (expanded to individual permissions)
        - "compute.instances.*"      # Expanded to compute.instances.get, etc.
```

#### Wildcard Subsumption

For providers that support wildcards, more specific permissions under a wildcard are automatically removed (subsumed):

```yaml
permissions:
  allow:
    - operations:
        - "ec2:*"                    # Wildcard
        - "ec2:DescribeInstances"    # Will be removed (subsumed by ec2:*)
        - "ec2:StartInstances"       # Will be removed (subsumed by ec2:*)
        - "s3:GetObject"             # Kept (not under ec2:*)

# After condensing, becomes:
# allow: ["ec2:*", "s3:GetObject"]
```

{: .note}
A wildcard does NOT subsume itself. For example, `ec2:*` is kept even when other `ec2:*` wildcards exist. Only more specific permissions (like `ec2:DescribeInstances`) are subsumed. Subsumption only applies to providers where `supports_wildcards` is `true`.

---

## Inheritance

Role inheritance is a powerful feature that allows roles to build upon each other, promoting reusability and consistent security patterns. Thand Agent features intelligent inheritance that properly handles complex permission merging, provider-specific role names, and conflict resolution.

### How Inheritance Works

When a role inherits from other roles:

1. **Provider Filtering**: Inherited permissions with provider prefixes are filtered to only include those matching the parent role's `providers` list. **Matching prefixes are stripped** from the output.
2. **Permission Expansion**: Condensed actions (e.g., `k8s:pods:get,list`) are expanded to individual permissions for merging
3. **GCP Permission Detection**: Permissions with dots in the action (e.g., `compute.instances.get`) are detected and kept atomic
4. **Intelligent Merging**: All `allow` and `deny` lists are combined with proper conflict resolution
5. **Conflict Resolution**: Parent Allow overrides Child Deny, Parent Deny overrides Child Allow
6. **Action Condensing**: Final condensable permissions are re-condensed for clean output (GCP-style permissions remain atomic)
7. **Scope Validation**: Inherited roles must be applicable to the requesting identity

### Inheritance Types

#### 1. Local Role Inheritance

Inherit from other Thand roles:

```yaml
roles:
  base-user:
    name: Base User
    permissions:
      allow:
        - operations:
            - "ec2:DescribeInstances,DescribeImages"
            - "s3:ListBuckets,GetBucketLocation"
  
  power-user:
    name: Power User
    inherits:
      - base-user  # Inherits base-user permissions
    permissions:
      allow:
        - operations:
            - "ec2:StartInstances,StopInstances,RebootInstances"  # Additional permissions
            - "s3:GetObject,PutObject"

# Resulting power-user permissions (intelligently merged):
# allow:
#   - "ec2:DescribeImages,DescribeInstances,RebootInstances,StartInstances,StopInstances"
#   - "s3:GetBucketLocation,GetObject,ListBuckets,PutObject"
```

#### 2. Provider Role Inheritance

Inherit from cloud provider managed roles using provider-specific syntax:

```yaml
roles:
  aws-admin:
    name: AWS Administrator
    inherits:
      # Direct AWS managed policy
      - "arn:aws:iam::aws:policy/AdministratorAccess"
      
      # Provider-scoped inheritance
      - "aws-prod:arn:aws:iam::aws:policy/ReadOnlyAccess"
    
  gcp-viewer:
    name: GCP Viewer
    inherits:
      # GCP predefined role
      - "roles/viewer"
      
      # Provider-scoped GCP role
      - "gcp-prod:roles/compute.viewer"
    permissions:
      allow:
        - operations:
            - "compute.instances.start,stop"  # Additional specific permissions

  azure-contributor:
    name: Azure Contributor
    inherits:
      # Azure built-in role
      - "Contributor"
      
      # Provider-scoped Azure role
      - "azure-prod:/subscriptions/12345/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"
```

#### 3. Complex Provider-Specific Inheritance

Handle complex role names with multiple colons (AWS ARNs, service accounts):

```yaml
roles:
  kubernetes-admin:
    name: Kubernetes Administrator
    inherits:
      # AWS ARN with multiple colons - uses first colon as delimiter
      - "aws-prod:arn:aws:iam::123456789012:role/KubernetesAdmin"
      
      # GCP service account with @ symbol
      - "gcp-prod:k8s-admin@my-project.iam.gserviceaccount.com"
      
      # Azure resource ID with multiple path segments
      - "azure-prod:/subscriptions/12345/resourceGroups/k8s/providers/Microsoft.ManagedIdentity/userAssignedIdentities/k8s-admin"

  multi-cloud-viewer:
    name: Multi-Cloud Viewer
    inherits:
      - local-base-viewer           # Local role
      - "aws-prod:arn:aws:iam::aws:policy/ReadOnlyAccess"
      - "gcp-prod:roles/viewer"
      - "azure-prod:Reader"
    permissions:
      allow:
        - operations:
            - "custom:audit,monitor"     # Additional custom permissions
```

#### 4. Mixed Inheritance with Intelligent Merging

Combine local and provider roles with complex permission merging:

```yaml
roles:
  base-k8s:
    name: Base Kubernetes
    permissions:
      allow:
        - operations:
            - "k8s:pods:get,list,watch"
            - "k8s:services:get,list"

  k8s-developer:
    name: Kubernetes Developer
    inherits: [base-k8s]
    permissions:
      allow:
        - operations:
            - "k8s:pods:create,update,patch"      # Merges with inherited get,list,watch
            - "k8s:services:create,update,delete" # Merges with inherited get,list
            - "k8s:configmaps:get,list,create,update,delete"
      deny:
        - operations:
            - "k8s:pods:delete"                   # Prevents pod deletion

  k8s-admin:
    name: Kubernetes Administrator  
    inherits: [k8s-developer]
    permissions:
      allow:
        - operations:
            - "k8s:pods:delete"                   # Overrides parent deny
            - "k8s:secrets:get,list,create"
            - "k8s:*:*"                          # Admin access to all
      deny:
        - operations:
            - "k8s:secrets:delete"                # Even admins can't delete secrets

# Final k8s-admin permissions after intelligent merging:
# allow:
#   - "k8s:*:*"  (covers everything including specific permissions)
# deny:  
#   - "k8s:secrets:delete"  (explicit restriction even for admin)
```

### Inheritance Resolution Process

The inheritance system resolves permissions in this order:

1. **Parse Inheritance**: Extract provider prefixes and role names (first colon is the delimiter)
2. **Provider Filtering**: Filter inherited items by the parent role's `providers` list
3. **Scope Validation**: Ensure each inherited role is applicable to the requesting identity
4. **Recursive Resolution**: Resolve inheritance chains (A inherits B inherits C)
5. **Permission Expansion**: Expand condensable actions to individual permissions (GCP-style permissions with dots are kept atomic)
6. **Intelligent Merging**: Combine permissions from all inheritance levels
7. **Conflict Resolution**: Apply Parent-over-Child conflict resolution (Parent Allow overrides Child Deny, Parent Deny overrides Child Allow)
8. **Action Condensing**: Condense related actions back for clean output (GCP-style permissions remain atomic)
9. **Wildcard Subsumption**: Remove permissions subsumed by wildcards (e.g., `ec2:*` subsumes `ec2:DescribeInstances`)

### Provider-Specific Inheritance Syntax

When inheriting from provider roles, use the provider name as a prefix:

```yaml
# Format: provider-name:role-identifier
inherits:
  - "aws-prod:arn:aws:iam::123456789012:role/MyRole"      # AWS role
  - "gcp-prod:roles/storage.admin"                         # GCP role  
  - "azure-prod:Storage Blob Data Contributor"             # Azure role
  - "k8s-prod:cluster-admin"                              # Kubernetes role
```

**Parser Behavior:**
- Uses the **first colon** as the delimiter between provider and role
- Everything before first colon = provider name
- Everything after first colon = role identifier
- Handles complex identifiers like AWS ARNs with multiple colons correctly
- Provider name is validated against configured providers

### Provider Filtering and Prefix Stripping

When permissions have provider prefixes, they are automatically filtered based on the role's `providers` list. **Matching prefixes are stripped** from the output:

```yaml
# Base role with provider-prefixed permissions
base-cloud-role:
  permissions:
    allow:
      - operations:
          - "aws-prod:ec2:DescribeInstances"
          - "gcp-prod:compute.instances.get"
          - "azure-prod:Microsoft.Compute/virtualMachines/read"

# Role that only uses AWS providers
aws-only-role:
  inherits: [base-cloud-role]
  providers: [aws-prod, aws-dev]  # Only AWS providers
  # Resulting permissions (prefix stripped):
  #   - "ec2:DescribeInstances"  (was "aws-prod:ec2:DescribeInstances")
  # GCP and Azure items are filtered out completely

# Role that uses multiple providers
multi-cloud-role:
  inherits: [base-cloud-role]
  providers: [aws-prod, gcp-prod]  # AWS and GCP
  # Resulting permissions (prefixes stripped):
  #   - "ec2:DescribeInstances"      (was "aws-prod:ec2:DescribeInstances")
  #   - "compute.instances.get"      (was "gcp-prod:compute.instances.get")
  # Azure items are filtered out completely
```

**How prefix matching works:**
- **Exact provider name match**: `aws-prod:permission` matches providers list containing `aws-prod`
- **Engine type match**: `aws:*` matches any provider with engine type `aws` (e.g., `aws-prod`, `aws-dev`)
- When a prefix matches, it is **removed** from the output
- Items without a provider prefix are always included as-is

{: .note}
Provider prefixes are stripped when they match, leaving clean permission strings without provider annotations. This allows the same base role to be used across different provider configurations.

### Inheritance Validation

The system validates inheritance chains:

#### Cyclic Inheritance Detection
```yaml
# This will be detected and rejected
role-a:
  inherits: [role-b]
role-b:
  inherits: [role-c]  
role-c:
  inherits: [role-a]  # Cycle detected!
```

#### Missing Role Detection
```yaml
# This will fail if 'nonexistent-role' doesn't exist
my-role:
  inherits: [nonexistent-role]  # Error: role not found
```

#### Scope Compatibility
```yaml
# Inherited role must be applicable to the requesting user
admin-role:
  scopes:
    allow:
      groups: [admins]
    
developer-role:
  inherits: [admin-role]  # Will fail if user is not in 'admins' group
  scopes:
    allow:
      groups: [developers]
```

### Inheritance Best Practices

#### 1. Build Role Hierarchies
```yaml
# Base roles with minimal permissions
readonly-base:
  permissions:
    allow:
      - operations: ["*:Describe*", "*:List*", "*:Get*"]

# Specialized roles building on base
ec2-readonly:
  inherits: [readonly-base]
  permissions:
    allow:
      - operations: ["ec2:*"]
        targets: ["arn:aws:ec2:*:*:*"]

# Team-specific roles
dev-team-ec2:
  inherits: [ec2-readonly]
  scopes:
    allow:
      groups: [developers]
```

#### 2. Use Provider Managed Roles
```yaml
# Leverage existing cloud roles
aws-power-user:
  inherits:
    - "aws-prod:arn:aws:iam::aws:policy/PowerUserAccess"
  # Add company-specific restrictions
  permissions:
    deny:
      - operations:
          - "s3:*"
        targets:
          - "arn:aws:s3:::sensitive-*"
```

#### 3. Layer Security Controls
```yaml
restrictive-admin:
  name: Restrictive Admin
  inherits:
    - "aws-prod:arn:aws:iam::aws:policy/AdministratorAccess"
  # Add explicit denials even for admins
  permissions:
    deny:
      - operations:
          - "iam:DeleteUser"
          - "iam:DeleteRole"
          - "s3:DeleteBucket"
```

---

## Scopes & Access Control

Scopes control **who** can request a role. Scopes use an **allow/deny** model with three identity types: users, groups, and domains. This enables fine-grained role-based access control at the identity level.

### Scope Structure

```yaml
scopes:
  allow:         # Identities permitted to request this role
    users:
      - user1@example.com
    groups:
      - group1
    domains:
      - example.com
  deny:          # Identities explicitly blocked from this role
    users:
      - restricted-user@example.com
    groups:
      - contractors
    domains:
      - external.com
```

### Allow Scopes

Define which identities are permitted to request the role:

#### User Scopes

```yaml
scopes:
  allow:
    users:
      - alice@example.com           # Email address
      - bob.smith@company.com       # Full name email
      - service-account@project.iam.gserviceaccount.com  # Service account
      - "123456789"                 # User ID
      - "alice.smith"               # Username
```

#### Group Scopes

```yaml
scopes:
  allow:
    groups:
      - developers                  # Simple group name
      - engineering                 # Department
      - on-call                     # Role-based group
      - team-alpha                  # Team designation
      - contractors                 # Employment type
```

#### Domain Scopes

Domain scopes allow or deny access based on the domain portion of a user's email address:

```yaml
scopes:
  allow:
    domains:
      - example.com               # All users with @example.com emails
      - subsidiary.example.com    # Specific subdomain
```

### Deny Scopes

Deny scopes explicitly block identities from requesting a role, even if they match an allow rule. **Deny always takes precedence over allow.**

```yaml
scopes:
  allow:
    groups:
      - engineering               # Allow all engineers
    domains:
      - example.com               # Allow all company users
  deny:
    groups:
      - interns                   # Block interns even if in engineering
    users:
      - suspended-user@example.com  # Block specific user
```

{: .important}
**Deny takes precedence**: If an identity matches both an allow and a deny rule, the deny rule wins. This applies across all identity types — a user denied by any deny rule (user, group, or domain) will be blocked regardless of allow rules.

### Identity Provider Integration

Different identity providers may have different group formats:

```yaml
scopes:
  allow:
    groups:
      # Active Directory groups
      - "DOMAIN\\Domain Users"
      - "CORP\\Engineering"
      
      # OIDC/OAuth groups  
      - "developers"
      - "admin-users"
      
      # SAML groups
      - "cn=developers,ou=groups,dc=company,dc=com"
      
      # GitHub teams
      - "my-org/developers"
      - "my-org/admin-team"
```

### Mixed Scopes

Combine users, groups, and domains for flexible access control:

```yaml
scopes:
  allow:
    users:
      - emergency-admin@example.com     # Emergency access user
      - service-bot@example.com         # Automated service
    groups:
      - on-call                         # On-call team members
      - security-team                   # Security personnel
      - senior-engineers                # Senior staff
    domains:
      - example.com                     # All company users
  deny:
    groups:
      - contractors                     # Exclude contractors
    domains:
      - external-vendor.com             # Block external vendor domain
```

### Public Roles

Omit `scopes` to allow any authenticated user to request the role:

```yaml
roles:
  basic-viewer:
    name: Basic Viewer Access
    description: Read-only access available to all authenticated users
    # No 'scopes' field - available to all users
    permissions:
      allow:
        - operations:
            - "*:Describe*"
            - "*:List*"
            - "*:Get*"
```

### Scope Inheritance

When roles inherit from other roles, scope checking is applied to each role in the inheritance chain:

```yaml
roles:
  admin-base:
    name: Admin Base
    scopes:
      allow:
        groups: [admins]
    permissions:
      allow:
        - operations: ["*:*"]
  
  senior-admin:
    name: Senior Admin
    inherits: [admin-base]          # User must be in 'admins' group
    scopes:
      allow:
        groups: [senior-staff]      # AND in 'senior-staff' group
    permissions:
      allow:
        - operations: ["sensitive:*"]

# For senior-admin role to work, user must be in BOTH groups:
# - 'admins' (required by admin-base)
# - 'senior-staff' (required by senior-admin)
```

### Scope Validation Examples

#### Successful Access
```yaml
# Role definition
developer-role:
  scopes:
    allow:
      groups: [developers, engineering]

# User identity
user:
  email: alice@example.com
  groups: [developers, qa-team]

# Result: Access granted (user in 'developers' group)
```

#### Failed Access
```yaml
# Role definition  
admin-role:
  scopes:
    allow:
      users: [admin@example.com]
      groups: [administrators]

# User identity
user:
  email: alice@example.com
  groups: [developers]

# Result: Access denied (user not in allowed users or groups)
```

#### Denied by Deny Rule
```yaml
# Role definition
team-role:
  scopes:
    allow:
      groups: [engineering]
    deny:
      groups: [interns]

# User identity
user:
  email: intern@example.com
  groups: [engineering, interns]

# Result: Access denied (user matches deny rule 'interns', which takes precedence)
```

---

## Provider Integration

Roles specify which provider instances can be used for role elevation. This enables multi-cloud and multi-environment access control.

### Single Provider

Restrict a role to a specific provider instance:

```yaml
roles:
  aws-dev-access:
    name: AWS Development Access
    providers:
      - aws-dev  # Only the aws-dev provider instance
    permissions:
      allow:
        - operations:
            - "ec2:*"
            - "s3:*"
```

### Multi-Provider

Allow a role to work across multiple provider instances:

```yaml
roles:
  multi-cloud-viewer:
    name: Multi-Cloud Viewer Access
    providers:
      - aws-prod
      - azure-prod  
      - gcp-prod
    permissions:
      allow:
        - operations:
            - "*:Describe*"
            - "*:List*"
            - "*:Get*"
```

### Environment-Specific Providers

Organize providers by environment:

```yaml
roles:
  development-admin:
    name: Development Administrator
    providers:
      - aws-dev
      - azure-dev
      - gcp-dev
      - k8s-dev
    permissions:
      allow:
        - operations: ["*:*"]
  
  production-readonly:
    name: Production Read-Only
    providers:
      - aws-prod
      - azure-prod
      - gcp-prod
    permissions:
      allow:
        - operations:
            - "*:Describe*"
            - "*:List*"
            - "*:Get*"
```

### Provider Inheritance Compatibility

When inheriting from provider roles, ensure the provider supports the inherited role:

```yaml
roles:
  aws-ec2-admin:
    name: EC2 Administrator
    providers:
      - aws-prod
    inherits:
      # This AWS managed policy must exist in the aws-prod provider
      - "aws-prod:arn:aws:iam::aws:policy/AmazonEC2FullAccess"
    
  gcp-compute-admin:
    name: GCP Compute Administrator  
    providers:
      - gcp-prod
    inherits:
      # This GCP role must be available in the gcp-prod provider
      - "gcp-prod:roles/compute.admin"
```

### Provider Validation

The system validates provider compatibility:

```yaml
# This will fail if aws-staging doesn't have the specified role
problematic-role:
  providers:
    - aws-staging
  inherits:
    - "aws-prod:arn:aws:iam::123456789012:role/CustomRole"  # Different provider!
```

**Correct approach:**
```yaml
correct-role:
  providers:
    - aws-staging
  inherits:
    - "aws-staging:arn:aws:iam::123456789012:role/CustomRole"  # Same provider
```

---

## Workflow Integration

Roles integrate with [workflows](../configuration/workflows/) to define approval processes, time limits, and other governance controls.

### Basic Workflow Assignment

```yaml
roles:
  sensitive-admin:
    name: Sensitive Admin Access
    workflows:
      - manager-approval     # Requires manager approval
      - security-review      # Additional security review
    permissions:
      allow:
        - operations: ["*:*"]
```

### Multiple Workflows

Multiple workflows can be applied to a single role for different purposes:

#### Sequential Execution
Workflows are typically executed in sequence, with each workflow having specific responsibilities:

```yaml
roles:
  production-access:
    name: Production Access
    workflows:
      - identity-verification    # Step 1: Verify identity
      - manager-approval         # Step 2: Manager approval  
      - security-approval        # Step 3: Security team approval
      - time-limit               # Step 4: Apply time limits
    permissions:
      allow:
        - operations: ["*:*"]
```

#### Scoped Workflows
Different workflows can be scoped to specific users, teams, or permissions:

```yaml
roles:
  multi-scoped-admin:
    name: Multi-Scoped Administrator
    workflows:
      # Base approval for all requests
      - manager-approval
      
      # Additional security review for sensitive operations
      - security-review
      
      # Emergency bypass for on-call team
      - emergency-bypass
      
      # Extended approval for high-privilege actions
      - ciso-approval
      
      # Audit logging for all actions
      - audit-trail
    permissions:
      allow:
        - operations: ["*:*"]
```

#### Resource-Scoped Workflow Example
```yaml
roles:
  database-admin:
    name: Database Administrator
    workflows:
      # Standard approval for read operations
      - team-lead-approval
      
      # DBA approval for schema changes
      - dba-approval
      
      # CISO approval for production databases
      - ciso-approval
      
      # Immediate notification for all access
      - security-notification
    permissions:
      allow:
        - operations:
            - "rds:Describe*,List*"           # Read operations
            - "rds:CreateDBSnapshot"          # Backup operations
          targets:
            - "arn:aws:rds:*:*:db:dev-*"     # Development databases
            - "arn:aws:rds:*:*:db:staging-*" # Staging databases
        - operations:
            - "rds:ModifyDBInstance"          # Configuration changes
            - "rds:CreateDBInstance"          # New instance creation
          targets:
            - "arn:aws:rds:*:*:db:dev-*"     # Development only
```

#### Team-Scoped Workflow Example
```yaml
roles:
  escalated-support:
    name: Escalated Support Access
    workflows:
      # Different approval chains for different teams
      - l2-approval
      - security-approval
      - manager-approval
      - emergency-access
      
      # Universal workflows
      - access-logging
      - time-restriction
    scopes:
      allow:
        groups:
          - l2-support
          - security-team
          - engineering-managers
          - on-call
    permissions:
      allow:
        - operations: ["*:*"]
```

#### Permission-Scoped Workflow Example
```yaml
roles:
  cloud-engineer:
    name: Cloud Engineer
    workflows:
      # Light approval for read operations
      - self-approval
      
      # Team approval for standard operations
      - peer-review
      
      # Management approval for destructive operations
      - manager-approval
      
      # Security review for IAM operations
      - security-review
      
      # Audit trail for all actions
      - comprehensive-audit
    permissions:
      allow:
        - operations:
            - "ec2:Describe*,List*,Get*"     # Read operations
            - "ec2:Start*,Stop*,Reboot*"     # Management operations
            - "ec2:Create*,Update*,Modify*"  # Creation operations
            - "ec2:Terminate*,Delete*"       # Destructive operations
            - "iam:List*,Get*,Describe*"     # IAM read operations
            - "iam:Create*,Update*,Delete*"  # IAM write operations
```

### Conditional Workflows

Workflows can implement conditional logic based on context:

```yaml
roles:
  adaptive-access:
    name: Adaptive Access Control
    workflows:
      # Risk-based routing workflow
      - risk-assessment
      
      # Conditional workflows based on risk assessment:
      # - Low risk: automatic approval
      # - Medium risk: manager approval + time limits
      # - High risk: security team + CISO approval + enhanced monitoring
      
      # Time-based workflows
      - business-hours-check
      
      # Location-based workflows  
      - geo-validation
      
      # Frequency-based workflows
      - usage-pattern-check
    permissions:
      allow:
        - operations: ["*:*"]
```

### Dynamic Workflow Selection

Workflows can be dynamically selected based on request attributes:

```yaml
roles:
  smart-admin:
    name: Smart Administrative Access
    workflows:
      # Base workflow engine that routes to appropriate sub-workflows
      - dynamic-router
      
      # Available sub-workflows (selected by dynamic-router):
      # For emergency situations:
      - emergency-fast-track
      
      # For business hours, standard requests:
      - standard-approval
      
      # For after-hours requests:
      - extended-approval
      
      # For high-risk operations:
      - enhanced-security
      
      # For audit/compliance requests:
      - compliance-track
    permissions:
      allow:
        - operations: ["*:*"]
```

### Workflow Context

Workflows receive rich context about the role request, enabling intelligent routing and scoped processing:

#### Request Context
- **Role name**: Which role is being requested
- **User identity**: Who is requesting access (user ID, email, groups)
- **Duration**: How long access is requested for
- **Justification**: User-provided reason for access
- **Requested targets**: Specific resources if applicable
- **Provider instance**: Which provider instance will be used
- **Time context**: Request time, business hours, timezone
- **Location context**: User's IP address, geolocation
- **Risk factors**: Unusual access patterns, privilege escalation

#### Permission Context
- **Requested permissions**: Specific actions being requested
- **Permission risk level**: Classification of permission sensitivity
- **Target sensitivity**: Classification of target resources
- **Blast radius**: Potential impact of the requested access

#### Historical Context
- **Access history**: User's previous access patterns
- **Approval history**: Past approval decisions for similar requests
- **Incident context**: Recent security incidents or alerts
- **Compliance status**: Current compliance posture

#### Example: Context-Aware Workflow
```yaml
roles:
  context-aware-admin:
    name: Context-Aware Administrator
    workflows:
      # Main routing workflow that uses all available context
      - intelligent-router
      
      # Context-specific workflows:
      - first-time-access
      - repeat-access
      - anomaly-detected
      - high-risk-resource
      - compliance-required
      - incident-response
    permissions:
      allow:
        - operations: ["*:*"]
```

### Integration Examples

#### Emergency Access with Multiple Safeguards
```yaml
roles:
  break-glass:
    name: Emergency Break Glass Access
    workflows:
      # Immediate notification workflows
      - emergency-notification
      - incident-tracking
      
      # Approval workflows (can run in parallel)
      - on-call-approval
      - security-notification
      
      # Monitoring and control workflows
      - enhanced-monitoring
      - time-enforcement
      
      # Post-access workflows
      - post-incident-review
      - access-report
    scopes:
      allow:
        groups: [on-call, security-team, incident-commanders]
    permissions:
      allow:
        - operations: ["*:*"]
```

#### Development Access with Tiered Approval
```yaml
roles:
  dev-access:
    name: Development Access
    workflows:
      # Automated workflows for low-risk operations
      - self-approval
      - usage-tracking
      
      # Peer review for moderate-risk operations
      - peer-review
      
      # Management approval for high-risk operations
      - tech-lead-approval
      
      # Governance workflows
      - compliance-check
      - audit-logging
    scopes:
      allow:
        groups: [developers, qa-engineers]
    permissions:
      allow:
        - operations: ["*:*"]
          targets:
            - "*dev*"              # Development resources (self-approval)
            - "*staging*"          # Staging resources (peer-review)
            - "*prod*"             # Production resources (tech-lead-approval)
```

#### Audit Access with Enhanced Oversight
```yaml
roles:
  audit-access:
    name: Audit Access
    workflows:
      # Pre-approval workflows
      - compliance-team-approval
      - legal-review
      
      # Access control workflows
      - just-in-time
      - session-recording
      
      # Oversight workflows
      - dual-control
      - supervisor-monitoring
      
      # Post-access workflows  
      - access-summary
      - evidence-preservation
    scopes:
      allow:
        groups: [auditors, compliance-team, legal-team]
    permissions:
      allow:
        - operations: ["*:List*", "*:Describe*", "*:Get*", "*:Read*"]
```

#### Service Account Access with Automation Controls
```yaml
roles:
  ci-cd-deployment:
    name: CI/CD Deployment Access
    workflows:
      # Automated approval workflows
      - ci-validation
      - deployment-window
      
      # Safety workflows
      - canary-deployment
      - rollback-preparation
      
      # Monitoring workflows
      - deployment-monitoring
      - security-scanning
      
      # Notification workflows
      - team-notification
      - stakeholder-update
    scopes:
      allow:
        users:
          - ci-service@example.com
          - deployment-bot@example.com
    permissions:
      allow:
        - operations:
            - "ec2:*Instance*"
            - "s3:GetObject,PutObject"
            - "ecs:*Service*"
            - "lambda:UpdateFunctionCode"
      deny:
        - operations:
            - "*:Delete*"             # No deletion permissions for automation
            - "*:Create*User*"        # No user creation
```

---

## Configuration Management

### File Structure Options

Roles can be organized in multiple ways to suit different organizational needs:

#### Single File Approach
```yaml
# roles.yaml
version: "1.0"
roles:
  aws-developer:
    name: AWS Developer
    permissions: { ... }
  gcp-admin:
    name: GCP Administrator  
    permissions: { ... }
  azure-viewer:
    name: Azure Viewer
    permissions: { ... }
```

#### Multiple Files by Provider
```
config/roles/
├── aws.yaml          # AWS-specific roles
├── azure.yaml        # Azure-specific roles
├── gcp.yaml          # GCP-specific roles
├── kubernetes.yaml   # Kubernetes-specific roles
└── common.yaml       # Cross-provider roles
```

**aws.yaml:**
```yaml
version: "1.0"
roles:
  aws-ec2-admin:
    name: AWS EC2 Administrator
    providers: [aws-prod, aws-dev]
    permissions:
      allow:
        - operations: ["ec2:*"]
  
  aws-s3-readonly:
    name: AWS S3 Read-Only
    providers: [aws-prod]
    permissions:
      allow:
        - operations: ["s3:Get*", "s3:List*"]
```

#### Multiple Files by Team/Function
```
config/roles/
├── developers.yaml    # Developer roles
├── admins.yaml       # Administrative roles
├── security.yaml     # Security team roles
├── readonly.yaml     # Read-only access roles
└── emergency.yaml    # Break-glass access roles
```

**developers.yaml:**
```yaml
version: "1.0"
roles:
  frontend-developer:
    name: Frontend Developer
    scopes:
      allow:
        groups: [frontend-team]
    permissions:
      allow:
        - operations: ["s3:GetObject", "cloudfront:*"]
  
  backend-developer:
    name: Backend Developer
    scopes:
      allow:
        groups: [backend-team]
    permissions:
      allow:
        - operations: ["ec2:*", "rds:Describe*"]
```

### Loading Configuration

Configure role loading in the main agent configuration:

#### Directory-Based Loading
```yaml
# Load all YAML files from directory
roles:
  path: "./config/roles"
  # Recursively loads all *.yaml and *.yml files
```

#### URL-Based Loading
```yaml
# Load from remote URL
roles:
  url:
    uri: "https://config.company.com/roles.yaml"
    headers:
      Authorization: "Bearer ${VAULT_TOKEN}"
    refresh_interval: "5m"      # Refresh every 5 minutes
```

#### Vault Integration
```yaml
# Load from HashiCorp Vault
roles:
  vault:
    path: "secret/agent/roles"
    key: "roles"              # Key within the secret
    refresh_interval: "10m"    # Refresh interval
```

#### Inline Definitions
```yaml
# Define roles directly in main config
roles:
  admin:
    name: Administrator
    permissions:
      allow:
        - operations: ["*:*"]
  readonly:
    name: Read-Only User
    permissions:
      allow:
        - operations: ["*:Describe*", "*:List*", "*:Get*"]
```

#### Combined Loading
```yaml
# Load from multiple sources
roles:
  sources:
    - path: "./config/roles/local"
    - url:
        uri: "https://config.company.com/shared-roles.yaml"
        headers:
          Authorization: "Bearer ${CONFIG_TOKEN}"
    - vault:
        path: "secret/team/roles"
        key: "definitions"
```

### Configuration Validation

The agent validates role configurations on startup:

#### Syntax Validation
- YAML syntax correctness
- Required field presence
- Data type validation
- Reference integrity

#### Semantic Validation
- Inheritance cycle detection
- Provider compatibility
- Permission format validation
- Statement structure validation

#### Runtime Validation
- Provider role existence
- User/group/domain scope resolution
- Workflow availability
- Authentication provider integration

### Hot Reloading

For certain loading methods, roles can be updated without restarting:

```yaml
roles:
  path: "./config/roles"
  auto_reload: true           # Enable hot reloading
  reload_interval: "30s"      # Check for changes every 30 seconds
```

**Supported for hot reloading:**
- File-based loading (`path`)
- URL-based loading (`url`)
- Vault-based loading (`vault`)

**Not supported for hot reloading:**
- Inline definitions
- Combined loading with inline components

---

## Best Practices

### 1. Role Design Principles

#### Principle of Least Privilege
```yaml
# Good - specific permissions with targeted resources
ec2-restart-role:
  name: EC2 Instance Restart
  permissions:
    allow:
      - operations:
          - "ec2:DescribeInstances,StartInstances,StopInstances,RebootInstances"
          - "ec2:DescribeInstanceStatus"
        targets:
          - "arn:aws:ec2:*:*:instance/i-app-*"  # Only app instances

# Avoid - overly broad permissions  
ec2-admin-role:
  name: EC2 Admin
  permissions:
    allow:
      - operations: ["ec2:*"]  # Too broad, no target restrictions
```

#### Time-Bounded Access
```yaml
# Configure time limits in workflows, not roles
time-limited-admin:
  name: Time-Limited Admin
  workflows:
    - time-limited-approval  # Implements max 2-hour access
  permissions:
    allow:
      - operations: ["*:*"]
```

#### Clear Naming and Documentation
```yaml
# Good - descriptive names and documentation
aws-rds-backup-operator:
  name: AWS RDS Backup Operator
  description: |
    Allows operators to manage RDS backups including:
    - Creating manual snapshots
    - Restoring from snapshots  
    - Managing automated backup settings
    - Read access to backup status and logs
    
    Does NOT allow:
    - Deleting production databases
    - Modifying database configurations
    - Creating new database instances
  
# Avoid - unclear names
role1:
  name: Some Database Access
  description: Database stuff
```

### 2. Inheritance Patterns

#### Build Logical Role Hierarchies
```yaml
# Base roles with fundamental permissions
cloud-readonly-base:
  name: Cloud Read-Only Base
  permissions:
    allow:
      - operations: ["*:Describe*", "*:List*", "*:Get*"]

# Service-specific roles
aws-readonly:
  name: AWS Read-Only
  inherits: [cloud-readonly-base]
  providers: [aws-prod, aws-dev]
  
ec2-readonly:
  name: EC2 Read-Only
  inherits: [aws-readonly]
  permissions:
    allow:
      - operations: ["ec2:*"]
        targets: ["arn:aws:ec2:*:*:*"]

# Team-specific roles
dev-team-ec2:
  name: Development Team EC2 Access
  inherits: [ec2-readonly]
  scopes:
    allow:
      groups: [developers]
  permissions:
    allow:
      - operations: ["ec2:StartInstances", "ec2:StopInstances"]
        targets: ["arn:aws:ec2:*:*:instance/i-dev-*"]
```

#### Leverage Provider Managed Roles
```yaml
# Good - use existing cloud roles as foundation
aws-power-user:
  name: AWS Power User
  inherits:
    - "aws-prod:arn:aws:iam::aws:policy/PowerUserAccess"
  # Add company-specific restrictions
  permissions:
    deny:
      - operations: ["iam:*User*", "iam:*Role*"]  # No user/role management
      - operations: ["s3:*"]
        targets: ["arn:aws:s3:::sensitive-*"]      # No sensitive buckets
```

### 3. Security Patterns

#### Defense in Depth
```yaml
production-admin:
  name: Production Administrator
  description: High-privilege production access with multiple security layers
  
  # Multiple approval layers
  workflows:
    - identity-verification
    - manager-approval
    - security-approval
    - time-restriction
  
  # Strict scope limitation
  scopes:
    allow:
      users: [emergency-admin@example.com]
      groups: [senior-sre, security-team]
  
  permissions:
    allow:
      - operations: ["*:*"]
        targets: ["arn:aws:*:us-east-1:123456789012:*"]  # Single region only
    deny:
      - operations:
          - "iam:DeleteUser"
          - "iam:DeleteRole"
          - "s3:DeleteBucket"
      - operations: ["*"]
        targets:
          - "arn:aws:s3:::audit-*"         # No audit data
          - "arn:aws:kms:*:*:key/*"        # No key access
```

#### Explicit Denials for High-Risk Actions
```yaml
developer-access:
  name: Developer Access
  permissions:
    allow:
      - operations:
          - "ec2:*"
          - "s3:*"
          - "rds:*"
    deny:
      - operations:
          # Explicit denials for dangerous actions
          - "ec2:TerminateInstances"
          - "s3:DeleteBucket"
          - "rds:DeleteDBInstance"
          - "iam:*"                         # No IAM access at all
```

### 4. Operational Patterns

#### Environment Separation
```yaml
# Good - clear environment separation
development-admin:
  name: Development Administrator
  providers: [aws-dev, azure-dev, gcp-dev]
  workflows: [self-approval]                        # Minimal approval for dev
  
staging-admin:
  name: Staging Administrator  
  providers: [aws-staging, azure-staging, gcp-staging]
  workflows: [lead-approval]                        # Team lead approval
  
production-readonly:
  name: Production Read-Only
  providers: [aws-prod, azure-prod, gcp-prod]
  workflows: [manager-approval, audit-logging]      # Strict controls for prod
  permissions:
    allow:
      - operations: ["*:Describe*", "*:List*", "*:Get*"]
```

#### Emergency Access Patterns
```yaml
break-glass-access:
  name: Emergency Break Glass Access
  description: |
    EMERGENCY USE ONLY
    This role provides unrestricted access for critical incidents.
    All usage is heavily audited and requires post-incident review.
  
  workflows:
    - emergency-notification
    - break-glass-logging
    - post-incident-review
  
  scopes:
    allow:
      groups: [on-call, incident-commanders]
  
  permissions:
    allow:
      - operations: ["*:*"]
    
    # Even emergency access has some limits
    deny:
      - operations: ["*"]
        targets:
          - "arn:aws:s3:::customer-data-*"   # Customer data protection
          - "arn:aws:kms:*:*:key/*"          # Encryption key protection
```

#### Service Account Patterns
```yaml
ci-cd-deployment:
  name: CI/CD Deployment Access
  description: Automated deployment service access
  
  scopes:
    allow:
      users:
        - ci-service@example.com
        - deployment-bot@example.com
  
  workflows:
    - automated-approval
    - deployment-logging
  
  permissions:
    allow:
      - operations:
          - "ec2:*Instance*"
          - "s3:GetObject,PutObject"
          - "ecs:*Service*"
          - "lambda:UpdateFunctionCode"
    deny:
      - operations:
          - "*:Delete*"             # No deletion permissions for automation
          - "*:Create*User*"        # No user creation
```

### 5. Maintenance Patterns

#### Regular Permission Audits
```yaml
# Use descriptive comments for audit trails
quarterly-access-review:
  name: Quarterly Access Review
  description: |
    Last reviewed: 2025-01-15
    Next review: 2025-04-15
    Approved by: Security Committee
    
    This role provides quarterly access review capabilities
    for compliance auditing purposes.
```

#### Version Control Integration
```yaml
# Include metadata for tracking
developer-role:
  name: Developer Access
  description: |
    Version: 2.1.0
    Last modified: 2025-01-15
    Modified by: alice@example.com
    Change reason: Migrated to statement-based permissions
    
    Change log:
    - 2.1.0: Migrated to statement format with targets and conditions
    - 2.0.0: Migrated to intelligent permission merging
    - 1.0.0: Initial role definition
```

---

## Troubleshooting

### Common Issues and Solutions

#### 1. Role Inheritance Errors

**Error:** `role admin inherits from non-existent role user`

**Cause:** Referenced role doesn't exist or isn't loaded yet

**Solution:**
```yaml
# Ensure base roles are defined before child roles
base-user:
  name: Base User
  permissions:
    allow:
      - operations: ["*:Describe*", "*:List*"]

admin-user:
  name: Administrator
  inherits: [base-user]  # Now this will work
  permissions:
    allow:
      - operations: ["*:*"]
```

#### 2. Provider Role Not Found

**Error:** `role inherits from arn:aws:iam::aws:policy/NonexistentPolicy`

**Cause:** Provider role ARN is incorrect or doesn't exist in target account

**Solutions:**
```bash
# Verify AWS managed policies
aws iam list-policies --scope AWS --query 'Policies[?PolicyName==`PowerUserAccess`]'

# Verify custom policies  
aws iam get-policy --policy-arn arn:aws:iam::123456789012:policy/CustomPolicy

# Check GCP roles
gcloud iam roles list --filter="name:roles/compute.viewer"

# Check Azure roles
az role definition list --name "Virtual Machine Contributor"
```

#### 3. Permission Validation Errors

**Error:** `permission ec2:InvalidAction not found in provider`

**Cause:** Permission name is incorrect or not supported by provider

**Solutions:**
```yaml
# Use correct AWS permission names
permissions:
  allow:
    - operations:
        - "ec2:DescribeInstances"     # Correct
        # - "ec2:ListInstances"       # Incorrect - no such permission

# Check provider documentation for correct names
# AWS: https://docs.aws.amazon.com/service-authorization/
# Azure: https://docs.microsoft.com/en-us/azure/role-based-access-control/
# GCP: https://cloud.google.com/iam/docs/understanding-roles
```

#### 4. Scope Resolution Issues

**Error:** `user alice@example.com cannot access role admin`

**Cause:** User not included in role scopes, or denied by a deny rule

**Solutions:**
```yaml
# Check role scopes include the user in allow (and not in deny)
admin-role:
  scopes:
    allow:
      users: [alice@example.com]  # Direct user access
      groups: [administrators]    # Or group membership
    # Ensure no deny rules block this user
    # deny:
    #   groups: [some-group-user-is-in]  # This would block access

# Verify user's group memberships
# Check identity provider for user's group assignments
```

#### 5. Provider-Specific Inheritance Issues

**Error:** `provider aws-prod does not support role arn:aws:iam::456:role/Role`

**Cause:** Cross-account role inheritance without proper trust relationship

**Solutions:**
```yaml
# Ensure role is in the correct account
aws-role:
  providers: [aws-prod]
  inherits:
    # Use role from same account as provider
    - "aws-prod:arn:aws:iam::123456789012:role/MyRole"  # Correct account

# Set up cross-account trust if needed
# In the target role's trust policy:
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "AWS": "arn:aws:iam::123456789012:root"  # Trust the source account
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```

#### 6. Condensed Action Parsing Issues

**Error:** `invalid condensed action format: k8s:pods:get,list,`

**Cause:** Trailing comma or empty action in condensed format

**Solutions:**
```yaml
# Incorrect - trailing comma
permissions:
  allow:
    - operations:
        - "k8s:pods:get,list,"     # Trailing comma

# Correct format
permissions:
  allow:
    - operations:
        - "k8s:pods:get,list"      # No trailing comma
        - "k8s:services:create,delete,get,list,update"  # Properly formatted
```

#### 7. GCP Permissions Being Condensed

**Error:** GCP permissions appear to be merged incorrectly

**Cause:** The system correctly detects GCP-style permissions (with dots in the action) and treats them atomically. If you see unexpected behavior, check that your permissions follow the correct format.

**Expected Behavior:**
```yaml
permissions:
  allow:
    - operations:
        # These GCP-style permissions are NEVER condensed
        - "gcp-prod:compute.instances.get"
        - "gcp-prod:compute.instances.list"
        - "gcp-prod:compute.instances.start"
        # They remain as separate entries, not merged like:
        # - "gcp-prod:compute.instances:get,list,start"  # This is NOT how GCP works

    - operations:
        # These AWS/K8s permissions CAN be condensed
        - "ec2:DescribeInstances,StartInstances"   # Condensable (no dots in action)
        - "k8s:pods:get,list,watch"                # Condensable (no dots in action)
```

**Detection Rule:** If the last segment (after the final colon) contains a dot, the permission is treated as atomic and never condensed.

#### 8. Provider Filtering Not Working

**Error:** Inherited permissions from other providers are showing up

**Cause:** Provider prefixes must exactly match entries in the role's `providers` list

**Solutions:**
```yaml
# Incorrect - provider prefix doesn't match providers list
my-role:
  providers: [aws-production]  # Note: "aws-production"
  inherits: [base-role]        # base-role has "aws-prod:ec2:*"
  # "aws-prod" != "aws-production", so permission is filtered out

# Correct - provider prefixes match
my-role:
  providers: [aws-prod]        # Matches the prefix
  inherits: [base-role]        # base-role has "aws-prod:ec2:*"
  # "aws-prod" == "aws-prod", so permission is included
```

#### 9. Statement Format Issues

**Error:** `invalid permission statement: missing operations field`

**Cause:** Permission statements must include at least the `operations` field

**Solutions:**
```yaml
# Incorrect - missing operations
permissions:
  allow:
    - targets:                     # Missing 'operations'
        - "arn:aws:s3:::bucket/*"

# Correct - operations is required
permissions:
  allow:
    - operations:
        - "s3:GetObject"
      targets:
        - "arn:aws:s3:::bucket/*"

# Also correct - targets and conditions are optional
permissions:
  allow:
    - operations:
        - "s3:GetObject"
```

#### 10. Deny Scope Confusion

**Error:** User can't access a role despite being in an allowed group

**Cause:** A deny scope rule is blocking access. Deny always takes precedence over allow.

**Solutions:**
```yaml
# Check for deny rules that might match the user
my-role:
  scopes:
    allow:
      groups: [engineering]       # User is in this group
    deny:
      groups: [contractors]       # But also in this group - DENIED!
      domains: [external.com]     # Or has this email domain - DENIED!

# Remove the conflicting deny rule, or remove the user from the denied group
```

### Debugging Tools and Techniques

#### Enable Debug Logging
```yaml
# In main agent configuration
logging:
  level: debug
  components:
    - roles
    - inheritance
    - permissions
```

#### Use CLI Tools for Testing
```bash
# Test role resolution (hypothetical CLI commands)
thand roles                                    # List all roles
```

#### Validate Configuration
```bash
# Validate role configuration files
thand config validate --roles-only
thand config validate --file ./config/roles/aws.yaml
```

#### Test Inheritance Resolution
```yaml
# Add temporary debug role to test inheritance
debug-inheritance:
  name: Debug Inheritance Test
  inherits: [problematic-role]
  permissions:
    allow:
      - operations: ["debug:test"]
  # This will show inheritance resolution issues
```

### Getting Help

#### Enable Verbose Logging
```yaml
logging:
  level: trace
  format: json
  outputs:
    - type: file
      path: /var/log/agent/roles.log
    - type: console
```

#### Check System Health
```bash
# Check provider connectivity
thand providers status

# Check identity provider integration  
thand providers auth status

# Check workflow system
thand workflows status
```

#### Contact Information
- **Documentation:** [https://docs.thand.io](https://docs.thand.io)
- **Community:** [GitHub Discussions](https://github.com/thand-io/agent/discussions)
- **Support:** [support@thand.io](mailto:support@thand.io)
- **Security Issues:** [security@thand.io](mailto:security@thand.io)

---

## Examples

For practical examples and templates of role configurations, see the [Role Examples](examples/) page which includes:

- **Basic Development Role** - Simple developer access patterns
- **Inherited Admin Role** - Multi-cloud administrative access using inheritance
- **Emergency Access Role** - Break-glass access for incidents
- **Read-Only Auditor Role** - Compliance and auditing access
- **Database Administrator Role** - Specialized database permissions
- **DevOps Engineer Role** - Infrastructure and deployment management
- **Security Analyst Role** - Security monitoring and investigation
- **Temporary Contractor Role** - Time-limited external access
- **Multi-Environment Role** - Different access across environments
- **Application-Specific Role** - Fine-grained application permissions

Each example includes complete YAML configurations with explanations of the patterns used.

---

{: .note}
**Provider Prefix Syntax:** When mixing multiple providers into a single role, you can use the provider name as a prefix to avoid ambiguity. For example, to inherit from an AWS role in the `aws-prod` provider instance, use `aws-prod:arn:aws:iam::aws:policy/ReadOnlyAccess`. The system uses the first colon as the delimiter between provider name and role identifier.

