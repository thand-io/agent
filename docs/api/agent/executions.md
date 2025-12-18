---
layout: default
title: Execution Management
parent: Agent
grand_parent: API Reference
nav_order: 8
---

# Execution Management

Manage workflow executions and their lifecycle.

## List Workflow Executions

Get all running workflow executions for the authenticated user.

**GET** `/executions`

### Availability

- Server Mode Only

### Response

```json
{
  "executions": [
    {
      "workflow_id": "wf_abc123",
      "run_id": "run_456789",
      "status": "running",
      "start_time": "2024-01-15T10:30:00Z",
      "execution_time": "PT5M",
      "task_queue": "thand-task-queue",
      "user": "alice@example.com",
      "workflow_type": {
        "name": "ElevateWorkflow"
      }
    }
  ]
}
```

## Create Workflow Execution

**POST** `/execution`

### Availability

- Server Mode Only

### Request Body

```json
{
  "workflow": "default",
  "input": {
    "role": "admin",
    "provider": "aws",
    "reason": "Emergency maintenance"
  }
}
```

### Response

```json
{
  "workflow_id": "wf_abc123",
  "run_id": "run_456789",
  "status": "running"
}
```

### Notes

- Only available in server mode
- Requires authentication
- Workflow must exist and be enabled
- Input is validated against workflow schema

## Get Execution Details

**GET** `/execution/{id}`

### Response

```json
{
  "workflow_id": "wf_abc123",
  "run_id": "run_456789",
  "status": "running",
  "start_time": "2024-01-15T10:30:00Z",
  "execution_time": "PT5M",
  "task_queue": "thand-task-queue",
  "user": "alice@example.com",
  "workflow_type": {
    "name": "ElevateWorkflow"
  },
  "history": [
    {
      "event_type": "WorkflowExecutionStarted",
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ]
}
```

## Cancel Execution

**GET** `/execution/{id}/cancel`

Gracefully cancel a running workflow execution.

### Response

```json
{
  "status": "ok",
  "message": "Workflow cancellation signal sent"
}
```

### Example Usage

```bash
curl "http://localhost:8080/api/v1/execution/wf_abc123/cancel"
```

### Notes

- Only available in server mode
- Requires authentication
- User must own the workflow execution
- Allows workflow to perform cleanup before stopping
- Workflow receives cancellation signal and can handle gracefully

## Terminate Execution

**GET** `/execution/{id}/terminate`

Forcefully terminate a running workflow execution.

### Response

```json
{
  "status": "ok",
  "message": "Workflow termination signal sent"
}
```

### Notes

- Only available in server mode
- User must own the workflow execution
- Forceful termination doesn't allow cleanup
- Use cancel for graceful shutdown

## Signal Execution

Send a signal event to a running workflow execution.

**GET** `/execution/{id}/signal`

### Query Parameters

- `input` - Required. Encoded signal data containing the CloudEvents signal

### Response

```json
{
  "message": "Signal sent successfully",
  "workflow_id": "wf_abc123"
}
```

### Example Usage

```bash
curl "http://localhost:8080/api/v1/execution/wf_abc123/signal?input=encrypted_signal_token"
```

### Notes

- Only available in server mode
- Requires authentication
- User must own the workflow execution
- Input must be encrypted CloudEvents signal data
- Used for workflow approvals and interactive decisions
- Signal data is validated before being sent to workflow
