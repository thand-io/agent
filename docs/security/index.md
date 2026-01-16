---
layout: default
title: Security
nav_order: 11
description: "Security architecture, threat model, and data handling for Thand Agent"
---

# Security
{: .no_toc }

Evaluating Thand's security architecture and threat model
{: .fs-6 .fw-300 }

## Table of contents
{: .no_toc .text-delta }

1. TOC
{:toc}

---

## Architecture Overview

Thand is a decentralized system with limited single points of failure, and no requirements to store credentials or keys outside of your own infrastructure. The architecture consists of:

- **Thand Agent** (your machine): Manages sessions, provides local callback endpoints
- **Thand Server** (your infrastructure): Processes authentication and elevation requests
- **Temporal** (workflow engine): Orchestrates workflows for deterministic execution and revocation
- **Thand Cloud** (optional): Proprietary SaaS service for centralized reporting, orchestration and analytics and an user access portal.

### Communication Flow

<pre class="mermaid">
flowchart TD
  Agent["Agent"]
  ThandServer["Thand Server/Cloud"]
  Temporal["Temporal"]
  Providers["Cloud/Local Agents (UAC/SUDO/AWS/GCP/Azure)"]

  Agent <-->|HTTPS| ThandServer
  ThandServer -.->|gRPC| Temporal
  Temporal -.->|gRPC| Providers
</pre>

---

## Threat Model

### Understaning the potential blast radius

If you're evaluating Thand, you probably wan to understand the potential impact of compromise with the components that make up the service.

#### Agent Compromise (Local Machine)

**Blast Radius**: Limited to single user and their active sessions

**What an attacker gains**:
- Access to user's current active sessions only. Limited to one hour by default. Sessions can only be used to execute new workflows.
- Locally cached encrypted session tokens. These are encryted by the server and can only be executed by the server
- Callback endpoints on localhost. These are used to store sessions to the users local file systems. Access is only granted by presigned time-bound codes.
- Any inflight tokens depeloyed to the users system post-elevation. These will be bound by the respective provider.

**What an attacker does NOT gain**:
- Other users' sessions or credentials
- Access to Thand Server or infrastructure
- Ability to elevate privileges beyond current sessions (depending on the workflow).
- Persistent access after session expiration

**Mitigations**:
- Sessions automatically expire (configurable timeout)
- Tokens are encrypted at rest
- No static credentials stored locally
- Session revocation triggered by server-side policies. Both the client and remote server and revoke / de-provision access at any time.
- Local agent requires re-authentication after timeout

#### Server Compromise

**Blast Radius**: Limited to workflow orchestration and metadata

**What an attacker gains**:
- Access to elevation request metadata (who, what, when)
- Access policy configurations
- User identity attributes from OAuth2/SAML providers
- Workflow state and execution history
- Direct access to cloud provider credentials. Depending on how the providers are configured. Most providers can use IAM.

**What an attacker does NOT gain**:
- User passwords or authentication secrets
- Session tokens (stored encrypted on agents)
- Ability to directly access target infrastructure
- Cross-tenant data (deployments are isolated)

**Mitigations**:
- Server acts as orchestrator, not credential vault
- Provider credentials configured separately for provider agents
- All sensitive operations flow through Temporal workflows
- Audit logs track all elevation requests
- Network isolation limits lateral movement
- Temporal workflows enforce deterministic revocation

#### Temporal Compromise

**Blast Radius**: Workflow orchestration and provider agent coordination

**What an attacker gains**:
- Visibility into workflow execution state
- Ability to interfere with workflow scheduling
- Access to workflow history and metadata

**What an attacker does NOT gain**:
- Direct credential access (credentials live in provider agents)
- Ability to bypass authentication flows
- Access to encrypted session tokens on agents
- Temporal does not store or process sensitive credentials. Only the workflow state

**Mitigations**:
- Use Temporal Cloud with mTLS and encryption at rest
- Network isolation for Temporal namespace
- Separate Temporal namespaces per environment
- Audit workflow executions regularly
- Configure Temporal authentication and authorization

#### Identity Provider Compromise (OAuth2/SAML)

**Blast Radius**: Ability to authenticate as users, subject to policies

**What an attacker gains**:
- User authentication capability
- Ability to request access elevation
- Access subject to configured role policies and workflows

**What an attacker does NOT gain**:
- Bypass of approval workflows
- Access outside of defined role permissions
- Ability to avoid audit logging

**Mitigations**:
- Use reputable identity providers (Okta, Google Workspace, Azure AD)
- Enable MFA at identity provider level
- Configure approval workflows for sensitive roles
- Monitor audit logs for anomalous patterns
- Implement conditional access policies
- Set appropriate session timeouts

### Attack Scenarios

| Scenario | Impact | Blast Radius | Mitigation |
|----------|--------|--------------|------------|
| Stolen agent binary | None | N/A | Agent requires OAuth2/SAML authentication with IdP |
| Agent machine compromise | Active sessions only | Single user | Sessions expire automatically; no static credentials |
| MITM agent-server | Session hijack attempt | Single user | TLS 1.3 encryption; certificate validation |
| Thand.io database breach | Metadata exposure | Elevation request history | No credentials stored; audit logs help detection |
| Temporal compromise | Workflow disruption | Workflow state only | Use Temporal Cloud with mTLS; no direct credential access |
| Provider agent compromise | Cloud provider access | Single provider, configured permissions | Least-privilege IAM; separate accounts per environment |
| IdP compromise | User impersonation | Subject to policies/workflows | MFA enforcement; approval workflows; audit monitoring |
| Compromised target system | Isolated to that system | Single resource | Auto-expiring sessions; no lateral credential reuse |

---

## Data Handling

### What We Store (and Don't Store)

#### ❌ Never Stored on Thand Server

- **User passwords**: Authentication delegated to OAuth2/SAML identity providers
- **User session tokens**: Stored encrypted on local agent only
- **SSH private keys**: Never collected or transmitted
- **Cloud provider credentials obtained by users**: Ephemeral, managed by provider agents
- **Target system data**: No access to customer workloads or application data
- **Command output or payloads**: No logging of session contents

#### ✅ What Thand Server Stores

- **Elevation request metadata**: User identity, role requested, provider, timestamp, reason, duration
- **Access policies**: Role definitions, provider configurations, workflow assignments
- **Audit logs**: Who requested what access, when, approval status, session lifecycle
- **User identity attributes**: Email, name, groups from OAuth2/SAML provider
- **Session metadata**: Expiry times, session status (active/expired/revoked)
- **Configuration** Server endpoint, logging preferences, local settings. Provider, role and workflow configurations.
- **Service credentials**: IAM roles, service account keys to interact with cloud providers
- **Provider configurations**: Account IDs, regions, API endpoints
- **Workflow context**: Temporal workflow state for deterministic execution

#### ✅ What Thand Agent Stores (Locally)

- **Encrypted session tokens**: Temporary credentials encrypted at rest
- **Configuration**: Server endpoint, logging preferences, local settings. Provider, role and workflow metadata (names and descriptions).


### Data Flows

<pre class="mermaid">
---
config:
  layout: elk
---
flowchart TD
  subgraph UB["User's System"]
    Browser["User's Browser/CLI"]
    LocalAgent["Thand Agent (UAC/SUDO)"]
  end
  subgraph YI["Your Infrastructure"]
    ThandServer["Thand Server (Your Infra)"]
    CloudAgent["Thand Agent (AWS/GCP/Azure)"]
  end
  subgraph TC["Thand Cloud (Optional)"]
    ThandCloud["Thand.io (Request router)"]
  end

  Browser -->|1. OAuth2/SAML Authentication| ThandServer
  Browser -->|1. OAuth2/SAML Authentication| ThandCloud
  Browser -->|3. Request Elevation| ThandServer
  Browser -->|3. Request Elevation| ThandCloud
  ThandServer -.gRPC.-> Temporal["Temporal (Workflow Engine)"]
  ThandCloud -.gRPC.-> Temporal
  Temporal -.gRPC.-> CloudAgent
  Temporal -.gRPC.-> LocalAgent 
  ThandServer --> |2. Encrypted Session Token| LocalAgent
  ThandCloud --> |2. Encrypted Session Token| LocalAgent
</pre>

<script src="https://cdn.jsdelivr.net/npm/mermaid@11.12.1/dist/mermaid.min.js"></script>
<script>
  mermaid.initialize({ startOnLoad: true });
</script>

**Key Points**:
1. User authenticates via OAuth2/SAML (browser-based flow)
2. Agent requests elevation from Thand Server or Thand.io
3. Server validates policy and triggers Temporal workflow
4. Local Provider agent or Thand Server (orchestrated by Temporal) provisions temporary credentials
5. Encrypted session token delivered to agent via callback endpoint
6. Agent uses ephemeral credentials to access target infrastructure
7. Temporal workflow ensures deterministic revocation on expiry

### In-Scope Information

**Thand Server has access to**:
- Elevation request metadata (who, what, when, why, duration)
- Access policy definitions and role mappings
- User identity claims from OAuth2/SAML provider
- Session lifecycle state (pending/active/expired/revoked)
- Workflow execution history
- Cloud provider service credentials (IAM roles, service accounts)
- Provider-specific APIs for credential provisioning
- Workflow context from Temporal

**Thand Server does NOT have access to**:
- Encrypted session tokens (stored on agent)
- User passwords or authentication secrets
- Commands executed during sessions
- Data accessed or transferred in target systems
- Application logs or customer workloads
- Decrypted credentials in transit

**Local Agents have access to**:
- Local agents can be configured to provide temporal activities to
  - Elevate local users with SUDO/UAC
  - Local provisioning workflow script execution
  - Local (Conditional access policies) CAP (osquery)

**Local Agents do NOT have access to**:
- User authentication credentials
- Thand Server internals
- Session contents or command history

---

## Telemetry & Logging

### Server Logging

Thand Server logs are designed for audit and compliance:

**Logged**:
- Authentication events (user, timestamp, provider, success/failure)
- Elevation requests (user, role, provider, reason, duration, workflow ID)
- Policy evaluation results (approved/denied, reasons)
- Session lifecycle events (created, expired, revoked)
- Workflow execution status

**NOT Logged**:
- User passwords or OAuth2 tokens
- Session credentials or ephemeral tokens
- Commands executed in sessions
- Data payloads or application responses
- Target system internals or configurations

### Local Agent Telemetry

Thand Agent collects minimal telemetry for operational purposes:

**Collected**:
- Agent version, platform, and architecture
- Session lifecycle events (start, end, expiry)
- Connection health (success/failure to server)
- Error codes and types (sanitized, no sensitive context)

**NOT Collected**:
- User credentials, tokens, or passwords
- Command-line arguments or environment variables
- Session contents, command output, or payloads
- File paths, hostnames, or resource identifiers from sessions
- Network traffic contents or intercepted data


### Log Retention

- **Audit logs**: Configurable retention (default: 90 days, configurable per compliance needs)
- **Operational logs**: Configurable retention (default: 30 days)
- **Workflow history**: Retained in Temporal per Temporal retention policies
- **Session credentials**: Never logged, never retained (ephemeral only)

---

## Security Best Practices

### Deployment Recommendations

1. **Use TLS Everywhere**
   - Deploy Thand Server with valid TLS certificates. Or behind Identity aware proxies
   - Use Temporal Cloud or self-hosted Temporal with mTLS
   - Enable HTTPS for all agent-server communication.

2. **Network Isolation**
   - Restrict Temporal access to Thand Server and local agents only
   - Limit inbound connections as far as practical
   - Segment agents per environment (AWS, GCP etc.). Thand servers should be deployed and configured per environment. Thand servers should really only have 1 provisioning provider.

3. **Least Privilege IAM**
   - Grant provider agents minimum required permissions
   - Use separate service accounts per cloud provider
   - Rotate service credentials regularly
   - Enable cloud provider audit logging (CloudTrail, Cloud Audit Logs, etc.)

4. **Identity and Access**
   - Use reputable identity providers (Okta, Google Workspace, Azure AD)
   - Enforce MFA at identity provider level
   - Configure conditional access policies
   - Set appropriate session timeout durations

5. **Monitoring and Auditing**
   - Enable audit logging for all components
   - Monitor elevation request patterns for anomalies
   - Alert on unusual access patterns or failed authentications
   - Integrate logs with SIEM systems
   - Regularly review access policies and role definitions

---

## Security Reviews

### Internal Security Practices

- Regular security audits of codebase
- Dependency scanning and vulnerability management
- Automated security testing in CI/CD pipeline
- Security-focused code review process
- Principle of least privilege by default

### Third-Party Assessments

Documentation of security reviews and assessments will be published here as they are completed:

- Penetration testing reports
- Security audit findings
- Compliance certifications (SOC 2, ISO 27001, etc.)
- Vulnerability disclosures and patches

---

## Reporting Security Vulnerabilities

If you discover a security vulnerability in Thand Agent:

**Email**: [security@thand.io](mailto:security@thand.io)

**Please do NOT** open public GitHub issues for security vulnerabilities.

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact (reference threat model above)
- Affected component (agent, server, provider agent)
- Suggested fix (if available)

We aim to respond to security reports within **48 hours**.

---

## Questions?

For security questions or clarifications about our threat model and data handling:

- **Email**: [security@thand.io](mailto:security@thand.io)

