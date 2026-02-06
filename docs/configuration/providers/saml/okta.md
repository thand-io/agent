---
layout: default
title: SAML Okta
description: Configure Okta as a SAML identity provider for SSO authentication
parent: Providers
grand_parent: Configuration
---

# Okta SAML Provider

Configure Okta as a SAML 2.0 identity provider to enable single sign-on (SSO) authentication for your users. This provider allows users to authenticate using their Okta credentials.

## Capabilities

**Authentication**: SAML 2.0 single sign-on authentication through Okta

**Authorization**: User identity and attribute-based access control

**Session Management**: SSO session lifecycle management through Okta

## Prerequisites

### Okta Account Requirements

1. An Okta organization (production or developer account)
2. Administrator access to the Okta Admin Console
3. Permissions to create SAML 2.0 applications
4. Ability to configure application settings and download certificates

### Application Requirements

1. A publicly accessible callback URL for your application
2. SSL/TLS certificate for production deployments
3. Access to configure your application's YAML configuration

## Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `idp_metadata_url` | string | Yes | - | URL to fetch the IdP metadata from Okta. Found in your SAML app settings under "Metadata URL" |
| `entity_id` | string | Yes | - | Service Provider entity ID (typically your callback URL: `https://yourdomain.com/api/v1/auth/callback/okta-saml`) |
| `root_url` | string | Yes | - | Root URL of your application (e.g., `https://yourdomain.com/`) |
| `cert` | string | Yes | - | X.509 certificate from Okta IdP in PEM format. Used to validate SAML assertions |
| `sign_requests` | boolean | No | `false` | Whether to sign outgoing SAML authentication requests. Set to `true` if Okta requires signed requests |

## Getting Credentials

### Step 1: Create a SAML Application in Okta

1. Log in to your Okta Admin Console
2. Navigate to **Applications** > **Applications**
3. Click **Create App Integration**
4. Select **SAML 2.0** as the sign-in method
5. Click **Next**

### Step 2: Configure SAML Settings

1. **General Settings**:
   - Enter an **App name** (e.g., "YourApp SAML")
   - Optionally upload an app logo
   - Click **Next**

2. **SAML Settings**:
   - **Single sign-on URL**: Enter your callback URL
     ```
     https://yourdomain.com/api/v1/auth/callback/okta-saml
     ```
   - Check **Use this for Recipient URL and Destination URL**
   - **Audience URI (SP Entity ID)**: Enter the same callback URL
     ```
     https://yourdomain.com/api/v1/auth/callback/okta-saml
     ```
   - **Default RelayState**: Leave empty or specify a landing page
   - **Name ID format**: `EmailAddress` (recommended)
   - **Application username**: `Email` (recommended)

3. **Attribute Statements** (Optional):
   Configure custom attributes to pass user information:
   - `email` → `user.email`
   - `firstName` → `user.firstName`
   - `lastName` → `user.lastName`
   - `groups` → `appuser.groups` (if using group-based authorization)

4. Click **Next** and complete the setup wizard

### Step 3: Retrieve SAML Configuration

1. In your SAML application, go to the **Sign On** tab
2. Scroll to **SAML 2.0** section
3. **Copy the Metadata URL**:
   - Click **Metadata URL** or copy the link
   - This will be your `idp_metadata_url`
   - Format: `https://your-domain.okta.com/app/exk.../sso/saml/metadata`

4. **Download the Certificate**:
   - Click **View SAML setup instructions**
   - Copy the **X.509 Certificate** (including `-----BEGIN CERTIFICATE-----` and `-----END CERTIFICATE-----` lines)
   - This will be your `cert` value

### Step 4: Assign Users to the Application

1. Go to the **Assignments** tab in your SAML app
2. Click **Assign** and select **Assign to People** or **Assign to Groups**
3. Add the users or groups that should have access

## Example Configurations

### Basic Configuration

```yaml
version: "1.0"
providers:
  okta-saml:
    name: "Okta SAML"
    description: Company SAML Identity Provider
    provider: saml
    enabled: true
    config:
      # URL to fetch IdP metadata from Okta
      idp_metadata_url: "https://your-domain.okta.com/app/app-id/sso/saml/metadata"
      
      # Service Provider entity ID (your callback URL)
      entity_id: "https://yourdomain.com/api/v1/auth/callback/okta-saml"
      
      # Root URL of your application
      root_url: "https://yourdomain.com/"
      
      # X.509 certificate from Okta
      cert: |
        -----BEGIN CERTIFICATE-----
        MIIDpDCCAoygAwIBAgIGAXMKpxe5MA0GCSqGSIb3DQEBCwUAMIGSMQswCQYDVQQG
        ...
        -----END CERTIFICATE-----

      # Whether to sign SAML requests (default: false)
      # If yes, you need to provide the private key.
      sign_requests: false
```

### Development/Testing Configuration (localhost)

```yaml
version: "1.0"
providers:
  okta-saml-dev:
    name: "Okta SAML (Dev)"
    description: Development Okta SAML Provider
    provider: saml
    enabled: true
    config:
      idp_metadata_url: "https://dev-123456.okta.com/app/exk.../sso/saml/metadata"
      entity_id: "http://localhost:9090/api/v1/auth/callback/okta-saml-dev"
      root_url: "http://localhost:9090/"
      cert: |
        -----BEGIN CERTIFICATE-----
        [Your Okta certificate content here]
        -----END CERTIFICATE-----
      sign_requests: false
```

### Configuration with Signed Requests

If your Okta application requires signed SAML requests:

```yaml
version: "1.0"
providers:
  okta-saml-prod:
    name: "Okta SAML Production"
    description: Production Okta SAML with signed requests
    provider: saml
    enabled: true
    config:
      idp_metadata_url: "https://your-domain.okta.com/app/exk.../sso/saml/metadata"
      entity_id: "https://yourdomain.com/api/v1/auth/callback/okta-saml-prod"
      root_url: "https://yourdomain.com/"
      cert: |
        -----BEGIN CERTIFICATE-----
        [Your Okta certificate content here]
        -----END CERTIFICATE-----
      # Enable request signing
      sign_requests: true
```

### Multiple Okta Tenants

You can configure multiple Okta tenants (e.g., for different environments or customer segregation):

```yaml
version: "1.0"
providers:
  okta-saml-us:
    name: "Okta SAML US"
    description: US Region Okta
    provider: saml
    enabled: true
    config:
      idp_metadata_url: "https://us-company.okta.com/app/exk.../sso/saml/metadata"
      entity_id: "https://yourdomain.com/api/v1/auth/callback/okta-saml-us"
      root_url: "https://yourdomain.com/"
      cert: |
        -----BEGIN CERTIFICATE-----
        [US Okta certificate]
        -----END CERTIFICATE-----

  okta-saml-eu:
    name: "Okta SAML EU"
    description: EU Region Okta
    provider: saml
    enabled: true
    config:
      idp_metadata_url: "https://eu-company.okta.com/app/exk.../sso/saml/metadata"
      entity_id: "https://yourdomain.com/api/v1/auth/callback/okta-saml-eu"
      root_url: "https://yourdomain.com/"
      cert: |
        -----BEGIN CERTIFICATE-----
        [EU Okta certificate]
        -----END CERTIFICATE-----
```

## Features

### Single Sign-On (SSO)

Users authenticate once with Okta and gain access to your application without additional login prompts. The SAML provider handles session management and token validation.

### Attribute Mapping

Configure attribute statements in Okta to pass user information (email, name, groups, custom attributes) to your application. These attributes can be used for authorization decisions and user profile enrichment.

### Group-Based Authorization

Use Okta groups for role-based access control by configuring group attribute statements. Map Okta groups to application roles for fine-grained access management.

### Session Lifecycle

The provider respects Okta's session policies including:
- Maximum session duration
- Idle session timeout
- Re-authentication requirements
- Multi-factor authentication (MFA) enforcement

## Troubleshooting

### Common Issues

1. **Certificate validation errors**
   - Ensure the certificate is copied completely including `-----BEGIN CERTIFICATE-----` and `-----END CERTIFICATE-----` lines
   - Verify the certificate matches the one in your Okta SAML app
   - Check that there are no extra spaces or line breaks in the certificate
   - The certificate should be in PEM format, not DER or other formats

2. **Metadata URL not accessible**
   - Verify the metadata URL is correct and publicly accessible
   - Check that your application has network access to Okta
   - Ensure the SAML app is activated in Okta
   - Test the metadata URL in a browser to confirm it returns XML

3. **Entity ID mismatch**
   - The `entity_id` in your configuration must exactly match the **Audience URI** configured in Okta
   - Both should typically be your callback URL
   - Entity IDs are case-sensitive

4. **Callback URL issues**
   - Ensure the **Single sign-on URL** in Okta matches your actual callback endpoint
   - For localhost testing, use `http://localhost:PORT` (not `127.0.0.1`)
   - For production, always use `https://` URLs
   - Verify your application is listening on the correct port and path

5. **Users cannot access the application**
   - Check that users or groups are assigned to the SAML app in Okta
   - Verify the application is activated in Okta
   - Review Okta system logs for authentication failures

6. **SAML assertion errors**
   - Enable debug logging to inspect SAML responses
   - Verify the Name ID format matches between Okta and your configuration
   - Check attribute mappings if using custom attributes
   - Ensure the assertion is not expired (check time synchronization)

### Debugging

Enable detailed logging to troubleshoot SAML authentication issues:

```bash
# Set log level to debug
export LOG_LEVEL=debug

# Run your application
./bin/agent
```

Check Okta system logs:
1. Log in to Okta Admin Console
2. Navigate to **Reports** > **System Log**
3. Filter by your SAML application
4. Look for authentication events and errors

Test SAML connectivity:
```bash
# Test metadata URL accessibility
curl -v "https://your-domain.okta.com/app/exk.../sso/saml/metadata"

# Verify certificate format
openssl x509 -in /path/to/cert.pem -text -noout
```

### Okta SAML vs. Okta Provider

Note the difference between this SAML provider and the separate Okta provider:

- **Okta SAML** (`provider: saml`): Authentication and SSO using SAML 2.0 protocol
- **Okta Provider** (`provider: okta`): Role-based access control and identity management using Okta API

Use Okta SAML for user authentication, and the Okta provider for managing Okta administrator roles and resources.

## Security Considerations

1. **Certificate Management**:
   - Rotate certificates before expiration
   - Store certificates securely (use environment variables or secret management)
   - Monitor certificate expiration dates

2. **HTTPS Required**:
   - Always use HTTPS in production for the `entity_id` and `root_url`
   - HTTP is only acceptable for local development

3. **Metadata URL Security**:
   - The metadata URL should be publicly accessible but doesn't contain sensitive information
   - Consider caching metadata to reduce dependency on Okta availability

4. **Request Signing**:
   - Enable `sign_requests: true` if required by your security policies
   - This adds an additional layer of authenticity verification

5. **Session Security**:
   - Configure appropriate session timeouts in Okta
   - Enable MFA in Okta for sensitive applications
   - Review Okta's authentication policies regularly

## Additional Resources

- [Okta SAML Documentation](https://developer.okta.com/docs/guides/build-sso-integration/saml2/main/)
- [Generic SAML Provider Configuration](../saml/)
- [Okta Provider (RBAC)](../okta/)
- [SAML 2.0 Specification](https://docs.oasis-open.org/security/saml/Post2.0/sstc-saml-tech-overview-2.0.html)
