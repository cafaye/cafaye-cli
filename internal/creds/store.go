package creds

import (
	"fmt"
	"os"

	keyring "github.com/zalando/go-keyring"
)

type Store interface {
	Set(ref string, token string) error
	Get(ref string) (string, error)
	Delete(ref string) error
}

type KeyringStore struct {
	service string
}

func NewKeyringStore(service string) *KeyringStore {
	return &KeyringStore{service: service}
}

func (s *KeyringStore) Set(ref string, token string) error {
	if os.Getenv("CAFAYE_CLI_DISABLE_KEYRING") == "1" {
		return fmt.Errorf("keyring disabled")
	}
	return keyring.Set(s.service, ref, token)
}

func (s *KeyringStore) Get(ref string) (string, error) {
	if os.Getenv("CAFAYE_CLI_DISABLE_KEYRING") == "1" {
		return "", fmt.Errorf("keyring disabled")
	}
	return keyring.Get(s.service, ref)
}

func (s *KeyringStore) Delete(ref string) error {
	if os.Getenv("CAFAYE_CLI_DISABLE_KEYRING") == "1" {
		return nil
	}
	return keyring.Delete(s.service, ref)
}

type MemoryStore struct {
	items map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]string{}}
}

func (m *MemoryStore) Set(ref string, token string) error {
	m.items[ref] = token
	return nil
}

func (m *MemoryStore) Get(ref string) (string, error) {
	t, ok := m.items[ref]
	if !ok {
		return "", fmt.Errorf("token not found")
	}
	return t, nil
}

func (m *MemoryStore) Delete(ref string) error {
	delete(m.items, ref)
	return nil
}
