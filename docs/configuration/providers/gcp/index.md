---
layout: default
title: GCP
description: Google Cloud Platform provider with IAM and resource management
parent: Providers
grand_parent: Configuration
---

# GCP Provider

The GCP provider enables integration with Google Cloud Platform, providing role-based access control (RBAC) capabilities through Google Cloud IAM and Resource Manager.

## Capabilities

- **Role-Based Access Control (RBAC)**: Supports GCP IAM roles and bindings
- **Project Management**: Access to GCP projects and resources
- **Tenant Support**: Automatically discovers and synchronizes GCP projects and folders
- **Permission Management**: Access to GCP IAM permissions and policies
- **Service Account Integration**: Support for service account authentication

## Prerequisites

### GCP Project Setup

1. **GCP Project**: Active Google Cloud Platform project with appropriate permissions
2. **IAM Permissions**: The agent needs permissions to:
   - Read IAM roles and bindings
   - Access project and resource information
   - List GCP service account details

### Required GCP Permissions

The following GCP IAM permissions are required for the agent to function properly:

#### Basic Permissions (Project-Level Access)

For accessing a single project:

```yaml
permissions:
  - iam.roles.list
  - iam.roles.get
  - resourcemanager.projects.get
  - resourcemanager.projects.getIamPolicy
  - iam.serviceAccounts.list
  - iam.serviceAccounts.get
```

**Recommended Role**: `roles/viewer` (includes all necessary read permissions)

#### Organization-Level Permissions (Tenant Discovery)

For discovering all projects and folders across your organization:

```yaml
permissions:
  - resourcemanager.projects.list
  - resourcemanager.folders.list
  - resourcemanager.organizations.get
```

**Recommended Role**: `roles/browser` (enables full tenant hierarchy discovery)

> **Note**: For full tenant synchronization capabilities (listing all projects and folders in your organization), grant the `roles/browser` role at the organization level. This allows the agent to discover and track your entire GCP resource hierarchy.

## Authentication Methods

The GCP provider supports multiple authentication methods:

### 1. Service Account Key File (Recommended)

Uses a service account key file for authentication:

```yaml
providers:
  gcp-prod:
    name: GCP Production
    provider: gcp
    config:
      project_id: YOUR_PROJECT_ID
      service_account_key_path: /path/to/service-account-key.json
```

### 2. Service Account Key JSON (Inline)

Uses service account key JSON content directly in configuration:

```yaml
providers:
  gcp-prod:
    name: GCP Production
    provider: gcp
    config:
      project_id: YOUR_PROJECT_ID
      service_account_key: |
        {
          "type": "service_account",
          "project_id": "YOUR_PROJECT_ID",
          "private_key_id": "YOUR_PRIVATE_KEY_ID",
          "private_key": "-----BEGIN PRIVATE KEY-----\nYOUR_PRIVATE_KEY\n-----END PRIVATE KEY-----\n",
          "client_email": "YOUR_SERVICE_ACCOUNT@YOUR_PROJECT_ID.iam.gserviceaccount.com",
          "client_id": "YOUR_CLIENT_ID",
          "auth_uri": "https://accounts.google.com/o/oauth2/auth",
          "token_uri": "https://oauth2.googleapis.com/token"
        }
```

### 3. Service Account Key (Structured)

Uses structured credentials object:

```yaml
providers:
  gcp-prod:
    name: GCP Production
    provider: gcp
    config:
      project_id: YOUR_PROJECT_ID
      credentials:
        type: service_account
        project_id: YOUR_PROJECT_ID
        private_key_id: YOUR_PRIVATE_KEY_ID
        private_key: |
          -----BEGIN PRIVATE KEY-----
          YOUR_PRIVATE_KEY
          -----END PRIVATE KEY-----
        client_email: YOUR_SERVICE_ACCOUNT@YOUR_PROJECT_ID.iam.gserviceaccount.com
        client_id: YOUR_CLIENT_ID
        auth_uri: https://accounts.google.com/o/oauth2/auth
        token_uri: https://oauth2.googleapis.com/token
```

### 4. Default Credentials (ADC)

When no credentials are provided, uses Application Default Credentials (environment variables, metadata service, etc.):

```yaml
providers:
  gcp-prod:
    name: GCP Production
    provider: gcp
    config:
      project_id: YOUR_PROJECT_ID
```

## Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `project_id` | string | Yes | - | GCP project ID |
| `service_account_key_path` | string | No | - | Path to service account key file |
| `service_account_key` | string | No | - | Service account key JSON content |
| `credentials` | object | No | - | Structured service account credentials |
| `stage` | string | No | `GA` | GCP API stage (GA, BETA, ALPHA) |
| `region` | string | No | - | Default GCP region (informational) |

## Getting Credentials

### Service Account Setup

1. **Create Service Account**: In GCP Console → IAM & Admin → Service Accounts → Create Service Account

2. **Set Permissions**: 
   - **For single project access**: Grant the service account the `Viewer` role at the project level
   - **For organization-wide tenant discovery**: Grant the `Browser` role at the organization level

   ```bash
   # Organization-level access (recommended for tenant discovery)
   ORG_ID=$(gcloud organizations list --format="value(ID)")
   gcloud organizations add-iam-policy-binding $ORG_ID \
       --member="serviceAccount:agent@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
       --role="roles/browser"
   
   # Optional: Add folder viewer for detailed folder information
   gcloud organizations add-iam-policy-binding $ORG_ID \
       --member="serviceAccount:agent@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
       --role="roles/resourcemanager.folderViewer"
   
   # OR: Project-level access (for single project)
   gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
       --member="serviceAccount:agent@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
       --role="roles/viewer"
   ```

3. **Create Key**: In the service account details → Keys → Add Key → Create New Key
   - Choose JSON format
   - Download the key file

4. **Configure Agent**: Use the downloaded key file path or content in your configuration

### gcloud CLI Setup

1. **Install gcloud CLI**: Follow the [gcloud CLI installation guide](https://cloud.google.com/sdk/docs/install)

2. **Initialize gcloud**:
   ```bash
   gcloud init
   ```

3. **Set Application Default Credentials**:
   ```bash
   gcloud auth application-default login
   ```

4. **Get Project ID**:
   ```bash
   gcloud config get-value project
   ```

### Compute Engine/GKE Setup (Default Credentials)

1. **Enable Default Service Account**: Ensure your Compute Engine instance or GKE cluster has a service account attached
2. **Grant Permissions**: Grant the service account appropriate IAM permissions
3. **No Configuration Needed**: Agent will automatically use the metadata service

## Example Configurations

### Production Environment with Service Account Key File

```yaml
version: "1.0"
providers:
  gcp-prod:
    name: GCP Production
    description: Production GCP environment
    provider: gcp
    enabled: true
    config:
      project_id: YOUR_PROD_PROJECT_ID
      service_account_key_path: /etc/agent/gcp-prod-key.json
      region: us-central1
```

### Development Environment with Inline Key

```yaml
version: "1.0"
providers:
  gcp-dev:
    name: GCP Development
    description: Development GCP environment
    provider: gcp
    enabled: true
    config:
      project_id: YOUR_DEV_PROJECT_ID
      service_account_key: |
        {
          "type": "service_account",
          "project_id": "YOUR_DEV_PROJECT_ID",
          "private_key_id": "YOUR_PRIVATE_KEY_ID",
          "private_key": "-----BEGIN PRIVATE KEY-----\nYOUR_PRIVATE_KEY\n-----END PRIVATE KEY-----\n",
          "client_email": "agent-dev@YOUR_DEV_PROJECT_ID.iam.gserviceaccount.com",
          "client_id": "YOUR_CLIENT_ID",
          "auth_uri": "https://accounts.google.com/o/oauth2/auth",
          "token_uri": "https://oauth2.googleapis.com/token"
        }
```

### Multi-Project Setup

```yaml
version: "1.0"
providers:
  gcp-prod:
    name: GCP Production
    description: Production project
    provider: gcp
    enabled: true
    config:
      project_id: YOUR_PROD_PROJECT_ID
      service_account_key_path: /etc/agent/prod-key.json
  
  gcp-staging:
    name: GCP Staging
    description: Staging project
    provider: gcp
    enabled: true
    config:
      project_id: YOUR_STAGING_PROJECT_ID
      service_account_key_path: /etc/agent/staging-key.json
  
  gcp-dev:
    name: GCP Development
    description: Development project
    provider: gcp
    enabled: true
    config:
      project_id: YOUR_DEV_PROJECT_ID
      # Uses Application Default Credentials
```

### Using Default Credentials

```yaml
version: "1.0"
providers:
  gcp-compute:
    name: GCP Compute
    description: GCP using default credentials
    provider: gcp
    enabled: true
    config:
      project_id: YOUR_PROJECT_ID
      region: us-central1
```

## Features

### Tenant Discovery and Synchronization

The GCP provider automatically discovers and synchronizes your GCP resource hierarchy:

- **Projects**: All active GCP projects accessible to the service account
- **Folders**: Organizational folders (requires organization-level permissions)
- **Hierarchical Structure**: Maintains parent-child relationships in your GCP organization

Tenant synchronization enables:
- Multi-project access management
- Organization-wide visibility
- Hierarchical role assignments
- Folder-based policy application

**Requirements for full tenant discovery:**
- Service account with `roles/browser` role at organization level
- Cloud Resource Manager API enabled

### GCP IAM Integration

The GCP provider automatically discovers and indexes GCP predefined and custom roles, making them available for role elevation requests.

### API Stage Support

Support for different GCP API stages:
- **GA** (Generally Available): Stable production APIs
- **BETA**: Beta APIs with additional features
- **ALPHA**: Alpha APIs with experimental features

### Permission Indexing

The provider includes comprehensive GCP IAM permissions data, enabling:
- Permission search and discovery
- Role analysis and recommendations
- Policy validation

## Troubleshooting

### Common Issues

1. **Authentication Failures**
   - Verify service account key is valid and properly formatted
   - Check project ID is correct and accessible
   - Ensure service account has necessary permissions

2. **Project Access Issues**
   - Verify the service account has access to the specified project
   - Check if billing is enabled on the project
   - Ensure required APIs are enabled (IAM, Resource Manager)

3. **Permission Issues**
   - Verify service account has `Viewer` role or equivalent permissions
   - For tenant discovery, verify `Browser` role is granted at organization level
   - Check if organization policies restrict access
   - Ensure IAM API is enabled

4. **Tenant Synchronization Issues**
   - Verify Cloud Resource Manager API is enabled
   - Check service account has `roles/browser` permission at organization level
   - Ensure projects and folders are in `ACTIVE` state
   - Review logs for API quota or rate limit errors

### Debugging

Enable debug logging to troubleshoot GCP provider issues:

```yaml
logging:
  level: debug
```

Look for GCP-specific log entries to identify authentication and permission issues.

### Environment Variables

The GCP provider also supports standard Google Cloud environment variables:

- `GOOGLE_APPLICATION_CREDENTIALS`: Path to service account key file
- `GOOGLE_CLOUD_PROJECT`: GCP project ID
- `GCLOUD_PROJECT`: Alternative project ID variable

### API Requirements

Ensure the following APIs are enabled in your GCP project:
- **Identity and Access Management (IAM) API**
- **Cloud Resource Manager API**
