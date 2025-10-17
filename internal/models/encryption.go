package models

import "context"

type EncryptionImpl interface {
	Initialize() error
	Shutdown() error

	Encrypt(ctx context.Context, plainText []byte) ([]byte, error)
	Decrypt(ctx context.Context, cipherText []byte) ([]byte, error)
}
