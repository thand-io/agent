---
layout: default
title: OAuth2 Provider
description: Generic OAuth2 provider for authentication with various services
parent: Providers
grand_parent: Configuration
---

# OAuth2 Provider

The OAuth2 provider enables browser-based sign-in against a generic OAuth2 or OpenID Connect provider. Use it when you need a flexible OIDC-capable provider without relying on a built-in integration such as `oauth2.google`.

## Capabilities

- **Authentication**: Interactive OAuth2 authorization code flow
- **Generic Integration**: Works with providers that expose standard OIDC discovery or standard authorization/token endpoints
- **Identity Discovery**: Builds user identities from an ID token or a compatible `userinfo` endpoint
- **OIDC Discovery**: Can derive OAuth2 endpoints from an OIDC discovery document
- **Customizable Endpoints**: Lets you override discovered endpoints explicitly when needed

## Prerequisites

### OAuth2 Service Setup

1. **OAuth2 Service**: Access to an OAuth2 or OpenID Connect provider
2. **Application Registration**: Registered application with that provider
3. **Client Credentials**: Client ID and client secret from the provider
4. **Redirect URI**: A redirect URI registered for your agent login callback

### Required OAuth2 Configuration

- **Authority or Endpoints**: Either an OIDC authority URL or explicit authorization and token endpoint URLs
- **Client ID**: OAuth2 application client identifier
- **Client Secret**: OAuth2 application client secret

## Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `client_id` | string | Yes | - | OAuth2 client ID |
| `client_secret` | string | Yes | - | OAuth2 client secret |
| `authority` | string | Yes* | - | OIDC issuer URL or full `/.well-known/openid-configuration` URL |
| `auth_url` | string | No | Derived from discovery | Explicit authorization endpoint override |
| `token_url` | string | No | Derived from discovery | Explicit token endpoint override |
| `userinfo_url` | string | No | Derived from discovery | Explicit userinfo endpoint override |
| `redirect_url` | string | No | - | Default redirect URI if one is not supplied at login time |
| `scopes` | array | No | `["openid"]` | Scopes requested during authorization |

\* Set either `authority`, or both `auth_url` and `token_url`.

## Behavior Notes

- This provider is built around the OAuth2 authorization code flow used for browser login.
- If `authority` is set, the provider fetches the OIDC discovery document lazily at login time and reads `authorization_endpoint`, `token_endpoint`, and `userinfo_endpoint` from it.
- `authority` accepts either an issuer base URL such as `https://auth.example.com/realms/demo` or a full discovery URL ending in `/.well-known/openid-configuration`.
- Explicit `auth_url`, `token_url`, and `userinfo_url` values override the discovered defaults when they are set.
- The provider always includes the `openid` scope during authorization if it is missing from the requested scope list.
- User details are read from the returned `id_token` when present. If no ID token is returned, the provider tries the resolved `userinfo` endpoint and finally falls back to deriving one from `token_url` by replacing `/token` with `/userinfo`.

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
      authority: https://auth.example.com
      redirect_url: https://agent.example.com/auth/callback
      scopes:
        - openid
        - profile
        - email
```

### OIDC Discovery With Explicit Overrides

```yaml
version: "1.0"
providers:
  internal-oidc:
    name: Internal OIDC
    description: OIDC authentication with an internal token and userinfo path override
    provider: oauth2
    enabled: true
    config:
      client_id: YOUR_CLIENT_ID
      client_secret: YOUR_CLIENT_SECRET
      authority: https://auth.example.com/realms/team-a
      token_url: https://auth.internal.example.com/realms/team-a/protocol/openid-connect/token
      userinfo_url: https://auth.internal.example.com/realms/team-a/protocol/openid-connect/userinfo
      scopes:
        - openid
        - profile
        - email
```

If you only need Google sign-in, prefer `oauth2.google`, which already includes Google's endpoint configuration.
