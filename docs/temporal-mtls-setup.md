# Temporal mTLS Setup Guide

This guide explains how to configure Thand Agent to connect to a Temporal server using mutual TLS (mTLS) authentication, with support for loading certificates from CSP keystores.

## Overview

Thand Agent supports multiple ways to provide mTLS certificates for Temporal connections:

1. **HSM-Backed Keys** (Most secure for production) - Private keys managed by CSP HSM (AWS KMS, Azure Key Vault Keys, GCP Cloud KMS), certificate in vault
2. **CSP Vault Secrets** (Recommended for production) - Combined cert+key in AWS Secrets Manager, Azure Key Vault, GCP Secret Manager
3. **File Paths** (Recommended for Kubernetes) - Mounted secrets from Secret objects
4. **Inline PEM** (Development/testing only) - Certificates directly in configuration

## Configuration Options

### Option 1: HSM-Backed Keys (Most Secure for Production)

This pattern uses Hardware Security Modules (HSM) to manage private keys. The private key never leaves the HSM hardware, and all cryptographic signing operations are performed within the secure enclave. This is the recommended approach for high-security production environments.

**How it works:**
- Certificate is stored in the CSP's secret store (AWS Secrets Manager, Azure Key Vault Secret, GCP Secret Manager)
- Private key is a reference to an HSM-managed key (AWS KMS, Azure Key Vault Key, GCP Cloud KMS)
- Signing operations during TLS handshake are performed by the HSM via API calls

**Status:** Currently returns "not implemented" error. Full implementation requires CSP-specific signing logic and will be added in a future update.

#### AWS KMS + Secrets Manager

**1. Create an RSA key in AWS KMS:**

```bash
# Create asymmetric RSA key for signing (required for TLS)
aws kms create-key \
  --key-usage SIGN_VERIFY \
  --key-spec RSA_2048 \
  --description "Thand Temporal mTLS private key"

# Note the KeyId from output (e.g., abc123-4567-890a-bcde-f1234567890)

# Create alias for easier reference
aws kms create-alias \
  --alias-name alias/thand-temporal-mtls \
  --target-key-id abc123-4567-890a-bcde-f1234567890
```

**2. Generate CSR and get certificate signed:**

```bash
# Since the private key is in KMS, you'll need to use AWS KMS to sign the CSR
# Or, use your PKI/CA to issue a certificate for the public key from KMS

# Get public key from KMS
aws kms get-public-key \
  --key-id alias/thand-temporal-mtls \
  --output text \
  --query PublicKey | base64 -d > temporal-public.der

# Convert to PEM and create CSR (you may need to work with your CA)
# Once you have the signed certificate (temporal-client.crt):

# Store certificate in Secrets Manager (certificate only, no private key!)
aws secretsmanager create-secret \
  --name thand/temporal/mtls/hsm-cert \
  --secret-string "$(cat temporal-client.crt)"
```

**3. Configure Thand Agent:**

```yaml
environment:
  platform: "aws"
  config:
    region: "us-west-2"

services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # HSM-backed key pattern
    mtls_cert_secret: "thand/temporal/mtls/hsm-cert"
    mtls_hsm_key_id: "arn:aws:kms:us-west-2:123456789012:key/abc123-4567-890a-bcde-f1234567890"
    # mtls_hsm_key_type: "aws-kms"  # Optional: auto-detected from platform
```

#### GCP Cloud KMS + Secret Manager

**1. Create asymmetric key in Cloud KMS:**

```bash
# Create key ring
gcloud kms keyrings create thand \
  --location us-west1 \
  --project your-project-id

# Create asymmetric signing key
gcloud kms keys create temporal-mtls \
  --keyring thand \
  --location us-west1 \
  --purpose asymmetric-signing \
  --default-algorithm rsa-sign-pkcs1-2048-sha256 \
  --project your-project-id

# Get public key
gcloud kms keys versions get-public-key 1 \
  --key temporal-mtls \
  --keyring thand \
  --location us-west1 \
  --output-file temporal-public.pem \
  --project your-project-id
```

**2. Get certificate from your CA and store in Secret Manager:**

```bash
# After getting certificate from your CA
gcloud secrets create temporal-mtls-hsm-cert \
  --data-file=temporal-client.crt \
  --project=your-project-id
```

**3. Configure Thand Agent:**

```yaml
environment:
  platform: "gcp"
  config:
    project_id: "your-project-id"

services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # HSM-backed key pattern
    mtls_cert_secret: "temporal-mtls-hsm-cert"
    mtls_hsm_key_id: "projects/your-project-id/locations/us-west1/keyRings/thand/cryptoKeys/temporal-mtls"
```

#### Azure Key Vault Keys + Secrets

**1. Create signing key in Azure Key Vault:**

```bash
# Create RSA key for signing
az keyvault key create \
  --vault-name your-keyvault \
  --name temporal-mtls-key \
  --kty RSA \
  --size 2048 \
  --ops sign verify

# Get public key (for CSR generation)
az keyvault key show \
  --vault-name your-keyvault \
  --name temporal-mtls-key
```

**2. Store certificate in Key Vault secret:**

```bash
# After obtaining certificate from your CA
az keyvault secret set \
  --vault-name your-keyvault \
  --name temporal-mtls-hsm-cert \
  --file temporal-client.crt
```

**3. Configure Thand Agent:**

```yaml
environment:
  platform: "azure"
  config:
    vault_url: "https://your-keyvault.vault.azure.net/"
    tenant_id: "your-tenant-id"
    client_id: "your-client-id"
    client_secret: "your-client-secret"

services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # HSM-backed key pattern
    mtls_cert_secret: "temporal-mtls-hsm-cert"
    mtls_hsm_key_id: "https://your-keyvault.vault.azure.net/keys/temporal-mtls-key"
```

### Option 2: CSP Vault Secrets (Combined Cert+Key)

Store your mTLS certificates in your cloud provider's secret management service.

#### AWS Secrets Manager

**1. Create secrets in AWS Secrets Manager:**

```bash
# Store combined cert+key
aws secretsmanager create-secret \
  --name thand/temporal/mtls/client-cert \
  --secret-string "$(cat <<EOF
-----BEGIN CERTIFICATE-----
$(cat client.crt | grep -v 'BEGIN\|END')
-----END CERTIFICATE-----
-----BEGIN PRIVATE KEY-----
$(cat client.key | grep -v 'BEGIN\|END')
-----END PRIVATE KEY-----
EOF
)"

# Store CA certificate (optional)
aws secretsmanager create-secret \
  --name thand/temporal/mtls/ca-cert \
  --secret-string "$(cat ca.crt)"
```

**2. Configure Thand Agent:**

```yaml
environment:
  platform: "aws"
  config:
    region: "us-west-2"
    # Optional: Use named profile or IAM role
    # profile: "your-profile"

services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # mTLS with AWS Secrets Manager
    mtls_cert_key_secret: "thand/temporal/mtls/client-cert"
    mtls_ca_secret: "thand/temporal/mtls/ca-cert"  # Optional
```

**3. Environment variables (alternative):**

```bash
export THAND_ENVIRONMENT_PLATFORM=aws
export THAND_ENVIRONMENT_CONFIG_REGION=us-west-2
export THAND_SERVICES_TEMPORAL_HOST=prod.temporal.example.com
export THAND_SERVICES_TEMPORAL_MTLS_CERT_KEY_SECRET=thand/temporal/mtls/client-cert
export THAND_SERVICES_TEMPORAL_MTLS_CA_SECRET=thand/temporal/mtls/ca-cert
```

#### GCP Secret Manager

**1. Create secrets in GCP Secret Manager:**

```bash
# Store combined cert+key
cat client.crt client.key | gcloud secrets create temporal-mtls-client-cert \
  --data-file=- \
  --project=your-project-id

# Store CA certificate (optional)
gcloud secrets create temporal-mtls-ca-cert \
  --data-file=ca.crt \
  --project=your-project-id
```

**2. Configure Thand Agent:**

```yaml
environment:
  platform: "gcp"
  config:
    project_id: "your-project-id"

services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # mTLS with GCP Secret Manager
    mtls_cert_key_secret: "temporal-mtls-client-cert"
    mtls_ca_secret: "temporal-mtls-ca-cert"  # Optional
```

#### Azure Key Vault

**1. Create secrets in Azure Key Vault:**

```bash
# Combined cert+key (must be in one line or properly escaped)
az keyvault secret set \
  --vault-name your-keyvault \
  --name temporal-mtls-client-cert \
  --file combined-cert.pem

# CA certificate (optional)
az keyvault secret set \
  --vault-name your-keyvault \
  --name temporal-mtls-ca-cert \
  --file ca.crt
```

**2. Configure Thand Agent:**

```yaml
environment:
  platform: "azure"
  config:
    vault_url: "https://your-keyvault.vault.azure.net/"
    tenant_id: "your-tenant-id"
    client_id: "your-client-id"
    client_secret: "your-client-secret"

services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # mTLS with Azure Key Vault
    mtls_cert_key_secret: "temporal-mtls-client-cert"
    mtls_ca_secret: "temporal-mtls-ca-cert"  # Optional
```

### Option 2: File Paths (Kubernetes Recommended)

When running in Kubernetes, mount certificates as secrets and reference the file paths.

**1. Create Kubernetes secret:**

```bash
kubectl create secret tls temporal-mtls \
  --cert=client.crt \
  --key=client.key \
  --namespace=thand

# Create CA secret (optional)
kubectl create secret generic temporal-ca \
  --from-file=ca.crt=ca.crt \
  --namespace=thand
```

**2. Mount secrets in your deployment:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thand-agent
spec:
  template:
    spec:
      containers:
      - name: thand
        volumeMounts:
        - name: temporal-mtls
          mountPath: /etc/temporal/certs
          readOnly: true
        - name: temporal-ca
          mountPath: /etc/temporal/ca
          readOnly: true
      volumes:
      - name: temporal-mtls
        secret:
          secretName: temporal-mtls
      - name: temporal-ca
        secret:
          secretName: temporal-ca
```

**3. Configure Thand Agent:**

```yaml
services:
  temporal:
    host: "prod.temporal.example.com"
    port: 7233
    namespace: "production"

    # mTLS with mounted files
    mtls_cert_file: "/etc/temporal/certs/tls.crt"
    mtls_key_file: "/etc/temporal/certs/tls.key"
    mtls_ca_file: "/etc/temporal/ca/ca.crt"  # Optional
```

**4. Environment variables (alternative):**

```bash
export THAND_SERVICES_TEMPORAL_MTLS_CERT_FILE=/etc/temporal/certs/tls.crt
export THAND_SERVICES_TEMPORAL_MTLS_KEY_FILE=/etc/temporal/certs/tls.key
export THAND_SERVICES_TEMPORAL_MTLS_CA_FILE=/etc/temporal/ca/ca.crt
```

### Option 3: Inline Certificates (Development Only)

For local development and testing, you can provide certificates directly in the configuration.

```yaml
services:
  temporal:
    host: "localhost"
    port: 7233
    namespace: "default"

    # mTLS with inline certificates (NOT recommended for production)
    mtls_cert: |
      -----BEGIN CERTIFICATE-----
      MIIDXTCCAkWgAwIBAgIJAKL0UG+mRhmfMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
      ...
      -----END CERTIFICATE-----
    mtls_key: |
      -----BEGIN PRIVATE KEY-----
      MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDXhzR7NqPNa1PE
      ...
      -----END PRIVATE KEY-----
    mtls_ca: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
```

## Certificate Generation

If you need to generate self-signed certificates for testing:

```bash
# Generate CA private key and certificate
openssl req -x509 -newkey rsa:4096 -days 365 -nodes \
  -keyout ca.key -out ca.crt \
  -subj "/CN=Temporal Test CA"

# Generate client private key
openssl genrsa -out client.key 4096

# Generate client certificate signing request
openssl req -new -key client.key -out client.csr \
  -subj "/CN=thand-agent"

# Sign client certificate with CA
openssl x509 -req -in client.csr -days 365 \
  -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt
```

For production, use certificates from your organization's PKI or a trusted CA.

## Configuration Reference

### All mTLS Configuration Options

```yaml
services:
  temporal:
    # Connection (required)
    host: "temporal.example.com"
    port: 7233
    namespace: "production"

    # Authentication: API Key (mutually exclusive with mTLS)
    api_key: "your-api-key"

    # Authentication: mTLS - Inline (development only)
    mtls_cert: "PEM-encoded certificate"
    mtls_key: "PEM-encoded private key"

    # Authentication: mTLS - File Paths (Kubernetes)
    mtls_cert_file: "/path/to/cert.pem"
    mtls_key_file: "/path/to/key.pem"

    # Authentication: mTLS - CSP Vault Secret (combined cert+key)
    mtls_cert_key_secret: "secret-name-for-combined-cert-key"

    # Authentication: mTLS - HSM-Backed Key (most secure, NOT YET IMPLEMENTED)
    mtls_cert_secret: "secret-name-for-certificate"
    mtls_hsm_key_id: "hsm-key-resource-identifier"  # ARN/URL/resource name
    mtls_hsm_key_type: "aws-kms"  # Optional: aws-kms, azure-keyvault, gcp-kms

    # CA Certificate (optional, for custom Temporal instances)
    mtls_ca: "PEM-encoded CA certificate"
    mtls_ca_file: "/path/to/ca.pem"
    mtls_ca_secret: "secret-name-for-ca-cert"
```

### Environment Variables

All configuration can be set via environment variables with the `THAND_` prefix:

```bash
# Connection
THAND_SERVICES_TEMPORAL_HOST=temporal.example.com
THAND_SERVICES_TEMPORAL_PORT=7233
THAND_SERVICES_TEMPORAL_NAMESPACE=production

# API Key
THAND_SERVICES_TEMPORAL_API_KEY=your-api-key

# mTLS - Inline
THAND_SERVICES_TEMPORAL_MTLS_CERT="..."
THAND_SERVICES_TEMPORAL_MTLS_KEY="..."

# mTLS - File Paths
THAND_SERVICES_TEMPORAL_MTLS_CERT_FILE=/path/to/cert.pem
THAND_SERVICES_TEMPORAL_MTLS_KEY_FILE=/path/to/key.pem

# mTLS - Vault Secret (combined cert+key)
THAND_SERVICES_TEMPORAL_MTLS_CERT_KEY_SECRET=secret-name

# mTLS - HSM-Backed Key
THAND_SERVICES_TEMPORAL_MTLS_CERT_SECRET=cert-secret-name
THAND_SERVICES_TEMPORAL_MTLS_HSM_KEY_ID=hsm-key-resource-id
THAND_SERVICES_TEMPORAL_MTLS_HSM_KEY_TYPE=aws-kms  # Optional: auto-detected

# CA Certificate
THAND_SERVICES_TEMPORAL_MTLS_CA="..."
THAND_SERVICES_TEMPORAL_MTLS_CA_FILE=/path/to/ca.pem
THAND_SERVICES_TEMPORAL_MTLS_CA_SECRET=ca-secret-name
```

## Validation

After configuring mTLS, you can verify the connection:

```bash
# Start the Thand server
./thand server

# Check logs for successful Temporal connection
# You should see:
# INFO Configuring Temporal client with mTLS authentication
# DEBUG Loaded mTLS certificate source=vault (or source=file, source=inline)
# INFO Connecting to Temporal at prod.temporal.example.com:7233 in namespace production
```

## Troubleshooting

### Error: "vault service is not available"

**Problem**: You configured a vault secret but the vault service failed to initialize.

**Solution**:
- Verify your environment platform is set correctly (`aws`, `gcp`, or `azure`)
- Check CSP credentials are properly configured
- For AWS: Ensure IAM permissions for Secrets Manager
- For GCP: Ensure service account has Secret Manager access
- For Azure: Verify Key Vault access policies

### Error: "multiple certificate sources configured"

**Problem**: You configured more than one certificate source (e.g., both inline and file paths).

**Solution**: Choose ONE certificate source and remove the others from your configuration.

### Error: "no certificate block found in combined PEM"

**Problem**: The vault secret doesn't contain a valid certificate.

**Solution**: Ensure the secret contains both certificate and private key in PEM format:
```
-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----
-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----
```

### Error: "failed to load certificate from files"

**Problem**: Certificate or key files don't exist or aren't readable.

**Solution**:
- Verify file paths are correct
- Check file permissions (Thand agent must be able to read them)
- In Kubernetes, verify the secret is mounted correctly

### Error: "x509: certificate signed by unknown authority"

**Problem**: Temporal server uses a self-signed or internal CA certificate.

**Solution**: Configure the CA certificate using one of:
- `mtls_ca_secret` (CSP vault)
- `mtls_ca_file` (Kubernetes)
- `mtls_ca` (inline)

## Security Best Practices

1. **Never commit certificates to version control**
   - Use `.gitignore` for certificate files
   - Store certificates in CSP vaults or Kubernetes secrets

2. **Use CSP vault secrets for production**
   - Enables centralized secret management
   - Supports secret rotation
   - Provides audit trails

3. **Restrict file permissions**
   - Certificate files: `0644` (read-only for others)
   - Private key files: `0600` (readable only by owner)

4. **Rotate certificates regularly**
   - Set certificate expiry appropriately
   - Plan for certificate rotation (requires app restart)

5. **Use custom CA certificates**
   - For self-hosted Temporal instances
   - Validates Temporal server identity
   - Prevents MITM attacks

## Integration with Existing Vault Configuration

The mTLS implementation uses the existing vault service configured for your platform:

```yaml
environment:
  platform: "aws"  # or "gcp", "azure"
  config:
    # AWS-specific config
    region: "us-west-2"
    # profile: "your-profile"  # Optional

    # GCP-specific config
    # project_id: "your-project"

    # Azure-specific config
    # vault_url: "https://your-vault.vault.azure.net/"
    # tenant_id: "..."
    # client_id: "..."
    # client_secret: "..."

services:
  temporal:
    # Uses the vault configured above
    mtls_cert_key_secret: "your-secret-name"
```

## Combined PEM Format for Vault Secrets

When storing certificates in CSP vaults, combine the certificate and private key in one secret:

```
-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL0UG+mRhmfMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
BAYTAlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRYwFAYDVQQHDA1TYW4gRnJhbmNp
...
-----END CERTIFICATE-----
-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDXhzR7NqPNa1PE
gPWzH8VvKJxJzJ5VZGkF8sXLKQB0x6Q+VfCJX5kF8sXLKQB0x6Q+VfCJX5k
...
-----END PRIVATE KEY-----
```

**Important**:
- The order doesn't matter (cert first or key first)
- Certificate chains are supported (multiple certificate blocks)
- Exactly ONE private key must be present
- Supported key types: RSA, EC, PKCS8

## Example Deployments

### Docker with AWS Secrets Manager

```bash
docker run -e THAND_ENVIRONMENT_PLATFORM=aws \
  -e THAND_ENVIRONMENT_CONFIG_REGION=us-west-2 \
  -e THAND_SERVICES_TEMPORAL_HOST=prod.temporal.example.com \
  -e THAND_SERVICES_TEMPORAL_MTLS_CERT_KEY_SECRET=thand/temporal/mtls/client-cert \
  -e AWS_ACCESS_KEY_ID=... \
  -e AWS_SECRET_ACCESS_KEY=... \
  ghcr.io/thand-io/agent:latest server
```

### Kubernetes with Mounted Secrets

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: thand-config
data:
  config.yaml: |
    services:
      temporal:
        host: "prod.temporal.example.com"
        port: 7233
        namespace: "production"
        mtls_cert_file: "/etc/temporal/certs/tls.crt"
        mtls_key_file: "/etc/temporal/certs/tls.key"
        mtls_ca_file: "/etc/temporal/ca/ca.crt"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thand-agent
spec:
  template:
    spec:
      containers:
      - name: thand
        image: ghcr.io/thand-io/agent:latest
        args: ["server"]
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: config.yaml
        - name: temporal-certs
          mountPath: /etc/temporal/certs
          readOnly: true
        - name: temporal-ca
          mountPath: /etc/temporal/ca
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: thand-config
      - name: temporal-certs
        secret:
          secretName: temporal-mtls
          defaultMode: 0444
      - name: temporal-ca
        secret:
          secretName: temporal-ca
          defaultMode: 0444
```

## Migration from Old Configuration

If you previously used the old `mtls_pem` or `mtls_cert_path` fields, update to the new field names:

**Old Configuration:**
```yaml
services:
  temporal:
    mtls_pem: "/path/to/cert.pem"  # DEPRECATED
```

**New Configuration:**
```yaml
services:
  temporal:
    mtls_cert_file: "/path/to/cert.pem"
    mtls_key_file: "/path/to/key.pem"
```

## Verification

To verify your mTLS configuration is working:

1. **Check logs during startup:**
   ```
   INFO Configuring Temporal client with mTLS authentication
   DEBUG Loaded mTLS certificate source=vault
   INFO Connecting to Temporal at prod.temporal.example.com:7233 in namespace production
   INFO Temporal namespace validation successful namespace=production state=REGISTERED
   ```

2. **Test Temporal connection:**
   - The agent should successfully connect and register as a worker
   - Workflows should execute normally
   - No TLS-related errors in logs

3. **Verify in Temporal UI:**
   - Check that your worker appears in the worker list
   - Worker should show as "REGISTERED" or "RUNNING"

## Additional Resources

- [Temporal mTLS Documentation](https://docs.temporal.io/self-hosted-guide/security#mtls)
- [Thand Configuration Guide](https://docs.thand.io/configuration/)
- [CSP Secret Management](https://docs.thand.io/security/secrets/)
