# Thand Agent [![Go Report Card](https://goreportcard.com/badge/github.com/thand-io/agent)](https://goreportcard.com/report/github.com/thand-io/agent) [![Build and Release](https://github.com/thand-io/agent/actions/workflows/test-and-build.yml/badge.svg)](https://github.com/thand-io/agent/actions/workflows/test-and-build.yml)

Open-source agent for privilege access management (PAM) and just-in-time access (JIT) to cloud infrastructure, SaaS applications and local systems. Thand uses [temporal.io](https://temporal.io/) to guarantee robust deterministic workflow execution and guarantees execution and revocation of permissions. Run it locally or on-prem to grant short-lived access to any programmatically accessible system or resource. Connect to Thand Cloud for enterprise features.

## 🎯 What is Thand?

Thand eliminates standing access to critical infrastructure and SaaS apps. Instead of permanent admin rights, users request access when needed, for only as long as needed. The Thand server is extensible, customisable and easy to deploy as a standalone service.

**The Security Crisis**:

- **Static credentials get leaked**: API keys in repos, AWS keys in logs, service account keys shared in Slack
- **Over-privileged users**: 90% of permissions are unused, but remain active attack vectors. Broad workflows and roles lead to excessive permissions. Review and revocation is often a time consuming, manual process.
- **Automatic grants**: Users are often granted access without understanding the implications, leading to accidental misuse.
- **Lack of visibility**: No clear audit trail of who accessed what, when, and why.
- **Persistent threats**: Users with admin access can maintain access indefinitely, even after leaving the company.

**The Thand Solution**:

- **Zero standing privileges**: No permanent admin access anywhere
- **No static credentials**: All access is temporary and tied to your identity
- **Just-in-time permissions**: Request the access you need, when you need it - and lose it once you're done
- **Complete audit trail**: Every access request and action logged for compliance. Access is automatically reviewed during usage and revoked if the user moves off-task.

Thand is licensed under the BSL 1.1 license. See [LICENSE.md](LICENSE.md) for more details.

## Thand Architecture

The Thand architecture breaks down into three components. Both the agent and server
are contained within this repository. All access keys are stored on your infrastructure.
The Thand server can be deployed ephemerally without any persistent storage, providing a
low maintenance, high security solution. Temporal.io is used to orchestrate all workflows
and ensure just-in-time access is granted and revoked correctly and guarantees state maintenance.

- **Agent**: Runs on the user's local machine, provides session management and local callback endpoints to attest to the user's authenticity.
- **Server**: This can run anywhere you need to provide access. This forms a "login server" to allow CLIs and other clients to request and be granted elevations.
- **Cloud**: This is Thand's proprietary cloud service that orchestrates all your servers, agents and centralizes management and remote revocations. The cloud also provides additional features such as AI-driven insights and analytics. See thand.io for all the capabilities.

```
Your Machine             Your Infrastructure             Thand Cloud (Optional)
─────────────            ───────────────────             ──────────────────────

Thand Agent  ──HTTPS──▶  Thand Server        ──HTTPS──▶  Thand Cloud
├─ CLI                   ├─ REST API                     ├─ Agent Management
├─ Sessions              ├─ Session Management           ├─ Role Management
├─ Local elevations      ├─ Workflow Worker              ├─ Workflow Management
├─ REST API              ├─ Audit Forwarder              ├─ Audit Dashboard
└─ Attestations          ├─ Basic Approvals              └─ etc
                         ├─ Event collection
                         └─ Platform Connectors

```



## 🚀 Quick Start - Choose Your Path

### Option 1:  Use Thand Cloud (SaaS)

Firstly, install the Thand agent on your local machine. You can do this via the install script:

```bash
# Install agent (https://github.com/thand-io/agent/blob/main/scripts/install.sh). Trust but verify!
curl -sSL https://get.thand.io | sh

```

Or via Homebrew on macOS / Linux:

```bash
brew tap thand-io/tap
brew install thand
```

```bash
# Connect to cloud for AI features
thand login

# Request with natural language
thand request "I need to debug customer data issue"

```

### Option 2: Self-Host Everything (Open Source)

Thand can be deployed quickly on your infrastructure. The server requires no persistent storage and can be run ephemerally. You can deploy via Docker, Kubernetes or even as an AWS Lambda function or GCP Cloud Function.

A full guide to self-hosting can be found in the [docs](https://github.com/thand-io/agent/wiki/Self-Hosting).

To get started quickly you can run the server locally via Docker. This will start your server with the default configuration defined in the examples directory. For production usage you should provide your own configuration file. See the [docs](https://github.com/thand-io/agent/wiki/Configuration) for more details.

```bash

# Run the server locally via Docker
docker run -p 8080:8080 ghcr.io/thand-io/agent:latest server

# Or build and run locally
git clone https://github.com/thand-io/agent.git
cd agent

docker build -t thand-dev/agent:latest .
docker run -p 8080:8080 thand-dev/agent:latest server

```

You can then connect the agent to your server.

```bash
# Install agent
curl -sSL https://get.thand.io | sh

# Connect to your server
thand login --login-server http://localhost:8080

```

## Documentation

Full documentation can be found at [docs](https://github.com/thand-io/agent/wiki).

- [Quick Start](https://github.com/thand-io/agent/wiki/Getting-started)
- [Self-Hosting](https://github.com/thand-io/agent/wiki/Self-Hosting)
- [Roles](https://github.com/thand-io/agent/wiki/Roles)
- [Providers](https://github.com/thand-io/agent/wiki/Providers)
- [Workflows](https://github.com/thand-io/agent/wiki/Workflows)
- [CLI Reference](https://github.com/thand-io/agent/wiki/CLI)
- [API Reference](https://github.com/thand-io/agent/wiki/API)
- [FAQ](https://github.com/thand-io/agent/wiki/FAQ)


## Status

Thand is released & we consider it stable; we follow [semver](https://semver.org/) for releases, so major versions indicate potentially breaking changes, command line or other behaviour. We try to minimise this where possible.

We're very happy to accept pull requests, feature requests, and bugs if it's not working for you. Thand is under active development.

Please see the [contributing guide](CONTRIBUTING.md) for more details.
