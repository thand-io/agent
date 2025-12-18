---
layout: default
title: SAML
description: SAML 2.0 provider for enterprise authentication and SSO
parent: Providers
grand_parent: Configuration
---

# SAML Provider

The SAML provider enables integration with SAML 2.0 Identity Providers (IdPs), providing enterprise authentication and Single Sign-On (SSO) capabilities.

## Capabilities

- **Authentication**: SAML 2.0 authentication and SSO
- **Enterprise Integration**: Works with enterprise identity providers
- **Metadata Exchange**: Automatic IdP metadata consumption
- **Certificate Management**: Support for signed SAML requests and responses

## Prerequisites

### SAML Identity Provider Setup

1. **SAML IdP**: Access to a SAML 2.0 compliant Identity Provider
2. **IdP Metadata**: IdP metadata URL or file
3. **Certificates**: X.509 certificates for SAML signing and encryption
4. **Service Provider Registration**: Register the agent as a Service Provider in your IdP

### Required SAML Configuration

- **IdP Metadata**: Identity Provider metadata URL or content
- **Entity ID**: Unique identifier for this service provider
- **Certificates**: Certificate and private key for SAML operations
- **Root URL**: Base URL for the agent application

## Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `idp_metadata_url` | string | Yes* | - | URL to fetch IdP metadata |
| `idp_metadata` | string | Yes* | - | IdP metadata content (alternative to URL) |
| `entity_id` | string | Yes | - | Service Provider entity ID |
| `root_url` | string | Yes | - | Root URL of the application |
| `cert_file` | string | Yes | - | Path to SAML certificate file |
| `key_file` | string | Yes | - | Path to SAML private key file |
| `sign_requests` | boolean | No | `false` | Whether to sign SAML requests |
| `encrypt_assertions` | boolean | No | `false` | Whether to encrypt SAML assertions |

*Either `idp_metadata_url` or `idp_metadata` is required.

## Getting Credentials

### Certificate Generation

Generate a self-signed certificate for SAML:

```bash
# Generate private key
openssl genrsa -out saml.key 2048

# Generate certificate
openssl req -new -x509 -key saml.key -out saml.cert -days 365 \
  -subj "/CN=your-app.example.com"
```

### IdP Configuration

1. **Register Service Provider**: Add your agent as a Service Provider in your IdP
2. **Configure Entity ID**: Use your chosen entity ID (e.g., `https://your-app.example.com/saml/metadata`)
3. **Set Assertion Consumer Service**: Configure ACS URL (e.g., `https://your-app.example.com/api/v1/auth/callback/company-saml`)
4. **Upload Certificate**: Upload your public certificate to the IdP

## Example Configurations

### Basic SAML Configuration

```yaml
version: "1.0"
providers:
  company-saml:
    name: Company SAML
    description: Company SAML Identity Provider
    provider: saml
    enabled: true
    config:
      idp_metadata_url: https://your-idp.example.com/saml/metadata
      entity_id: https://your-app.example.com/saml/metadata
      root_url: https://your-app.example.com
      cert_file: /etc/agent/saml.cert
      key_file: /etc/agent/saml.key
```

### SAML with Signing

```yaml
version: "1.0"
providers:
  secure-saml:
    name: Secure SAML
    description: SAML with request signing
    provider: saml
    enabled: true
    config:
      idp_metadata_url: https://your-idp.example.com/saml/metadata
      entity_id: https://your-app.example.com/saml/metadata
      root_url: https://your-app.example.com
      cert_file: /etc/agent/saml.cert
      key_file: /etc/agent/saml.key
      sign_requests: true
      encrypt_assertions: true
```

### SAML with Inline Metadata

```yaml
version: "1.0"
providers:
  saml-inline:
    name: SAML Inline Metadata
    description: SAML with inline IdP metadata
    provider: saml
    enabled: true
    config:
      idp_metadata: |
        <?xml version="1.0"?>
        <EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata"
                          entityID="https://your-idp.example.com">
          <!-- IdP metadata content -->
        </EntityDescriptor>
      entity_id: https://your-app.example.com/saml/metadata
      root_url: https://your-app.example.com
      cert_file: /etc/agent/saml.cert
      key_file: /etc/agent/saml.key
```

## Features

### Metadata Exchange

The SAML provider supports:
- Automatic IdP metadata fetching
- Service Provider metadata generation
- Dynamic metadata updates

### Security Features

- X.509 certificate validation
- SAML request signing
- SAML assertion encryption
- Replay attack protection

### Attribute Mapping

Automatic mapping of SAML attributes to user properties:
- `NameID`: User identifier
- `email`: User email address
- `displayName`: User display name
- Custom attribute mapping support

## Troubleshooting

### Common Issues

1. **Certificate Issues**
   - Verify certificate and key files are readable
   - Check certificate validity and format
   - Ensure certificate matches IdP configuration

2. **Metadata Issues**
   - Verify IdP metadata URL is accessible
   - Check metadata format and validity
   - Ensure IdP is reachable from agent

3. **Entity ID Mismatch**
   - Verify entity ID matches IdP configuration
   - Check for URL encoding issues
   - Ensure consistent entity ID usage

### Debugging

Enable debug logging for SAML troubleshooting:

```yaml
logging:
  level: debug
```

### Testing SAML Configuration

Use SAML testing tools to validate your configuration:
1. **SAML Tracer**: Browser extension for SAML request/response inspection
2. **Online SAML Validators**: Validate SAML metadata and responses
3. **IdP Test Tools**: Use your IdP's built-in testing tools

## Security Considerations

1. **Certificate Security**: Protect private keys and rotate certificates regularly
2. **Metadata Validation**: Verify IdP metadata authenticity and integrity
3. **Secure Transmission**: Use HTTPS for all SAML communications
4. **Clock Synchronization**: Ensure accurate time synchronization for assertion validation
5. **Attribute Validation**: Validate and sanitize SAML attributes