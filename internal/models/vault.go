package models

type VaultImpl interface {
	Initialize() error
	Shutdown() error

	GetSecret(key string) ([]byte, error)
	StoreSecret(key string, value []byte) error
}
