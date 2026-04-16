# Thand Agent - Configuration Examples

This directory contains example configurations for Thand's agent, which is designed to enhance security and streamline access management across various platforms.

## Directory Structure

```
examples/
├── providers/          # Provider configuration examples
│   ├── aws.example.yaml
│   ├── azure.example.yaml
│   ├── cloudflare.example.yaml
│   ├── gcp.example.yaml
│   ├── github.example.yaml
│   ├── gsuite.example.yaml
│   ├── salesforce.example.yaml
│   └── ...
├── roles/             # Role definition examples
│   ├── aws.example.yml
│   ├── cloudflare.example.yml
│   ├── gcp.example.yml
│   └── ...
└── workflows/         # Workflow configuration examples
    └── ...
```

## Provider Examples

### Cloud Providers

- **[AWS](providers/aws.example.yaml)** - Amazon Web Services with IAM and SSO
- **[Azure](providers/azure.example.yaml)** - Microsoft Azure with RBAC
- **[GCP](providers/gcp.example.yaml)** - Google Cloud Platform with IAM
- **[Cloudflare](providers/cloudflare.example.yaml)** - Cloudflare with account roles and policies

### Authentication & Identity

- **[OAuth2](providers/oauth.example.yaml)** - Generic OAuth2 or OpenID Connect authentication
- **[Google OAuth2](providers/oauth2.google.example.yaml)** - Google authentication
- **[SAML](providers/saml.example.yaml)** - SAML 2.0 SSO
- **[Google Workspace](providers/gsuite.example.yaml)** - G Suite user management

### Development & Tools

- **[GitHub](providers/github.example.yaml)** - GitHub repository access
- **[Salesforce](providers/salesforce.example.yaml)** - Salesforce CRM access

### Communication

- **[Email](providers/email.example.yaml)** - SMTP email notifications

## Role Examples

Role configurations define what access users can request and under what conditions.

### Cloud Platform Roles

- **[AWS Roles](roles/aws.example.yml)** - EC2, S3, RDS access patterns
- **[GCP Roles](roles/gcp.example.yml)** - GCP project and resource access
- **[Cloudflare Roles](roles/cloudflare.example.yml)** - DNS, firewall, and zone management

### Key Concepts in Roles

- **Permissions**: What actions can be performed (e.g., `dns:edit`, `s3:*`)
- **Resources**: What resources can be accessed (e.g., `zone:example.com`, `aws:*`)
- **Scopes**: Who can request this role (users, groups)
- **Workflows**: What approval process is required
- **Providers**: Which provider configurations this role applies to

## Quick Start

### 1. Copy Examples to Config Directory

```bash
# Copy provider examples
cp examples/providers/cloudflare.example.yaml config/providers/cloudflare.yaml

# Copy role examples
cp examples/roles/cloudflare.example.yml config/roles/cloudflare.yml
```

### 2. Update with Your Credentials

```yaml
# config/providers/cloudflare.yaml
providers:
  cloudflare-prod:
    config:
      account_id: "YOUR_ACCOUNT_ID"
      api_token: "YOUR_API_TOKEN"
```

### 3. Customize Roles for Your Organization

```yaml
# config/roles/cloudflare.yml
roles:
  cloudflare-dns-editor:
    inherits:
      - DNS
    resources:
      allow:
        - zone:your-domain.com  # Change to your domain
    scopes:
      groups:
        - oidc:your-team  # Change to your team
```

## Security Best Practices

### Never Commit Secrets

```yaml
# ❌ BAD - Hardcoded credentials
config:
  api_token: "actual-token-here"

# ✅ GOOD - Environment variables
config:
  api_token: "${CLOUDFLARE_API_TOKEN}"
```

### Use Least Privilege

```yaml
# ❌ BAD - Overly broad access
resources:
  allow:
    - "*"

# ✅ GOOD - Specific resources
resources:
  allow:
    - zone:app.example.com
    - zone:api.example.com
```

### Require Approvals

```yaml
# ❌ BAD - Self-service for sensitive access
workflows:
  - self_service

# ✅ GOOD - Approval workflow
workflows:
  - manager_approval
```

## Example Use Cases

### Cloudflare Examples

#### 1. Emergency On-Call Access
```yaml
cloudflare-oncall:
  name: On-Call Engineer
  description: Emergency access for on-call rotation
  workflows:
    - oncall_auto_approve  # Auto-approve if on PagerDuty
  inherits:
    - DNS
    - Firewall Services
    - Cache Purge
  resources:
    allow:
      - zone:*  # All zones for incident response
```

#### 2. Developer Self-Service
```yaml
cloudflare-dev-zones:
  name: Development Zones
  description: Self-service access to dev/staging
  workflows:
    - self_service  # Instant access
  inherits:
    - DNS
    - Workers Scripts
  resources:
    allow:
      - zone:dev.example.com
      - zone:staging.example.com
```

#### 3. Security Team Monitoring
```yaml
cloudflare-security:
  name: Security Monitoring
  description: Read-only security monitoring
  workflows:
    - self_service
  permissions:
    allow:
      - Analytics
      - Logs
      - Firewall Services  # Read-only via permission groups
  resources:
    allow:
      - zone:*  # All zones
```

## Documentation

For detailed documentation, see:

- [Cloudflare Provider Documentation](../docs/configuration/providers/cloudflare/)
- [Cloudflare Quick Start Guide](../docs/configuration/providers/cloudflare/quickstart.md)
- [Access Control Models Comparison](../docs/configuration/providers/cloudflare/access-models.md)
- [Full Configuration Guide](../docs/configuration/)

## Support

For issues or questions:
- Check the [documentation](../docs/)
- Review provider-specific troubleshooting guides
- Open an issue on GitHub

