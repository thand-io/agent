#!/bin/bash
# Script to generate test certificates for certificate detection tests

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
CERT_DIR="${SCRIPT_DIR}"

echo "Generating test certificates in ${CERT_DIR}..."

# Generate private key
openssl ecparam -genkey -name prime256v1 -out "${CERT_DIR}/test_key.pem"

# Generate self-signed certificate
openssl req -new -x509 -key "${CERT_DIR}/test_key.pem" \
    -out "${CERT_DIR}/test_cert.pem" \
    -days 3650 \
    -subj "/C=US/ST=Test/L=Test/O=TestOrg/CN=test.example.com"

# Create combined PEM (certificate + private key)
cat "${CERT_DIR}/test_cert.pem" "${CERT_DIR}/test_key.pem" > "${CERT_DIR}/test_combined.pem"

# Create DER format certificate (certificate only, no private key)
openssl x509 -in "${CERT_DIR}/test_cert.pem" -out "${CERT_DIR}/test_cert.der" -outform DER

# Create PKCS12 without password
openssl pkcs12 -export -out "${CERT_DIR}/test_cert.p12" \
    -inkey "${CERT_DIR}/test_key.pem" \
    -in "${CERT_DIR}/test_cert.pem" \
    -passout pass:

# Create PKCS12 with password
openssl pkcs12 -export -out "${CERT_DIR}/test_cert_encrypted.p12" \
    -inkey "${CERT_DIR}/test_key.pem" \
    -in "${CERT_DIR}/test_cert.pem" \
    -passout pass:testpass123

echo "Test certificates generated successfully:"
echo "  - test_cert.pem (PEM certificate only)"
echo "  - test_key.pem (PEM private key only)"
echo "  - test_combined.pem (PEM certificate + key)"
echo "  - test_cert.der (DER format certificate)"
echo "  - test_cert.p12 (PKCS12 without password)"
echo "  - test_cert_encrypted.p12 (PKCS12 with password: testpass123)"
