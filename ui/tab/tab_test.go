package tab_test

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

func TestCaption_DescribesNewTabsWithoutStartingTheirResources(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		kind                     tab.TabKind
		account                  save.Account
		channel, title, identity string
	}{
		{tab.BroadcastKind, save.Account{DisplayName: "main"}, "alpha", "alpha", "main"},
		{tab.BroadcastKind, save.Account{IsAnonymous: true}, "alpha", "alpha", "Anonymous"},
		{tab.MentionKind, save.Account{}, "", "mentioned", "all"},
		{tab.LiveNotificationKind, save.Account{}, "", "live notifications", "all"},
	} {
		title, identity := tab.Caption(test.kind, test.account, test.channel)
		require.Equal(t, test.title, title)
		require.Equal(t, test.identity, identity)
	}
}
