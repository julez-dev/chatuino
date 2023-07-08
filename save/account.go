package save

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	chatuinoConfigDir = "chatuino"
	accountFileName   = "accounts.json"
)

type AccountList struct {
	Accounts []Account `json:"accounts"`
}

func (a *AccountList) Save() error {
	f, err := openCreateConfigFile(accountFileName)

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

type Account struct {
	DisplayName  string `json:"display_name"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func openCreateConfigFile(file string) (*os.File, error) {
	configDir, err := os.UserConfigDir() // get users config directory, depending on OS
	if err != nil {
		return nil, err
	}

	// ensure dir config dir exists
	configDirChatuino := filepath.Join(configDir, chatuinoConfigDir)
	err = os.Mkdir(configDir, os.ModePerm)

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

func AccountListFromDisk() (AccountList, error) {
	f, err := openCreateConfigFile(accountFileName)

	if err != nil {
		return AccountList{}, err
	}

	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return AccountList{}, err
	}

	var list = AccountList{}
	err = json.Unmarshal(data, &list)

	if err != nil {
		syntaxErr := &json.SyntaxError{}
		if errors.As(err, &syntaxErr) {
			return AccountList{}, nil
		}
		return AccountList{}, err
	}

	return list, nil
}
