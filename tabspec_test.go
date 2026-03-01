package main

import (
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/mainui"
	"github.com/stretchr/testify/require"
)

func TestParseTabSpec(t *testing.T) {
	t.Parallel()

	t.Run("simple channel", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("streamer1")
		require.NoError(t, err)
		require.Equal(t, mainui.BroadcastTabKind, spec.kind)
		require.Equal(t, "streamer1", spec.channel)
		require.Empty(t, spec.account)
	})

	t.Run("user@channel", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("myuser@streamer1")
		require.NoError(t, err)
		require.Equal(t, mainui.BroadcastTabKind, spec.kind)
		require.Equal(t, "streamer1", spec.channel)
		require.Equal(t, "myuser", spec.account)
	})

	t.Run("notification keyword", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("notification")
		require.NoError(t, err)
		require.Equal(t, mainui.LiveNotificationTabKind, spec.kind)
		require.Empty(t, spec.channel)
		require.Empty(t, spec.account)
	})

	t.Run("notifications alias", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("notifications")
		require.NoError(t, err)
		require.Equal(t, mainui.LiveNotificationTabKind, spec.kind)
	})

	t.Run("mention keyword", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("mention")
		require.NoError(t, err)
		require.Equal(t, mainui.MentionTabKind, spec.kind)
	})

	t.Run("mentions alias", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("mentions")
		require.NoError(t, err)
		require.Equal(t, mainui.MentionTabKind, spec.kind)
	})

	t.Run("case insensitive keywords", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("NOTIFICATION")
		require.NoError(t, err)
		require.Equal(t, mainui.LiveNotificationTabKind, spec.kind)

		spec, err = parseTabSpec("Mention")
		require.NoError(t, err)
		require.Equal(t, mainui.MentionTabKind, spec.kind)
	})

	t.Run("strips whitespace", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("  streamer1  ")
		require.NoError(t, err)
		require.Equal(t, "streamer1", spec.channel)
	})

	t.Run("strips hash prefix", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("#streamer1")
		require.NoError(t, err)
		require.Equal(t, "streamer1", spec.channel)
	})

	t.Run("lowercases channel and account", func(t *testing.T) {
		t.Parallel()
		spec, err := parseTabSpec("MyUser@Streamer1")
		require.NoError(t, err)
		require.Equal(t, "myuser", spec.account)
		require.Equal(t, "streamer1", spec.channel)
	})

	t.Run("empty string errors", func(t *testing.T) {
		t.Parallel()
		_, err := parseTabSpec("")
		require.Error(t, err)
	})

	t.Run("empty channel after @ errors", func(t *testing.T) {
		t.Parallel()
		_, err := parseTabSpec("user@")
		require.Error(t, err)
	})

	t.Run("empty account before @ errors", func(t *testing.T) {
		t.Parallel()
		_, err := parseTabSpec("@channel")
		require.Error(t, err)
	})
}

func TestBuildInitialState(t *testing.T) {
	t.Parallel()

	mainAccount := save.Account{
		ID:          "main-id",
		IsMain:      true,
		LoginName:   "mainuser",
		DisplayName: "MainUser",
	}
	secondAccount := save.Account{
		ID:          "second-id",
		LoginName:   "seconduser",
		DisplayName: "SecondUser",
	}
	anonAccount := save.Account{
		ID:          "anonymous-account",
		IsAnonymous: true,
		LoginName:   "justinfan123123",
		DisplayName: "justinfan123123",
	}

	allAccounts := []save.Account{mainAccount, secondAccount, anonAccount}
	anonOnly := []save.Account{anonAccount}

	t.Run("single channel uses main account", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"streamer1"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
		require.Equal(t, int(mainui.BroadcastTabKind), state.Tabs[0].Kind)
		require.Equal(t, "streamer1", state.Tabs[0].Channel)
		require.Equal(t, "main-id", state.Tabs[0].IdentityID)
		require.True(t, state.Tabs[0].IsFocused)
	})

	t.Run("single channel no main uses anonymous", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"streamer1"}, anonOnly)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
		require.Equal(t, "anonymous-account", state.Tabs[0].IdentityID)
	})

	t.Run("anonymous@channel resolves to anonymous account", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"anonymous@streamer1"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
		require.Equal(t, "anonymous-account", state.Tabs[0].IdentityID)
		require.Equal(t, "streamer1", state.Tabs[0].Channel)
	})

	t.Run("user@channel resolves by login name", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"seconduser@streamer1"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
		require.Equal(t, "second-id", state.Tabs[0].IdentityID)
		require.Equal(t, "streamer1", state.Tabs[0].Channel)
	})

	t.Run("user@channel resolves by display name fallback", func(t *testing.T) {
		t.Parallel()
		// Account without LoginName set (pre-migration)
		legacyAccount := save.Account{
			ID:          "legacy-id",
			DisplayName: "LegacyUser",
		}
		state, err := buildInitialState([]string{"legacyuser@streamer1"}, []save.Account{legacyAccount, anonAccount})
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
		require.Equal(t, "legacy-id", state.Tabs[0].IdentityID)
	})

	t.Run("user@channel unknown account errors", func(t *testing.T) {
		t.Parallel()
		_, err := buildInitialState([]string{"unknown@streamer1"}, allAccounts)
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown")
		require.Contains(t, err.Error(), "MainUser")
	})

	t.Run("notification tab", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"notification", "streamer1"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 2)
		require.Equal(t, int(mainui.LiveNotificationTabKind), state.Tabs[0].Kind)
		require.Equal(t, int(mainui.BroadcastTabKind), state.Tabs[1].Kind)
		require.True(t, state.Tabs[0].IsFocused)
		require.False(t, state.Tabs[1].IsFocused)
	})

	t.Run("duplicate notification deduplicated", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"notification", "notification"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
	})

	t.Run("duplicate mention deduplicated", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"mention", "mention"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 1)
	})

	t.Run("mention without non-anonymous account errors", func(t *testing.T) {
		t.Parallel()
		_, err := buildInitialState([]string{"mention"}, anonOnly)
		require.Error(t, err)
		require.Contains(t, err.Error(), "non-anonymous")
	})

	t.Run("multiple tabs first gets focus", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"streamer1", "streamer2"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 2)
		require.True(t, state.Tabs[0].IsFocused)
		require.False(t, state.Tabs[1].IsFocused)
	})

	t.Run("focus correct after dedup removes first", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{"notification", "notification", "streamer1"}, allAccounts)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 2)
		require.True(t, state.Tabs[0].IsFocused)
	})

	t.Run("full combo", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState(
			[]string{"notification", "mention", "seconduser@streamer1", "streamer2"},
			allAccounts,
		)
		require.NoError(t, err)
		require.Len(t, state.Tabs, 4)
		require.Equal(t, int(mainui.LiveNotificationTabKind), state.Tabs[0].Kind)
		require.Equal(t, int(mainui.MentionTabKind), state.Tabs[1].Kind)
		require.Equal(t, int(mainui.BroadcastTabKind), state.Tabs[2].Kind)
		require.Equal(t, "second-id", state.Tabs[2].IdentityID)
		require.Equal(t, int(mainui.BroadcastTabKind), state.Tabs[3].Kind)
		require.Equal(t, "main-id", state.Tabs[3].IdentityID)
	})

	t.Run("empty specs returns empty state", func(t *testing.T) {
		t.Parallel()
		state, err := buildInitialState([]string{}, allAccounts)
		require.NoError(t, err)
		require.Empty(t, state.Tabs)
	})
}
