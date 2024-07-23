package save

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/julez-dev/chatuino/twitch/command"
)

const (
	chatuinoConfigDir = "chatuino"
	stateFileName     = "state.json"
)

type AppState struct {
	Tabs []TabState `json:"tabs"`
}

type TabState struct {
	Channel       string                    `json:"channel"`
	IsFocused     bool                      `json:"is_focused"`
	IdentityID    string                    `json:"identity_id"`
	SelectedIndex int                       `json:"selected_index"`
	IRCMessages   []*command.PrivateMessage `json:"irc_messages"`
}

func (a *AppState) Save() error {
	f, err := openCreateConfigFile(stateFileName)
	if err != nil {
		return err
	}

	defer f.Close()

	data, err := json.Marshal(a)
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

func AppStateFromDisk() (AppState, error) {
	f, err := openCreateConfigFile(stateFileName)
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

func openCreateConfigFile(file string) (*os.File, error) {
	configDir, err := os.UserConfigDir() // get users config directory, depending on OS
	if err != nil {
		return nil, err
	}

	// ensure dir config dir exists
	configDirChatuino := filepath.Join(configDir, chatuinoConfigDir)
	err = os.Mkdir(configDirChatuino, 0o755)
	var alreadyExistsError bool

	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			alreadyExistsError = true
		} else {
			return nil, err
		}
	}

	if err != nil && !alreadyExistsError {
		return nil, err
	}

	path := filepath.Join(configDirChatuino, file)

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	return f, nil
}
