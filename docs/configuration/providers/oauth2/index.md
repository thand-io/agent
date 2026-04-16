---
layout: default
title: OAuth2 Provider
description: Generic OAuth2 provider for authentication with various services
parent: Providers
grand_parent: Configuration
---

# OAuth2 Provider

The OAuth2 provider enables browser-based sign-in against a generic OAuth2 or OpenID Connect provider. Use it when you need to supply custom authorization and token endpoint URLs instead of relying on a built-in provider such as `oauth2.google`.

## Capabilities

- **Authentication**: Interactive OAuth2 authorization code flow
- **Generic Integration**: Works with providers that expose standard authorization and token endpoints
- **Identity Discovery**: Builds user identities from an ID token or a compatible `userinfo` endpoint
- **Customizable Endpoints**: Lets you provide full authorization and token URLs directly

## Prerequisites

### OAuth2 Service Setup

1. **OAuth2 Service**: Access to an OAuth2 or OpenID Connect provider
2. **Application Registration**: Registered application with that provider
3. **Client Credentials**: Client ID and client secret from the provider
4. **Redirect URI**: A redirect URI registered for your agent login callback

### Required OAuth2 Configuration

- **Authorization Endpoint**: Full authorization URL
- **Token Endpoint**: Full token exchange URL
- **Client ID**: OAuth2 application client identifier
- **Client Secret**: OAuth2 application client secret

## Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `client_id` | string | Yes | - | OAuth2 client ID |
| `client_secret` | string | Yes | - | OAuth2 client secret |
| `auth_url` | string | Yes* | - | Full authorization endpoint URL |
| `token_url` | string | Yes* | - | Full token endpoint URL |
| `redirect_url` | string | No | - | Default redirect URI if one is not supplied at login time |
| `scopes` | array | No | `["openid"]` | Scopes requested during authorization |

\* The schema accepts omitted values, but the generic provider does not supply defaults. In practice you should set both `auth_url` and `token_url`.

## Behavior Notes

- This provider is built around the OAuth2 authorization code flow used for browser login.
- The provider always includes the `openid` scope during authorization if it is missing from the requested scope list.
- User details are read from the returned `id_token` when present. If no ID token is returned, the provider tries a `userinfo` endpoint derived from `token_url` by replacing `/token` with `/userinfo`.
- This provider does not perform OIDC discovery. You must configure the endpoint URLs explicitly.

## Example Configurations

### Generic OAuth2 / OIDC Service

```yaml
version: "1.0"
providers:
  oauth2-service:
    name: OAuth2 Service
    description: Generic OAuth2 authentication
    provider: oauth2
    enabled: true
    config:
      client_id: YOUR_CLIENT_ID
      client_secret: YOUR_CLIENT_SECRET
      auth_url: https://auth.example.com/oauth2/authorize
      token_url: https://auth.example.com/oauth2/token
      redirect_url: https://agent.example.com/auth/callback
      scopes:
        - openid
        - profile
        - email
```

### Google via Generic OAuth2

```yaml
version: "1.0"
providers:
  google-oauth2:
    name: Google OAuth2
    description: Google OAuth2 authentication
    provider: oauth2
    enabled: true
    config:
      client_id: YOUR_GOOGLE_CLIENT_ID.apps.googleusercontent.com
      client_secret: YOUR_GOOGLE_CLIENT_SECRET
      auth_url: https://accounts.google.com/o/oauth2/v2/auth
      token_url: https://oauth2.googleapis.com/token
      scopes:
        - openid
        - profile
        - email
```

If you only need Google sign-in, prefer `oauth2.google`, which already includes Google's endpoint configuration.
