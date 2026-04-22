---
layout: default
title: Workflows
parent: Configuration
nav_order: 10
description: Detailed documentation for Thand Agent workflows
has_children: true
---

# Workflows

Workflows define the approval and authorization processes for Thand Agent role requests. They are built on the [Serverless Workflow Specification](https://serverlessworkflow.io/) and provide a declarative way to define complex access control processes with approval chains, notifications, monitoring, and revocation.

## Overview

Thand workflows orchestrate the complete lifecycle of access requests:

1. **Validation** - Verify request validity and user permissions
2. **Approval** - Route requests through approval chains with notifications
3. **Execution Planning** - Compile the final request into an execution plan
4. **Authorization** - Grant temporary access to requested resources
5. **Monitoring** - Track usage and detect policy violations
6. **Revocation** - Remove access when complete or violated

Workflows leverage the [Serverless Workflow DSL](https://serverlessworkflow.io/specification/) for standardized process definition while providing custom Thand-specific tasks for access control operations.

## Workflow Concepts

### What is a Workflow?

A Thand workflow is a serverless workflow specification that defines:
- **Process Flow**: Step-by-step access request handling
- **Custom Tasks**: Thand-specific operations (validate, approve, authorize, etc.)
- **Integrations**: External system interactions (Slack, email, webhooks)
- **Conditions**: Dynamic routing based on request properties
- **Events**: Lifecycle notifications and monitoring hooks

### Workflow vs Role Relationship

Workflows and roles work together:
- **Roles** define *what* access is possible and *who* can request it
- **Workflows** define *how* access requests are processed and approved

```yaml
# Role references workflow
roles:
  aws-admin:
    workflows:
      - security-approval  # References workflow name

# Workflow defines the process
workflows:
  security-approval:
    workflow:
      do:
        - validate: { thand: validate }
        - approve: { thand: approvals }
        - grant: { thand: authorize }
```

## Execution Planning

Access-granting workflows should treat execution planning as part of the standard `authorize` lifecycle.

`authorize` runs an internal execution-plan activity after validation, approvals, and any other step that can still change the final request. That activity compiles the request into an internal execution plan that `authorize` and `revoke` later consume.

In practice, the safe patterns are:

- `validate -> authorize`
- `validate -> approvals -> authorize`

If a workflow grants access, do not put any request-shaping step after `authorize`, because the execution plan is snapped there.

## Workflow Structure

### Basic Configuration

```yaml
version: "1.0"
workflows:
  workflow-name:
    description: Human readable description
    authentication: provider-name  # Auth provider for this workflow
    enabled: true
    
    workflow:
      document:
        dsl: "1.0.0-alpha5"
        namespace: "thand"
        name: "workflow-name"
        version: "1.0.0"
      
      # Workflow definition using Serverless Workflow DSL
      do:
        - step-name:
            thand: task-name  # Custom Thand task
            # or
            call: external-function  # External API call
            with:
              parameter: value
            then: next-step
```

### Complete Workflow Example

```yaml
version: "1.0"
workflows:
  manager-approval:
    description: Manager approval workflow with Slack notifications
    authentication: google-oauth
    enabled: true
    
    workflow:
      document:
        dsl: "1.0.0-alpha5"
        namespace: "thand"
        name: "manager-approval"
        version: "1.0.0"
      
      use:
        secrets:
          - slack-credentials
        
      do:
        # Step 1: Validate the request
        - validate:
            thand: validate
            with:
              validator: static  # or 'llm' for AI validation
            then: notify-manager
        
        # Step 2: Send approval request to manager
        - notify-manager:
            thand: approvals
            with:
              approvals: 1
              notifiers:
                slack:
                  provider: slack
                  to: "#security-team"
                  message: >
                    User ${ .user.name } is requesting ${ .role.name } access.
                    Reason: ${ .reason }
            on:
              approved: authorize
              denied: deny-notification
            then: deny-notification

        # Step 3: Grant access if approved
        - authorize:
            thand: authorize
            then: monitor
        
        # Step 4: Monitor usage
        - monitor:
            thand: monitor
            with:
              monitor: llm
              threshold: 10
            then: revoke
        
        # Step 5: Revoke access
        - revoke:
            thand: revoke
            with:
              reason: "Session complete"
            then: finished
        
        # Denial path
        - deny-notification:
            call: slack.postMessage
            with:
              provider: slack
              to: "#security-team"
              message: "Access request denied for ${ .user.name }"
            then: end
        
        # Completion notification
        - finished:
            call: slack.postMessage
            with:
              provider: slack
              to: "#security-team"
              message: "Access revoked for ${ .user.name }"
            then: end
```

## Serverless Workflow Integration

Thand workflows are built on the [Serverless Workflow Specification v1.0.0-alpha5](https://serverlessworkflow.io/). This provides:

### Standard DSL Features

- **Flow Control**: Sequential, parallel, and conditional execution
- **Event Handling**: CloudEvents integration for external triggers
- **Error Handling**: Try-catch patterns and retry logic
- **Data Manipulation**: JSONPath and expression evaluation
- **External Integrations**: HTTP, gRPC, OpenAPI, and AsyncAPI calls

### Key DSL Components

| Component | Purpose | Example |
|-----------|---------|---------|
| `do` | Sequential task execution | `do: [validate, approve, authorize]` |
| `call` | External API/function calls | `call: slack.postMessage` |
| `for` | Iteration over collections | `for: { each: .approvers, do: notify }` |
| `fork` | Parallel execution | `fork: { parallel: [notify-slack, notify-email] }` |
| `switch` | Conditional branching | `switch: [{ when: .approved, then: grant }]` |
| `try` | Error handling | `try: authorize, catch: rollback` |

### External Documentation

For complete DSL reference, see:
- **[Serverless Workflow Specification](https://serverlessworkflow.io/specification/)** - Full DSL documentation
- **[Serverless Workflow Examples](https://github.com/serverlessworkflow/specification/tree/main/examples)** - Reference implementations
- **[CloudEvents Specification](https://cloudevents.io/)** - Event format used by workflows

## Custom Thand Tasks

Thand provides custom workflow tasks for access control operations. These tasks are invoked using the `thand:` keyword:

```yaml
do:
  - step-name:
      thand: task-name
      with:
        parameter: value
```

### Available Thand Tasks

| Task | Purpose | Documentation |
|------|---------|---------------|
| `validate` | Validate access requests | [Tasks Documentation](tasks/#validate) |
| `approvals` | Handle approval workflows | [Tasks Documentation](tasks/#approvals) |
| `authorize` | Grant temporary access | [Tasks Documentation](tasks/#authorize) |
| `monitor` | Monitor access usage | [Tasks Documentation](tasks/#monitor) |
| `revoke` | Remove granted access | [Tasks Documentation](tasks/#revoke) |
| `notify` | Send notifications | [Tasks Documentation](tasks/#notify) |

For detailed documentation of each task, see the [Tasks Reference](tasks/).

## Configuration Management

### File Structure

Workflows can be organized multiple ways:

#### Single File
```yaml
# workflows.yaml
version: "1.0"
workflows:
  approval1: { ... }
  approval2: { ... }
```

#### Multiple Files by Purpose
```
config/workflows/
├── approval.yaml      # Approval workflows
├── emergency.yaml     # Emergency/break-glass workflows
├── automated.yaml     # Automated workflows
└── monitoring.yaml    # Monitoring workflows
```

#### Multiple Files by Environment
```
config/workflows/
├── dev.yaml
├── staging.yaml
└── prod.yaml
```

### Loading Configuration

Configure workflow loading in the main config:

```yaml
# Load from directory
workflows:
  path: "./config/workflows"

# Load from URL
workflows:
  url:
    uri: "https://config.company.com/workflows.yaml"
    headers:
      Authorization: "Bearer YOUR_API_TOKEN"

# Load from Vault
workflows:
  vault: "secret/agent/workflows"

# Inline definitions
workflows:
  simple-approval:
    description: Simple approval
    workflow:
      do:
        - validate: { thand: validate }
        - approve: { thand: approvals }
        - authorize: { thand: authorize }
```

## Workflow Patterns

### Basic Approval Pattern

```yaml
workflows:
  basic-approval:
    workflow:
      do:
        - validate: { thand: validate, then: approve }
        - approve: { thand: approvals, then: authorize }
        - authorize: { thand: authorize, then: end }
```

### Multi-Step Approval

```yaml
workflows:
  multi-approval:
    workflow:
      do:
        - validate: { thand: validate, then: manager-approval }
        - manager-approval:
            thand: approvals
            with:
              approvals: 1
              notifiers:
                email:
                  provider: email
                  to: "manager@company.com"
            on:
              approved: security-approval
              denied: denied
        - security-approval:
            thand: approvals
            with:
              approvals: 1
              notifiers: 
                slack:
                  provider: slack
                  to: C1234567890
            on:
              approved: authorize
              denied: denied
        - authorize: 
            thand: authorize
            then: end
        - denied:
            call: notify-denial
            then: end
```

### Time-Based Approval

```yaml
workflows:
  time-limited:
    workflow:
      schedule:
        after: "PT2H"  # ISO 8601 duration (2 hours)
      do:
        - validate: { thand: validate, then: authorize }
        - authorize: { thand: authorize, then: scheduled-revoke }
        - scheduled-revoke:
            wait:
              duration: "PT2H"
            then: revoke
        - revoke: { thand: revoke, then: end }
```

### Emergency/Break-Glass Workflow

```yaml
workflows:
  emergency-access:
    workflow:
      do:
        - validate: { thand: validate, then: immediate-grant }
        - immediate-grant: { thand: authorize, then: alert-security }
        - alert-security:
            fork:
              parallel:
                - slack-alert: { call: slack.postMessage }
                - email-alert: { call: email.send }
                - webhook-alert: { call: security.webhook }
            then: enhanced-monitoring
        - enhanced-monitoring:
            thand: monitor
            with:
              monitor: llm
              threshold: 5  # Lower threshold for emergency access
            then: auto-revoke
        - auto-revoke: { thand: revoke, then: end }
```

### Conditional Workflows

```yaml
workflows:
  conditional-approval:
    workflow:
      do:
        - validate: { thand: validate, then: route-request }
        - route-request:
            switch:
              - when: '${ .role.name == "admin" }'
                then: security-approval
              - when: '${ .duration > "PT4H" }'
                then: manager-approval
              - when: '${ .user.department == "security" }'
                then: authorize
            default: standard-approval
        
        - security-approval: { thand: approvals, then: authorize }
        - manager-approval: { thand: approvals, then: authorize }
        - standard-approval: { thand: approvals, then: authorize }
        - authorize: { thand: authorize, then: end }
```

## Integration Examples

### Slack Integration

```yaml
workflows:
  slack-workflow:
    workflow:
      use:
        secrets: [slack-credentials]
      do:
        - notify:
            call: slack.postMessage
            with:
              provider: slack
              channel: "#approvals"
              message: |
                :warning: Access Request
                User: ${ .user.name }
                Role: ${ .role.name }
                Reason: ${ .reason }
              blocks:
                - type: section
                  text: "New access request requires approval"
                - type: actions
                  elements:
                    - type: button
                      text: "Approve"
                      action_id: "approve_request"
                    - type: button
                      text: "Deny"
                      action_id: "deny_request"
```

### Email Integration

```yaml
workflows:
  email-workflow:
    workflow:
      do:
        - notify:
            call: email.send
            with:
              provider: email
              to: ["manager@company.com", "security@company.com"]
              subject: "Access Request: ${ .role.name }"
              template: "access-request"
              data:
                user: ${ .user }
                role: ${ .role }
                reason: ${ .reason }
```

### Webhook Integration

```yaml
workflows:
  webhook-workflow:
    workflow:
      do:
        - external-approval:
            call: http
            with:
              method: POST
              url: "https://approval.company.com/api/requests"
              headers:
                Authorization: "Bearer YOUR_API_TOKEN"
                Content-Type: "application/json"
              body:
                user: ${ .user.email }
                role: ${ .role.name }
                duration: ${ .duration }
                reason: ${ .reason }
```

## Security Considerations

### Authentication

Workflows can specify required authentication:

```yaml
workflows:
  secure-workflow:
    authentication: saml-sso  # Require SAML authentication
    workflow:
      # ... workflow definition
```

### Secrets Management

Use secrets for sensitive data:

```yaml
workflows:
  workflow-with-secrets:
    workflow:
      use:
        secrets:
          - api-keys
          - database-credentials
      do:
        - api-call:
            call: http
            with:
              headers:
                Authorization: "Bearer ${ $api-keys.token }"
```

### Input Validation

Always validate inputs in workflows:

```yaml
workflows:
  validated-workflow:
    workflow:
      do:
        - validate:
            thand: validate
            with:
              validator: static  # or 'llm'
              rules:
                - max_duration: "PT8H"
                - allowed_roles: ["developer", "admin"]
```

## Troubleshooting

### Common Issues

#### 1. Workflow Not Found
```
Error: workflow 'approval' not found
```
**Solution**: Verify workflow name matches role configuration and workflow definition.

#### 2. Task Execution Failed
```
Error: thand task 'validate' failed: invalid request
```
**Solution**: Check task parameters and input data format.

#### 3. External Call Failed
```
Error: call to 'slack.postMessage' failed: unauthorized
```
**Solution**: Verify API credentials and permissions.

#### 4. Workflow Timeout
```
Error: workflow execution timed out
```
**Solution**: Check for infinite loops and add appropriate timeouts.

### Debugging Workflows

Enable workflow debugging:

```yaml
logging:
  level: debug
  workflows:
    trace: true
```

Use workflow testing:

```bash
# Test workflow execution
thand workflow test --workflow approval --input request.json

# Validate workflow syntax
thand workflow validate --file workflows.yaml
```

## Best Practices

### 1. Workflow Design

**Keep Workflows Simple**
```yaml
# Good - clear, linear flow
do:
  - validate: { thand: validate }
  - approve: { thand: approvals }
  - authorize: { thand: authorize }

# Avoid - overly complex branching
```

**Use Meaningful Names**
```yaml
# Good - descriptive step names
- manager-approval: { thand: approvals }
- security-review: { thand: approvals }

# Avoid - unclear names
- step1: { thand: approvals }
- approval: { thand: approvals }
```

### 2. Error Handling

**Implement Proper Error Handling**
```yaml
workflows:
  robust-workflow:
    workflow:
      do:
        - validate:
            try:
              thand: validate
            catch:
              - log-error: { call: logger.error }
              - notify-admin: { call: slack.postMessage }
              - fail: { raise: { error: validation-failed } }
```

### 3. Performance

**Optimize Approval Flows**
```yaml
# Use parallel notifications when possible
- notify-approvers:
    fork:
      parallel:
        - slack-notify: { call: slack.postMessage }
        - email-notify: { call: email.send }
```

### 4. Security

**Principle of Least Privilege**
```yaml
workflows:
  secure-workflow:
    authentication: multi-factor  # Require strong auth
    workflow:
      do:
        - validate:
            thand: validate
            with:
              validator: llm  # Use AI for enhanced validation
              strict_mode: true
```

## Examples

For complete workflow examples and templates, see the [Workflow Examples](examples/) page which includes:

- **Basic Approval Workflow** - Simple manager approval
- **Multi-Stage Approval** - Complex approval chains
- **Emergency Access Workflow** - Break-glass patterns
- **Automated Workflows** - AI-driven approvals
- **Integration Examples** - Slack, email, and webhook workflows
- **Conditional Workflows** - Dynamic routing based on request properties
- **Time-Based Workflows** - Scheduled and time-limited access

Each example includes complete YAML configurations with detailed explanations.
