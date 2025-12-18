---
layout: default
title: Configuration Reference
parent: Configuration
nav_order: 1
description: "Complete reference for all Thand Agent configuration options"
---

# Configuration Reference
{: .no_toc }

Complete reference for all Thand Agent configuration options and their default values.
{: .fs-6 .fw-300 }

## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

Thand Agent supports comprehensive configuration through YAML files, environment variables, and command-line flags. Configuration is loaded in the following order (later sources override earlier ones):

1. Default values (hardcoded)
2. Configuration file (`config.yaml`, `~/.config/thand/config.yaml`, etc.)
3. Environment variables (prefixed with `THAND_`)
4. Command line flags

---

## Environment Configuration

Core environment settings that define the runtime context and platform.

### Basic Environment Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `environment.name` | string | Automatic | Name of the environment or hostname. Will be automatically set if not provided |
| `environment.hostname` | string | Automatic | Hostname of the machine. Will be automatically set if not provided |
| `environment.platform` | string | Automatic | Platform type: `aws`, `gcp`, `azure`, `kubernetes`, `local`. Will be automatically set if not provided |
| `environment.os` | string | Automatic | Operating system: `windows`, `darwin`, `linux`. Will be automatically set if not provided |
| `environment.os_version` | string | Automatic | Operating system version. Will be automatically set if not provided |
| `environment.arch` | string | Automatic | System architecture: `amd64`, `arm64`. Will be automatically set if not provided |
| `environment.ephemeral` | boolean | `false` | Whether running in ephemeral environment. Will be automatically set if not provided |

### Local Environment Config

Platform-specific configuration settings:

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `environment.config.password` | string | `changeme` | Default encryption password |
| `environment.config.salt` | string | `changeme` | Default encryption salt |

#### AWS-Specific Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `environment.config.profile` | string | - | AWS profile to use |
| `environment.config.region` | string | - | AWS region |
| `environment.config.access_key_id` | string | - | AWS access key ID |
| `environment.config.secret_access_key` | string | - | AWS secret access key |
| `environment.config.kms_arn` | string | - | AWS KMS key ARN for encryption |
| `environment.config.imds_disable` | boolean | - | Disable AWS instance metadata service |

#### GCP-Specific Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `environment.config.project_id` | string | - | GCP project ID |
| `environment.config.location` | string | - | GCP location/region |
| `environment.config.key_ring` | string | - | Cloud KMS key ring name |
| `environment.config.key_name` | string | - | Cloud KMS key name |

#### Azure-Specific Config

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `environment.config.vault_url` | string | - | Azure Key Vault URL |

---

## Server Configuration

Settings for the Thand server when running in server mode.

### Basic Server Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `server.host` | string | `0.0.0.0` | Server bind address |
| `server.port` | integer | `5225` | Server listen port |

### Server Limits

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `server.limits.read_timeout` | duration | `30s` | HTTP read timeout |
| `server.limits.write_timeout` | duration | `30s` | HTTP write timeout |
| `server.limits.idle_timeout` | duration | `120s` | HTTP idle timeout |
| `server.limits.requests_per_minute` | integer | `100` | Rate limit for requests per minute |
| `server.limits.burst` | integer | `10` | Rate limit burst size |

### Metrics Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `server.metrics.enabled` | boolean | `true` | Enable Prometheus metrics endpoint |
| `server.metrics.path` | string | `/metrics` | Metrics endpoint path |
| `server.metrics.namespace` | string | `thand` | Metrics namespace prefix |

### Health Checks

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `server.health.enabled` | boolean | `true` | Enable health check endpoint |
| `server.health.path` | string | `/health` | Health check endpoint path |
| `server.ready.enabled` | boolean | `true` | Enable readiness check endpoint |
| `server.ready.path` | string | `/ready` | Readiness check endpoint path |

### CORS Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `server.security.cors.allowed_origins` | []string | `["https://thand.io", "https://*.thand.io"]` | Allowed CORS origins |
| `server.security.cors.allowed_methods` | []string | `["GET", "POST", "PUT", "DELETE", "OPTIONS"]` | Allowed HTTP methods |
| `server.security.cors.allowed_headers` | []string | `["Authorization", "Content-Type", "X-Requested-With"]` | Allowed headers |
| `server.security.cors.expose_headers` | []string | - | Exposed headers |
| `server.security.cors.allow_credentials` | boolean | `false` | Allow credentials |
| `server.security.cors.max_age` | integer | `86400` | CORS preflight cache duration (seconds) |

---

## Login Server Configuration

Settings for connecting to the Thand login server.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `login.endpoint` | string | `https://auth.thand.io/` | Login server endpoint URL |
| `login.base` | string | `/` | Base path for login endpoints |
| `login.api_key` | string | - | API key for login server authentication |

---

## Thand Cloud Configuration

Settings for connecting to Thand Cloud services (thand.io).

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `thand.endpoint` | string | `https://app.thand.io/` | Thand Cloud endpoint URL |
| `thand.base` | string | `/` | Base path for Thand Cloud endpoints |
| `thand.api_key` | string | - | API key for authenticating with Thand Cloud |
| `thand.sync` | boolean | `true` | Enable synchronization with Thand Cloud |

---

## API Configuration

Settings for the REST API.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `api.version` | string | `/api/v1` | API version |
| `api.rate_limit.requests_per_minute` | integer | - | API-specific rate limit |
| `api.rate_limit.burst` | integer | - | API-specific burst limit |

---

## Logging Configuration

Control logging behavior and output format.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `logging.level` | string | `info` | Log level: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` |
| `logging.format` | string | `json` | Log format: `json`, `text` |
| `logging.output` | string | `stdout` | Log output destination |
| `logging.open_telemetry.enabled` | boolean | `false` | Enable OpenTelemetry logging |
| `logging.open_telemetry.endpoint` | [Endpoint](#endpoint-configuration) | - | OpenTelemetry endpoint configuration |

---

## Services Configuration

External service integrations and configurations.

### Encryption Service

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.encryption.provider` | string | `local` | Encryption provider: `aws`, `gcp`, `azure`, `local` |
| `services.encryption.config.*` | map | - | Provider-specific encryption config |

### Vault Service

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.vault.provider` | string | `local` | Vault provider: `aws`, `gcp`, `azure`, `local` |
| `services.vault.config.*` | map | - | Provider-specific vault config |

### Scheduler Service (Temporal)

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.scheduler.provider` | string | `local` | Scheduler provider |
| `services.scheduler.config.*` | map | - | Provider-specific scheduler config |

### Temporal Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.temporal.host` | string | `localhost` | Temporal server host |
| `services.temporal.port` | integer | `7233` | Temporal server port |
| `services.temporal.namespace` | string | `default` | Temporal namespace |
| `services.temporal.api_key` | string | - | Temporal Cloud API key |
| `services.temporal.mtls_cert` | string | - | mTLS certificate content |
| `services.temporal.mtls_cert_path` | string | - | Path to mTLS certificate file |
| `services.temporal.disable_versioning` | boolean | `false` | Disable worker versioning |

### Large Language Model (LLM) Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `services.llm.provider` | string | - | LLM provider: `openai`, `gemini`, `anthropic` |
| `services.llm.api_key` | string | - | API key for LLM provider |
| `services.llm.base_url` | string | - | Custom base URL for LLM API |
| `services.llm.model` | string | - | Model name (e.g., `gpt-4`, `gemini-pro`) |

---

## Roles Configuration

Define and load role definitions.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `roles.path` | string | `./examples/roles` | Local directory for role files |
| `roles.url` | [Endpoint](#endpoint-configuration) | - | Remote URL endpoint for roles |
| `roles.vault` | string | - | Vault secret path for roles |
| `roles.*` | map | - | Inline role definitions |

### External Role Loading

Roles can be loaded from external sources:

```yaml
# Load from local directory
roles:
  path: "./config/roles"

# Load from remote URL
roles:
  url:
    uri: "https://example.com/roles.yaml"
    method: "GET"
    headers:
      Authorization: "Bearer token"

# Load from vault
roles:
  vault: "secret/roles"
```

---

## Workflows Configuration

Define and load workflow definitions.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `workflows.path` | string | `./examples/workflows` | Local directory for workflow files |
| `workflows.url` | [Endpoint](#endpoint-configuration) | - | Remote URL endpoint for workflows |
| `workflows.vault` | string | - | Vault secret path for workflows |
| `workflows.plugins.path` | string | - | Local directory for workflow plugins |
| `workflows.plugins.url` | string | - | Remote URL for workflow plugins |
| `workflows.*` | map | - | Inline workflow definitions |

---

## Providers Configuration

Define and load provider configurations.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `providers.path` | string | `./examples/providers` | Local directory for provider files |
| `providers.url` | [Endpoint](#endpoint-configuration) | - | Remote URL endpoint for providers |
| `providers.vault` | string | - | Vault secret path for providers |
| `providers.plugins.path` | string | - | Local directory for provider plugins |
| `providers.plugins.url` | string | - | Remote URL for provider plugins |
| `providers.*` | map | - | Inline provider definitions |

---

## Security Configuration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `secret` | string | `changeme` | Secret key for signing cookies and tokens |

---

## Endpoint Configuration

The `Endpoint` object is used to configure remote connections, such as fetching configuration files or sending telemetry data. It follows the [Serverless Workflow Specification](https://github.com/serverlessworkflow/specification/blob/main/dsl-reference.md#endpoint) for endpoint definitions.

### Structure

| Field | Type | Description |
|-------|------|-------------|
| `uri` | string | The URI of the endpoint |
| `authentication` | object | Authentication configuration (optional) |

### Authentication

The `authentication` object supports various authentication methods, including Basic Auth and Bearer Token. For more details on how to configure authentication, refer to the [HTTP Authentication](https://github.com/serverlessworkflow/specification/blob/main/dsl-reference.md#authentication).

#### Basic Authentication

```yaml
endpoint:
  uri: "https://api.example.com/v1/resource"
  authentication:
    basic:
      username: "myuser"
      password: "mypassword"
```

#### Bearer Token Authentication

```yaml
endpoint:
  uri: "https://api.example.com/v1/resource"
  authentication:
    bearer:
      token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

---

## Environment Variables

All configuration options can be set via environment variables using the `THAND_` prefix and converting nested keys to uppercase with underscores:

### Examples

```bash
# Environment settings
export THAND_ENVIRONMENT_PLATFORM="aws"
export THAND_ENVIRONMENT_CONFIG_REGION="us-west-2"

# Server settings
export THAND_SERVER_HOST="0.0.0.0"
export THAND_SERVER_PORT="8080"

# Logging
export THAND_LOGGING_LEVEL="debug"
export THAND_LOGGING_FORMAT="json"

# Services
export THAND_SERVICES_LLM_PROVIDER="openai"
export THAND_SERVICES_LLM_API_KEY="sk-..."
export THAND_SERVICES_LLM_MODEL="gpt-4"

# Temporal
export THAND_SERVICES_TEMPORAL_HOST="temporal.example.com"
export THAND_SERVICES_TEMPORAL_NAMESPACE="production"

# External sources
export THAND_ROLES_VAULT="secret/roles"
export THAND_WORKFLOWS_VAULT="secret/workflows"
export THAND_PROVIDERS_VAULT="secret/providers"
```

---

## Configuration File Examples

### Minimal Configuration

```yaml
environment:
  name: "production"
  platform: "aws"

logging:
  level: "info"
  format: "json"

server:
  host: "0.0.0.0"
  port: 5225
```

### Complete Configuration

```yaml
# Environment configuration
environment:
  name: "production-agent"
  platform: "aws"
  config:
    region: "us-west-2"
    timeout: "10s"

# Server configuration
server:
  host: "0.0.0.0"
  port: 5225
  limits:
    read_timeout: "30s"
    write_timeout: "30s"
    requests_per_minute: 200
  metrics:
    enabled: true
    namespace: "thand-prod"
  security:
    cors:
      allowed_origins: ["https://app.example.com"]

# Login server
login:
  endpoint: "https://auth.example.com"
  api_key: "${LOGIN_API_KEY}"

# Services
services:
  llm:
    provider: "openai"
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4"
  
  temporal:
    host: "temporal.example.com"
    port: 7233
    namespace: "production"
    api_key: "${TEMPORAL_API_KEY}"

  vault:
    provider: "aws"
    config:
      region: "us-west-2"

# Logging
logging:
  level: "info"
  format: "json"

# External sources
roles:
  vault: "secret/production/roles"

workflows:
  vault: "secret/production/workflows"

providers:
  vault: "secret/production/providers"

# Security
secret: "${THAND_SECRET}"
```

---

## Validation

Configuration validation occurs at startup. Common validation rules include:

- Required fields must be present
- Enum values must match allowed options
- Duration fields must be valid Go duration strings
- URL fields must be valid URLs
- Port numbers must be in valid range (1-65535)

Invalid configurations will cause the agent to fail startup with descriptive error messages.
