package save

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/julez-dev/chatuino/twitch/command"
)

const (
	stateFileName = "state.json"
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
