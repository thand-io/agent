---
layout: default
title: Google OAuth2
description: Google OAuth2 provider for Google account authentication
parent: Providers
grand_parent: Configuration
---

# Google OAuth2 Provider

The Google OAuth2 provider enables authentication with Google accounts using Google's standard OAuth2 endpoints. Use this provider when you want Google sign-in without manually configuring `auth_url` and `token_url`.

## Capabilities

- **Authentication**: Google account OAuth2 authentication
- **Google User Info**: Fetches profile information from Google's OAuth2 userinfo API
- **Built-In Endpoints**: Uses Google's OAuth2 authorization and token endpoints automatically
- **Scope Management**: Configurable OAuth2 scopes with provider defaults

## Prerequisites

1. Create an OAuth 2.0 client in Google Cloud for a web application.
2. Register the redirect URI that your agent uses for Google sign-in.
3. Copy the generated client ID and client secret into the provider config.

## Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `client_id` | string | Yes | - | Google OAuth2 client ID |
| `client_secret` | string | Yes | - | Google OAuth2 client secret |
| `scopes` | array | No | `["email", "profile"]` | Scopes to request during authorization |

## Behavior Notes

- The provider uses Google's built-in OAuth2 endpoints; you do not need to set `auth_url` or `token_url`.
- If `scopes` is omitted, the provider defaults to `email` and `profile`.
- This provider does not currently expose a `hosted_domain` or domain restriction setting in its config schema.

## Example Configuration

```yaml
version: "1.0"
providers:
  google-oauth:
    name: Google OAuth2
    description: Google account authentication
    provider: oauth2.google
    enabled: true
    config:
      client_id: YOUR_GOOGLE_CLIENT_ID.apps.googleusercontent.com
      client_secret: YOUR_GOOGLE_CLIENT_SECRET
      scopes:
        - openid
        - profile
        - email
```

### Getting Google OAuth2 Credentials

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create or select a project
3. Configure the OAuth consent screen
4. Go to Credentials and create an OAuth 2.0 Client ID for a web application
5. Add your agent's authorized redirect URI
6. Copy the client ID and client secret into the provider configuration

For detailed setup instructions, refer to the [Google OAuth2 documentation](https://developers.google.com/identity/protocols/oauth2/).
