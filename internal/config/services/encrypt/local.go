package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/pbkdf2"
)

type localVault struct {
	config *models.BasicConfig
	key    []byte
	gcm    cipher.AEAD
	mu     sync.Mutex // Protects GCM operations for thread safety
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

	// TODO(hugh): Come up with a better way to have a final default secret that isn't
	// changeme. See the client.go - this will try and create a better default based on
	// the environment config identifier if none is provided.
	masterPassword := l.config.GetStringWithDefault("password", common.DefaultServerSecret)
	salt := l.config.GetStringWithDefault("salt", common.DefaultServerSecret) // Use hostname as default salt

	// Fail if using default secrets - these are not secure for production use
	if strings.EqualFold(masterPassword, common.DefaultServerSecret) ||
		strings.EqualFold(salt, common.DefaultServerSecret) {
		logrus.Warningln("local encryption service configured with default secrets. See https://docs.thand.io/configuration/file.html#encryption-service")
	}

	// Validate salt length - minimum 16 bytes for security
	if len(salt) < 16 {
		logrus.Warningln("local encryption service configured with weak salt (less than 16 bytes). See https://docs.thand.io/configuration/file.html#encryption-service")	
	}

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

// EncryptKey implements KEKProvider interface - encrypts a DEK with the local master key
func (l *localVault) EncryptKey(ctx context.Context, dek []byte) ([]byte, error) {
	// Generate random nonce
	nonce := make([]byte, l.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt DEK with master key (mutex protects concurrent GCM access)
	l.mu.Lock()
	ciphertext := l.gcm.Seal(nonce, nonce, dek, nil)
	l.mu.Unlock()

	return ciphertext, nil
}

// DecryptKey implements KEKProvider interface - decrypts a DEK with the local master key
func (l *localVault) DecryptKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	nonceSize := l.gcm.NonceSize()
	if len(encryptedDEK) < nonceSize {
		return nil, fmt.Errorf("encrypted DEK too short")
	}

	nonce, ciphertext := encryptedDEK[:nonceSize], encryptedDEK[nonceSize:]

	// Decrypt DEK with master key (mutex protects concurrent GCM access)
	l.mu.Lock()
	dek, err := l.gcm.Open(nil, nonce, ciphertext, nil)
	l.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	return dek, nil
}

func (l *localVault) Encrypt(ctx context.Context, plainText []byte) ([]byte, error) {
	// Use envelope encryption with local master key as KEK provider
	return EnvelopeEncrypt(ctx, plainText, l)
}

func (l *localVault) Decrypt(ctx context.Context, cipherText []byte) ([]byte, error) {
	// Use envelope decryption with local master key as KEK provider
	return EnvelopeDecrypt(ctx, cipherText, l)
}
