# SAML Provider

The SAML provider enables authentication using SAML 2.0 Identity Providers (IdP). This provider implements the Service Provider (SP) role and can integrate with any SAML 2.0 compliant Identity Provider.

## Features

- **SAML 2.0 Authentication**: Full support for SAML 2.0 authentication flows
- **IdP Integration**: Fetches metadata automatically from IdP metadata URLs
- **Flexible Configuration**: Supports both signed and unsigned requests
- **Session Management**: Handles session creation, validation, and renewal
- **Certificate-based Security**: Uses X.509 certificates for request signing and validation

## Configuration

The SAML provider requires the following configuration parameters:

### Required Parameters

- `idp_metadata_url`: URL to fetch the Identity Provider's SAML metadata
- `entity_id`: Unique identifier for this service provider
- `root_url`: Base URL of your application where SAML responses will be sent
- `cert_file`: Path to the X.509 certificate file for SAML signing
- `key_file`: Path to the private key file corresponding to the certificate

### Optional Parameters

- `sign_requests`: Whether to sign SAML authentication requests (default: false)

## Setup Instructions

### 1. Generate SAML Certificates

Generate a certificate and private key for SAML signing:

```bash
# Generate private key
openssl genrsa -out saml.key 2048

# Generate certificate
openssl req -new -x509 -key saml.key -out saml.cert -days 365 \
  -subj "/CN=your-app.example.com"
```

### 2. Configure Your Identity Provider

Register your service provider with your IdP using these details:

- **Entity ID**: The value from your `entity_id` configuration
- **ACS URL**: `{root_url}/saml/acs` (Assertion Consumer Service)
- **Metadata URL**: `{root_url}/saml/metadata`
- **Certificate**: Upload your `saml.cert` file

### 3. Configure the Provider

Create a provider configuration in your `config.yaml`:

```yaml
providers:
  - name: my-saml-idp
    description: My Company SAML IdP
    provider: saml
    enabled: true
    config:
      idp_metadata_url: "https://idp.example.com/saml/metadata"
      entity_id: "https://my-app.example.com/saml/metadata"
      root_url: "https://my-app.example.com"
      cert_file: "/path/to/saml.cert"
      key_file: "/path/to/saml.key"
      sign_requests: true
```

## Authentication Flow

1. **Initiate Authentication**: Call `AuthorizeSession()` to get the IdP login URL
2. **User Redirected**: User is redirected to the IdP for authentication
3. **SAML Response**: IdP sends SAML response back to your application
4. **Create Session**: Call `CreateSession()` with the SAML response to create a user session
5. **Session Management**: Use `ValidateSession()` and `RenewSession()` as needed

## Example Usage

```go
// Initialize SAML provider
provider := &samlProvider{}
err := provider.Initialize(providerConfig)

// Start authentication flow
authResp, err := provider.AuthorizeSession(ctx, &models.AuthorizeUser{
    RedirectUri: "https://my-app.example.com/callback",
})

// User is redirected to authResp.Url for authentication
// After IdP authentication, user returns with SAML response

// Create session from SAML response
session, err := provider.CreateSession(ctx, &models.AuthorizeUser{
    Code: samlResponseCode,
})

// Validate session
err = provider.ValidateSession(ctx, session)
```

## Security Considerations

- **Certificate Security**: Keep your private key secure and rotate certificates regularly
- **HTTPS Only**: Always use HTTPS for SAML endpoints and metadata URLs
- **Signature Validation**: The provider automatically validates IdP signatures
- **Session Expiry**: Configure appropriate session timeout values
- **IdP Trust**: Only configure trusted Identity Providers

## Troubleshooting

### Common Issues

1. **Metadata Fetch Errors**: Ensure the `idp_metadata_url` is accessible and returns valid SAML metadata
2. **Certificate Errors**: Verify certificate and key files are valid and readable
3. **Entity ID Mismatch**: Ensure the `entity_id` matches what's configured in your IdP
4. **URL Configuration**: Verify `root_url` is accessible from your IdP

### Debug Logs

Enable debug logging to see detailed SAML flow information:

```go
logrus.SetLevel(logrus.DebugLevel)
```

## Limitations

- **IdP-Managed Roles**: Role and permission management is typically handled at the IdP level
- **Single IdP**: Each provider instance supports one IdP (configure multiple providers for multiple IdPs)
- **Basic Session Management**: Session storage and management is handled by the application layer

## Integration with Identity Providers

This SAML provider has been tested with:

- **Active Directory Federation Services (ADFS)**
- **Azure AD SAML**
- **Okta**
- **Auth0**
- **Generic SAML 2.0 IdPs**

For IdP-specific configuration guides, consult your Identity Provider's documentation on configuring SAML service providers.