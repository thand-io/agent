---
layout: default
title: Identity Management
parent: Agent
grand_parent: API Reference
nav_order: 9
---

# Identity Management

Manage user and group identities across providers.

## List Identities

Get available identities (users/groups) from identity providers.

**GET** `/identities`

### Availability

- Server Mode Only

### Query Parameters

- `q` - Search filter for identity name or email

### Response

```json
{
  "identities": [
    {
      "id": "alice@example.com",
      "label": "Alice Smith",
      "user": {
        "email": "alice@example.com",
        "name": "Alice Smith",
        "id": "alice@example.com"
      }
    },
    {
      "id": "developers",
      "label": "Development Team",
      "group": {
        "name": "Development Team",
        "id": "developers"
      }
    }
  ],
  "providers": 2
}
```

### Notes

- Requires authentication
- Aggregates identities from all configured identity providers
- Removes duplicates across providers
- Respects provider permissions for the authenticated user
