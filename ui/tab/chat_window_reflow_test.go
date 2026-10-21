package tab

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func newRenderedChatWindow(t *testing.T, width, height int) *chatWindow {
	t.Helper()

	return newChatWindow(width, height, save.BuildDefaultSettings(), save.BuildDefaultTheme(), save.BuildDefaultKeyMap())
}

func addPrivateMessage(t *testing.T, c *chatWindow, id, content string) *chatEntry {
	t.Helper()

	c.handleMessage(ChatEventMessage{Message: &twitchirc.PrivateMessage{
		ID:          id,
		DisplayName: "Alice",
		LoginName:   "alice",
		Message:     content,
		TMISentTS:   time.Date(2026, 3, 4, 5, 6, 7, 0, time.Local),
	}})

	entries := c.activeEntries()
	require.NotEmpty(t, entries)

	return entries[len(entries)-1]
}

// TestChatWindow_SelectionSurvivesReflow ensures the rendered-line index is
// rebuilt without losing the selected message when the width changes.
func TestChatWindow_SelectionSurvivesReflow(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 6)

	for i := 0; i < 5; i++ {
		addPrivateMessage(t, c, fmt.Sprintf("msg-%d", i), strings.Repeat("word ", 10))
	}

	entries := c.activeEntries()
	target := entries[1]
	beforeLines := len(c.lines)

	c.selectLine(target.Position.CursorStart)
	require.True(t, target.Selected)
	require.Equal(t, target.Position.CursorStart, c.cursor)

	c.Resize(30, 6)

	require.Same(t, target, c.selectedEntry(), "the same message stays selected after reflow")
	require.Greater(t, len(c.lines), beforeLines, "narrower width must produce more rendered rows")
	require.GreaterOrEqual(t, c.cursor, target.Position.CursorStart)
	require.LessOrEqual(t, c.cursor, target.Position.CursorEnd)

	for i, e := range c.activeEntries() {
		wantStart := 0
		if i > 0 {
			wantStart = c.activeEntries()[i-1].Position.CursorEnd + 1
		}

		require.Equal(t, wantStart, e.Position.CursorStart, "positions stay contiguous after reflow")
	}
}

// selectedEntry returns the currently selected entry, failing the test when there
// is none.
func (c *chatWindow) selectedEntry() *chatEntry {
	_, entry := c.entryForCurrentCursor()
	if entry != nil && entry.Selected {
		return entry
	}

	for _, e := range c.entries {
		if e.Selected {
			return e
		}
	}

	return nil
}

// TestChatWindow_DeletionRebuildsLinePositions checks that marking a message
// deleted restyles it and recalculates every position.
func TestChatWindow_DeletionRebuildsLinePositions(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 10)

	first := addPrivateMessage(t, c, "msg-1", "first message")
	second := addPrivateMessage(t, c, "msg-2", "second message")

	c.handleMessageDeletion(ChatEventMessage{Message: &twitchirc.ClearMessage{
		Login:       "bob",
		TargetMsgID: second.Event.Message.(*twitchirc.PrivateMessage).ID,
	}})

	require.True(t, second.IsDeleted)
	require.True(t, second.Event.DisplayModifier.Strikethrough)
	require.False(t, first.IsDeleted)

	// Re-rendered lines carry the strike-through sequence.
	require.Contains(t, strings.Join(c.lines, "\n"), "\x1b[9m")

	for i, e := range c.activeEntries() {
		wantStart := 0
		if i > 0 {
			wantStart = c.activeEntries()[i-1].Position.CursorEnd + 1
		}

		require.Equal(t, wantStart, e.Position.CursorStart)
	}
}

// TestChatWindow_SearchFiltersRebuildLinePositions covers the search seam: only
// matching messages keep rendered rows, and their positions are recalculated.
func TestChatWindow_SearchFiltersRebuildLinePositions(t *testing.T) {
	t.Parallel()

	c := newRenderedChatWindow(t, 80, 10)

	addPrivateMessage(t, c, "msg-1", "alpha")
	addPrivateMessage(t, c, "msg-2", "beta target")
	addPrivateMessage(t, c, "msg-3", "gamma")

	c.state = searchChatWindowState
	c.searchInput.SetValue("beta")
	c.applySearch()

	active := c.activeEntries()
	require.Len(t, active, 1)
	require.Equal(t, 0, active[0].Position.CursorStart)
	require.Equal(t, 1, c.matchCount)

	// The visible lines only contain the matching message.
	require.Len(t, c.lines, 1)
	require.Contains(t, plain(c.lines), "beta target")

	c.handleStopSearchMode()

	active = c.activeEntries()
	require.Len(t, active, 3)

	for i, e := range active {
		wantStart := 0
		if i > 0 {
			wantStart = active[i-1].Position.CursorEnd + 1
		}

		require.Equal(t, wantStart, e.Position.CursorStart)
	}
}
