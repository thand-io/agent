---
layout: default
title: Services Configuration
parent: Configuration
nav_order: 2
description: "Detailed configuration reference for Thand Agent services"
---

# Services Configuration
{: .no_toc }

Complete reference for configuring Thand Agent's backend services including encryption, vault, scheduler, LLM, and Temporal.
{: .fs-6 .fw-300 }

## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

Thand Agent uses pluggable services for core functionality like encryption, secrets management, scheduling, and workflow orchestration. Each service supports multiple providers (AWS, GCP, Azure, Local) allowing you to choose the best fit for your infrastructure.

Services are configured under the `services` key in your configuration file:

```yaml
services:
  encryption:
    provider: local
    password: "your-secure-password"
    salt: "your-unique-salt"
  vault:
    provider: aws
  scheduler:
    provider: local
  temporal:
    host: localhost
    port: 7233
  llm:
    provider: gemini
    api_key: "your-api-key"
```

---

## Configuration Inheritance

All service configuration options can be defined in `environment.config`, and each service will automatically inherit these values as the base configuration. You can then override specific options for individual services as needed.

This is particularly useful when:
- Running on a cloud platform where most services should use the same credentials
- You want to mix providers (e.g., use GCP Secret Manager but local encryption)
- You need different configuration for specific services

### How It Works

1. **Base Configuration**: Values defined in `environment.config` are available to all services
2. **Service Override**: Values defined in `services.<service_name>` override the base configuration
3. **Provider Selection**: Each service independently selects its provider

### Example: Mixed Provider Configuration

In this example, the agent runs on GCP and uses GCP Secret Manager for the vault, but uses local encryption for performance:

```yaml
environment:
  platform: gcp
  config:
    # These values are inherited by ALL services as defaults
    project_id: my-gcp-project
    location: us-central1
    key_ring: thand-keyring
    key_name: thand-key

services:
  # Vault uses GCP - inherits project_id, location from environment.config
  vault:
    provider: gcp
    # No need to specify project_id - inherited from environment.config
  
  # Encryption uses local provider - ignores GCP config, uses its own
  encryption:
    provider: local
    password: "my-secure-password"
    salt: "my-unique-salt"
  
  # If you needed GCP encryption, it would inherit the key_ring and key_name
  # encryption:
  #   provider: gcp
  #   # project_id, location, key_ring, key_name all inherited
```

### Example: AWS with Service-Specific Overrides

```yaml
environment:
  platform: aws
  config:
    region: us-east-1
    profile: production
    kms_arn: "arn:aws:kms:us-east-1:123456789012:key/default-key"

services:
  # Uses all inherited values from environment.config
  encryption:
    provider: aws
  
  # Overrides region for vault (maybe secrets are in a different region)
  vault:
    provider: aws
    region: us-west-2
  
  # Uses local scheduler (doesn't need AWS config)
  scheduler:
    provider: local
```

### Configuration Priority

Configuration values are resolved in this order (later values override earlier ones):

1. Default values (hardcoded in the application)
2. `environment.config.*` values
3. `services.<service_name>.*` values

{: .note }
> The `provider` option must always be specified at the service level. It is not inherited from `environment.platform`.

---

## Encryption Service

The encryption service handles encrypting and decrypting sensitive data within the agent. This is used for session data, credentials, and other sensitive information.

### Provider Selection

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.encryption.provider` | string | `local` | Encryption provider: `aws`, `gcp`, `azure`, `local` |

### Local Provider

The local provider uses AES-256-GCM encryption with PBKDF2 key derivation. This is suitable for development and single-instance deployments.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.encryption.password` | string | `changeme` | Master password for key derivation |
| `services.encryption.salt` | string | `changeme` | Salt for key derivation |

{: .warning }
> **Security Warning**: Always change the default `password` and `salt` values in production. Using default values will trigger a warning in the logs.

**Example Configuration:**

```yaml
services:
  encryption:
    provider: local
    password: "my-secure-master-password"
    salt: "unique-environment-identifier"
```

**Environment Variables:**

```bash
THAND_SERVICES_ENCRYPTION_PROVIDER=local
THAND_SERVICES_ENCRYPTION_PASSWORD=my-secure-master-password
THAND_SERVICES_ENCRYPTION_SALT=unique-environment-identifier
```

### AWS Provider (KMS)

Uses AWS Key Management Service (KMS) for encryption operations.

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.encryption.kms_arn` | string | **Yes** | ARN of the KMS key to use |
| `services.encryption.region` | string | No | AWS region (defaults to environment config) |
| `services.encryption.profile` | string | No | AWS profile name |
| `services.encryption.access_key_id` | string | No | AWS access key ID |
| `services.encryption.secret_access_key` | string | No | AWS secret access key |

**Example Configuration:**

```yaml
services:
  encryption:
    provider: aws
    kms_arn: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
    region: us-east-1
```

**Environment Variables:**

```bash
THAND_SERVICES_ENCRYPTION_PROVIDER=aws
THAND_SERVICES_ENCRYPTION_KMS_ARN=arn:aws:kms:us-east-1:123456789012:key/...
AWS_REGION=us-east-1
AWS_PROFILE=my-profile
```

### GCP Provider (Cloud KMS)

Uses Google Cloud Key Management Service for encryption operations.

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.encryption.project_id` | string | **Yes** | GCP project ID |
| `services.encryption.location` | string | No | KMS location (default: `global`) |
| `services.encryption.key_ring` | string | **Yes** | Cloud KMS key ring name |
| `services.encryption.key_name` | string | **Yes** | Cloud KMS key name |

**Example Configuration:**

```yaml
services:
  encryption:
    provider: gcp
    project_id: my-gcp-project # This will be auto-detected on GCE
    location: us-central1
    key_ring: thand-keyring
    key_name: thand-encryption-key
```

**Environment Variables:**

```bash
THAND_SERVICES_ENCRYPTION_PROVIDER=gcp
THAND_SERVICES_ENCRYPTION_PROJECT_ID=my-gcp-project
THAND_SERVICES_ENCRYPTION_LOCATION=us-central1
THAND_SERVICES_ENCRYPTION_KEY_RING=thand-keyring
THAND_SERVICES_ENCRYPTION_KEY_NAME=thand-encryption-key
```

### Azure Provider (Key Vault)

Uses Azure Key Vault for encryption operations with RSA-OAEP algorithm.

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.encryption.vault_url` | string | **Yes** | Azure Key Vault URL |
| `services.encryption.key_name` | string | **Yes** | Key name in the vault |

**Example Configuration:**

```yaml
services:
  encryption:
    provider: azure
    vault_url: "https://my-keyvault.vault.azure.net"
    key_name: thand-encryption-key
```

**Environment Variables:**

```bash
THAND_SERVICES_ENCRYPTION_PROVIDER=azure
THAND_SERVICES_ENCRYPTION_VAULT_URL=https://my-keyvault.vault.azure.net
THAND_SERVICES_ENCRYPTION_KEY_NAME=thand-encryption-key
```

---

## Vault Service

The vault service handles secure storage and retrieval of secrets like API keys, credentials, and tokens.

### Provider Selection

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.vault.provider` | string | `local` | Vault provider: `aws`, `gcp`, `azure`, `hashicorp`, `local` |

### Local Provider

The local vault provider is a placeholder for development. It does not actually store secrets persistently.

{: .note }
> The local vault provider is not recommended for production use. Use a cloud provider or HashiCorp Vault for secure secret storage.

### AWS Provider (Secrets Manager)

Uses AWS Secrets Manager for secure secret storage.

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.vault.region` | string | No | AWS region (defaults to environment config) |
| `services.vault.profile` | string | No | AWS profile name |
| `services.vault.access_key_id` | string | No | AWS access key ID |
| `services.vault.secret_access_key` | string | No | AWS secret access key |

**Example Configuration:**

```yaml
services:
  vault:
    provider: aws
    region: us-east-1
```

**Environment Variables:**

```bash
THAND_SERVICES_VAULT_PROVIDER=aws
AWS_REGION=us-east-1
```

### GCP Provider (Secret Manager)

Uses Google Cloud Secret Manager for secure secret storage.

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.vault.project_id` | string | No | GCP project ID (auto-detected on GCE) |

**Example Configuration:**

```yaml
services:
  vault:
    provider: gcp
    project_id: my-gcp-project
```

**Environment Variables:**

```bash
THAND_SERVICES_VAULT_PROVIDER=gcp
THAND_SERVICES_VAULT_PROJECT_ID=my-gcp-project
```

### Azure Provider (Key Vault Secrets)

Uses Azure Key Vault for secure secret storage.

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.vault.vault_url` | string | **Yes** | Azure Key Vault URL |

**Example Configuration:**

```yaml
services:
  vault:
    provider: azure
    vault_url: "https://my-keyvault.vault.azure.net"
```

**Environment Variables:**

```bash
THAND_SERVICES_VAULT_PROVIDER=azure
THAND_SERVICES_VAULT_VAULT_URL=https://my-keyvault.vault.azure.net
```

### HashiCorp Vault Provider

Uses HashiCorp Vault for enterprise-grade secret management.

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `services.vault.vault_url` | string | **Yes** | - | HashiCorp Vault server URL |
| `services.vault.token` | string | No | - | Vault authentication token |
| `services.vault.mount_path` | string | No | `secret` | KV secrets engine mount path |
| `services.vault.secret_path` | string | No | `data` | Path within the mount (use `data` for KV v2) |
| `services.vault.timeout` | duration | No | - | Request timeout |

**Example Configuration:**

```yaml
services:
  vault:
    provider: hashicorp
    vault_url: "https://vault.example.com:8200"
    token: "hvs.your-vault-token"
    mount_path: secret
    secret_path: data
```

**Environment Variables:**

```bash
THAND_SERVICES_VAULT_PROVIDER=hashicorp
THAND_SERVICES_VAULT_VAULT_URL=https://vault.example.com:8200
VAULT_TOKEN=hvs.your-vault-token
```

{: .note }
> The HashiCorp Vault provider will also check the `VAULT_TOKEN` environment variable and `~/.vault-token` file for authentication.

---

## Scheduler Service

The scheduler service handles job scheduling for time-based operations like session expiration and cleanup tasks.

### Provider Selection

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.scheduler.provider` | string | `local` | Scheduler provider: `aws`, `gcp`, `azure`, `local` |

### Local Provider

Uses an in-memory scheduler based on gocron. Suitable for single-instance deployments.

**Example Configuration:**

```yaml
services:
  scheduler:
    provider: local
```

{: .note }
> The local scheduler runs in-process and does not persist scheduled jobs across restarts. For production deployments requiring persistence, use Temporal workflows instead.

### Cloud Providers (AWS, GCP, Azure)

{: .warning }
> Cloud scheduler providers (AWS EventBridge Scheduler, GCP Cloud Scheduler, Azure Logic Apps) are not yet implemented. Use the local scheduler or Temporal for production scheduling.

---

## Temporal Service

Temporal provides durable workflow orchestration for access request workflows, approvals, and complex multi-step operations.

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.temporal.host` | string | `localhost` | Temporal server hostname |
| `services.temporal.port` | integer | `7233` | Temporal server port |
| `services.temporal.namespace` | string | `default` | Temporal namespace |
| `services.temporal.disable_versioning` | boolean | `false` | Disable worker versioning for testing |

### Authentication Modes

Thand Agent supports three authentication modes for Temporal:

1. **API Key** - For Temporal Cloud
2. **mTLS Inline** - Certificates embedded in configuration
3. **mTLS File** - Certificates loaded from files
4. **mTLS Vault** - Certificates stored in vault (supports PEM, PKCS12)

---

### Local Development

For local development, you can run Temporal using Docker:

```bash
docker run -d --name temporal \
  -p 7233:7233 \
  temporalio/auto-setup:latest
```

**Configuration:**

```yaml
services:
  temporal:
    host: localhost
    port: 7233
    namespace: default
```

---

### API Key Authentication (Temporal Cloud)

For production deployments using Temporal Cloud:

**Configuration:**

```yaml
services:
  temporal:
    host: my-namespace.tmprl.cloud
    port: 7233
    namespace: my-namespace.my-account
    api_key: "your-temporal-cloud-api-key"
```

**Environment Variables:**

```bash
THAND_SERVICES_TEMPORAL_HOST=my-namespace.tmprl.cloud
THAND_SERVICES_TEMPORAL_PORT=7233
THAND_SERVICES_TEMPORAL_NAMESPACE=my-namespace.my-account
THAND_SERVICES_TEMPORAL_API_KEY=your-temporal-cloud-api-key
```

---

### mTLS Authentication

For self-hosted Temporal or Temporal Cloud with mTLS, you'll need client certificates. The agent supports three ways to provide these certificates.

#### Generating Test Certificates

You can generate test certificates using the `tcld` CLI tool:

```bash
# Install tcld (Temporal Cloud CLI)
# See: https://docs.temporal.io/cloud/tcld

# Generate a Certificate Authority (CA)
tcld gen ca --org "your-org" -d 365d --ca-cert ca.pem --ca-key ca.key

# Generate a client certificate signed by the CA
tcld gen leaf \
  --org "your-org" \
  -d 364d \
  --ca-cert ca.pem \
  --ca-key ca.key \
  --cert client.pem \
  --key client.key

# For inline configuration, combine cert and key
cat client.pem client.key > client-combined.pem

# For PKCS12 format (useful for Windows/.pfx)
# Unencrypted
openssl pkcs12 -export -out client.p12 \
  -inkey client.key -in client.pem -passout pass:

# Encrypted with password
openssl pkcs12 -export -out client-encrypted.p12 \
  -inkey client.key -in client.pem -passout pass:your-password
```

{: .note }
> For production use with Temporal Cloud, generate certificates using `tcld generate-certificates` and follow Temporal's security best practices.

---

#### Option 1: mTLS Inline (Embedded Certificates)

Embed certificate and key content directly in the configuration. Useful for environment variables or secret management systems.

**Configuration Options:**

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.temporal.mtls_cert` | string | **Yes** | Client certificate in PEM format |
| `services.temporal.mtls_key` | string | **Yes** | Private key in PEM format |

**Example Configuration:**

```yaml
services:
  temporal:
    host: temporal.example.com
    port: 7233
    namespace: production
    mtls_cert: |
      -----BEGIN CERTIFICATE-----
      MIICpDCCAYwCCQDU+pQ3ZUD30jANBgkqhkiG9w0BAQsFADAUMRIwEAYDVQQDDAls
      ...
      -----END CERTIFICATE-----
    mtls_key: |
      -----BEGIN RSA PRIVATE KEY-----
      MIIEpAIBAAKCAQEAu1SU1LfVLPHCozMxH2Mo4lgOEePzNm0tfn1iHD5teQPLzC5M
      ...
      -----END RSA PRIVATE KEY-----
```

**Environment Variables:**

```bash
THAND_SERVICES_TEMPORAL_HOST=temporal.example.com
THAND_SERVICES_TEMPORAL_MTLS_CERT="$(cat client.pem)"
THAND_SERVICES_TEMPORAL_MTLS_KEY="$(cat client.key)"
```

**Combined Certificate (cert + key in single value):**

```yaml
services:
  temporal:
    host: temporal.example.com
    port: 7233
    namespace: production
    mtls_cert: |
      -----BEGIN CERTIFICATE-----
      ...certificate content...
      -----END CERTIFICATE-----
      -----BEGIN RSA PRIVATE KEY-----
      ...key content...
      -----END RSA PRIVATE KEY-----
    mtls_key: ""  # Leave empty when using combined cert
```

---

#### Option 2: mTLS File (Certificate Files)

Load certificates from filesystem paths. Recommended for local development and when certificates are managed by configuration management tools.

**Configuration Options:**

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.temporal.mtls_cert_file` | string | **Yes** | Path to client certificate file (PEM) |
| `services.temporal.mtls_key_file` | string | **Yes** | Path to private key file (PEM) |

**Example Configuration:**

```yaml
services:
  temporal:
    host: temporal.example.com
    port: 7233
    namespace: production
    mtls_cert_file: /etc/thand/certs/temporal-client.pem
    mtls_key_file: /etc/thand/certs/temporal-client.key
```

**Environment Variables:**

```bash
THAND_SERVICES_TEMPORAL_HOST=temporal.example.com
THAND_SERVICES_TEMPORAL_MTLS_CERT_FILE=/etc/thand/certs/temporal-client.pem
THAND_SERVICES_TEMPORAL_MTLS_KEY_FILE=/etc/thand/certs/temporal-client.key
```

**Docker Volume Mount:**

```bash
docker run -d \
  -v /path/to/certs:/etc/thand/certs:ro \
  -e THAND_SERVICES_TEMPORAL_HOST=temporal.example.com \
  -e THAND_SERVICES_TEMPORAL_MTLS_CERT_FILE=/etc/thand/certs/client.pem \
  -e THAND_SERVICES_TEMPORAL_MTLS_KEY_FILE=/etc/thand/certs/client.key \
  thand-agent
```

---

#### Option 3: mTLS Vault (Certificate in Secret Storage)

Store certificates in your configured vault service. The agent supports multiple certificate formats with automatic detection.

**Configuration Options:**

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.temporal.mtls_vault.mtls_vault_name` | string | **Yes** | Vault secret key/name containing certificate |
| `services.temporal.mtls_vault.mtls_vault_type` | string | No | Certificate format: `pem`, `pkcs12`, `p12`, `pfx`, `der` (auto-detected if not specified) |
| `services.temporal.mtls_vault.mtls_vault_password` | string | No | Password for encrypted PKCS12 certificates |

**Supported Certificate Formats:**

- **PEM** - Text-based format (combined cert+key or separate)
- **PKCS12/P12/PFX** - Binary format (with or without password)
- **DER** - Binary format

**Example: PEM Format in Vault**

```yaml
services:
  temporal:
    host: temporal.example.com
    port: 7233
    namespace: production
    mtls_vault:
      mtls_vault_name: temporal-client-cert
      mtls_vault_type: pem  # Optional - will auto-detect
```

Store in vault (combined cert+key):
```bash
# AWS Secrets Manager
aws secretsmanager create-secret \
  --name temporal-client-cert \
  --secret-string "$(cat client-combined.pem)"

# GCP Secret Manager
cat client-combined.pem | gcloud secrets create temporal-client-cert \
  --data-file=-

# HashiCorp Vault
vault kv put secret/temporal-client-cert value=@client-combined.pem
```

**Example: Encrypted PKCS12 in Vault**

```yaml
services:
  temporal:
    host: temporal.example.com
    port: 7233
    namespace: production
    mtls_vault:
      mtls_vault_name: temporal-client-p12
      mtls_vault_type: pkcs12
      mtls_vault_password: "your-pkcs12-password"
```

Store in vault:
```bash
# Create encrypted PKCS12
openssl pkcs12 -export -out client.p12 \
  -inkey client.key -in client.pem \
  -passout pass:your-pkcs12-password

# AWS Secrets Manager (base64 encode binary)
aws secretsmanager create-secret \
  --name temporal-client-p12 \
  --secret-binary fileb://client.p12

# GCP Secret Manager
gcloud secrets create temporal-client-p12 --data-file=client.p12
```

**Example: Unencrypted PKCS12 (Auto-Detected)**

```yaml
services:
  temporal:
    host: temporal.example.com
    port: 7233
    namespace: production
    mtls_vault:
      mtls_vault_name: temporal-client-p12
      # No type or password needed - will auto-detect format
```

**Format Auto-Detection:**

The agent automatically detects certificate formats in this order:
1. PEM (looks for `-----BEGIN` markers)
2. PKCS12 (tries to parse as PKCS12)
3. DER (tries to parse as DER-encoded certificate)

If auto-detection fails, specify `mtls_vault_type` explicitly.

---

### Complete mTLS Examples

#### Development with File-Based Certificates

```yaml
# config.yaml
services:
  temporal:
    host: localhost
    port: 7233
    namespace: default
    mtls_cert_file: ./certs/dev-client.pem
    mtls_key_file: ./certs/dev-client.key
```

#### Production with Vault-Stored Certificates

```yaml
# config.yaml
environment:
  platform: aws
  config:
    region: us-east-1

services:
  vault:
    provider: aws
  
  temporal:
    host: production.tmprl.cloud
    port: 7233
    namespace: production.my-org
    mtls_vault:
      mtls_vault_name: prod/temporal-client-cert
      mtls_vault_type: pem
```

#### Kubernetes with Secrets

```yaml
# kubernetes-deployment.yaml
apiVersion: v1
kind: Secret
metadata:
  name: temporal-certs
type: Opaque
data:
  client.pem: <base64-encoded-cert>
  client.key: <base64-encoded-key>
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thand-agent
spec:
  template:
    spec:
      containers:
      - name: agent
        env:
        - name: THAND_SERVICES_TEMPORAL_HOST
          value: temporal.example.com
        - name: THAND_SERVICES_TEMPORAL_MTLS_CERT_FILE
          value: /etc/temporal/certs/client.pem
        - name: THAND_SERVICES_TEMPORAL_MTLS_KEY_FILE
          value: /etc/temporal/certs/client.key
        volumeMounts:
        - name: temporal-certs
          mountPath: /etc/temporal/certs
          readOnly: true
      volumes:
      - name: temporal-certs
        secret:
          secretName: temporal-certs
```

---

## LLM Service (Large Language Model)

The LLM service provides AI capabilities for intelligent access request processing and natural language understanding.

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `services.llm.provider` | string | **Yes** | LLM provider: `gemini`, `openai`, `anthropic` |
| `services.llm.api_key` | string | **Yes** | API key for the LLM provider |
| `services.llm.model` | string | No | Model name (provider-specific default) |
| `services.llm.base_url` | string | No | Custom API base URL |

### Google Gemini

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.llm.model` | string | `gemini-2.5-flash` | Gemini model to use |

**Example Configuration:**

```yaml
services:
  llm:
    provider: gemini
    api_key: "your-google-api-key"
    model: gemini-2.5-flash
```

**Environment Variables:**

```bash
THAND_SERVICES_LLM_PROVIDER=gemini
THAND_SERVICES_LLM_API_KEY=your-google-api-key
THAND_SERVICES_LLM_MODEL=gemini-2.5-flash
```

### OpenAI

{: .note }
> OpenAI provider integration is planned but not yet implemented.

**Example Configuration:**

```yaml
services:
  llm:
    provider: openai
    api_key: "sk-your-openai-api-key"
    model: gpt-4
    base_url: https://api.openai.com/v1
```

### Anthropic

{: .note }
> Anthropic Claude provider integration is planned but not yet implemented.

**Example Configuration:**

```yaml
services:
  llm:
    provider: anthropic
    api_key: "your-anthropic-api-key"
    model: claude-3-opus-20240229
```

---

## Complete Example Configuration

Here's a complete example showing all services configured:

### Local Development

```yaml
# config.yaml - Local Development
services:
  encryption:
    provider: local
    password: "dev-password-change-in-prod"
    salt: "dev-salt-change-in-prod"
  
  vault:
    provider: local
  
  scheduler:
    provider: local
  
  temporal:
    host: localhost
    port: 7233
    namespace: default
  
  llm:
    provider: gemini
    api_key: "your-gemini-api-key"
```

### AWS Production

```yaml
# config.yaml - AWS Production
environment:
  platform: aws
  config:
    region: us-east-1
    profile: production

services:
  encryption:
    provider: aws
    kms_arn: "arn:aws:kms:us-east-1:123456789012:key/..."
  
  vault:
    provider: aws
  
  scheduler:
    provider: local
  
  temporal:
    host: production.tmprl.cloud
    port: 7233
    namespace: production.my-org
    api_key: "${TEMPORAL_API_KEY}"
  
  llm:
    provider: gemini
    api_key: "${GEMINI_API_KEY}"
```

### GCP Production

```yaml
# config.yaml - GCP Production
environment:
  platform: gcp
  config:
    project_id: my-gcp-project
    location: us-central1

services:
  encryption:
    provider: gcp
    key_ring: thand-keyring
    key_name: thand-key
  
  vault:
    provider: gcp
  
  scheduler:
    provider: local
  
  temporal:
    host: production.tmprl.cloud
    port: 7233
    namespace: production.my-org
    api_key: "${TEMPORAL_API_KEY}"
  
  llm:
    provider: gemini
    api_key: "${GEMINI_API_KEY}"
```

### Azure Production

```yaml
# config.yaml - Azure Production
environment:
  platform: azure
  config:
    vault_url: "https://my-keyvault.vault.azure.net"

services:
  encryption:
    provider: azure
    vault_url: "https://my-keyvault.vault.azure.net"
    key_name: thand-encryption-key
  
  vault:
    provider: azure
    vault_url: "https://my-keyvault.vault.azure.net"
  
  scheduler:
    provider: local
  
  temporal:
    host: production.tmprl.cloud
    port: 7233
    namespace: production.my-org
    api_key: "${TEMPORAL_API_KEY}"
  
  llm:
    provider: gemini
    api_key: "${GEMINI_API_KEY}"
```

---

## Troubleshooting

### Encryption Service

**Problem:** Warning about default secrets  
**Solution:** Set custom `password` and `salt` values in your configuration.

```yaml
services:
  encryption:
    password: "your-secure-password"
    salt: "your-unique-salt"
```

**Problem:** AWS KMS encryption failing  
**Solution:** Verify the `kms_arn` is correct and your IAM role has `kms:Encrypt` and `kms:Decrypt` permissions.

### Vault Service

**Problem:** HashiCorp Vault connection failing  
**Solution:** Verify the `vault_url` is accessible and check your token authentication.

```bash
# Test vault connection
curl -H "X-Vault-Token: $VAULT_TOKEN" https://vault.example.com:8200/v1/sys/health
```

### Temporal Service

**Problem:** Cannot connect to Temporal  
**Solution:** Verify the host, port, and namespace are correct. For Temporal Cloud, ensure your API key is valid.

```bash
# Test Temporal connection
temporal operator namespace describe --address your-host:7233 --namespace your-namespace
```
