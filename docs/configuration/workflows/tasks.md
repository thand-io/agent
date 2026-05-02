---
layout: default
title: Workflow Tasks
parent: Workflows
grand_parent: Configuration
nav_order: 1
description: Documentation for custom Thand workflow tasks
---

# Workflow Tasks

Thand provides custom workflow tasks that extend the Serverless Workflow specification with access control operations. These tasks are invoked using the `thand:` keyword and handle the core functionality of the Thand Agent.

## Task Overview

Custom Thand tasks handle the complete access control lifecycle:

| Task | Purpose | Phase |
|------|---------|-------|
| `validate` | Validate access requests and user permissions | Pre-authorization |
| `approvals` | Handle approval workflows with notifications | Authorization |
| `authorize` | Grant temporary access to requested resources | Authorization |
| `monitor` | Monitor usage and detect policy violations | Post-authorization |
| `revoke` | Remove granted access | Post-authorization |
| `notify` | Send notifications to users and administrators | Cross-cutting |

## Task Syntax

All Thand tasks follow this syntax:

```yaml
- step-name:
    thand: task-name
    with:
      parameter1: value1
      parameter2: value2
    then: next-step
    on:
      condition1: target-step1
      condition2: target-step2
```

## validate

The `validate` task verifies that access requests are valid and that users have permission to request specific roles.

### Syntax

```yaml
- validate:
    thand: validate
    with:
      validator: static|llm  # Validation method (optional, defaults to "static")
    then: next-step
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `validator` | string | No | `static` | Validation method: `static` for rule-based, `llm` for AI-enhanced |

### Validation Methods

#### Static Validation

Uses predefined rules to validate requests. This is the default validation method:

```yaml
- validate:
    thand: validate
    with:
      validator: static
    then: approvals
```

The static validator:
- Validates that the user is provided
- Validates that the role is provided  
- Validates that the reason is provided
- Validates that the duration format is correct (converts to ISO 8601)
- Validates that providers are specified
- Calls the provider's role validation

#### LLM Validation

Uses AI to validate requests for enhanced validation:

```yaml
- validate:
    thand: validate
    with:
      validator: llm
      model: gemini-2.5-pro  # Optional model specification
    then: approvals
```

**Note**: LLM validation is currently implemented but the functionality is limited to model specification.

### Examples

**Basic Static Validation**
```yaml
- validate-request:
    thand: validate
    then: approve-request
```

**LLM Validation with Model**
```yaml
- intelligent-validate:
    thand: validate
    with:
      validator: llm
      model: gemini-2.5-pro
    then: risk-assessment
```

## approvals

The `approvals` task handles approval workflows by sending notifications to approvers and waiting for approval decisions.

### Syntax

```yaml
- approvals:
    thand: approvals
    with:
      approvals: number          # Required approvals
      timeout: duration          # Optional overall deadline; requires on.timeout
      notifiers:                  # Notification configuration
        key:
          provider: string
          to: string
          message: string
    on:
      approved: target-step
      denied: target-step
      timeout: target-step        # Required when with.timeout is set
    then: default-step
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `approvals` | number | Yes | Number of approvals required |
| `timeout` | duration | No | Overall approval deadline. Requires `on.timeout` when set |
| `notifiers` | object | No | Notification configuration |

### Local Presence Configuration

Use a normal notifier entry with `provider: local-presence` to add a device-routed macOS owner-authentication prompt. The routed agent reports the prompt result back into the same approval result path used by Slack and email callbacks, so an approved local presence check can satisfy the configured `approvals` count.

```yaml
notifiers:
  touch_id:
    provider: local-presence
    device: "${ .device }"
    prompt: "Approve this access request on your Mac"
```

Device resolution uses this order:

1. the notifier entry `device`
2. the elevation request `device`
3. request metadata `device_id`

Local presence uses the approvals task's remaining `with.timeout` deadline for device routing and the native macOS prompt. If the target device is offline or the prompt times out, the task routes to `on.timeout` instead of treating the result as a denial. User cancel or authentication failure is a denial and routes to `on.denied`.

Local presence is not ordered ahead of other notifiers inside one approvals task. If you want "try Touch ID first, then fall back to humans," model that as two approvals tasks: a short local-presence task whose `on.timeout` branch points to a second Slack/email approvals task.

The check requires a live macOS agent in an interactive login session. It proves that macOS device-owner authentication succeeded on the routed device; it is not a cryptographic enrolled-device attestation.

### Notifiers Configuration

The `notifiers` object configures how approval requests are sent:

```yaml
notifiers:
  slack:
    provider: slack               # Notification provider
    to: "C0123456789"            # Channel ID or user ID
    message: "Approval needed"    # Custom message (supports templating)
```

### Supported Providers

| Provider | Purpose | Target Format |
|----------|---------|---------------|
| `slack` | Slack notifications | Channel ID: `C0123456789` or User ID |
| `email` | Email notifications | Email address |
| `local-presence` | macOS device-owner approval prompt | `device_id` |
| `local-notification` | macOS local user notification | `device_id` |

### Flow Control

The approvals task uses the `on` directive for conditional flow:

```yaml
- approvals:
    thand: approvals
    with:
      approvals: 2
      timeout: 15m
      notifiers: ...
    on:
      approved: grant-access     # If approved
      denied: send-denial        # If denied
      timeout: timeout-handler   # If no final answer before timeout
```

### Approval Logic

The approvals task implements the following logic:
1. Sends notifications using the specified notifier
2. Listens for approval events (`com.thand.approval`)
3. Collects approvals in the workflow context
4. If any approval is `false` (denied), routes to the `denied` state
5. If the number of `true` approvals meets the required count, routes to the `approved` state
6. If `with.timeout` expires before approval or denial, routes to the `timeout` state
7. Otherwise, loops back to wait for more approvals

### Examples

**Basic Slack Approval**
```yaml
- manager-approval:
    thand: approvals
    with:
      approvals: 1
      timeout: 30m
      notifiers:
        slack:
          provider: slack
          to: "C0123456789"
          message: >
            Access request needs your approval.
    on:
      approved: grant-access
      denied: deny-request
      timeout: deny-request
```

**Email Approval**
```yaml
- email-approval:
    thand: approvals
    with:
      approvals: 1
      notifiers:
        email:
          provider: email
          to: "manager@company.com"
          message: "Access request needs approval"
    on:
      approved: authorize
      denied: denied
```

**macOS Local Presence-Gated Sudo**
```yaml
- presence:
    thand: approvals
    with:
      approvals: 1
      timeout: 2m
      notifiers:
        touch_id:
          provider: local-presence
          device: "${ .device }"
          prompt: "Approve this sudo request on your Mac"
    on:
      approved: authorize
      denied: denied
      timeout: human_approval
```

## Execution Planning

`authorize` runs an internal execution-plan activity that compiles the current workflow request into the execution plan later used by `authorize` and `revoke`.

Treat `authorize` as the last request-shaping step before access is granted. It should run only after approvals, form collection, and other workflow logic that might change the final request shape.

### Execution-Plan Process

The execution-plan activity:

1. **Reads** the normalized elevate request from workflow context
2. **Resolves** the provider, identity, device, and local policy data needed for execution
3. **Compiles** one or more provider authorization requests into an internal execution plan
4. **Stores** that execution plan in workflow context/history for later `authorize` and `revoke` steps

### Requirements and Constraints

- execution planning is required before provider authorization work starts
- `authorize` should appear immediately after approvals or any other request-shaping step
- `authorize` and `revoke` depend on the recorded execution plan and do not rebuild it later
- `revoke` fails if the execution plan is missing

### Failure Behavior

- if execution planning cannot compile the request, the workflow fails before authorization starts
- if `revoke` runs without a recorded execution plan, it fails instead of trying to recover implicitly

### Examples

**Validation, Approval, and Authorization**
```yaml
- validate:
    thand: validate
    then: approvals

- approvals:
    thand: approvals
    on:
      approved: authorize
      denied: denied
    then: denied

- authorize:
    thand: authorize
```

**Validation and Authorization Without Approval**
```yaml
- validate:
    thand: validate
    then: authorize
```

## authorize

The `authorize` task grants temporary access to the requested role and resources.

### Syntax

```yaml
- authorize:
    thand: authorize
    with:
      revocation: string         # Optional revocation step name
    then: next-step
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `revocation` | string | No | Step to call for revocation |

### Authorization Process

The authorize task:

1. **Builds or reuses** the recorded execution plan from workflow context/history
2. **Validates** the request is approved (checks workflow context)
3. **Creates** temporary credentials/access across all specified providers
4. **Registers** the session information
5. **Returns** authorization details with timestamps

### Authorization Context

The authorize task checks if the request has been approved by looking at the workflow context. It also snapshots the final request into the execution plan it and `revoke` later consume.

For any workflow that grants access, treat `authorize` as the first step that may perform provider-side effects.

### Examples

**Basic Authorization**
```yaml
- grant-access:
    thand: authorize
    then: monitor-usage
```

**Authorization with Revocation Step**
```yaml
- conditional-grant:
    thand: authorize
    with:
      revocation: "emergency-revoke"
    then: enhanced-monitoring
```

## monitor

The `monitor` task tracks access usage and detects policy violations or suspicious activity.

### Syntax

```yaml
- monitor:
    thand: monitor
    with:
      monitor: string            # Monitoring method (optional)
      threshold: number          # Alert threshold (optional)
    then: next-step
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `monitor` | string | No | - | Monitoring method (currently not used in implementation) |
| `threshold` | number | No | - | Alert threshold (currently not used in implementation) |

### Monitoring Process

The monitor task:
1. **Requires** Temporal workflow context (only works with Temporal)
2. **Listens** for alert events of type `com.thand.alert`
3. **Evaluates** alert level - if "critical", handles the alert
4. **Loops back** to continue monitoring for more alerts

### Examples

**Basic Monitoring**
```yaml
- track-usage:
    thand: monitor
    then: normal-completion
```

**Monitoring with Parameters**
```yaml
- enhanced-monitor:
    thand: monitor
    with:
      monitor: llm
      threshold: 10
    then: assessment-required
```

**Note**: The current implementation only supports basic alert listening. The `monitor` and `threshold` parameters are accepted but not actively used in the monitoring logic.

## revoke

The `revoke` task removes granted access and cleans up temporary credentials.

### Syntax

```yaml
- revoke:
    thand: revoke
    with:
      reason: string             # Revocation reason (optional)
    then: next-step
```

### Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `reason` | string | No | `"Access no longer needed"` | Reason for revocation |

### Revocation Process

The revoke task:

1. **Reads** the recorded execution plan from workflow context/history
2. **Uses** the stored authorization request shape to build revocation work
3. **Calls** provider-specific revocation methods
4. **Logs** revocation events
5. **Returns** revocation status with timestamp

### Examples

**Standard Revocation**
```yaml
- end-session:
    thand: revoke
    with:
      reason: "Access period completed"
    then: cleanup-notifications
```

**Default Revocation**
```yaml
- auto-revoke:
    thand: revoke
    then: end
```

## notify

The `notify` task sends notifications to users, administrators, or external systems. This task is used internally by the `approvals` task but can also be used standalone.

### Syntax

```yaml
- notify:
    thand: notify
    with:
      provider: string
      to: string
      message: string
      device: string             # Optional; used by local-notification
    then: next-step
```

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider` | string | Yes | Notification provider |
| `to` | string or array | Yes | Recipient identity, channel, or email address |
| `message` | string | No | Notification body |
| `device` | string | No | Target device for `local-notification` |

### Notification Process

The notify task:
1. **Validates** the notification request
2. **Checks** for matching notification providers
3. **Sends** notifications via the specified provider
4. **Creates** callback URLs for interactive approvals (Slack)

### Supported Providers

- **Slack**: Sends rich notifications with approval buttons
- **Email**: Sends email notifications
- **Local Notification**: Sends a macOS user notification on a routed agent device with `provider: local-notification`

`local-notification` requires a live macOS agent and a target device. Device resolution follows the explicit notifier `device` field first, then the request `device`, then request metadata `device_id`. The same routing applies when `local-notification` is used in `thand: notify`, `authorize.with.notifiers`, `revoke.with.notifiers`, or `approvals.with.notifiers` approval prompts. It uses the signed helper and `UNUserNotificationCenter`; it does not replace broker-triggered sudo lease notifications, which remain in place for now.

### Examples

**Slack Notification**
```yaml
- slack-notify:
    thand: notify
    with:
      provider: slack
      to: "C0123456789"
      message: "Access granted to user"
```

**macOS Local Notification**
```yaml
- local-notify:
    thand: notify
    with:
      provider: local-notification
      to: ${ $context.user.email }
      device: "${ .device }"
      message: "Your local sudo request was approved"
```

**Note**: The notify task is primarily used internally by the approvals task. For standalone notifications, consider using standard Serverless Workflow `call` tasks to external APIs.

## Task Chaining and Flow Control

### Sequential Execution

```yaml
do:
  - validate: { thand: validate, then: approve }
  - approve: { thand: approvals, then: authorize }
  - authorize: { thand: authorize, then: monitor }
  - monitor: { thand: monitor, then: revoke }
  - revoke: { thand: revoke, then: end }
```

### Conditional Flow with Approvals

```yaml
do:
  - validate:
      thand: validate
      then: request-approval
  
  - request-approval:
      thand: approvals
      with:
        approvals: 1
        notifiers:
          slack:
            provider: slack
            to: "C0123456789"
            message: "Approval needed"
      on:
        approved: grant-access
        denied: deny-access
      then: deny-access
  
  - grant-access: { thand: authorize, then: monitor }
  - deny-access: { thand: revoke, then: end }
```

### Error Handling

Use standard Serverless Workflow error handling:

```yaml
do:
  - safe-authorize:
      try:
        thand: authorize
      catch:
        - log-error:
            call: logger.error
            with:
              message: "Authorization failed"
        - cleanup: { thand: revoke }
```

## Best Practices

### 1. Task Configuration

**Always Use Validate First**
```yaml
# Correct flow
do:
  - validate: { thand: validate }
  - approve: { thand: approvals }
  - authorize: { thand: authorize }
```

**Use Appropriate Validation Method**
```yaml
# For standard validation
- validate:
    thand: validate
    with:
      validator: static

# For AI-enhanced validation
- validate:
    thand: validate
    with:
      validator: llm
```

### 2. Approval Configuration

**Always Specify Both Approval States**
```yaml
- approvals:
    thand: approvals
    with:
      approvals: 1
      notifiers: ...
    on:
      approved: grant-access     # Required
      denied: send-denial        # Required
    then: send-denial            # Fallback for insufficient approvals
```

### 3. Monitoring

**Monitor After Authorization**
```yaml
do:
  - authorize: { thand: authorize, then: monitor }
  - monitor: { thand: monitor, then: scheduled-revoke }
```

**Note**: Monitoring requires Temporal workflow context.

### 4. Revocation

**Always End with Revocation**
```yaml
do:
  - authorize: { thand: authorize, then: monitor }
  - monitor: { thand: monitor, then: revoke }
  - revoke: { thand: revoke, then: end }
```

## Troubleshooting

### Common Task Issues

#### 1. Validation Errors
```
Error: role must be provided
Error: reason must be provided
Error: no providers specified in elevate request
```
**Solution**: Ensure the elevate request in workflow context contains all required fields.

#### 2. Approval Configuration Errors
```
Error: both approved and denied states must be specified in the on block
```
**Solution**: Always specify both `approved` and `denied` states in the `on` block.

#### 3. Authorization Failures
```
Error: authorization failed for user 'alice' role 'admin'
```
**Solution**: Verify the request has been properly approved and the user has permission to request the role.

#### 4. Missing Execution Plan
```
Error: failed to get execution plan from workflow context
```
**Solution**: Ensure `thand: revoke` only runs on paths where `thand: authorize` has already executed and recorded the execution plan.

#### 5. Monitoring Limitations
```
Error: Monitoring is only supported with temporal
```
**Solution**: The monitor task only works within Temporal workflows. Use alternative monitoring approaches for other workflow engines.

### Debugging Tasks

Enable task debugging in your workflow configuration:

```yaml
logging:
  level: debug
```

Check workflow context to understand task inputs and state.
