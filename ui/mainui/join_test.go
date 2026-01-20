package mainui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func TestJoin_EnterKeyWithValidChannel(t *testing.T) {
	t.Parallel()

	deps := createTestDependencies()
	j := newJoin(100, 50, deps)
	j.focused = true
	j.hasLoaded = true

	// Set up accounts
	j.accounts = []save.Account{
		{ID: "test-id", DisplayName: "testuser"},
	}
	j.accountList.SetItems([]list.Item{
		listItem{title: "testuser"},
	})

	// Set channel tab as selected
	j.tabKindList.SetItems([]list.Item{
		listItem{title: "Channel (Default)", kind: broadcastTabKind},
	})
	j.tabKindList.Select(0)

	// Set channel input value
	j.input.SetValue("xqc")
	j.selectedInput = channelInput

	// Simulate Enter key press
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedJoin, cmd := j.Update(enterMsg)

	require.NotNil(t, cmd)
	require.NotNil(t, updatedJoin)

	// Execute the command to get the message
	msg := cmd()
	joinMsg, ok := msg.(joinChannelMessage)
	require.True(t, ok, "Expected joinChannelMessage")
	require.Equal(t, broadcastTabKind, joinMsg.tabKind)
	require.Equal(t, "xqc", joinMsg.channel)
	require.Equal(t, "testuser", joinMsg.account.DisplayName)
}

func TestJoin_EnterKeyWithEmptyChannelDoesNothing(t *testing.T) {
	t.Parallel()

	deps := createTestDependencies()
	j := newJoin(100, 50, deps)
	j.focused = true
	j.hasLoaded = true

	// Set up accounts
	j.accounts = []save.Account{
		{ID: "test-id", DisplayName: "testuser"},
	}

	// Set channel tab as selected
	j.tabKindList.SetItems([]list.Item{
		listItem{title: "Channel (Default)", kind: broadcastTabKind},
	})
	j.tabKindList.Select(0)

	// Empty channel input
	j.input.SetValue("")
	j.selectedInput = channelInput

	// Simulate Enter key press
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := j.Update(enterMsg)

	// Should not trigger join
	require.Nil(t, cmd)
}

func TestJoin_EnterKeyOnMentionTab(t *testing.T) {
	t.Parallel()

	deps := createTestDependencies()
	j := newJoin(100, 50, deps)
	j.focused = true
	j.hasLoaded = true

	// Set up accounts
	j.accounts = []save.Account{
		{ID: "test-id", DisplayName: "testuser"},
	}

	// Set mention tab as selected
	j.tabKindList.SetItems([]list.Item{
		listItem{title: "Mention", kind: mentionTabKind},
	})
	j.tabKindList.Select(0)

	j.selectedInput = tabSelect

	// Simulate Enter key press (no channel needed for mention tab)
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := j.Update(enterMsg)

	require.NotNil(t, cmd)

	// Execute the command to get the message
	msg := cmd()
	joinMsg, ok := msg.(joinChannelMessage)
	require.True(t, ok, "Expected joinChannelMessage")
	require.Equal(t, mentionTabKind, joinMsg.tabKind)
}

func TestJoin_TabNavigationCycle(t *testing.T) {
	t.Parallel()

	deps := createTestDependencies()
	j := newJoin(100, 50, deps)
	j.focused = true
	j.hasLoaded = true

	// Set up for regular channel tab (not mention/live notification)
	j.tabKindList.SetItems([]list.Item{
		listItem{title: "Channel (Default)", kind: broadcastTabKind},
	})
	j.tabKindList.Select(0)

	// Start at tabSelect
	j.selectedInput = tabSelect

	// Press Tab (Next)
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	j, _ = j.Update(tabMsg)
	require.Equal(t, accountSelect, j.selectedInput, "Should move to accountSelect")

	// Press Tab again
	j, _ = j.Update(tabMsg)
	require.Equal(t, channelInput, j.selectedInput, "Should move to channelInput")

	// Press Tab again (should cycle back to tabSelect)
	j, _ = j.Update(tabMsg)
	require.Equal(t, tabSelect, j.selectedInput, "Should cycle back to tabSelect")
}

func TestJoin_WindowResize(t *testing.T) {
	t.Parallel()

	deps := createTestDependencies()
	j := newJoin(100, 50, deps)

	// Initial width should be 60% of 100 = 60
	require.Equal(t, 60, j.width)

	// Resize to larger terminal
	j.handleResize(200, 50)

	// New width should be 60% of 200 = 120
	require.Equal(t, 120, j.width)

	// Resize to very small terminal (should use minimum of 40)
	j.handleResize(50, 50)
	require.Equal(t, 40, j.width, "Should use minimum width of 40")
}

// Helper function to create test dependencies
func createTestDependencies() *DependencyContainer {
	keymap := save.BuildDefaultKeyMap()
	theme := save.BuildDefaultTheme()

	return &DependencyContainer{
		Keymap: keymap,
		UserConfig: UserConfiguration{
			Theme: theme,
		},
		APIUserClients: map[string]APIClient{},
	}
}
