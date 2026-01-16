package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack/v5"
)

// KEKProvider is an interface for Key Encryption Key providers
// This can be implemented by Azure Key Vault, local config, or other providers
type KEKProvider interface {
	// EncryptKey encrypts a data encryption key (DEK)
	EncryptKey(ctx context.Context, dek []byte) ([]byte, error)
	// DecryptKey decrypts an encrypted data encryption key
	DecryptKey(ctx context.Context, encryptedDEK []byte) ([]byte, error)
}

// envelopeEncryptedData represents the structure of envelope-encrypted data (legacy JSON format)
type envelopeEncryptedData struct {
	Version      string `json:"version"`       // Version identifier for envelope format
	EncryptedDEK string `json:"encrypted_dek"` // Base64-encoded encrypted DEK
	Nonce        string `json:"nonce"`         // Base64-encoded nonce for AES-GCM
	Ciphertext   string `json:"ciphertext"`    // Base64-encoded encrypted data
}

// binaryEnvelope represents the compact MessagePack format (saves ~200-250 bytes vs JSON+base64)
type binaryEnvelope struct {
	Version      byte   `msgpack:"v"` // Version byte (0x01)
	EncryptedDEK []byte `msgpack:"d"` // Encrypted DEK (raw bytes)
	Nonce        []byte `msgpack:"n"` // GCM nonce (raw bytes)
	Ciphertext   []byte `msgpack:"c"` // Encrypted data (raw bytes)
}

const (
	// envelopeVersion is the current version of the envelope format
	envelopeVersion       = "v1"
	envelopeVersionBinary = 0x01 // Binary format version
	gcmNonceSize          = 12   // GCM nonce size in bytes
	dekSize               = 32   // 256-bit AES key
)

// EnvelopeEncrypt encrypts data using envelope encryption with compact binary format:
// 1. Generate random AES-256 key (Data Encryption Key - DEK)
// 2. Encrypt plaintext with AES-GCM using the DEK
// 3. Encrypt the DEK using the provided KEKProvider
// 4. Return binary envelope (saves ~200-250 bytes vs JSON+base64)
func EnvelopeEncrypt(ctx context.Context, plaintext []byte, kekProvider KEKProvider) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("plaintext cannot be empty")
	}

	// Generate random 32-byte AES-256 key (DEK)
	dek := make([]byte, dekSize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		logrus.WithError(err).Errorln("Failed to generate DEK")
		return nil, fmt.Errorf("failed to generate DEK: %w", err)
	}

	// Create AES cipher with DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Encrypt DEK with KEK provider first (Azure Key Vault, local key, etc.)
	encryptedDEK, err := kekProvider.EncryptKey(ctx, dek)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to encrypt DEK")
		return nil, fmt.Errorf("failed to encrypt DEK: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcmNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		logrus.WithError(err).Errorln("Failed to generate nonce")
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Use encrypted DEK as Additional Authenticated Data (AAD)
	// This binds the ciphertext to the specific encrypted DEK, preventing substitution attacks
	aad := encryptedDEK

	// Encrypt plaintext with DEK using AAD
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	// Clear DEK from memory
	for i := range dek {
		dek[i] = 0
	}
	// Prevent compiler from optimizing away the memory clearing
	runtime.KeepAlive(dek)

	// Create compact binary envelope using MessagePack
	envelope := binaryEnvelope{
		Version:      envelopeVersionBinary,
		EncryptedDEK: encryptedDEK,
		Nonce:        nonce,
		Ciphertext:   ciphertext,
	}

	// Marshal to MessagePack format
	data, err := msgpack.Marshal(&envelope)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to marshal envelope data")
		return nil, fmt.Errorf("failed to marshal envelope data: %w", err)
	}

	return data, nil
}

// EnvelopeDecrypt decrypts envelope-encrypted data with backward compatibility:
// 1. Parse the envelope (supports both binary and legacy JSON formats)
// 2. Decrypt the DEK using the KEKProvider
// 3. Decrypt the ciphertext using the DEK
func EnvelopeDecrypt(ctx context.Context, envelopeData []byte, kekProvider KEKProvider) ([]byte, error) {
	if len(envelopeData) == 0 {
		return nil, fmt.Errorf("envelope data cannot be empty")
	}

	var encryptedDEK, nonce, ciphertext []byte

	// Try MessagePack format first (current format)
	var binEnvelope binaryEnvelope
	if err := msgpack.Unmarshal(envelopeData, &binEnvelope); err == nil {
		// MessagePack format
		if binEnvelope.Version != envelopeVersionBinary {
			return nil, fmt.Errorf("unsupported binary envelope version: %d (expected %d)", binEnvelope.Version, envelopeVersionBinary)
		}
		encryptedDEK = binEnvelope.EncryptedDEK
		nonce = binEnvelope.Nonce
		ciphertext = binEnvelope.Ciphertext
	} else if envelopeData[0] == '{' {
		// Legacy JSON format (backward compatibility)
		var envelope envelopeEncryptedData
		if err := json.Unmarshal(envelopeData, &envelope); err != nil {
			logrus.WithError(err).Errorln("Failed to parse envelope data")
			return nil, fmt.Errorf("failed to parse envelope data: %w", err)
		}

		// Validate envelope version
		if envelope.Version != envelopeVersion {
			return nil, fmt.Errorf("unsupported envelope version: %s (expected %s)", envelope.Version, envelopeVersion)
		}

		// Decode base64 data
		var err error
		encryptedDEK, err = base64.StdEncoding.DecodeString(envelope.EncryptedDEK)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to decode encrypted DEK")
			return nil, fmt.Errorf("failed to decode encrypted DEK: %w", err)
		}

		nonce, err = base64.StdEncoding.DecodeString(envelope.Nonce)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to decode nonce")
			return nil, fmt.Errorf("failed to decode nonce: %w", err)
		}

		ciphertext, err = base64.StdEncoding.DecodeString(envelope.Ciphertext)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to decode ciphertext")
			return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unrecognized envelope format")
	}

	// Decrypt DEK with KEK provider
	dek, err := kekProvider.DecryptKey(ctx, encryptedDEK)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to decrypt DEK")
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	// Ensure DEK is cleared from memory after use
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
		// Prevent compiler from optimizing away the memory clearing
		runtime.KeepAlive(dek)
	}()

	// Create AES cipher with decrypted DEK
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Validate nonce size to prevent panics
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid nonce size: got %d, expected %d", len(nonce), gcm.NonceSize())
	}

	// Use encrypted DEK as Additional Authenticated Data (AAD)
	// This verifies the ciphertext is bound to the specific encrypted DEK
	aad := encryptedDEK

	// Decrypt ciphertext with DEK using AAD
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to decrypt data")
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}
