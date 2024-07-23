package save

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keychainService = "chatuino"
	keychainAccount = "account-save"
)

var ErrAccountNotFound = errors.New("account not found")

var anonymousAccount = Account{
	ID:          "anonymous-account",
	IsMain:      false,
	IsAnonymous: true,
	DisplayName: "justinfan123123",
	AccessToken: "oauth:123123123",
	CreatedAt:   time.Now(),
}

type Account struct {
	ID           string    `json:"id"`
	IsMain       bool      `json:"is_main"`
	IsAnonymous  bool      `json:"-"`
	DisplayName  string    `json:"display_name"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	CreatedAt    time.Time `json:"created_at"`
}

type accountFile struct {
	Accounts []Account `json:"accounts"`
}

type AccountProvider struct {
	keychain keyring.Keyring
}

func NewAccountProvider(keychain keyring.Keyring) AccountProvider {
	return AccountProvider{keychain: keychain}
}

func (a AccountProvider) GetAccountBy(id string) (Account, error) {
	accounts, err := a.loadAccounts()
	if err != nil {
		return Account{}, err
	}

	if i := slices.IndexFunc(accounts, func(a Account) bool { return a.ID == id }); i != -1 {
		return accounts[i], nil
	}

	return Account{}, ErrAccountNotFound
}

func (a AccountProvider) GetMainAccount() (Account, error) {
	accounts, err := a.loadAccounts()
	if err != nil {
		return Account{}, err
	}

	if i := slices.IndexFunc(accounts, func(a Account) bool { return a.IsMain }); i != -1 {
		return accounts[i], nil
	}

	return Account{}, ErrAccountNotFound
}

func (a AccountProvider) GetAllAccounts() ([]Account, error) {
	accounts, err := a.loadAccounts()
	if err != nil {
		return nil, err
	}

	return accounts, nil
}

func (a AccountProvider) Remove(id string) error {
	accounts, err := a.loadAccounts()
	if err != nil {
		return err
	}

	i := slices.IndexFunc(accounts, func(a Account) bool { return a.ID == id })

	if i == -1 {
		return ErrAccountNotFound
	}

	// If account was main account, select a new main account if available
	if accounts[i].IsMain {
		indexNewMain := slices.IndexFunc(accounts, func(a Account) bool { return a.ID != id && !a.IsAnonymous })

		if indexNewMain != -1 {
			accounts[indexNewMain].IsMain = true
		}
	}

	accounts = slices.Delete(accounts, i, i+1)

	if err = a.saveAccounts(accounts); err != nil {
		return err
	}

	return nil
}

func (a AccountProvider) Add(account Account) error {
	accounts, err := a.loadAccounts()
	if err != nil {
		return err
	}

	// If account already exists, throw error
	if i := slices.IndexFunc(accounts, func(a Account) bool { return a.ID == account.ID }); i != -1 {
		return fmt.Errorf("account with id %s already exists", account.ID)
	}

	// Don't allow anonymous account
	account.IsAnonymous = false

	if account.CreatedAt.IsZero() {
		account.CreatedAt = time.Now()
	}

	// If account is main account, set all other accounts to not main
	if account.IsMain {
		for i := range accounts {
			accounts[i].IsMain = false
		}
	}

	accounts = append(accounts, account)

	if err = a.saveAccounts(accounts); err != nil {
		return err
	}

	return nil
}

func (a AccountProvider) UpdateTokensFor(id, accessToken, refreshToken string) error {
	accounts, err := a.loadAccounts()
	if err != nil {
		return err
	}

	i := slices.IndexFunc(accounts, func(a Account) bool { return a.ID == id })

	if i == -1 {
		return ErrAccountNotFound
	}

	accounts[i].AccessToken = accessToken
	accounts[i].RefreshToken = refreshToken

	if err = a.saveAccounts(accounts); err != nil {
		return err
	}

	return nil
}

func (a AccountProvider) MarkAccountAsMain(id string) error {
	accounts, err := a.loadAccounts()
	if err != nil {
		return err
	}

	accountIndex := slices.IndexFunc(accounts, func(a Account) bool { return a.ID == id })

	if accountIndex == -1 {
		return ErrAccountNotFound
	}

	for i := range accounts {
		accounts[i].IsMain = false
	}

	accounts[accountIndex].IsMain = true

	if err = a.saveAccounts(accounts); err != nil {
		return err
	}

	return nil
}

func (a AccountProvider) loadAccounts() ([]Account, error) {
	var fileData accountFile

	data, err := a.keychain.Get(keychainService, keychainAccount)

	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			fileData.Accounts = append(fileData.Accounts, anonymousAccount)
			return fileData.Accounts, nil
		}

		return nil, err
	}

	err = json.Unmarshal([]byte(data), &fileData)

	if err != nil {
		syntaxErr := &json.SyntaxError{}
		if errors.As(err, &syntaxErr) {
			fileData.Accounts = append(fileData.Accounts, anonymousAccount)
			return fileData.Accounts, nil
		}

		return nil, err
	}

	fileData.Accounts = append(fileData.Accounts, anonymousAccount)
	return fileData.Accounts, nil
}

func (a AccountProvider) saveAccounts(accounts []Account) error {
	accountsCopy := make([]Account, len(accounts))
	copy(accountsCopy, accounts)

	accountsCopy = slices.DeleteFunc(accountsCopy, func(a Account) bool {
		return a.IsAnonymous
	})

	data := accountFile{
		Accounts: accountsCopy,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	if err := a.keychain.Set(keychainService, keychainAccount, string(bytes)); err != nil {
		return err
	}

	return nil
}
