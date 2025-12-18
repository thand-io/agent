---
layout: default
title: Session Management
parent: Agent
grand_parent: API Reference
nav_order: 3
---

# Session Management

Manage user sessions across providers.

## List Sessions

Get all active sessions for the authenticated user.

**GET** `/sessions`

### Availability

- Server Mode
- Agent Mode
- Client Mode

### Response (Server Mode)

```json
{
  "version": "1",
  "timestamp": "2024-01-15T10:30:00Z",
  "sessions": {
    "aws": {
      "version": 1,
      "expiry": "2024-01-15T12:30:00Z"
    },
    "gcp": {
      "version": 1,
      "expiry": "2024-01-15T11:45:00Z"
    }
  }
}
```

### Response (Agent Mode)

```json
{
  "version": "1",
  "timestamp": "2024-01-15T10:30:00Z",
  "sessions": {
    "aws": {
      "version": 1,
      "expiry": "2024-01-15T12:30:00Z",
      "session": "encrypted_session_token"
    }
  }
}
```

## Get Session by Provider

Get session details for a specific provider.

**GET** `/session/{provider}`

### Availability

- Agent Mode
- Client Mode

### Response

```json
{
  "session": {
    "version": 1,
    "expiry": "2024-01-15T12:30:00Z",
    "session": "encrypted_session_token"
  }
}
```

## Create Session

Create a new session from an encoded session token.

**POST** `/sessions`

### Availability

- Agent Mode
- Client Mode

### Request Body

```json
{
  "code": "encrypted_verification_code",
  "provider": "aws",
  "session": "encoded_session_token"
}
```

### Response

```json
{
  "message": "Session created successfully",
  "expiry": "2024-01-15T12:30:00Z"
}
```

## Delete Session

Delete a session for a specific provider.

**DELETE** `/session/{provider}`

### Availability

- Agent Mode
- Client Mode

### Response

```json
{
  "message": "Session deleted successfully"
}
```

### Notes

- Only available in agent mode
- Removes session from local session manager
- Does not revoke tokens at provider
- Session is permanently deleted from local storage
