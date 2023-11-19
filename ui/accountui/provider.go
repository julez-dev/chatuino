package accountui

import "github.com/julez-dev/chatuino/save"

// staticAccountProvider is a implementation of the accountProvider interface
// that always returns the same account.
type staticAccountProvider struct {
	account *save.Account
}

func newStaticAccountProvider(account *save.Account) *staticAccountProvider {
	return &staticAccountProvider{account: account}
}

func (s *staticAccountProvider) GetAccountBy(id string) (save.Account, error) {
	if id != s.account.ID {
		return save.Account{}, save.ErrAccountNotFound
	}

	return *s.account, nil
}

func (s *staticAccountProvider) UpdateTokensFor(id, accessToken, refreshToken string) error {
	if id != s.account.ID {
		return save.ErrAccountNotFound
	}

	s.account.AccessToken = accessToken
	s.account.RefreshToken = refreshToken

	return nil
}
