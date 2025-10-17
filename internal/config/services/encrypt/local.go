package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"golang.org/x/crypto/pbkdf2"
)

type localVault struct {
	config *models.BasicConfig
	key    []byte
	gcm    cipher.AEAD
}

type encryptedData struct {
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func NewLocalEncryptionFromConfig(config *models.BasicConfig) models.EncryptionImpl {
	return &localVault{
		config: config,
	}
}

// Derive a 256-bit key from password using PBKDF2
func deriveKey(password string, salt string) []byte {
	return pbkdf2.Key([]byte(password), []byte(salt), 100000, 32, sha256.New)
}

func (l *localVault) Initialize() error {

	masterPassword := l.config.GetStringWithDefault("password", "changeme")
	salt := l.config.GetStringWithDefault("salt", "changeme")

	l.key = deriveKey(masterPassword, salt)

	// Create AES cipher
	block, err := aes.NewCipher(l.key)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	l.gcm, err = cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	return nil
}

func (l *localVault) Shutdown() error {
	// Clear sensitive data
	for i := range l.key {
		l.key[i] = 0
	}
	return nil
}

func (l *localVault) Encrypt(ctx context.Context, plainText []byte) ([]byte, error) {

	if len(plainText) == 0 {
		return nil, fmt.Errorf("plaintext cannot be empty")
	}

	// Generate random nonce
	nonce := make([]byte, l.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		logrus.WithError(err).Errorln("Failed to generate nonce")
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the value
	ciphertext := l.gcm.Seal(nil, nonce, plainText, nil)

	// Prepare encrypted data structure
	encData := encryptedData{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}

	// Marshal to JSON
	data, err := json.Marshal(encData)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to marshal encrypted data")
		return nil, fmt.Errorf("failed to marshal encrypted data: %w", err)
	}

	return data, nil
}

func (l *localVault) Decrypt(ctx context.Context, cipherText []byte) ([]byte, error) {

	if len(cipherText) == 0 {
		return nil, fmt.Errorf("ciphertext cannot be empty")
	}

	// Parse encrypted data
	var encData encryptedData
	if err := json.Unmarshal(cipherText, &encData); err != nil {
		logrus.WithError(err).Errorln("Failed to parse encrypted data")
		return nil, fmt.Errorf("failed to parse encrypted data: %w", err)
	}

	// Decode base64 data
	nonce, err := base64.StdEncoding.DecodeString(encData.Nonce)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to decode nonce")
		return nil, fmt.Errorf("failed to decode nonce: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encData.Ciphertext)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to decode ciphertext")
		return nil, fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	// Decrypt
	plaintext, err := l.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to decrypt secret")
		return nil, fmt.Errorf("failed to decrypt secret: %w", err)
	}

	return plaintext, nil
}
