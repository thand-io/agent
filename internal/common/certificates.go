package common

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// DetectCertificateFormat attempts to detect the format of certificate data
// by examining byte patterns and ASN.1 structure. Returns "pem", "pkcs12", "der", or empty string.
func DetectCertificateFormat(data []byte) string {
	if len(data) == 0 {
		logrus.Warn("No data provided for certificate format detection")
		return ""
	}

	// Check for PEM format (text-based, starts with -----BEGIN)
	if len(data) >= 11 && data[0] == '-' && data[1] == '-' && data[2] == '-' && data[3] == '-' && data[4] == '-' {
		if len(data) >= 15 && string(data[0:11]) == "-----BEGIN " {
			logrus.Debug("Detected PEM format by header")
			return "pem"
		}
	}

	// Check for binary formats starting with ASN.1 SEQUENCE tag (0x30)
	if len(data) >= 2 && data[0] == 0x30 {
		// Both PKCS12 and DER start with 0x30, need to distinguish
		// PKCS12 structure: SEQUENCE { version INTEGER, authSafe ContentInfo, ... }
		// ContentInfo contains OID for pkcs7-data (1.2.840.113549.1.7.1) or pkcs7-encryptedData
		
		if IsPKCS12(data) {
			logrus.Debug("Detected PKCS12 format by structure analysis")
			return "pkcs12"
		}
		
		// Check if it's a valid DER certificate
		if IsDERCertificate(data) {
			logrus.Debug("Detected DER format by X.509 structure")
			return "der"
		}
	}

	// Default to PEM if uncertain (most common format)
	endIdx := len(data)
	if endIdx > 16 {
		endIdx = 16
	}
	logrus.WithField("first_bytes", fmt.Sprintf("%x", data[0:endIdx])).Debug("Unable to detect format, defaulting to PEM")
	return "pem"
}

// IsPKCS12 checks if data has PKCS12 structure by examining ASN.1 encoding
func IsPKCS12(data []byte) bool {
	if len(data) < 20 {
		return false
	}

	// PKCS12 PFX structure starts with:
	// SEQUENCE {
	//   version INTEGER (0x02 0x01 0x03 for version 3)
	//   authSafe ContentInfo SEQUENCE {
	//     contentType OBJECT IDENTIFIER (pkcs7-data: 1.2.840.113549.1.7.1)
	//     ...
	//   }
	// }

	idx := 1 // Skip SEQUENCE tag (0x30)
	
	// Parse length
	_, lengthSize := ParseASN1Length(data[idx:])
	if lengthSize == 0 {
		return false
	}
	idx += lengthSize

	// Check for version INTEGER tag (0x02)
	if idx >= len(data) || data[idx] != 0x02 {
		return false
	}
	idx++

	// Skip version length and value
	if idx >= len(data) {
		return false
	}
	versionLen := int(data[idx])
	idx += 1 + versionLen

	// Check for authSafe SEQUENCE tag (0x30)
	if idx >= len(data) || data[idx] != 0x30 {
		return false
	}
	idx++

	// Skip authSafe length
	_, lengthSize = ParseASN1Length(data[idx:])
	if lengthSize == 0 {
		return false
	}
	idx += lengthSize

	// Check for OID tag (0x06)
	if idx >= len(data) || data[idx] != 0x06 {
		return false
	}
	idx++

	// Check OID length and value for pkcs7-data (1.2.840.113549.1.7.1)
	// Encoded as: 0x09 0x2a 0x86 0x48 0x86 0xf7 0x0d 0x01 0x07 0x01
	if idx >= len(data) {
		return false
	}
	oidLen := int(data[idx])
	idx++

	if oidLen == 9 && idx+9 <= len(data) {
		oid := data[idx : idx+9]
		// Check for pkcs7-data OID: 1.2.840.113549.1.7.1
		if oid[0] == 0x2a && oid[1] == 0x86 && oid[2] == 0x48 &&
			oid[3] == 0x86 && oid[4] == 0xf7 && oid[5] == 0x0d &&
			oid[6] == 0x01 && oid[7] == 0x07 && (oid[8] == 0x01 || oid[8] == 0x06) {
			return true
		}
	}

	return false
}

// IsDERCertificate checks if data is a valid DER-encoded X.509 certificate
func IsDERCertificate(data []byte) bool {
	if len(data) < 10 {
		return false
	}

	// X.509 Certificate structure:
	// SEQUENCE {
	//   tbsCertificate SEQUENCE {
	//     version [0] EXPLICIT INTEGER DEFAULT v1
	//     serialNumber INTEGER
	//     ...
	//   }
	//   signatureAlgorithm AlgorithmIdentifier SEQUENCE
	//   signature BIT STRING
	// }

	idx := 1 // Skip SEQUENCE tag (0x30)

	// Parse length
	certLen, lengthSize := ParseASN1Length(data[idx:])
	if lengthSize == 0 || certLen == 0 {
		return false
	}
	idx += lengthSize

	// Check that certificate length matches data length (accounting for header)
	expectedLen := 1 + lengthSize + int(certLen)
	if expectedLen != len(data) {
		// Length mismatch, probably not a standalone DER certificate
		return false
	}

	// Check for tbsCertificate SEQUENCE tag (0x30)
	if idx >= len(data) || data[idx] != 0x30 {
		return false
	}
	idx++

	// Parse tbsCertificate length
	_, lengthSize = ParseASN1Length(data[idx:])
	if lengthSize == 0 {
		return false
	}
	idx += lengthSize

	// Check for version tag (0xA0 for explicit [0]) or INTEGER tag (0x02 for v1)
	if idx >= len(data) {
		return false
	}
	if data[idx] == 0xA0 || data[idx] == 0x02 {
		return true
	}

	return false
}

// ParseASN1Length parses ASN.1 length encoding and returns (length, bytesUsed)
func ParseASN1Length(data []byte) (int64, int) {
	if len(data) == 0 {
		return 0, 0
	}

	firstByte := data[0]

	// Short form: length < 128
	if firstByte&0x80 == 0 {
		return int64(firstByte), 1
	}

	// Long form: first byte indicates number of length bytes
	numLengthBytes := int(firstByte & 0x7f)
	if numLengthBytes == 0 || numLengthBytes > 4 || len(data) < 1+numLengthBytes {
		return 0, 0
	}

	var length int64
	for i := 0; i < numLengthBytes; i++ {
		length = (length << 8) | int64(data[1+i])
	}

	return length, 1 + numLengthBytes
}
