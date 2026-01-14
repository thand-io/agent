package certificates

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

// ParseCombinedPEM parses a combined PEM format containing both certificate(s) and private key.
// This format is used when storing certificates in CSP vault secrets.
//
// The combined PEM should contain:
// - One or more CERTIFICATE blocks (certificate chain)
// - Exactly one PRIVATE KEY block (RSA, EC, or PKCS8 format)
//
// Returns:
// - certPEM: All certificate blocks combined (maintains order for cert chains)
// - keyPEM: The single private key block
// - error: If no certificate or key found, or multiple keys found
func ParseCombinedPEM(data []byte) (certPEM, keyPEM []byte, err error) {
	var certBlocks [][]byte
	var keyBlock []byte
	var keyBlockFound bool

	remaining := data
	for {
		var block *pem.Block
		block, remaining = pem.Decode(remaining)
		if block == nil {
			break
		}

		switch block.Type {
		case "CERTIFICATE":
			// Encode back to PEM format and append
			certBlocks = append(certBlocks, pem.EncodeToMemory(block))

		case "RSA PRIVATE KEY", "EC PRIVATE KEY", "PRIVATE KEY":
			if keyBlockFound {
				return nil, nil, fmt.Errorf("multiple private key blocks found in combined PEM (only one is allowed)")
			}
			keyBlock = pem.EncodeToMemory(block)
			keyBlockFound = true

			// Ignore other block types (e.g., PUBLIC KEY, comments)
		}
	}

	// Validate that we found at least one certificate
	if len(certBlocks) == 0 {
		return nil, nil, fmt.Errorf("no certificate block found in combined PEM")
	}

	// Validate that we found exactly one private key
	if !keyBlockFound {
		return nil, nil, fmt.Errorf("no private key block found (RSA, EC, or PKCS8 required)")
	}

	// Combine all certificate blocks (for certificate chains)
	for _, cert := range certBlocks {
		certPEM = append(certPEM, cert...)
	}

	return certPEM, keyBlock, nil
}

// ValidateCertificate performs basic validation on a PEM-encoded certificate
// to ensure it can be parsed as a valid X.509 certificate.
func ValidateCertificate(certPEM []byte) error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	if block.Type != "CERTIFICATE" {
		return fmt.Errorf("PEM block is not a certificate (type: %s)", block.Type)
	}

	_, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	return nil
}
