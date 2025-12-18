---
layout: default
title: Audit and Logging
parent: Agent
grand_parent: API Reference
nav_order: 10
---

# Audit and Logging

Access system logs and audit events.

## Get Logs

Get system logs and audit events.

**GET** `/logs`

### Availability

- Server Mode
- Agent Mode
- Client Mode

### Response

```json
{
  "logs": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "level": "info",
      "message": "User alice@example.com requested elevation to admin role",
      "provider": "aws",
      "user": "alice@example.com",
      "workflow_id": "wf_abc123"
    }
  ]
}
```

### Notes

- Requires authentication in server mode
- Returns up to 500 recent log entries
- Includes system events, user actions, and audit trail
- Available in both JSON and HTML formats

## Submit Logs

Submit logs to the server.

**POST** `/logs`

### Availability

- Server Mode Only

### Notes

- Currently a stub endpoint.
