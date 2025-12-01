package save

import (
	"io"
	"sync"

	"github.com/spf13/afero"
	"github.com/zalando/go-keyring"
)

// enforce that PlainKeyringFallback implements the Keyring interface
var _ keyring.Keyring = &PlainKeyringFallback{}

const plainAuthFile = "accounts.json"

type PlainKeyringFallback struct {
	m  *sync.RWMutex
	fs afero.Fs
}

func NewPlainKeyringFallback(fs afero.Fs) *PlainKeyringFallback {
	return &PlainKeyringFallback{
		m:  &sync.RWMutex{},
		fs: fs,
	}
}

func (p *PlainKeyringFallback) Set(_, _, jsonData string) error {
	return p.write(jsonData)
}

func (p *PlainKeyringFallback) Get(_, _ string) (string, error) {
	return p.read()
}

func (p *PlainKeyringFallback) Delete(_, _ string) error {
	return nil
}

func (p *PlainKeyringFallback) DeleteAll(_ string) error {
	return nil
}

func (p *PlainKeyringFallback) read() (string, error) {
	p.m.RLock()
	defer p.m.RUnlock()

	f, err := openCreateConfigFile(p.fs, plainAuthFile)
	if err != nil {
		return "", err
	}

	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (p *PlainKeyringFallback) write(data string) error {
	p.m.Lock()
	defer p.m.Unlock()

	f, err := openCreateConfigFile(p.fs, plainAuthFile)
	if err != nil {
		return err
	}

	defer f.Close()

	if f.Truncate(0) != nil {
		return err
	}

	if _, err := f.Seek(0, 0); err != nil {
		return err
	}

	_, err = io.WriteString(f, data)
	if err != nil {
		return err
	}

	return nil
}
