# GCP Identity-Aware Proxy (IAP) Authentication

This document explains how to configure your application to authenticate users via GCP Identity-Aware Proxy.

## Overview

When your application is deployed behind [GCP Identity-Aware Proxy](https://cloud.google.com/iap), IAP handles user authentication and adds a signed JWT to every request in the `X-Goog-IAP-JWT-Assertion` header. Your application can verify this JWT to:

1. Ensure requests are coming through IAP (not bypassing it)
2. Extract verified user information (email, subject/user ID)
3. Check access levels and device policies if configured

## How It Works

1. User accesses your application through IAP
2. IAP authenticates the user (via Google Account or external IdP)
3. IAP adds `X-Goog-IAP-JWT-Assertion` header with signed JWT
4. Your application verifies the JWT signature using Google's public keys
5. User information is extracted and a session is created

**Note:** The `__Host-GCP_IAP_AUTH_TOKEN_` cookie you see is IAP's internal session cookie. Your application should NOT try to decode this cookie. Instead, use the JWT header that IAP provides.

## Configuration

### 1. Get Your IAP Audience

The audience claim in the JWT identifies your specific backend service. You need to configure this in your provider settings.

#### Using Google Cloud Console

1. Go to [Security > Identity-Aware Proxy](https://console.cloud.google.com/security/iap)
2. Find your resource (backend service, App Engine app, etc.)
3. Click **More** (⋮) next to the resource
4. Select **Signed Header JWT Audience**
5. Copy the audience string

#### Using gcloud CLI

For **Compute Engine/GKE** backend services:

```bash
# Get project number
gcloud projects describe PROJECT_ID --format="value(projectNumber)"

# Get service ID
gcloud compute backend-services describe SERVICE_NAME \
  --project=PROJECT_ID \
  --global \
  --format="value(id)"

# Audience format: /projects/PROJECT_NUMBER/global/backendServices/SERVICE_ID
```

For **App Engine**:

```bash
# Get project number
gcloud projects describe PROJECT_ID --format="value(projectNumber)"

# Audience format: /projects/PROJECT_NUMBER/apps/PROJECT_ID
```

For **Cloud Run**:

```bash
# Audience format: /projects/PROJECT_NUMBER/locations/REGION/services/SERVICE_NAME
```

### 2. Configure Your Application

Add the GCP IAP provider to your `config.yaml`:

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    description: "GCP IAP Authentication"
    enabled: true
    config:
      audience: "/projects/123456789/global/backendServices/567890"
```

The provider will automatically:
- Check for the `X-Goog-IAP-JWT-Assertion` header on incoming requests
- Verify the JWT signature using Google's public keys
- Validate the audience, issuer, and expiration
- Create a session with user information

**Note:** Unlike the old configuration method, you now configure IAP as a **provider** rather than a server-level setting. This allows for better integration with the provider system and supports multiple authentication methods.

Or set it via environment variables:

```bash
export THAND_PROVIDERS_GCP_IAP_PROVIDER="gcp.iap"
export THAND_PROVIDERS_GCP_IAP_ENABLED="true"
export THAND_PROVIDERS_GCP_IAP_CONFIG_AUDIENCE="/projects/123456789/global/backendServices/567890"
```

## Testing

### 1. Verify IAP is Working

When IAP is properly configured, every request will have the JWT header:

```bash
# From within your app, log the headers
curl -H "X-Goog-IAP-JWT-Assertion: ..." http://localhost:5225/api/endpoint
```

### 2. Test JWT Verification

The application will automatically:
- Extract the JWT from `X-Goog-IAP-JWT-Assertion` header
- Verify signature against Google's public keys
- Validate issuer, audience, and expiration
- Create a session with user information

You can check the logs for:
```
"User authenticated via GCP IAP" email="user@example.com" subject="accounts.google.com:1234567890"
```

### 3. Test Invalid JWT

IAP provides a test mode to verify your validation logic. Add `?gcp-iap-mode=SECURE_TOKEN_TEST` to your URL:

```
https://your-app.example.com/path?gcp-iap-mode=SECURE_TOKEN_TEST
```

IAP will send an invalid JWT, and your application should reject it.

## Security Considerations

### Why Verify the JWT?

Even though IAP is protecting your application, you should still verify the JWT because:

1. **IAP could be accidentally disabled** - JWT verification provides defense-in-depth
2. **Misconfigured firewalls** - Someone might access your backend directly
3. **Internal threats** - Users within your project/organization might bypass IAP

### Health Checks

Health check requests from GCP Load Balancers don't include JWT headers. Make sure your health check endpoint doesn't require authentication:

```yaml
server:
  health:
    path: "/health"  # This endpoint is exempt from auth requirements
```

### JWT Lifetime
 // or whatever you named your provider

user := iapSession.User
// user.Email = "user@example.com"
// user.ID = "accounts.google.com:1234567890" (unique, stable identifier)
// user.Source = "gcp.
## User Information

Once authenticated, user information is available via the session:

```go
sessions := c.MustGet(SessionContextKey).(map[string]*models.Session)
iapSession := sessions["gcp-iap"]

user := iapSession.User
// user.Email = "user@example.com"
// user.ID = "accounts.google.com:1234567890" (unique, stable identifier)
// user.Source = "gcp-iap"
```

### External Identities

If you're using IAP with external identity providers (via Identity Platform), the JWT will include additional claims in the `gcip` field with provider-specific information.

## Troubleshooting

### "IAP JWT found but no GCP IAP provider configured"

**Problem:** Application sees the JWT header but no GCP IAP provider is configured.

**Solution:** Add the GCP IAP provider to your configuration as shown above.

### "Failed to verify IAP JWT: invalid audience"

**Problem:** The audience in the JWT doesn't match your configuration.

**Solution:** 
1. Verify you copied the correct audience from Cloud Console
2. Make sure you're using the audience for the correct resource (backend service, not the load balancer)
3. Check for typos in the audience string

### "No X-Goog-IAP-JWT-Assertion header found"

**Problem:** The application doesn't see the IAP JWT header.

**Possible causes:**
1. **Not behind IAP** - You're testing locally or IAP isn't enabled
2. **Proxy stripping headers** - A reverse proxy between IAP and your app is removing headers
3. **Direct access** - Traffic is bypassing IAP (check firewall rules)

**Solution:**
1. Ensure IAP is enabled for your resource in Cloud Console
2. Verify firewall rules only allow traffic from IAP (ingress from load balancer)
3. Check if any proxies are modifying headers

### "Only seeing cookies, no JWT header"

**Problem:** You only see `__Host-GCP_IAP_AUTH_TOKEN_` cookie.

**Explanation:** This is expected! The cookie is for IAP's internal use. IAP adds the JWT as a header (`X-Goog-IAP-JWT-Assertion`), not in the cookie. Your application backend should look for the header, not the cookie.

## References

- [GCP IAP Documentation](https://cloud.google.com/iap/docs)
- [Securing Your App with Signed Headers](https://cloud.google.com/iap/docs/signed-headers-howto)
- [IAP JWT Structure](https://cloud.google.com/iap/docs/signed-headers-howto#retrieving_the_user_identity)
