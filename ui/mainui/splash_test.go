package mainui

import (
	"errors"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func newTestSplash() splash {
	return splash{
		width:  100,
		height: 40,
		keymap: save.BuildDefaultKeyMap(),
		userConfiguration: UserConfiguration{
			Theme: save.BuildDefaultTheme(),
		},
	}
}

func TestSplash_MentionsCreateAndQuickJoin(t *testing.T) {
	t.Parallel()

	view := newTestSplash().View()

	require.Contains(t, view, "ctrl+t", "create tab hint")
	require.Contains(t, view, "ctrl+j", "quick join hint")
	require.Contains(t, view, "create a new tab")
	require.Contains(t, view, "quick join a channel")
}

func TestSplash_ErrorStillShowsJoinHints(t *testing.T) {
	t.Parallel()

	err := errors.New("failed to load state")
	view := newTestSplash().ViewError(err)

	require.Contains(t, view, err.Error())
	require.Contains(t, view, "ctrl+t")
	require.Contains(t, view, "ctrl+j")
}

func TestSplash_OmitsUnboundHints(t *testing.T) {
	t.Parallel()

	s := newTestSplash()
	s.keymap.Create.SetKeys()
	s.keymap.QuickJoin.SetKeys()

	view := s.View()

	require.NotContains(t, view, "create a new tab")
	require.NotContains(t, view, "quick join a channel")
}
