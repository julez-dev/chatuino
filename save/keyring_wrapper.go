package save

import (
	"sync"

	"github.com/zalando/go-keyring"
)

// enforce that KeyringWrapper implements the Keyring interface
var _ keyring.Keyring = KeyringWrapper{}

type KeyringWrapper struct {
	m *sync.Mutex
}

func NewKeyringWrapper() *KeyringWrapper {
	return &KeyringWrapper{
		m: &sync.Mutex{},
	}
}

func (k KeyringWrapper) Set(service, user, password string) error {
	k.m.Lock()
	defer k.m.Unlock()
	return keyring.Set(service, user, password)
}

func (k KeyringWrapper) Get(service, user string) (string, error) {
	k.m.Lock()
	defer k.m.Unlock()
	return keyring.Get(service, user)
}

func (k KeyringWrapper) Delete(service, user string) error {
	k.m.Lock()
	defer k.m.Unlock()
	return keyring.Delete(service, user)
}

func (k KeyringWrapper) DeleteAll(service string) error {
	k.m.Lock()
	defer k.m.Unlock()
	return keyring.DeleteAll(service)
}
