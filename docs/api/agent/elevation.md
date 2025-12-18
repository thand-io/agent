---
layout: default
title: Elevation (Access Requests)
parent: Agent
grand_parent: API Reference
nav_order: 4
---

# Elevation (Access Request) Endpoints

Request elevated access to resources through approval workflows.

## Static Elevation Request

Request elevated access using predefined roles.

**GET** `/elevate`

### Availability

- Server Mode Only

### Query Parameters

- `role` - Role name (required)
- `provider` - Target provider (required)
- `workflow` - Workflow name (optional, uses role default if not specified)
- `reason` - Justification for access (required)
- `duration` - Access duration in ISO 8601 format (optional)
- `identities` - Comma-separated list of identities to elevate (optional)
- `session` - Encoded session token (optional)

### Response

```json
{
  "status": "pending",
  "output": {
    "workflow_id": "wf_abc123",
    "execution_id": "exec_456789"
  }
}
```

## Dynamic Elevation Request

Submit elevation request via JSON or form data.

**POST** `/elevate`

### Availability

- Server Mode Only

### Request Body (JSON - Static Request)

```json
{
  "role": {
    "name": "admin",
    "description": "Administrative access",
    "providers": ["aws", "gcp"],
    "workflows": ["default"],
    "enabled": true
  },
  "providers": ["aws"],
  "authenticator": "aws",
  "workflow": "default",
  "reason": "Emergency maintenance required",
  "duration": "PT2H",
  "identities": ["alice@example.com"],
  "session": {
    "version": 1,
    "expiry": "2024-01-15T12:30:00Z",
    "session": "encrypted_token"
  }
}
```

### Request Body (Form Data - Dynamic Request)

```
authenticator=aws
workflow=default
reason=Emergency maintenance
duration=PT2H
identities=alice@example.com
providers=aws,gcp
permissions=ec2:*,s3:GetObject
resources=arn:aws:ec2:*:*:instance/*
groups=admins
users=alice@example.com
```

### Response

```json
{
  "status": "pending",
  "output": {
    "workflow_id": "wf_abc123",
    "execution_id": "exec_456789"
  }
}
```

## LLM-Assisted Elevation

Request access using natural language description.

**GET** `/elevate/llm`

Returns HTML form for LLM-assisted elevation.

**POST** `/elevate/llm`

### Request Body

```json
{
  "reason": "I need to check the EC2 instances in production to investigate high CPU usage alerts"
}
```

### Response

```json
{
  "status": "pending",
  "output": {
    "suggested_role": "ec2-read-only",
    "suggested_duration": "PT1H",
    "suggested_providers": ["aws"],
    "workflow_id": "wf_abc123"
  }
}
```

## Resume Elevation Workflow

Resume a paused or interrupted elevation workflow.

**GET** `/elevate/resume`

### Query Parameters

- `state` - Required. Encrypted workflow state token

### Response

**Redirect (307)**: Redirects to next workflow step or completion page.

### Example Usage

```bash
curl -L "http://localhost:8080/api/v1/elevate/resume?state=encrypted_state_token"
```

**POST** `/elevate/resume`

### Query Parameters

- `state` - Optional. If provided, behaves like GET request

### Request Body

Raw encrypted workflow state or task token for resuming workflows.

### Response

**Redirect (307)**: Redirects to next workflow step.

**JSON Response** (if Accept: application/json):
```json
{
  "workflow_id": "wf_abc123",
  "status": "completed",
  "output": {
    "approved": true,
    "session_created": true
  }
}
```

### Notes

- Used internally by workflow engine to resume paused workflows
- State parameter contains encrypted workflow context
- Supports both query parameter and body-based resumption
