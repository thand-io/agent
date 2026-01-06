# GCP IAP Provider

This provider enables authentication via Google Cloud Platform's Identity-Aware Proxy (IAP).

## Overview

The GCP IAP provider validates JWT tokens from IAP and creates user sessions. When your application is behind IAP, this provider:

1. Extracts the `X-Goog-IAP-JWT-Assertion` header from incoming requests
2. Verifies the JWT signature using Google's public keys
3. Validates the audience, issuer, and expiration
4. Creates a session with the authenticated user's information

## Configuration

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    description: "GCP IAP Authentication"
    enabled: true
    config:
      audience: "/projects/PROJECT_NUMBER/global/backendServices/SERVICE_ID"
```

### Getting Your Audience

#### Option 1: Cloud Console
1. Go to **Security > Identity-Aware Proxy**
2. Click **More** (⋮) next to your resource
3. Select **Signed Header JWT Audience**
4. Copy the audience string

#### Option 2: gcloud CLI

```bash
# Get project number
PROJECT_NUMBER=$(gcloud projects describe PROJECT_ID --format="value(projectNumber)")

# Audience formats:
# App Engine:     /projects/${PROJECT_NUMBER}/apps/${PROJECT_ID}
# Compute/GKE:    /projects/${PROJECT_NUMBER}/global/backendServices/${SERVICE_ID}
# Cloud Run:      /projects/${PROJECT_NUMBER}/locations/${REGION}/services/${SERVICE_NAME}
```

## How It Works

1. User accesses your application through IAP
2. IAP authenticates the user and adds the JWT header
3. The middleware checks for IAP providers
4. The provider validates the JWT and creates a session
5. The session is available in the request context

## Session Information

The provider creates sessions with:
- **User ID**: Stable identifier from JWT `sub` claim
- **Email**: User's email address
- **Source**: Set to `gcp.iap`
- **Verified**: Always `true` (IAP has verified the user)
- **Expiry**: JWT expiration time (typically 10 minutes)

## Usage

Access user information in your handlers:

```go
sessions := c.MustGet(daemon.SessionContextKey).(map[string]*models.Session)
if iapSession, exists := sessions["gcp-iap"]; exists {
    user := iapSession.User
    email := user.Email
    userID := user.ID
}
```

## Security

### Defense in Depth

Even though IAP protects your application, JWT verification provides additional security:
- Protects against accidentally disabled IAP
- Guards against firewall misconfigurations
- Prevents unauthorized internal access

### Token Lifecycle

- IAP JWTs are short-lived (max 10 minutes)
- Tokens cannot be renewed programmatically
- Users must re-authenticate through IAP for new tokens

## Debugging

Enable debug logging to see all headers:

```yaml
logging:
  level: "debug"
```

When the IAP JWT header is not found, the provider logs all request headers to help troubleshoot configuration issues.

## Limitations

1. **No programmatic authorization**: The `AuthorizeSession` method is not supported because authorization happens at the IAP layer
2. **No token renewal**: `RenewSession` is not supported; users must re-authenticate through IAP
3. **Short-lived sessions**: Sessions expire with the JWT (typically 10 minutes)

# Links

https://discuss.google.dev/t/cannot-authenticate-to-iap-when-using-desktop-oauth-client/189332/2

