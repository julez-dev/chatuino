package save

import "github.com/zalando/go-keyring"

// enforce that KeyringWrapper implements the Keyring interface
var _ keyring.Keyring = KeyringWrapper{}

type KeyringWrapper struct{}

func (k KeyringWrapper) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}

func (k KeyringWrapper) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (k KeyringWrapper) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

func (k KeyringWrapper) DeleteAll(service string) error {
	return keyring.DeleteAll(service)
}
