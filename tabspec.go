package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/tab"
)

// tabSpec is a parsed but unresolved tab specification from a CLI --tab flag.
type tabSpec struct {
	account string // login or display name (lowercase), empty = default
	channel string // channel login (lowercase)
	kind    tab.TabKind
}

// parseTabSpec parses a single --tab flag value.
//
// Formats:
//   - "channel"        -> broadcast tab, default account
//   - "user@channel"   -> broadcast tab, specific account
//   - "notification"   -> live notification tab
//   - "mention"        -> mention tab
func parseTabSpec(raw string) (tabSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tabSpec{}, fmt.Errorf("empty tab spec")
	}

	lower := strings.ToLower(raw)

	switch lower {
	case "notification", "notifications":
		return tabSpec{kind: tab.LiveNotificationKind}, nil
	case "mention", "mentions":
		return tabSpec{kind: tab.MentionKind}, nil
	}

	spec := tabSpec{kind: tab.BroadcastKind}

	if idx := strings.Index(lower, "@"); idx != -1 {
		spec.account = lower[:idx]
		spec.channel = lower[idx+1:]
	} else {
		spec.channel = lower
	}

	spec.channel = strings.TrimPrefix(spec.channel, "#")

	if spec.channel == "" {
		return tabSpec{}, fmt.Errorf("empty channel in tab spec %q", raw)
	}

	if spec.account == "" && strings.Contains(raw, "@") {
		return tabSpec{}, fmt.Errorf("empty account name before @ in tab spec %q", raw)
	}

	return spec, nil
}

// buildInitialState parses --tab flag values and resolves account names
// against the provided accounts, returning an AppState suitable for
// initializing a detached session.
func buildInitialState(rawSpecs []string, accounts []save.Account) (save.AppState, error) {
	specs := make([]tabSpec, 0, len(rawSpecs))
	for _, raw := range rawSpecs {
		spec, err := parseTabSpec(raw)
		if err != nil {
			return save.AppState{}, err
		}
		specs = append(specs, spec)
	}

	defaultAccount := findDefaultAccount(accounts)

	state := save.AppState{}
	var hasNotification, hasMention bool

	for _, spec := range specs {
		var ts save.TabState

		switch spec.kind {
		case tab.BroadcastKind:
			account, err := resolveAccount(spec.account, accounts, defaultAccount)
			if err != nil {
				return save.AppState{}, err
			}
			ts = save.TabState{
				Kind:       int(tab.BroadcastKind),
				Channel:    spec.channel,
				IdentityID: account.ID,
			}
		case tab.MentionKind:
			if hasMention {
				continue
			}

			hasNonAnon := slices.ContainsFunc(accounts, func(a save.Account) bool {
				return !a.IsAnonymous
			})
			if !hasNonAnon {
				return save.AppState{}, fmt.Errorf("mention tab requires at least one non-anonymous account")
			}

			hasMention = true
			ts = save.TabState{
				Kind: int(tab.MentionKind),
			}
		case tab.LiveNotificationKind:
			if hasNotification {
				continue
			}

			hasNotification = true
			ts = save.TabState{
				Kind: int(tab.LiveNotificationKind),
			}
		}

		state.Tabs = append(state.Tabs, ts)
	}

	if len(state.Tabs) > 0 {
		state.Tabs[0].IsFocused = true
	}

	return state, nil
}

func findDefaultAccount(accounts []save.Account) save.Account {
	if i := slices.IndexFunc(accounts, func(a save.Account) bool { return a.IsMain }); i != -1 {
		return accounts[i]
	}
	if i := slices.IndexFunc(accounts, func(a save.Account) bool { return a.IsAnonymous }); i != -1 {
		return accounts[i]
	}
	return save.Account{}
}

// resolveAccount matches by login name first, then display name (case-insensitive).
// The keyword "anonymous" resolves to the anonymous account.
func resolveAccount(name string, accounts []save.Account, defaultAccount save.Account) (save.Account, error) {
	if name == "" {
		return defaultAccount, nil
	}

	lower := strings.ToLower(name)

	// "anonymous" keyword
	if lower == "anonymous" {
		if i := slices.IndexFunc(accounts, func(a save.Account) bool { return a.IsAnonymous }); i != -1 {
			return accounts[i], nil
		}
	}

	// Match by login name (preferred, always lowercase on Twitch), then display name
	if i := slices.IndexFunc(accounts, func(a save.Account) bool {
		return a.LoginName == lower || strings.EqualFold(lower, a.DisplayName)
	}); i != -1 {
		return accounts[i], nil
	}

	available := []string{"anonymous"}
	for _, a := range accounts {
		if !a.IsAnonymous {
			available = append(available, a.DisplayName)
		}
	}

	return save.Account{}, fmt.Errorf("account %q not found, available: %s", name, strings.Join(available, ", "))
}
