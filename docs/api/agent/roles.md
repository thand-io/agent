---
layout: default
title: Role Management
parent: Agent
grand_parent: API Reference
nav_order: 6
---

# Role Management

Manage access roles and their configurations.

## List Roles

Get all available roles with optional provider filtering.

**GET** `/roles`

### Availability

- Server Mode (via `/api/v1/roles`)
- Agent Mode (via `/roles`)

### Query Parameters

- `provider` - Filter by provider (comma-separated)

### Example Usage

```bash
# Get all roles
curl http://localhost:8080/api/v1/roles

# Filter by providers
curl "http://localhost:8080/api/v1/roles?provider=aws,gcp"
```

### Response

```json
{
  "version": "1.0",
  "roles": {
    "admin": {
      "role": {
        "name": "admin",
        "description": "Administrative access to all systems",
        "providers": ["aws", "gcp"],
        "workflows": ["default", "emergency"],
        "enabled": true,
        "permissions": {
          "users": ["alice@example.com", "bob@example.com"],
          "groups": ["admins"]
        }
      }
    },
    "developer": {
      "role": {
        "name": "developer",
        "description": "Development environment access",
        "providers": ["aws"],
        "workflows": ["development"],
        "enabled": true,
        "permissions": {
          "groups": ["developers"]
        }
      }
    }
  }
}
```

### Notes

- Only available in server mode
- Requires authentication
- Filters roles based on user permissions
- Only returns enabled roles
- Supports both JSON and HTML responses

## Get Role Details

**GET** `/role/{role}`

### Response

```json
{
  "name": "admin",
  "description": "Administrative access to all systems",
  "providers": ["aws", "gcp"],
  "workflows": ["default", "emergency"],
  "enabled": true,
  "permissions": {
    "users": ["alice@example.com"],
    "groups": ["admins"]
  }
}
```

### Notes

- Only available in server mode
- Requires authentication
- User must have permission to view the role
- Returns complete role configuration including permissions and workflows
