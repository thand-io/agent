package common

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"software.sslmate.com/src/go-pkcs12"
)

// Test data file paths
var (
	testDataDir = "testdata"
)

func getTestDataPath(filename string) string {
	return filepath.Join(testDataDir, filename)
}

func loadTestFile(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(getTestDataPath(filename))
	require.NoError(t, err, "Failed to load test file: %s", filename)
	return data
}

// Tests for DetectCertificateFormat

func TestDetectCertificateFormat_EmptyData(t *testing.T) {
	result := DetectCertificateFormat([]byte{})
	assert.Equal(t, "", result, "Empty data should return empty string")
}

func TestDetectCertificateFormat_PEM(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{
			name: "Full PEM certificate and key",
			file: "test_combined.pem",
		},
		{
			name: "PEM certificate only",
			file: "test_cert.pem",
		},
		{
			name: "PEM private key",
			file: "test_key.pem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFile(t, tt.file)
			result := DetectCertificateFormat(data)
			assert.Equal(t, "pem", result, "Should detect PEM format")
		})
	}
}

func TestDetectCertificateFormat_PKCS12(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		password string
	}{
		{
			name:     "PKCS12 without password",
			file:     "test_cert.p12",
			password: "",
		},
		{
			name:     "PKCS12 with password",
			file:     "test_cert_encrypted.p12",
			password: "testpass123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFile(t, tt.file)
			result := DetectCertificateFormat(data)
			assert.Equal(t, "pkcs12", result, "Should detect PKCS12 format")
		})
	}
}

func TestDetectCertificateFormat_DER(t *testing.T) {
	data := loadTestFile(t, "test_cert.der")
	result := DetectCertificateFormat(data)
	assert.Equal(t, "der", result, "Should detect DER format")
}

func TestDetectCertificateFormat_AmbiguousData(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "Random binary data",
			data:     []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			expected: "pem", // Should default to PEM
		},
		{
			name:     "Data starting with 0x30 but not valid ASN.1",
			data:     []byte{0x30, 0x00, 0x00, 0x00},
			expected: "pem", // Should default to PEM when neither PKCS12 nor DER
		},
		{
			name:     "Partial PEM header",
			data:     []byte("-----"),
			expected: "pem", // Should default to PEM
		},
		{
			name:     "Almost PEM header",
			data:     []byte("----BEGIN"),
			expected: "pem", // Should default to PEM
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCertificateFormat(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectCertificateFormat_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "Single byte",
			data:     []byte{0x30},
			expected: "pem",
		},
		{
			name:     "Two bytes starting with 0x30",
			data:     []byte{0x30, 0x80},
			expected: "pem", // Too short to be valid
		},
		{
			name:     "PEM header exactly",
			data:     []byte("-----BEGIN "),
			expected: "pem",
		},
		{
			name:     "Whitespace before PEM",
			data:     []byte("  -----BEGIN CERTIFICATE-----"),
			expected: "pem", // Should still default to PEM
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectCertificateFormat(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for helper functions

func TestParseASN1Length(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		expectedLength int64
		expectedBytes  int
	}{
		{
			name:           "Short form - small length",
			data:           []byte{0x05, 0xff},
			expectedLength: 5,
			expectedBytes:  1,
		},
		{
			name:           "Short form - max short length",
			data:           []byte{0x7f, 0xff},
			expectedLength: 127,
			expectedBytes:  1,
		},
		{
			name:           "Long form - 1 byte length",
			data:           []byte{0x81, 0x80, 0xff},
			expectedLength: 128,
			expectedBytes:  2,
		},
		{
			name:           "Long form - 2 byte length",
			data:           []byte{0x82, 0x01, 0x00, 0xff},
			expectedLength: 256,
			expectedBytes:  3,
		},
		{
			name:           "Long form - 3 byte length",
			data:           []byte{0x83, 0x01, 0x00, 0x00, 0xff},
			expectedLength: 65536,
			expectedBytes:  4,
		},
		{
			name:           "Empty data",
			data:           []byte{},
			expectedLength: 0,
			expectedBytes:  0,
		},
		{
			name:           "Invalid long form - not enough bytes",
			data:           []byte{0x82, 0x01},
			expectedLength: 0,
			expectedBytes:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			length, bytesUsed := ParseASN1Length(tt.data)
			assert.Equal(t, tt.expectedLength, length, "Length should match")
			assert.Equal(t, tt.expectedBytes, bytesUsed, "Bytes used should match")
		})
	}
}

func TestIsPKCS12(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		expected bool
	}{
		{
			name:     "Valid PKCS12 without password",
			file:     "test_cert.p12",
			expected: true,
		},
		{
			name:     "Valid PKCS12 with password",
			file:     "test_cert_encrypted.p12",
			expected: true,
		},
		{
			name:     "DER certificate",
			file:     "test_cert.der",
			expected: false,
		},
		{
			name:     "PEM certificate",
			file:     "test_cert.pem",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFile(t, tt.file)
			result := IsPKCS12(data)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Additional edge case tests with synthetic data
	t.Run("Too short data", func(t *testing.T) {
		result := IsPKCS12([]byte{0x30, 0x0e, 0x02, 0x01})
		assert.False(t, result)
	})

	t.Run("Invalid structure", func(t *testing.T) {
		result := IsPKCS12([]byte{0x30, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		assert.False(t, result)
	})
}

func TestIsDERCertificate(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		expected bool
	}{
		{
			name:     "Valid DER certificate",
			file:     "test_cert.der",
			expected: true,
		},
		{
			name:     "PKCS12 data",
			file:     "test_cert.p12",
			expected: false,
		},
		{
			name:     "PEM certificate",
			file:     "test_cert.pem",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := loadTestFile(t, tt.file)
			result := IsDERCertificate(data)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Additional edge case tests with synthetic data
	t.Run("Too short data", func(t *testing.T) {
		result := IsDERCertificate([]byte{0x30, 0x05, 0x30, 0x03})
		assert.False(t, result)
	})

	t.Run("Invalid structure - wrong tag", func(t *testing.T) {
		result := IsDERCertificate([]byte{0x30, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		assert.False(t, result)
	})
}

func TestMin(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"a < b", 5, 10, 5},
		{"a > b", 10, 5, 5},
		{"a == b", 7, 7, 7},
		{"negative numbers", -5, -10, -10},
		{"zero", 0, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Min(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Integration tests to verify detection works with actual parsing

func TestDetectAndParse_PEM(t *testing.T) {
	data := loadTestFile(t, "test_combined.pem")
	format := DetectCertificateFormat(data)
	require.Equal(t, "pem", format)

	// Verify it can be parsed as PEM
	var foundCert, foundKey bool
	remaining := data
	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			foundCert = true
		}
		if block.Type == "EC PRIVATE KEY" {
			foundKey = true
		}
		remaining = rest
	}

	assert.True(t, foundCert, "Should find certificate in PEM data")
	assert.True(t, foundKey, "Should find private key in PEM data")
}

func TestDetectAndParse_PKCS12(t *testing.T) {
	password := "testpass123"
	data := loadTestFile(t, "test_cert_encrypted.p12")
	format := DetectCertificateFormat(data)
	require.Equal(t, "pkcs12", format)

	// Verify it can be parsed as PKCS12
	privateKey, cert, _, err := pkcs12.DecodeChain(data, password)
	require.NoError(t, err)
	assert.NotNil(t, privateKey, "Should have private key")
	assert.NotNil(t, cert, "Should have certificate")
}

func TestDetectAndParse_DER(t *testing.T) {
	data := loadTestFile(t, "test_cert.der")
	format := DetectCertificateFormat(data)
	require.Equal(t, "der", format)

	// Verify it can be parsed as DER
	cert, err := x509.ParseCertificate(data)
	require.NoError(t, err)
	assert.NotNil(t, cert, "Should parse DER certificate")
}
