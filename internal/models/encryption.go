package models

type EncryptionImpl interface {
	Initialize() error
	Shutdown() error

	Encrypt(plainText []byte) ([]byte, error)
	Decrypt(cipherText []byte) ([]byte, error)
}
