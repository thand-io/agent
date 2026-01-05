---
layout: default
title: GCP IAP
description: Google Cloud Platform Identity-Aware Proxy authentication provider
parent: Providers
grand_parent: Configuration
---

# GCP IAP Provider

The GCP IAP (Identity-Aware Proxy) provider enables authentication when your application is deployed behind Google Cloud's Identity-Aware Proxy. IAP handles user authentication and adds a signed JWT to each request, which this provider verifies to establish user sessions.

## Overview

When your application is behind IAP:
1. User accesses your application through IAP
2. IAP authenticates the user (via Google Account or external IdP)
3. IAP adds `X-Goog-IAP-JWT-Assertion` header with signed JWT to the request
4. The provider verifies the JWT signature and creates a user session

## Capabilities

- **Authentication**: Verifies IAP JWT tokens and creates user sessions
- **Identity Management**: Extracts user email, subject ID, and hosted domain
- **Defense in Depth**: Provides secondary verification even if IAP is bypassed
- **Automatic Token Validation**: Checks signature, audience, issuer, and expiration

## Prerequisites

### GCP Setup

1. **GCP Project**: Active Google Cloud Platform project
2. **IAP-Protected Resource**: Application deployed on Cloud Run, App Engine, Compute Engine, or GKE
3. **IAP Enabled**: Identity-Aware Proxy must be enabled for your resource
4. **OAuth Consent Screen**: Configured for user authentication

### Required Permissions

The application itself doesn't need special GCP permissions - IAP handles authentication. However, to set up IAP, you need:

```
iap.web.getIamPolicy
iap.web.setIamPolicy
compute.backendServices.update (for GCE/GKE)
run.services.getIamPolicy (for Cloud Run)
```

## Configuration

### Basic Configuration

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    description: "GCP IAP Authentication"
    enabled: true
    config:
      # REQUIRED: JWT audience claim for validating incoming IAP JWTs
      audience: "/projects/PROJECT_NUMBER/locations/REGION/services/SERVICE_NAME"
      
      # REQUIRED for programmatic access: OAuth 2.0 client ID for generating user tokens
      oauth_client_id: "XXXXX.apps.googleusercontent.com"
      oauth_client_secret: "YOUR_CLIENT_SECRET"
```

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `audience` | string | Yes | JWT audience claim for validating incoming IAP JWTs from the backend service |
| `oauth_client_id` | string | Yes* | OAuth 2.0 client ID for generating user ID tokens for programmatic access |
| `oauth_client_secret` | string | No | OAuth client secret (if required by your OAuth client type) |

\* Required if you need to generate tokens that clients can use to authenticate to IAP-protected resources.

## Understanding IAP Authentication Flow

IAP supports two different authentication modes:

### 1. Server-Side Verification (Basic Mode)

This is the traditional IAP flow where your application only validates incoming requests:

1. User accesses your application through IAP
2. IAP authenticates the user and adds `X-Goog-IAP-JWT-Assertion` header
3. Your application verifies the JWT signature
4. User session is created based on JWT claims

**Important**: The IAP JWT cannot be reused to make authenticated requests back to IAP-protected resources. It's only valid for the single incoming request.

### 2. Programmatic Access with OIDC (Full Mode)

For applications that need to provide tokens to clients (e.g., mobile apps, SPAs, CLIs), you must implement an OAuth 2.0 flow to generate OIDC tokens that clients can use:

1. User authenticates through IAP (extracts user identity)
2. Application initiates OAuth flow to generate an OIDC token
3. OIDC token is issued with the OAuth client ID as the audience
4. Client receives this token and can use it to authenticate to IAP-protected resources

This flow is required when you need to:
- Give clients tokens they can use directly
- Make authenticated API calls to IAP-protected services
- Support programmatic access from external applications

**Reference**: [GCP IAP Programmatic Authentication](https://docs.cloud.google.com/iap/docs/authentication-howto#authenticate_a_service_account)

## Getting Configuration Values

### Getting Your Audience String

The audience string uniquely identifies your backend service for JWT validation. It varies by deployment type.

### Method 1: Using Google Cloud Console

1. Go to [IAP Settings](https://console.cloud.google.com/security/iap)
2. Find your resource in the list
3. Click **More** (⋮) next to the resource
4. Select **Signed Header JWT Audience**
5. Copy the audience string

### Method 2: Using gcloud CLI

#### For Cloud Run

```bash
# Get project number
PROJECT_NUMBER=$(gcloud projects describe PROJECT_ID \
  --format="value(projectNumber)")

# Get service details
SERVICE_NAME="your-service-name"
REGION="us-central1"

# Construct audience
echo "/projects/${PROJECT_NUMBER}/locations/${REGION}/services/${SERVICE_NAME}"
```

#### For Compute Engine / GKE

```bash
# Get project number
PROJECT_NUMBER=$(gcloud projects describe PROJECT_ID \
  --format="value(projectNumber)")

# Get backend service ID
SERVICE_ID=$(gcloud compute backend-services describe SERVICE_NAME \
  --global \
  --format="value(id)")

# Construct audience
echo "/projects/${PROJECT_NUMBER}/global/backendServices/${SERVICE_ID}"
```

#### For App Engine

```bash
# Get project details
PROJECT_ID="your-project-id"
PROJECT_NUMBER=$(gcloud projects describe $PROJECT_ID \
  --format="value(projectNumber)")

# Construct audience
echo "/projects/${PROJECT_NUMBER}/apps/${PROJECT_ID}"
```

### Getting Your OAuth Client ID

For programmatic access, you need to create an OAuth 2.0 client and add it to IAP's allowlist.

#### Method 1: From IAP Console

1. Go to [IAP Settings](https://console.cloud.google.com/security/iap)
2. Find your IAP-protected resource
3. Click **Settings** or the three dots menu (⋮)
4. Navigate to **OAuth clients for programmatic access**
5. If you don't have one, click **Create OAuth Client**
6. Copy the **Client ID** (format: `XXXXX.apps.googleusercontent.com`)

#### Method 2: From Credentials Page

1. Go to [APIs & Services > Credentials](https://console.cloud.google.com/apis/credentials)
2. Click **Create Credentials** → **OAuth 2.0 Client ID**
3. Choose application type:
   - **Web application**: For web-based clients
   - **Desktop app**: For CLI tools and desktop applications
4. Configure authorized redirect URIs if needed
5. Copy the **Client ID** and **Client Secret**

#### Method 3: Using gcloud

```bash
# List existing OAuth clients
gcloud iap oauth-clients list \
  --resource-type=app-engine-app \
  --service=SERVICE_NAME

# Or create a new one (requires manual step in console)
# Note: gcloud doesn't directly support OAuth client creation
# Use the console or create via API
```

#### Add OAuth Client to IAP Allowlist

**CRITICAL**: After creating the OAuth client, you **MUST** add it to IAP's programmatic access allowlist. This is a required step that configures IAP to accept tokens generated with your OAuth client ID.

**For Load Balancer / GKE / App Engine (UI Method)**:

1. Go to [IAP Settings](https://console.cloud.google.com/security/iap)
2. Select your IAP-protected resource
3. Click **Settings** (gear icon or ⋮ menu)
4. Navigate to the **OAuth Clients** or **OAuth clients for programmatic access** section
5. Click **Add OAuth Client** or **Add**
6. Enter your OAuth client ID (e.g., `123456789-abc123.apps.googleusercontent.com`)
7. Click **Add** or **Save**

**For Cloud Run (Command Line Method)**:

Cloud Run deployments do not support adding OAuth clients via the UI. You must use gcloud commands:

```bash
# Method 1: Update IAP settings with OAuth client
# Create a policy.yaml file:
cat > policy.yaml <<EOF
bindings:
- members:
  - user:user@example.com
  role: roles/iap.httpsResourceAccessor
iap:
  oauth2ClientId: "YOUR_CLIENT_ID.apps.googleusercontent.com"
  oauth2ClientSecret: "YOUR_CLIENT_SECRET"
EOF

# Apply the policy
gcloud iap web set-iam-policy BACKEND_SERVICE_NAME \
  --resource-type=backend-services \
  policy.yaml

# Method 2: Use the oauth-brands API
# Create OAuth brand (if needed)
gcloud iap oauth-brands create \
  --application_title="My Application" \
  --support_email="support@example.com"

# Get brand ID
BRAND_ID=$(gcloud iap oauth-brands list --format="value(name)")

# Add OAuth client
gcloud iap oauth-clients create $BRAND_ID \
  --display_name="Programmatic Access Client"
```

**Important**: 
- Without adding the OAuth client ID to IAP's allowlist, IAP will reject all authentication attempts using tokens generated with that client
- For Load Balancers, this is done through the UI in the IAP console
- For Cloud Run, this requires gcloud commands as shown above
- This is separate from creating the OAuth client - the client must be explicitly added to each IAP-protected resource
- Each IAP-protected resource can have multiple OAuth clients in its allowlist

For more details, see [GCP IAP Programmatic Authentication Documentation](https://docs.cloud.google.com/iap/docs/authentication-howto).

## Setting Up IAP

### Cloud Run

#### 1. Deploy Your Application

```bash
gcloud run deploy SERVICE_NAME \
  --image gcr.io/PROJECT_ID/IMAGE_NAME \
  --region REGION \
  --platform managed \
  --allow-unauthenticated
```

#### 2. Enable IAP and Configure OAuth Client

```bash
# Get the backend service name (created automatically with Cloud Run)
gcloud compute backend-services list

# Enable IAP on the backend service
gcloud iap web enable \
  --resource-type=backend-services \
  --service=SERVICE_NAME
```

#### 3. Add OAuth Client to IAP Settings (Cloud Run Specific)

**Important**: For Cloud Run deployments, you must use gcloud commands to add your OAuth client ID to the IAP settings. Unlike load balancer deployments where you can use the UI, Cloud Run requires the following manual steps:

```bash
# First, create an OAuth client if you haven't already
# Go to Console > APIs & Services > Credentials > Create OAuth Client ID
# Choose "Web application" type
# Note the CLIENT_ID (format: XXXXX.apps.googleusercontent.com)

# Get the backend service that was created for your Cloud Run service
BACKEND_SERVICE=$(gcloud compute backend-services list \
  --filter="name~k8s.*" \
  --format="value(name)")

# Update the IAP settings to include your OAuth client
gcloud iap web set-iam-policy $BACKEND_SERVICE \
  --resource-type=backend-services \
  policy.yaml
```

Create a `policy.yaml` file with your OAuth client configuration:

```yaml
# policy.yaml
bindings:
- members:
  - user:user@example.com
  role: roles/iap.httpsResourceAccessor
iap:
  oauth2ClientId: "YOUR_CLIENT_ID.apps.googleusercontent.com"
  oauth2ClientSecret: "YOUR_CLIENT_SECRET"
```

Or use the `gcloud iap oauth-brands` command to configure programmatic access:

```bash
# Create OAuth brand (if not exists)
gcloud iap oauth-brands create \
  --application_title="My Application" \
  --support_email="support@example.com"

# Add OAuth client for programmatic access
gcloud iap oauth-clients create BRAND_ID \
  --display_name="Programmatic Access Client"
```

**Note**: This step is **required** for the provider to generate tokens for programmatic access. Without adding the OAuth client ID to IAP settings, authentication will fail.

#### 4. Grant User Access

```bash
# Grant specific users access
gcloud iap web add-iam-policy-binding \
  --resource-type=backend-services \
  --service=SERVICE_NAME \
  --member='user:user@example.com' \
  --role='roles/iap.httpsResourceAccessor'

# Or grant a group access
gcloud iap web add-iam-policy-binding \
  --resource-type=backend-services \
  --service=SERVICE_NAME \
  --member='group:team@example.com' \
  --role='roles/iap.httpsResourceAccessor'
```

#### 5. Configure Provider

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "/projects/123456789/locations/us-central1/services/my-service"
      oauth_client_id: "123456789-abc123.apps.googleusercontent.com"
      oauth_client_secret: "GOCSPX-xxxxxxxxxxxxx"
```

**Note**: The `oauth_client_id` and `oauth_client_secret` enable programmatic access, allowing your application to generate tokens that clients can use to authenticate to the IAP-protected service. **You must also add this OAuth client ID to your IAP settings** (Security > Identity-Aware Proxy > Your Resource > Settings > OAuth clients for programmatic access) for authentication to work.

### GKE (Google Kubernetes Engine)

#### 1. Create Ingress with IAP-Enabled Backend Service

```yaml
# backend-config.yaml
apiVersion: cloud.google.com/v1
kind: BackendConfig
metadata:
  name: my-backend-config
spec:
  iap:
    enabled: true
    oauthclientCredentials:
      secretName: oauth-client-secret
```

#### 2. Create OAuth Client Secret

```bash
# Create OAuth client in Cloud Console first, then:
kubectl create secret generic oauth-client-secret \
  --from-literal=client_id=YOUR_CLIENT_ID \
  --from-literal=client_secret=YOUR_CLIENT_SECRET
```

#### 3. Create Service and Ingress

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  annotations:
    cloud.google.com/backend-config: '{"default": "my-backend-config"}'
spec:
  type: NodePort
  selector:
    app: my-app
  ports:
    - port: 80
      targetPort: 5225
---
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
spec:
  rules:
    - host: myapp.example.com
      http:
        paths:
          - path: /*
            pathType: ImplementationSpecific
            backend:
              service:
                name: my-service
                port:
                  number: 80
```

#### 4. Get Backend Service ID and Configure Provider

```bash
# Get backend service ID
gcloud compute backend-services list --filter="name:k8s-*"
SERVICE_ID=$(gcloud compute backend-services describe BACKEND_SERVICE_NAME \
  --global --format="value(id)")

PROJECT_NUMBER=$(gcloud projects describe PROJECT_ID --format="value(projectNumber)")

# Audience will be:
echo "/projects/${PROJECT_NUMBER}/global/backendServices/${SERVICE_ID}"
```

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "/projects/123456789/global/backendServices/567890123456"
      oauth_client_id: "123456789-abc123.apps.googleusercontent.com"
      oauth_client_secret: "GOCSPX-xxxxxxxxxxxxx"
```

### App Engine

#### 1. Enable IAP via Console

1. Go to [IAP Settings](https://console.cloud.google.com/security/iap)
2. Find your App Engine app
3. Toggle IAP to **On**
4. Configure OAuth consent screen if prompted
5. Add authorized users/groups

#### 2. Add OAuth Client to IAP Settings

**Required for programmatic access**:
1. In the IAP console, select your App Engine app
2. Click **Settings** (gear icon or ⋮)
3. Under **OAuth clients for programmatic access**, click **Add OAuth Client**
4. Enter your OAuth client ID (e.g., `123456789-abc123.apps.googleusercontent.com`)
5. Click **Add**

This step configures IAP to accept tokens generated with your OAuth client ID.

#### 3. Configure Provider

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "/projects/123456789/apps/my-project-id"
      oauth_client_id: "123456789-abc123.apps.googleusercontent.com"
      oauth_client_secret: "GOCSPX-xxxxxxxxxxxxx"
```

**Note**: The OAuth client ID specified here must match the one added to IAP's programmatic access allowlist in step 2.

## Security Considerations

### Why Verify the JWT?

Even though IAP protects your application, JWT verification provides defense in depth:

1. **Accidental IAP Disable**: If IAP is accidentally turned off, your app still rejects unauthenticated requests
2. **Firewall Misconfiguration**: Protects against direct access to your backend
3. **Internal Threats**: Prevents unauthorized access from within your project

### JWT Token Lifecycle

- **Lifetime**: IAP JWTs are short-lived (typically 10 minutes maximum)
- **Cannot Be Reused**: IAP JWTs validate only the incoming request and cannot be used to make authenticated requests to IAP-protected resources
- **For Programmatic Access**: Use the OAuth flow to generate OIDC tokens with the OAuth client ID as the audience

### OAuth OIDC Token Lifecycle

When using programmatic access with `oauth_client_id`:

- **Lifetime**: OIDC tokens are typically valid for 1 hour
- **Audience**: Token audience is the OAuth client ID (e.g., `XXXXX.apps.googleusercontent.com`)
- **Usage**: Can be used by clients to make authenticated requests to IAP-protected resources
- **Renewal**: Tokens can be refreshed using the refresh token (if provided)
- **Validation**: IAP validates these tokens against the OAuth client allowlist

## Troubleshooting

### Invalid Bearer Token / Invalid JWT Audience

**Symptoms**: Log shows "Invalid bearer token" or "Invalid JWT audience" when using tokens

**Causes**:
1. Using the IAP JWT to make API calls (IAP JWTs cannot be reused)
2. OAuth client not added to IAP's programmatic access allowlist
3. Wrong OAuth client ID in token audience
4. Trying to use server-side validation audience for programmatic access

**Solution**:
```yaml
# Ensure you have BOTH values configured:
providers:
  gcp-iap:
    provider: gcp.iap
    config:
      # For validating incoming IAP JWTs
      audience: "/projects/123/locations/region/services/name"
      
      # For generating tokens clients can use
      oauth_client_id: "XXXXX.apps.googleusercontent.com"
      oauth_client_secret: "secret"
```

**Then verify the OAuth client is added to IAP's allowlist**:
1. Go to [IAP Settings](https://console.cloud.google.com/security/iap)
2. Select your IAP-protected resource
3. Click **Settings** (gear icon or ⋮)
4. Check the **OAuth clients for programmatic access** section
5. Your OAuth client ID must be listed here
6. If not listed, click **Add OAuth Client** and add it

You can also verify via gcloud:
```bash
# Check IAP settings for your resource
gcloud iap web get-iam-policy \
  --resource-type=backend-services \
  --service=SERVICE_NAME
```

### No JWT Header Found

**Symptoms**: Log shows "No IAP JWT header found"

**Possible Causes**:
1. IAP is not enabled for your resource
2. Testing locally without going through IAP
3. Firewall rules allow direct access, bypassing IAP

**Solution**:
```bash
# Verify IAP is enabled
gcloud iap web get-iam-policy \
  --resource-type=backend-services \
  --service=SERVICE_NAME

# Check firewall rules - should only allow traffic from load balancer
gcloud compute firewall-rules list
```

### JWT Verification Failed

**Symptoms**: "Failed to verify IAP JWT: invalid audience"

**Causes**:
1. Wrong audience string in configuration
2. Using audience from different resource/environment

**Solution**:
```bash
# Get the correct audience from Cloud Console
# Or use gcloud to verify:
gcloud compute backend-services describe SERVICE_NAME \
  --global --format="value(id)"
```

### IAP JWT Found but Provider Not Configured

**Symptoms**: "IAP JWT found but no GCP IAP provider configured"

**Solution**: Add the provider to your configuration:

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "YOUR_AUDIENCE_HERE"
```

### Users Can't Access Application

**Symptoms**: Users get "You don't have access" error

**Solution**: Grant IAP access to users:

```bash
# List current IAP access
gcloud iap web get-iam-policy \
  --resource-type=backend-services \
  --service=SERVICE_NAME

# Grant access to user
gcloud iap web add-iam-policy-binding \
  --resource-type=backend-services \
  --service=SERVICE_NAME \
  --member='user:user@example.com' \
  --role='roles/iap.httpsResourceAccessor'
```

## External Identities

If using IAP with external identity providers (via Identity Platform), the JWT includes additional claims:

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "/projects/123456789/apps/my-app"
    # The provider automatically handles external identities
    # User information is extracted from the gcip claim
```

## Environment Variables

You can configure the provider using environment variables:

```bash
export THAND_PROVIDERS_GCP_IAP_PROVIDER="gcp.iap"
export THAND_PROVIDERS_GCP_IAP_ENABLED="true"
export THAND_PROVIDERS_GCP_IAP_CONFIG_AUDIENCE="/projects/123456789/locations/us-central1/services/my-service"
export THAND_PROVIDERS_GCP_IAP_CONFIG_OAUTH_CLIENT_ID="123456789-abc123.apps.googleusercontent.com"
export THAND_PROVIDERS_GCP_IAP_CONFIG_OAUTH_CLIENT_SECRET="GOCSPX-xxxxxxxxxxxxx"
```

## Best Practices

1. **Configure Both Modes**: Set up both `audience` (for IAP JWT validation) and `oauth_client_id` (for programmatic access)
2. **Allowlist OAuth Clients**: Always add OAuth clients to IAP's programmatic access allowlist
3. **Use Separate Providers**: Create different IAP providers for different environments (dev, staging, prod)
4. **Monitor Access**: Regularly review IAP access logs in Cloud Logging
5. **Limit Access**: Use groups instead of individual users for easier management
6. **Test Locally**: Use alternative authentication methods for local development
7. **Health Checks**: Ensure health check endpoints don't require authentication
8. **Token Security**: Store OAuth client secrets securely (use Secret Manager in production)

## Related Documentation

- [GCP IAP Documentation](https://cloud.google.com/iap/docs)
- [GCP IAP Programmatic Authentication](https://docs.cloud.google.com/iap/docs/authentication-howto)
- [Securing Your App with Signed Headers](https://cloud.google.com/iap/docs/signed-headers-howto)
- [IAP Access Control](https://cloud.google.com/iap/docs/managing-access)
- [Sharing OAuth Clients](https://cloud.google.com/iap/docs/sharing-oauth-clients)
- [Thand IAP Configuration Guide](../../iap)

## Examples

See complete configuration examples:
- [Basic IAP Configuration](../../../../examples/providers/gcp.iap.example.yaml)
- [IAP Authentication Handler](../../../../examples/iap_authentication.go)
