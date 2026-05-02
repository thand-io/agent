# Thand Agent CLI

The Thand Agent CLI provides interactive access to cloud infrastructure and SaaS applications through a just-in-time access model.

The CLI connects to a running instance of the Thand Agent, which hosts, manages and orchestrates access to these resources.

## Quick Start

```bash
# Login to your Thand server
thand login --login-server <server-url>

# Request access using AI (natural language)
thand request "I need admin access to AWS production for database maintenance"

# Or use specific access request
thand request access --provider aws-prod --role admin --duration 4h --reason "Database maintenance"
```

## Global Options

All commands support these global flags:

- `--config <path>` - Config file (default: `$HOME/.config/thand/config.yaml`)
- `--login-server <url>` - Override default login server URL (e.g., `http://localhost:8080`)
- `--verbose`, `-v` - Enable verbose output

## Commands

### Core Commands

#### `thand` (default)
Interactive access request wizard when no subcommand is specified.

**Usage:**
```bash
thand
```

When run without arguments, launches an interactive wizard to request access if a login server is configured.

#### `thand login`
Authenticate with the login server and establish a session.

**Usage:**
```bash
thand login
```

Opens a browser to authenticate with the configured login server and establishes a session for subsequent requests.

#### `thand request <reason>`
Request access using AI to determine the appropriate role and permissions.

**Usage:**
```bash
thand request "I need access to production database for troubleshooting"
thand request "Grant me read access to S3 bucket for data analysis"
```

The AI analyzes your natural language request and automatically determines the appropriate provider, role, and permissions needed.

#### `thand request access`
Request access to a specific provider with explicit parameters.

**Usage:**
```bash
thand request access --provider <provider> --role <role> --duration <duration> --reason <reason>
```

**Options:**
- `--provider`, `-p` - Provider to access (alias for resource)
- `--role`, `-o` - Role to assume (e.g., `analyst`, `admin`, `readonly`)
- `--duration`, `-d` - Duration of access (e.g., `1h`, `4h`, `8h`)
- `--reason`, `-e` - Reason for access request

**Example:**
```bash
thand request access --provider aws-prod --role admin --duration 4h --reason "Database maintenance required"
```

### Session Management

#### `thand sessions`
Interactive session manager for authentication sessions.

**Usage:**
```bash
thand sessions
```

Launches an interactive terminal interface that allows you to:

1. **List all sessions** - View current authentication sessions and expiration status
2. **Create new sessions** - Authenticate with providers and create new sessions 
3. **Remove sessions** - Delete existing authentication sessions
4. **Refresh/Re-auth sessions** - Extend or renew existing sessions

**Features:**
- Visual status indicators for active vs expired sessions
- Provider selection from configured providers
- Automatic session detection during authentication flows
- Session validation with expiration times and remaining duration
- Safe removal with confirmation prompts

### Configuration and Information

#### `thand config`
Display current agent configuration.

**Usage:**
```bash
thand config
```

Shows current configuration including:
- Server host and port
- Login endpoint
- Logging level

#### `thand config device-id`
Print the canonical device ID for the current machine.

**Usage:**
```bash
thand config device-id
```

This prints the effective `device_id` only, with no extra label text, so it can be copied directly into device configuration.

#### `thand roles`
List available roles from the remote login server.

**Usage:**
```bash
thand roles [--provider <provider>]
```

**Options:**
- `--provider` - Filter roles by provider (e.g., `aws`, `gcp`, `azure`)

**Examples:**
```bash
thand roles                    # List all available roles
thand roles --provider aws     # List only AWS roles
```

#### `thand version`
Show version information.

**Usage:**
```bash
thand version
```

### Server Management

#### `thand server`
Run the agent server directly in the foreground.

**Usage:**
```bash
thand server
```

Starts the Thand Agent server that handles authentication and authorization requests. Useful for development or when running the agent as a standalone service.

#### `thand service`
System service management commands.

**Usage:**
```bash
thand service <subcommand>
```

**Subcommands:**
- `install` - Install the agent as a system service
- `start` - Start the agent service
- `stop` - Stop the agent service  
- `status` - Check service status
- `remove` - Uninstall the agent service

**Examples:**
```bash
thand service install    # Install as system service
thand service start      # Start the service
thand service status     # Check if service is running
thand service stop       # Stop the service
thand service remove     # Uninstall the service
```

### Maintenance

#### `thand update`
Update the agent to the latest version.

**Usage:**
```bash
thand update [--force] [--check]
```

**Options:**
- `--force`, `-f` - Force update without confirmation
- `--check`, `-c` - Only check for updates, don't install

Checks GitHub repository for the latest release and automatically updates the binary if a newer version is available.

**Examples:**
```bash
thand update           # Check and install updates with confirmation
thand update --check   # Only check for available updates
thand update --force   # Update without confirmation prompt
```

#### `thand wizard` (hidden)
Interactive wizard to configure access requests with validation.

**Usage:**
```bash
thand wizard
```

Launches a guided wizard that walks through creating an access request with proper validation using your configured workflows, roles, and providers.

## Configuration

The agent uses a YAML configuration file located at `$HOME/.config/thand/config.yaml` by default. You can specify a different config file using the `--config` flag.

**Example configuration:**
```yaml
login:
  endpoint: https://your-login-server.com

server:
  host: localhost
  port: 8080

logging:
  level: info
```

## Exit Codes

- `0` - Success
- `1` - General error (authentication failed, request failed, etc.)

## Examples

```bash
# Basic workflow
thand login --login-server https://thand.company.com
thand request "I need read access to the customer database"

# Explicit access request
thand request access \
  --provider database-prod \
  --role readonly \
  --duration 2h \
  --reason "Customer support ticket investigation"

# Manage sessions
thand sessions

# Check available roles for AWS
thand roles --provider aws

# Install as system service
sudo thand service install
thand service start

# Update to latest version
thand update --check
```
