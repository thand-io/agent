package encrypt

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKEKProvider implements KEKProvider for testing
type mockKEKProvider struct {
	encryptedKeySize int // Simulates RSA-2048 (256 bytes) or RSA-4096 (512 bytes)
}

func (m *mockKEKProvider) EncryptKey(ctx context.Context, dek []byte) ([]byte, error) {
	// Simulate RSA encryption by returning random bytes of specified size
	encrypted := make([]byte, m.encryptedKeySize)
	if _, err := io.ReadFull(rand.Reader, encrypted); err != nil {
		return nil, err
	}
	return encrypted, nil
}

func (m *mockKEKProvider) DecryptKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	// For testing, just return a valid 32-byte AES key
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, err
	}
	return dek, nil
}

func TestEnvelopeEncrypt_BinaryFormatSizeOptimization(t *testing.T) {
	ctx := context.Background()

	// Use real test provider that can actually decrypt
	provider := &testKEKProvider{}
	
	// Test with realistic session data size (1500 bytes - typical for JWT with user info)
	sessionData := make([]byte, 1500)
	_, err := rand.Read(sessionData)
	require.NoError(t, err)

	// Encrypt with MessagePack format
	encrypted, err := EnvelopeEncrypt(ctx, sessionData, provider)
	require.NoError(t, err)

	t.Logf("MessagePack format - Plaintext: %d bytes, Encrypted envelope: %d bytes", 
		len(sessionData), len(encrypted))

	// MessagePack adds small overhead for field tags and structure
	// Expected: ~1803 bytes (msgpack overhead ~16 bytes for 4 fields)
	// Still much better than JSON which would be ~2464 bytes
	
	assert.Less(t, len(encrypted), 1850, 
		"MessagePack format should be under 1850 bytes")

	// Verify it can be decrypted
	decrypted, err := EnvelopeDecrypt(ctx, encrypted, provider)
	require.NoError(t, err)
	assert.Equal(t, sessionData, decrypted, "Decrypted data should match original")
}

func TestEnvelopeEncrypt_BinaryVsJSON_SizeComparison(t *testing.T) {
	// This test demonstrates the size savings of MessagePack format vs legacy JSON format
	ctx := context.Background()
	provider := &mockKEKProvider{encryptedKeySize: 256} // RSA-2048
	
	sessionData := make([]byte, 1500)
	_, err := rand.Read(sessionData)
	require.NoError(t, err)

	// MessagePack format
	binaryEncrypted, err := EnvelopeEncrypt(ctx, sessionData, provider)
	require.NoError(t, err)

	// Calculate what the old JSON format size would be:
	// {
	//   "version": "v1",
	//   "encrypted_dek": "<base64 of 256 bytes = 342 chars>",
	//   "nonce": "<base64 of 12 bytes = 16 chars>",
	//   "ciphertext": "<base64 of (1500+16) bytes = 2022 chars>"
	// }
	// JSON overhead: field names (~60 chars), quotes/colons/braces (~20 chars)
	// Total base64 data: 342 + 16 + 2022 = 2380 chars
	// Total JSON: ~2460 chars

	base64DEKSize := ((256 + 2) / 3) * 4         // 342 chars
	base64NonceSize := ((12 + 2) / 3) * 4        // 16 chars
	base64CiphertextSize := ((1516 + 2) / 3) * 4 // 2024 chars
	jsonOverhead := 80                           // field names, quotes, braces, commas

	estimatedJSONSize := base64DEKSize + base64NonceSize + base64CiphertextSize + jsonOverhead

	savings := estimatedJSONSize - len(binaryEncrypted)
	savingsPercent := float64(savings) / float64(estimatedJSONSize) * 100

	t.Logf("MessagePack format: %d bytes", len(binaryEncrypted))
	t.Logf("Estimated JSON format: ~%d bytes", estimatedJSONSize)
	t.Logf("Savings: ~%d bytes (%.1f%%)", savings, savingsPercent)

	assert.Greater(t, savings, 600,
		"MessagePack format should save at least 600 bytes compared to JSON format")
	assert.Greater(t, savingsPercent, 25.0,
		"MessagePack format should be at least 25%% smaller than JSON format")
}

func TestEnvelopeDecrypt_BackwardCompatibility(t *testing.T) {
	ctx := context.Background()

	// Create a mock provider that can decrypt
	provider := &testKEKProvider{}

	plaintext := []byte("test data for backward compatibility")

	// Create legacy JSON format manually (simulating old encrypted data)
	legacyJSON := createLegacyJSONEnvelope(t, plaintext, provider)

	// Should be able to decrypt legacy format
	decrypted, err := EnvelopeDecrypt(ctx, legacyJSON, provider)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted, "Should decrypt legacy JSON format")

	// Create new MessagePack format
	binaryEnvelope, err := EnvelopeEncrypt(ctx, plaintext, provider)
	require.NoError(t, err)

	// Should be able to decrypt MessagePack format
	decrypted, err = EnvelopeDecrypt(ctx, binaryEnvelope, provider)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted, "Should decrypt new MessagePack format")
}

func TestEnvelopeEncrypt_AzureRealWorldScenario(t *testing.T) {
	// Test with realistic Azure deployment constraints:
	// - RSA-2048 Key Vault key (256 byte encrypted DEK)
	// - Cookie size limit: 4096 bytes
	// - securecookie adds ~60% overhead (base64 + HMAC)

	ctx := context.Background()
	azureProvider := &mockKEKProvider{encryptedKeySize: 256}

	// Calculate maximum session size that will fit in 4096 byte cookie
	// Formula: (envelope_size * 1.6) + overhead <= 4096
	// Solving for envelope_size: envelope_size <= (4096 - 100) / 1.6 ≈ 2497 bytes

	maxEnvelopeSize := 2497

	// Work backwards to find session data size:
	// envelope = 1 + 2 + 256 + 12 + (session + 16)
	// session = envelope - 287
	maxSessionSize := maxEnvelopeSize - 287

	t.Logf("Maximum session size to fit in 4096 byte cookie: %d bytes", maxSessionSize)

	// Test with maximum size
	sessionData := make([]byte, maxSessionSize)
	_, err := rand.Read(sessionData)
	require.NoError(t, err)

	encrypted, err := EnvelopeEncrypt(ctx, sessionData, azureProvider)
	require.NoError(t, err)

	t.Logf("Envelope size: %d bytes", len(encrypted))

	// Simulate securecookie overhead
	estimatedCookieSize := int(float64(len(encrypted)) * 1.6)

	t.Logf("Estimated cookie size after securecookie: %d bytes", estimatedCookieSize)

	assert.LessOrEqual(t, estimatedCookieSize, 4096,
		"Should fit within 4096 byte cookie limit with securecookie overhead")

	// Log the savings vs JSON format
	estimatedJSONSize := len(encrypted) + 230 // JSON adds ~230 bytes overhead vs MessagePack
	t.Logf("Old JSON format would be: ~%d bytes", estimatedJSONSize)
	t.Logf("MessagePack format saves: ~%d bytes", 230)
}

// testKEKProvider implements KEKProvider for testing with actual encryption/decryption
type testKEKProvider struct {
	key []byte
}

func (t *testKEKProvider) EncryptKey(ctx context.Context, dek []byte) ([]byte, error) {
	// Use AES-GCM to encrypt the DEK (simpler than RSA for testing)
	if t.key == nil {
		t.key = make([]byte, 32)
		rand.Read(t.key)
	}

	block, err := aes.NewCipher(t.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)

	ciphertext := gcm.Seal(nonce, nonce, dek, nil)
	return ciphertext, nil
}

func (t *testKEKProvider) DecryptKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	if t.key == nil {
		t.key = make([]byte, 32)
		rand.Read(t.key)
	}

	block, err := aes.NewCipher(t.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	nonce, ciphertext := encryptedDEK[:nonceSize], encryptedDEK[nonceSize:]

	return gcm.Open(nil, nonce, ciphertext, nil)
}

// createLegacyJSONEnvelope creates an envelope in the old JSON format for testing
func createLegacyJSONEnvelope(t *testing.T, plaintext []byte, provider KEKProvider) []byte {
	ctx := context.Background()

	// Generate DEK
	dek := make([]byte, 32)
	rand.Read(dek)

	// Encrypt DEK
	encryptedDEK, err := provider.EncryptKey(ctx, dek)
	require.NoError(t, err)

	// Encrypt plaintext
	block, err := aes.NewCipher(dek)
	require.NoError(t, err)

	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)

	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)

	ciphertext := gcm.Seal(nil, nonce, plaintext, encryptedDEK)

	// Create JSON envelope (legacy format)
	legacy := map[string]string{
		"version":       "v1",
		"encrypted_dek": base64.StdEncoding.EncodeToString(encryptedDEK),
		"nonce":         base64.StdEncoding.EncodeToString(nonce),
		"ciphertext":    base64.StdEncoding.EncodeToString(ciphertext),
	}

	jsonData, err := json.Marshal(legacy)
	require.NoError(t, err)

	return jsonData
}
