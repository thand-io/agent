---
layout: default
title: CLI Reference
parent: Configuration
nav_order: 2
description: "Complete reference for all Thand Agent CLI commands and options"
---

# CLI Reference
{: .no_toc }

Complete reference for all Thand Agent command-line interface options and subcommands.
{: .fs-6 .fw-300 }

## Table of Contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Overview

The Thand Agent CLI provides multiple commands for different use cases:
- **Client mode**: Request just-in-time access to resources
- **Server mode**: Run the agent as a service for handling requests
- **Service management**: Install and manage the agent as a system service

---

## Global Flags

These flags are available for all commands:

| Flag | Short | Type | Description |
|------|-------|------|-------------|
| `--config` | - | string | Config file (default is `$HOME/.config/thand/config.yaml`) |
| `--verbose` | `-v` | boolean | Enable verbose output for debugging |
| `--login-server` | - | string | Override the default login server URL |
| `--help` | `-h` | boolean | Show help for any command |

### Examples

```bash
# Use custom config file
thand --config /path/to/config.yaml roles

# Override login server
thand --login-server https://auth.example.com login

# Enable verbose logging
thand --verbose server

```

---

## Main Command

### `thand`

The main command runs an interactive request wizard when called without subcommands.

```bash
thand [reason for access]
```

**Examples:**
```bash
# Interactive wizard
thand

# Direct request with reason
thand "Need access to production database for debugging"
```

**Behavior:**
- If no login server is configured, prompts for setup
- If configured, launches interactive access request wizard
- Collects provider, role, duration, and reason for access
- Submits elevation request automatically

---

## Authentication Commands

### `login`

Authenticate with the login server and establish a session.

```bash
thand login
```

**What it does:**
- Opens browser to login server authentication page
- Establishes local callback server to receive auth tokens
- Stores session for future CLI operations
- Validates successful authentication

**Examples:**
```bash
# Login to configured server
thand login

# Login with custom server
thand --login-server https://auth.example.com login
```

### `sessions`

Interactive session management interface.

```bash
thand sessions
```

**Features:**
- List all active authentication sessions
- Create new provider-specific sessions
- Remove expired or unwanted sessions
- Refresh existing sessions
- Interactive menu-driven interface

### `sessions register`

Register a session from an encoded token.

```bash
thand sessions register [flags]
```

**Description:**

This command allows you to import a session that was provided by another source by pasting an encoded session token. This is useful when you need to use a session token that was generated externally or shared with you.

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--provider` | string | Provider name (e.g., `thand`) |

**Examples:**
```bash
# Register a session with provider flag
thand sessions register --provider thand

# Register a session (will prompt for provider)
thand sessions register
```

**How it works:**
1. Prompts for provider name if not specified via `--provider` flag
2. Prompts for the encoded session token to paste
3. Decodes and validates the session token
4. Warns if the session has expired (with option to continue)
5. Stores the session in the local session manager

**Output includes:**
- Login server the session is registered to
- Provider name
- Session expiry time
- Time remaining until expiry (if valid)

### `sessions list`

List all active authentication sessions.

```bash
thand sessions list
```

**Description:**

Displays all current authentication sessions with their status, including provider name, session status (active/expired), expiry time, and version information.

**Example output:**
```
Current Sessions

Provider: aws
  ACTIVE
  Expires: 2024-10-27 15:30:00 (2 hours, 30 minutes)
  Version: 1

Provider: gcp
  EXPIRED
  Expired: 2024-10-27 10:00:00
  Version: 2
```

### `sessions create`

Create a new authentication session.

```bash
thand sessions create
```

**Description:**

Guides you through creating a new authentication session for a provider. Displays available providers, opens the authentication flow in your browser, and waits for completion.

**How it works:**
1. Displays list of available providers from configuration
2. Prompts to select a provider
3. Checks if an active session already exists (prompts to replace if so)
4. Opens browser to complete authentication
5. Waits for session creation (Ctrl+C to cancel)
6. Confirms successful session creation

### `sessions remove`

Remove an existing authentication session.

```bash
thand sessions remove
```

**Description:**

Displays a list of active sessions and prompts for selection. Asks for confirmation before removing the selected session.

**How it works:**
1. Loads and displays all current sessions
2. Prompts to select a session to remove
3. Asks for confirmation
4. Removes the session from the local session manager

### `sessions refresh`

Refresh or re-authenticate an existing session.

```bash
thand sessions refresh
```

**Description:**

Initiates the authentication flow again for the selected provider to obtain a new session token with extended expiry. Useful for sessions that are about to expire or have expired.

**How it works:**
1. Loads and displays all current sessions
2. Prompts to select a session to refresh
3. Opens browser to complete re-authentication
4. Waits for session refresh (Ctrl+C to cancel)
5. Confirms successful session refresh with new expiry time

---

## Access Request Commands

### `request`

Make AI-powered access requests using natural language.

```bash
thand request [reason]
```

**Examples:**
```bash
thand request "Need to debug production issue in AWS"
thand request "Quarterly analysis requires Snowflake access"
thand request "Emergency database maintenance required"
```

**How it works:**
- Sends natural language reason to login server LLM
- AI determines appropriate role and resources
- Automatically submits elevation request
- Returns request status and next steps

### `request access`

Make structured access requests with specific parameters.

```bash
thand request access --provider <provider> --role <role> --duration <duration> --reason <reason>
```

**Required Flags:**

| Flag | Short | Description | Example |
|------|-------|-------------|---------|
| `--provider` | `-p` | Provider to access (alias for resource) | `aws-prod`, `snowflake-dev` |
| `--role` | `-o` | Role to assume | `admin`, `analyst`, `readonly` |
| `--duration` | `-d` | Access duration | `1h`, `4h`, `8h` |
| `--reason` | `-e` | Justification for access | `Emergency maintenance` |

**Examples:**
```bash
# Request AWS admin access
thand request access \
  --provider aws-prod \
  --role admin \
  --duration 2h \
  --reason "Emergency security patch deployment"

# Request read-only Snowflake access
thand request access \
  -r snowflake-prod \
  -o analyst \
  -d 4h \
  -e "Monthly report generation"
```

---

## Information Commands

### `roles`

List available roles and their descriptions.

```bash
thand roles [flags]
```

**Flags:**

| Flag | Type | Description |
|------|------|-------------|
| `--provider` | string | Filter roles by provider |

**Examples:**
```bash
# List all roles
thand roles

# List AWS-specific roles
thand roles --provider aws

# List roles for multiple providers
thand roles --provider gcp
```

**Output Format:**
```
Available roles:

NAME                 PROVIDERS       DESCRIPTION
----                 ---------       -----------
aws-admin           aws             Full administrative access to AWS
aws-readonly        aws             Read-only access to AWS resources
snowflake-analyst   snowflake       Data analysis access to Snowflake
gcp-developer       gcp             Development access to GCP

Total: 4 roles
```

### `config`

Display current agent configuration.

```bash
thand config
```

**Shows:**
- Server host and port settings
- Login server endpoint
- Current logging level
- Other key configuration values

### `version`

Display version information and check for updates.

```bash
thand version
```

**Output includes:**
- Current version number
- Git commit hash (if available)
- Update availability status
- Instructions for updating

---

## Server Commands

### `server`

Run the agent server in the foreground.

```bash
thand server
```

**What it does:**
- Starts HTTP server on configured host:port
- Loads roles, workflows, and providers
- Handles authentication callbacks
- Provides API endpoints for elevation requests
- Runs until interrupted (Ctrl+C)

**Output includes:**
- Environment information
- Server startup status
- Request handling logs

---

## Service Management Commands

The service commands manage the Thand Agent as a system service.

### `service install`

Install the agent as a system service.

```bash
thand service install
```

**Requirements:**
- Administrative/root privileges on most systems
- Service will start automatically on boot

**Platform-specific instructions:**
```bash
# Linux/macOS
sudo thand service install

# Windows (run as Administrator)
thand service install
```

### `service start`

Start the agent system service.

```bash
thand service start
```

### `service stop`

Stop the agent system service.

```bash
thand service stop
```

### `service status`

Check the agent service status.

```bash
thand service status
```

**Output:**
- 🟢 Running: Service is active
- Stopped: Service is not running
- 🟡 Unknown: Service state unclear

### `service remove`

Uninstall the agent system service.

```bash
thand service remove
```

**What it does:**
- Stops the service if running
- Removes service from system startup
- Cleans up service files

---

## Update Commands

### `update`

Update the agent to the latest version.

```bash
thand update [flags]
```

**Flags:**

| Flag | Short | Description |
|------|-------|-------------|
| `--force` | `-f` | Update without confirmation prompt |
| `--check` | `-c` | Check for updates without installing |

**Examples:**
```bash
# Interactive update
thand update

# Force update without prompts
thand update --force

# Check for updates only
thand update --check
```

**Update Process:**
1. Checks GitHub for latest release
2. Shows release notes and version info
3. Prompts for confirmation (unless `--force`)
4. Downloads and installs new version
5. Requires agent restart to use new version

---

## Environment Variables

Configuration options can be set via environment variables with the `THAND_` prefix. However, CLI-specific flags like `--verbose`, `--config`, `--login-server` are **only available as command-line flags** and do not have corresponding environment variables.

The following environment variables are available for configuration settings:

```bash
# Environment settings
export THAND_ENVIRONMENT_PLATFORM="aws"
export THAND_ENVIRONMENT_CONFIG_API_KEY="your-api-key"
export THAND_ENVIRONMENT_CONFIG_TIMEOUT="10s"

# Cloud provider settings (AWS)
export THAND_ENVIRONMENT_CONFIG_PROFILE="default"
export THAND_ENVIRONMENT_CONFIG_REGION="us-west-2"
export THAND_ENVIRONMENT_CONFIG_ACCESS_KEY_ID="your-access-key"
export THAND_ENVIRONMENT_CONFIG_SECRET_ACCESS_KEY="your-secret-key"
export THAND_ENVIRONMENT_CONFIG_KMS_ARN="arn:aws:kms:..."
export THAND_ENVIRONMENT_CONFIG_IMDS_DISABLE="true"

# Cloud provider settings (GCP)
export THAND_ENVIRONMENT_CONFIG_PROJECT_ID="my-project"
export THAND_ENVIRONMENT_CONFIG_LOCATION="us-central1"
export THAND_ENVIRONMENT_CONFIG_KEY_RING="my-key-ring"
export THAND_ENVIRONMENT_CONFIG_KEY_NAME="my-key"

# Cloud provider settings (Azure)
export THAND_ENVIRONMENT_CONFIG_VAULT_URL="https://vault.vault.azure.net/"

# Vault settings
export THAND_ENVIRONMENT_CONFIG_SECRET_PATH="secret/path"
export THAND_ENVIRONMENT_CONFIG_MOUNT_PATH="secret"

# Logging
export THAND_LOGGING_LEVEL="debug"
export THAND_LOGGING_FORMAT="json"
export THAND_LOGGING_OUTPUT="stdout"

# Services
export THAND_SERVICES_LLM_PROVIDER="openai"
export THAND_SERVICES_LLM_API_KEY="sk-..."
export THAND_SERVICES_LLM_BASE_URL="https://api.openai.com/v1"
export THAND_SERVICES_LLM_MODEL="gpt-4"

# Temporal
export THAND_SERVICES_TEMPORAL_HOST="temporal.example.com"
export THAND_SERVICES_TEMPORAL_PORT="7233"
export THAND_SERVICES_TEMPORAL_NAMESPACE="production"
export THAND_SERVICES_TEMPORAL_MTLS_PEM="-----BEGIN CERTIFICATE-----..."
export THAND_SERVICES_TEMPORAL_API_KEY="your-temporal-api-key"

# External sources
export THAND_ROLES_VAULT="secret/roles"
export THAND_WORKFLOWS_VAULT="secret/workflows"
export THAND_PROVIDERS_VAULT="secret/providers"
```

**Note**: CLI flags like `--verbose`, `--config`, `--login-server`, and `--api-key` must be specified on the command line and cannot be set via environment variables.

---

## Configuration Integration

The CLI integrates with the configuration system:

### Precedence Order
1. Command-line flags (highest priority)
2. Environment variables
3. Configuration file
4. Default values (lowest priority)

### Example Workflow
```bash
# 1. Configure login server
cat > ~/.config/thand/config.yaml << EOF
login:
  endpoint: "https://auth.company.com"
EOF

# 2. Login and authenticate
thand login

# 3. View available access options
thand roles

# 4. Request access
thand request "Need production access for incident response"
```

---

## Error Handling

### Common Error Messages

**"No login server configured"**
- Solution: Configure login server in config file or use `--login-server` flag

**"Authentication required but login was declined"**
- Solution: Run `thand login` to authenticate

**"Role not found"**
- Solution: Check available roles with `thand roles`

**"Failed to install service"**
- Solution: Run with elevated privileges (`sudo` or "Run as Administrator")

### Debug Mode

Enable verbose output for troubleshooting:

```bash
thand --verbose [command]
```

This provides detailed logging for:
- Configuration loading
- API requests/responses
- Authentication flows
- Error diagnostics

---

## Interactive Features

### Request Wizard

The main `agent` command provides an interactive wizard:

1. **Provider Selection**: Choose from configured providers
2. **Role Selection**: Pick appropriate role for selected provider
3. **Duration**: Select access duration (1h, 2h, 4h, 8h, custom)
4. **Reason**: Enter justification for access
5. **Summary**: Review and confirm request

### Session Manager

The `thand sessions` command provides interactive session management:

- Navigate with arrow keys
- Select actions from menu
- View detailed session information
- Manage multiple provider sessions

---

## Integration Examples

### CI/CD Pipeline
```bash
# Automated access request in pipeline
thand request access \
  --provider aws-prod \
  --role deployer \
  --duration 1h \
  --reason "Automated deployment pipeline"
```

### Emergency Access
```bash
# Quick emergency access
thand request "Production outage - need immediate admin access"
```

### Scheduled Maintenance
```bash
# Planned maintenance window
thand request access \
  --provider all-systems \
  --role maintenance \
  --duration 4h \
  --reason "Scheduled maintenance window MW-2024-10-27"
```
