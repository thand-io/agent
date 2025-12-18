---
layout: default
title: Authentication
parent: Agent
grand_parent: API Reference
nav_order: 2
---

# Authentication Endpoints

OAuth2 authentication flows and session management.

## Auth Page

Display authentication page with available providers.

**GET** `/auth`

### Query Parameters

- `callback` - Callback URL after authentication (optional)
- `provider` - Provider name for direct authentication (optional)

### Response

**HTML Response**: Returns authentication page with list of available providers.

**Redirect Response**: If `provider` parameter is specified, redirects to:
```
/api/v1/auth/request/{provider}?callback={callback}
```

### Example Usage

```bash
# Show auth page with all providers
curl http://localhost:8080/auth

# Direct authentication with specific provider
curl "http://localhost:8080/auth?provider=aws&callback=http://localhost:8080"
```

### Notes

- Available in both agent and server modes
- In agent mode, redirects to login server for authentication
- In server mode, displays local authentication page

## Auth Request

Initiate OAuth2 authentication flow for a provider.

**GET** `/auth/request/{provider}`

### Availability

- Server Mode Only

### Query Parameters

- `callback` - Callback URL after authentication

### Response

Redirects to provider's OAuth2 authorization URL.

## Auth Callback

Handle OAuth2 callback from provider.

**GET** `/auth/callback/{provider}`

### Availability

- Server Mode Only

### Query Parameters

- `code` - Authorization code from provider
- `state` - State parameter for CSRF protection

### Response

Redirects to callback URL or shows success page.

## Logout

Clear authentication session.

**GET** `/auth/logout/{provider}` or `/auth/logout`

### Availability

- Server Mode Only

Clears authentication session for specific provider or all providers.
