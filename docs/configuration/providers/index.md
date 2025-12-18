---
layout: default
title: Providers
parent: Configuration
nav_order: 8
description: Detailed documentation for Thand Agent providers
has_children: true
---

# Providers

Providers are the core integration components of the Thand Agent that enable connectivity with various external services, identity providers, and cloud platforms. Each provider implements specific capabilities to support authentication, authorization, role-based access control, and notifications.

## Provider Capabilities

The Thand Agent supports four main provider capabilities:

### 1. Authentication (Authorizor)
**Capability**: `authorizor`

Enables user authentication and session management. Providers with this capability can:
- Authenticate users against external identity systems
- Create and manage user sessions
- Handle OAuth2, SAML, and other authentication flows
- Validate and renew authentication tokens

### 2. Role-Based Access Control (RBAC)
**Capability**: `rbac`

Provides role and permission management capabilities. Providers with this capability can:
- Grant temporary access to roles and permissions
- Manage user entitlements and access levels
- Integrate with cloud IAM systems (AWS, Azure, GCP)
- Handle role elevation and access requests

### 3. Notifications
**Capability**: `notifier`

Enables sending notifications and messages. Providers with this capability can:
- Send notifications via email, Slack, or other channels
- Deliver access request notifications
- Provide audit and alert messaging
- Support custom notification templates

### 4. Identity Management
**Capability**: `identities`

Provides user and group discovery capabilities. Providers with this capability can:
- Discover users and groups from directory services
- Sync identity information from external systems
- Provide user profile and membership data
- Support identity federation scenarios

## Available Providers

### Cloud & Infrastructure Providers

| Provider | Capabilities | Description |
|----------|-------------|-------------|
| [AWS](aws/) | RBAC | Amazon Web Services IAM and SSO integration |
| [Azure](azure/) | RBAC | Microsoft Azure RBAC and subscription management |
| [GCP](gcp/) | RBAC | Google Cloud Platform IAM and resource management |
| [Cloudflare](cloudflare/) | RBAC, Identities | Cloudflare account management with role and policy-based access control |
| [Kubernetes](kubernetes/) | Authorizor, RBAC | Kubernetes cluster authentication and RBAC |

### Development & DevOps

| Provider | Capabilities | Description |
|----------|-------------|-------------|
| [GitHub](github/) | Authorizor, RBAC | GitHub repository and organization management |
| [Terraform](terraform/) | Authorizor, RBAC | Terraform Cloud/Enterprise workspace management |

### Enterprise Authentication

| Provider | Capabilities | Description |
|----------|-------------|-------------|
| [SAML](saml/) | Authorizor | SAML 2.0 SSO integration for enterprise identity providers |
| [OAuth2](oauth2/) | Authorizor | Generic OAuth2 authentication for any compliant service |
| [Google OAuth2](oauth2.google/) | Authorizor | Google account authentication with OAuth2 |
| [Thand](thand/) | Authorizor | Thand federated OIDC authentication service |
| [Okta](okta/) | RBAC, Identities | Okta identity management and administrator role control |

### Business Applications

| Provider | Capabilities | Description |
|----------|-------------|-------------|
| [Salesforce](salesforce/) | RBAC, Authorizor | Salesforce CRM user and permission management |
| [Google Workspace](gsuite/) | Authorizor, Identities | Google Workspace user and group management |

### Infrastructure & Communication

| Provider | Capabilities | Description |
|----------|-------------|-------------|
| [Slack](slack/) | Notifier | Slack team communication and notifications |
| [Email](email/) | Notifier | SMTP email notifications and communication |

## Provider Configuration

All providers follow a common configuration structure:

```yaml
version: "1.0"
providers:
  provider-name:
    name: Display Name
    description: Provider description
    provider: provider-type
    enabled: true
    config:
      # Provider-specific configuration options
```

### Common Configuration Options

- **name**: Human-readable display name for the provider
- **description**: Brief description of the provider's purpose
- **provider**: The provider type (e.g., `aws`, `azure`, `github`)
- **enabled**: Whether the provider is active
- **config**: Provider-specific configuration parameters

### Capabilities Configuration

Providers can optionally configure specific capabilities to control which features are enabled and how they behave. This is useful for disabling specific features of a provider or tuning synchronization intervals.

```yaml
providers:
  my-provider:
    # ... basic config ...
    capabilities:
      roles:
        enabled: true
        synchronizable: true
        interval: 60
      users:
        enabled: true
        synchronizable: false
      notifier:
        enabled: false
```

#### Configuration Options

For synchronizable capabilities (Roles, Permissions, Resources, Identities, Users, Groups):

- **enabled** (bool): Whether the capability is active. You cannot enable a capability that is not supported by the provider.
- **synchronizable** (bool): Whether the agent should periodically synchronize data for this capability.
- **interval** (int): Synchronization interval in minutes. Default is typically 360 minutes (6 hours).

For other capabilities (Authorizer, Notifier, Provisioning):

- **enabled** (bool): Whether the capability is active.

## Dynamic Configuration with Environment Variables

Provider configurations support dynamic value resolution using [jq](https://jqlang.github.io/jq/) expressions. This allows you to:

- Reference environment variables in your configuration
- Use conditional logic to change settings based on environment
- Concatenate strings dynamically
- Provide default values for optional settings

### Syntax

Dynamic expressions use the `${ }` syntax with jq expressions inside. Environment variables and any values passed to the configuration are accessible using dot notation (`.VARIABLE_NAME`).

### Basic Environment Variable Access

Reference environment variables directly in your configuration:

```yaml
providers:
  aws-prod:
    name: AWS Production
    provider: aws
    config:
      region: ${ .AWS_REGION }
      account_id: ${ .AWS_ACCOUNT_ID }
      role_arn: ${ .AWS_ROLE_ARN }
```

### Default Values

Use the jq alternative operator (`//`) to provide fallback values when an environment variable is not set:

```yaml
providers:
  aws:
    name: AWS
    provider: aws
    config:
      region: ${ .AWS_REGION // "us-east-1" }
      timeout: ${ .AWS_TIMEOUT // "30" }
```

### String Concatenation

Build dynamic strings by concatenating values:

```yaml
providers:
  aws:
    name: AWS
    provider: aws
    config:
      role_arn: ${ "arn:aws:iam::" + .AWS_ACCOUNT_ID + ":role/" + .AWS_ROLE_NAME }
```

### Conditional Configuration

Use jq conditionals to change configuration based on environment:

```yaml
providers:
  database:
    name: Database
    provider: postgres
    config:
      # Switch between production and development settings
      host: ${ if .MODE == "prod" then "db.production.example.com" else "localhost" end }
      port: ${ if .MODE == "prod" then "5432" else "5433" end }
      ssl_mode: ${ if .MODE == "prod" then "require" else "disable" end }
      
  aws:
    name: AWS
    provider: aws
    config:
      # Use different accounts based on environment
      account_id: ${ if .ENV == "production" then .PROD_ACCOUNT_ID else .DEV_ACCOUNT_ID end }
      region: ${ if .ENV == "production" then "us-east-1" else "us-west-2" end }
```

### Nested Object Access

Access nested values from structured input:

```yaml
providers:
  kubernetes:
    name: Kubernetes
    provider: kubernetes
    config:
      cluster: ${ .cluster.name }
      endpoint: ${ .cluster.endpoint }
      namespace: ${ .cluster.namespace // "default" }
```

### Real-World Examples

#### Multi-Environment AWS Configuration

```yaml
providers:
  aws:
    name: ${ "AWS " + (.ENV // "Development") }
    description: ${ if .ENV == "prod" then "Production AWS Environment" else "Development AWS Environment" end }
    provider: aws
    enabled: true
    config:
      region: ${ .AWS_REGION // "us-east-1" }
      account_id: ${ .AWS_ACCOUNT_ID }
      role_arn: ${ "arn:aws:iam::" + .AWS_ACCOUNT_ID + ":role/" + (.AWS_ROLE // "ThandAgentRole") }
      session_duration: ${ if .ENV == "prod" then "3600" else "7200" end }
```

#### Conditional Notification Provider

```yaml
providers:
  notifications:
    name: Notifications
    provider: ${ if .NOTIFICATION_TYPE == "slack" then "slack" else "email" end }
    config:
      # Slack configuration
      webhook_url: ${ .SLACK_WEBHOOK_URL // null }
      channel: ${ .SLACK_CHANNEL // "#alerts" }
      
      # Email configuration (used when NOTIFICATION_TYPE != "slack")
      smtp_host: ${ .SMTP_HOST // "smtp.example.com" }
      smtp_port: ${ .SMTP_PORT // "587" }
      from_address: ${ .EMAIL_FROM // "noreply@example.com" }
```

### Environment Variable Setup

Set environment variables before running the agent:

```bash
# Linux/macOS
export MODE=prod
export AWS_REGION=us-east-1
export AWS_ACCOUNT_ID=123456789012

# Or inline with the command
MODE=prod AWS_REGION=us-east-1 thand-agent start
```

```powershell
# Windows PowerShell
$env:MODE = "prod"
$env:AWS_REGION = "us-east-1"
$env:AWS_ACCOUNT_ID = "123456789012"
```

### Available jq Features

The configuration resolver supports standard jq syntax including:

| Feature | Example | Description |
|---------|---------|-------------|
| Field access | `.FIELD_NAME` | Access a field value |

## Developing New Providers

If you need to integrate with a service that is not currently supported, you can create a custom provider. See the [Creating a New Provider](../../development/providers) guide for detailed instructions on implementing the provider interface and capabilities.
| Nested access | `.parent.child` | Access nested fields |
| Alternative | `.VAR // "default"` | Provide default if null |
| Conditionals | `if .X == "y" then "a" else "b" end` | Conditional logic |
| String concat | `"prefix" + .VAR + "suffix"` | Concatenate strings |
| Comparison | `.VAR == "value"` | Equality comparison |
| Boolean ops | `.A and .B`, `.A or .B` | Logical operations |

## Getting Started

1. **Choose Your Providers**: Select providers based on your infrastructure and requirements
2. **Review Prerequisites**: Check each provider's documentation for setup requirements
3. **Obtain Credentials**: Follow the credential setup guides for each provider
4. **Configure**: Add provider configurations to your agent configuration file
5. **Test**: Verify provider connectivity and functionality

## Security Best Practices

- **Use Placeholders**: Never commit actual credentials to version control
- **Least Privilege**: Grant minimal required permissions to provider credentials
- **Rotate Credentials**: Regularly rotate API keys, tokens, and certificates
- **Secure Storage**: Use secure credential storage solutions (vaults, secrets managers)
- **Monitor Access**: Enable audit logging and monitor provider access patterns

## Troubleshooting

For provider-specific troubleshooting:
1. Check the individual provider documentation
2. Enable debug logging in your agent configuration
3. Verify credentials and permissions
4. Check network connectivity and firewall settings
5. Review provider service status and API limits
