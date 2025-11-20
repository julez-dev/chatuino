package save

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

const (
	chatuinoConfigDir = "chatuino"
	stateFileName     = "state.json"
)

type AppState struct {
	Tabs []TabState `json:"tabs"`
}

type TabState struct {
	IsLocalUnique bool   `json:"is_local_unique"`
	IsLocalSub    bool   `json:"is_local_sub"`
	Channel       string `json:"channel"`
	IsFocused     bool   `json:"is_focused"`
	IdentityID    string `json:"identity_id"`
	Kind          int    `json:"kind"`
}

type AppStateManager struct {
	fs afero.Fs
}

func NewAppStateManager(fs afero.Fs) *AppStateManager {
	return &AppStateManager{fs: fs}
}

func (a *AppStateManager) SaveAppState(state AppState) error {
	f, err := openCreateConfigFile(a.fs, stateFileName)
	if err != nil {
		return err
	}

	defer f.Close()

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	err = f.Truncate(0)
	if err != nil {
		return err
	}

	_, err = io.Copy(f, bytes.NewReader(data))
	if err != nil {
		return err
	}

	return nil
}

func (a *AppStateManager) LoadAppState() (AppState, error) {
	f, err := openCreateConfigFile(a.fs, stateFileName)
	if err != nil {
		return AppState{}, err
	}

	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return AppState{}, err
	}

	state := AppState{}
	err = json.Unmarshal(data, &state)
	if err != nil {
		syntaxErr := &json.SyntaxError{}
		if errors.As(err, &syntaxErr) {
			return AppState{}, nil
		}
		return AppState{}, err
	}

	return state, nil
}

func openCreateFile(fs afero.Fs, base string, file string) (afero.File, error) {
	// ensure dir config dir exists
	configDirChatuino := filepath.Join(base, chatuinoConfigDir)
	err := os.Mkdir(configDirChatuino, 0o755)
	var alreadyExistsError bool

	if err != nil {
		if errors.Is(err, afero.ErrFileExists) {
			alreadyExistsError = true
		} else {
			return nil, err
		}
	}

	if err != nil && !alreadyExistsError {
		return nil, err
	}

	path := filepath.Join(configDirChatuino, file)

	f, err := fs.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func openCreateConfigFile(fs afero.Fs, file string) (afero.File, error) {
	configDir, err := os.UserConfigDir() // get users config directory, depending on OS
	if err != nil {
		return nil, err
	}

	return openCreateFile(fs, configDir, file)
}
