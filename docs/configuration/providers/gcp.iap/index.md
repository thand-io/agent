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
      # REQUIRED: JWT audience claim for your backend service
      audience: "/projects/PROJECT_NUMBER/locations/REGION/services/SERVICE_NAME"
```

### Configuration Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `audience` | string | Yes | JWT audience claim that identifies your backend service |

## Getting Your Audience String

The audience string uniquely identifies your backend service. It varies by deployment type.

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

#### 2. Enable IAP

```bash
# Get the backend service name (created automatically with Cloud Run)
gcloud compute backend-services list

# Enable IAP on the backend service
gcloud iap web enable \
  --resource-type=backend-services \
  --service=SERVICE_NAME
```

#### 3. Grant User Access

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

#### 4. Configure Provider

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "/projects/123456789/locations/us-central1/services/my-service"
```

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
```

### App Engine

#### 1. Deploy Application

```bash
gcloud app deploy app.yaml
```

#### 2. Enable IAP via Console

1. Go to [IAP Settings](https://console.cloud.google.com/security/iap)
2. Find your App Engine app
3. Toggle IAP to **On**
4. Configure OAuth consent screen if prompted
5. Add authorized users/groups

#### 3. Configure Provider

```yaml
providers:
  gcp-iap:
    provider: gcp.iap
    enabled: true
    config:
      audience: "/projects/123456789/apps/my-project-id"
```

## Complete Example

```yaml
# config.yaml

# Server Configuration
server:
  host: "0.0.0.0"
  port: 5225
  
  security:
    cors:
      allowed_origins:
        - "https://*.example.com"

# Logging
logging:
  level: "info"
  format: "json"

# Providers
providers:
  # GCP IAP for authentication
  gcp-iap:
    provider: gcp.iap
    description: "GCP IAP Authentication"
    enabled: true
    config:
      audience: "/projects/123456789/locations/us-central1/services/thand-agent"
  
  # Additional providers (AWS, Azure, etc.)
  aws-prod:
    provider: aws
    # ... other config
```

## Usage in Code

Access authenticated user information in your handlers:

```go
import (
    "github.com/gin-gonic/gin"
    "github.com/thand-io/agent/internal/daemon"
    "github.com/thand-io/agent/internal/models"
)

func myHandler(c *gin.Context) {
    // Get sessions from context
    sessionsInterface, exists := c.Get(daemon.SessionContextKey)
    if !exists {
        c.JSON(401, gin.H{"error": "Not authenticated"})
        return
    }
    
    sessions := sessionsInterface.(map[string]*models.Session)
    
    // Get IAP session (use your provider name)
    if iapSession, ok := sessions["gcp-iap"]; ok {
        user := iapSession.User
        
        // User information available:
        email := user.Email           // user@example.com
        userID := user.ID              // Stable identifier
        name := user.Name              // Extracted from email
        verified := user.Verified      // Always true for IAP
        
        c.JSON(200, gin.H{
            "message": "Welcome",
            "email": email,
        })
    }
}
```

## Security Considerations

### Why Verify the JWT?

Even though IAP protects your application, JWT verification provides defense in depth:

1. **Accidental IAP Disable**: If IAP is accidentally turned off, your app still rejects unauthenticated requests
2. **Firewall Misconfiguration**: Protects against direct access to your backend
3. **Internal Threats**: Prevents unauthorized access from within your project

### JWT Token Lifecycle

- **Lifetime**: IAP JWTs are short-lived (typically 10 minutes maximum)
- **No Renewal**: Tokens cannot be renewed programmatically - users must re-authenticate through IAP
- **Automatic Refresh**: IAP automatically issues new tokens as needed

### Health Checks

Health check requests from Google Cloud Load Balancers don't include JWT headers. Ensure your health check endpoint doesn't require authentication:

```yaml
# The /health endpoint should not require IAP authentication
server:
  health:
    path: "/health"
```

## Troubleshooting

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
```

## Best Practices

1. **Use Separate Providers**: Create different IAP providers for different environments (dev, staging, prod)
2. **Monitor Access**: Regularly review IAP access logs in Cloud Logging
3. **Limit Access**: Use groups instead of individual users for easier management
4. **Test Locally**: Use alternative authentication methods for local development
5. **Health Checks**: Ensure health check endpoints don't require authentication

## Related Documentation

- [GCP IAP Documentation](https://cloud.google.com/iap/docs)
- [Securing Your App with Signed Headers](https://cloud.google.com/iap/docs/signed-headers-howto)
- [IAP Access Control](https://cloud.google.com/iap/docs/managing-access)
- [Thand IAP Configuration Guide](../../iap)

## Examples

See complete configuration examples:
- [Basic IAP Configuration](../../../../examples/providers/gcp.iap.example.yaml)
- [IAP Authentication Handler](../../../../examples/iap_authentication.go)
