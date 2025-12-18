---
layout: default
title: Workflow Management
parent: Agent
grand_parent: API Reference
nav_order: 7
---

# Workflow Management

Manage approval workflows and their execution.

## List Workflows

Get all available workflows.

**GET** `/workflows`

### Availability

- Server Mode (via `/api/v1/workflows`)
- Agent Mode (via `/workflows`)

### Response

```json
{
  "version": "1.0",
  "workflows": {
    "default": {
      "name": "default",
      "description": "Standard approval workflow",
      "enabled": true
    },
    "emergency": {
      "name": "emergency",
      "description": "Emergency bypass workflow",
      "enabled": true
    }
  }
}
```

### Notes

- Requires authentication in server mode
- Returns only enabled workflows
- Filters workflows based on user permissions
- Supports both JSON and HTML responses

## Get Workflow Details

**GET** `/workflow/{name}`

### Response

```json
{
  "name": "default",
  "description": "Standard approval workflow",
  "enabled": true,
  "document": {
    "dsl": "1.0.0",
    "namespace": "thand",
    "name": "default",
    "version": "1.0.0",
    "do": [
      {
        "request": {
          "call": "http",
          "with": {
            "method": "post",
            "uri": "https://api.example.com/approve"
          }
        }
      }
    ]
  }
}
```

### Notes

- Only available in server mode
- Requires authentication
- Returns complete workflow definition including ServerlessWorkflow DSL
- User must have permission to view the workflow
- Supports both JSON and HTML responses