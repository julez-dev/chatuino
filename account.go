package main

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/ui/accountui"
	"github.com/rs/zerolog/log"
	"github.com/spf13/afero"
	"github.com/urfave/cli/v3"
	"github.com/zalando/go-keyring"
)

// migrateAccountLoginNames backfills LoginName for accounts created before
// it was stored. Fetches user data by ID from the Twitch API and persists
// the login name.
func migrateAccountLoginNames(ctx context.Context, accounts []save.Account, provider save.AccountProvider, api *server.Client) error {
	var needMigration []save.Account
	for _, acc := range accounts {
		if acc.IsAnonymous || acc.LoginName != "" {
			continue
		}
		needMigration = append(needMigration, acc)
	}

	if len(needMigration) == 0 {
		return nil
	}

	ids := make([]string, len(needMigration))
	for i, acc := range needMigration {
		ids[i] = acc.ID
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := api.GetUsers(ctx, nil, ids)
	if err != nil {
		return fmt.Errorf("fetch users for login name migration: %w", err)
	}

	lookup := make(map[string]string, len(resp.Data))
	for _, u := range resp.Data {
		lookup[u.ID] = u.Login
	}

	for _, acc := range needMigration {
		login, ok := lookup[acc.ID]
		if !ok {
			log.Logger.Warn().Str("account_id", acc.ID).Msg("could not find login name for account during migration")
			continue
		}

		if err := provider.UpdateLoginNameFor(acc.ID, login); err != nil {
			return fmt.Errorf("persist login name for %s: %w", acc.DisplayName, err)
		}

		log.Logger.Info().Str("account_id", acc.ID).Str("login", login).Msg("migrated account login name")
	}

	return nil
}

var accountCMD = &cli.Command{
	Name:        "account",
	Description: "Chatuino account management",
	Usage:       "Manage accounts used by Chatuino",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "api-host",
			Usage: "Host of the Chatuino API",
			Value: "https://chatuino.net",
		},
		&cli.StringFlag{
			Name:  "client-id",
			Usage: "OAuth Client-ID",
			Value: defaultClientID,
		},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		keys, err := save.CreateReadKeyMap()
		if err != nil {
			return fmt.Errorf("failed to read keymap file: %w", err)
		}

		theme, err := save.ThemeFromDisk()
		if err != nil {
			return fmt.Errorf("failed to read theme file: %w", err)
		}

		var keyringBackend keyring.Keyring

		if command.Bool("plain-auth-storage") {
			keyringBackend = save.NewPlainKeyringFallback(afero.NewOsFs())
		} else {
			keyringBackend = save.NewKeyringWrapper()
		}

		p := tea.NewProgram(
			accountui.NewList(command.String("client-id"), command.String("api-host"), save.NewAccountProvider(keyringBackend), keys, theme),
			tea.WithContext(ctx),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("error while running TUI: %w", err)
		}

		return nil
	},
}
