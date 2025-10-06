# Thand Agent CLI

The Thand Agent CLI provides interactive access to cloud infrastructure and SaaS applications through a just-in-time access model.

The CLI connects to a running instance of the Thand Agent, which hosts, manages and orchestrates access to these resources.

## Quick Start

```bash
# Login to your Thand server
agent login --login-server <server-url>

# Request access using AI (natural language)
agent request "I need admin access to AWS production for database maintenance"

# Or use specific access request
agent request access --resource aws-prod --role admin --duration 4h --reason "Database maintenance"
```

## Global Options

All commands support these global flags:

- `--config <path>` - Config file (default: `$HOME/.thand/config.yaml`)
- `--login-server <url>` - Override default login server URL (e.g., `http://localhost:8080`)
- `--verbose`, `-v` - Enable verbose output

## Commands

### Core Commands

#### `agent` (default)
Interactive access request wizard when no subcommand is specified.

**Usage:**
```bash
agent
```

When run without arguments, launches an interactive wizard to request access if a login server is configured.

#### `agent login`
Authenticate with the login server and establish a session.

**Usage:**
```bash
agent login
```

Opens a browser to authenticate with the configured login server and establishes a session for subsequent requests.

#### `agent request <reason>`
Request access using AI to determine the appropriate role and permissions.

**Usage:**
```bash
agent request "I need access to production database for troubleshooting"
agent request "Grant me read access to S3 bucket for data analysis"
```

The AI analyzes your natural language request and automatically determines the appropriate provider, role, and permissions needed.

#### `agent request access`
Request access to a specific resource with explicit parameters.

**Usage:**
```bash
agent request access --resource <resource> --role <role> --duration <duration> --reason <reason>
```

**Options:**
- `--resource`, `-r` - Resource to access (e.g., `snowflake-prod`, `aws-prod`)
- `--role`, `-o` - Role to assume (e.g., `analyst`, `admin`, `readonly`)
- `--duration`, `-d` - Duration of access (e.g., `1h`, `4h`, `8h`)
- `--reason`, `-e` - Reason for access request

**Example:**
```bash
agent request access --resource aws-prod --role admin --duration 4h --reason "Database maintenance required"
```

### Session Management

#### `agent sessions`
Interactive session manager for authentication sessions.

**Usage:**
```bash
agent sessions
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

#### `agent config`
Display current agent configuration.

**Usage:**
```bash
agent config
```

Shows current configuration including:
- Server host and port
- Login endpoint
- Logging level

#### `agent roles`
List available roles from the remote login server.

**Usage:**
```bash
agent roles [--provider <provider>]
```

**Options:**
- `--provider` - Filter roles by provider (e.g., `aws`, `gcp`, `azure`)

**Examples:**
```bash
agent roles                    # List all available roles
agent roles --provider aws     # List only AWS roles
```

#### `agent version`
Show version information.

**Usage:**
```bash
agent version
```

### Server Management

#### `agent server`
Run the agent server directly in the foreground.

**Usage:**
```bash
agent server
```

Starts the Thand Agent server that handles authentication and authorization requests. Useful for development or when running the agent as a standalone service.

#### `agent service`
System service management commands.

**Usage:**
```bash
agent service <subcommand>
```

**Subcommands:**
- `install` - Install the agent as a system service
- `start` - Start the agent service
- `stop` - Stop the agent service  
- `status` - Check service status
- `remove` - Uninstall the agent service

**Examples:**
```bash
agent service install    # Install as system service
agent service start      # Start the service
agent service status     # Check if service is running
agent service stop       # Stop the service
agent service remove     # Uninstall the service
```

### Maintenance

#### `agent update`
Update the agent to the latest version.

**Usage:**
```bash
agent update [--force] [--check]
```

**Options:**
- `--force`, `-f` - Force update without confirmation
- `--check`, `-c` - Only check for updates, don't install

Checks GitHub repository for the latest release and automatically updates the binary if a newer version is available.

**Examples:**
```bash
agent update           # Check and install updates with confirmation
agent update --check   # Only check for available updates
agent update --force   # Update without confirmation prompt
```

#### `agent wizard` (hidden)
Interactive wizard to configure access requests with validation.

**Usage:**
```bash
agent wizard
```

Launches a guided wizard that walks through creating an access request with proper validation using your configured workflows, roles, and providers.

## Configuration

The agent uses a YAML configuration file located at `$HOME/.thand/config.yaml` by default. You can specify a different config file using the `--config` flag.

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
agent login --login-server https://thand.company.com
agent request "I need read access to the customer database"

# Explicit access request
agent request access \
  --resource database-prod \
  --role readonly \
  --duration 2h \
  --reason "Customer support ticket investigation"

# Manage sessions
agent sessions

# Check available roles for AWS
agent roles --provider aws

# Install as system service
sudo agent service install
agent service start

# Update to latest version
agent update --check
```