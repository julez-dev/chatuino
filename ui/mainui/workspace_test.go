package mainui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/ui/workspace"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// workspaceTab is a small interactive Tab adapter. Its deliberately oversized
// output exercises the shell's actual clipping rather than a layout mock.
type workspaceTab struct {
	*fakeTab
	width, height int
	composing     bool
	searching     bool
	bodyOverride  string
	closed        bool
	clicks        [][2]int
	wheels        [][2]int
}

func (t *workspaceTab) Update(msg tea.Msg) (tab.Tab, tea.Cmd) {
	t.fakeTab.Update(msg)
	return t, nil
}
func (t *workspaceTab) View() string { return t.ViewWithoutStatusBar() }
func (t *workspaceTab) ViewWithoutStatusBar() string {
	if t.bodyOverride != "" {
		return t.bodyOverride
	}
	return strings.Repeat("BODY "+t.id+" 界"+strings.Repeat(".", 140)+"\n", max(1, t.height+2))
}
func (t *workspaceTab) StatusBarView() string     { return "STATUS " + t.id }
func (t *workspaceTab) SetSize(width, height int) { t.width, t.height = width, height }
func (t *workspaceTab) IsComposing() bool         { return t.composing }
func (t *workspaceTab) IsSearching() bool         { return t.searching }
func (t *workspaceTab) Close()                    { t.closed = true }
func (t *workspaceTab) HandleMouseClick(_ tea.MouseClickMsg, x, y int) tea.Cmd {
	t.clicks = append(t.clicks, [2]int{x, y})
	return nil
}
func (t *workspaceTab) HandleMouseWheel(_ tea.MouseWheelMsg, x, y int) tea.Cmd {
	t.wheels = append(t.wheels, [2]int{x, y})
	return nil
}

func workspaceRoot(t *testing.T) (*Root, *workspaceTab, *workspaceTab, *workspaceTab) {
	t.Helper()
	root := newTestRoot(t, nil, nil, nil)
	root.hasLoadedSession = true
	root.width, root.height = 101, 33
	makeTab := func(id string) *workspaceTab {
		model := newFakeTab(id)
		model.channel = id
		return &workspaceTab{fakeTab: model}
	}
	a, b, c := makeTab("alpha"), makeTab("beta"), makeTab("gamma")
	root.tabs.Add(a)
	root.tabs.Add(b)
	root.tabs.Add(c)
	root.handleResize()
	return root, a, b, c
}

func workspaceKey(t *testing.T, root *Root, msg tea.KeyPressMsg) {
	t.Helper()
	_, cmd := root.Update(msg)
	for _, result := range execCmd(t, cmd) {
		root.Update(result)
	}
}

func TestWorkspace_PaneModeAndAtomicSplitDoNotSendInputToChats(t *testing.T) {
	t.Parallel()
	root, a, b, _ := workspaceRoot(t)
	a.composing = true
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	require.Contains(t, root.View().Content, "-- Pane --")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	require.NotZero(t, root.stack.Len(), "split must choose a tab before changing the layout")
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotContains(t, root.View().Content, "BODY beta")
	require.Contains(t, root.View().Content, "-- Pane --", "cancellation restores pane-control state")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Zero(t, root.stack.Len())
	require.Contains(t, root.View().Content, "BODY alpha")
	require.Contains(t, root.View().Content, "BODY beta")
	active, _ := root.tabs.Active()
	require.Equal(t, b.ID(), active.ID())
	require.Empty(t, a.messages(), "pane-control input must not touch the previous composer's draft")
	require.Empty(t, b.messages(), "split confirmation must not reach the new chat")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.False(t, b.closed, "close-pane is not close-tab")
	require.Equal(t, 3, root.tabs.Len())
	require.NotContains(t, root.View().Content, "BODY beta")
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	a.composing = false
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'i', Text: "i"})
	require.Len(t, a.messages(), 1)
	require.Empty(t, b.messages(), "keyboard input must only reach the focused tab")
}

func TestWorkspace_NextNavigationIsNonDestructiveAndClosureKeepsSiblingFocus(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)
	// These adapters own no real IRC resources.
	a.kind, b.kind, c.kind = tab.MentionKind, tab.MentionKind, tab.MentionKind
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	before := root.workspace.Arrange(101, 33)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, c.ID(), root.activeID())
	require.Contains(t, root.View().Content, "BODY gamma")
	require.NotContains(t, root.View().Content, "BODY beta")
	require.Equal(t, before, root.workspace.Arrange(101, 33), "Tab navigation must not rewrite pane assignments")
	root.tabs.Notify(b.ID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, a.ID(), root.activeID(), "assigned tab returns to the preserved tiled workspace")
	require.Contains(t, root.View().Content, "BODY alpha")
	require.Contains(t, root.View().Content, "BODY beta")
	require.True(t, root.tabs.Rows()[1].HasNotification, "a visible but unfocused tab stays notified")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	require.True(t, a.closed)
	require.Equal(t, b.ID(), root.activeID(), "pane sibling wins over tab-order fallback")
	root.Update(CloseTabMessage{TabID: b.ID()})
	require.True(t, b.closed)
	require.Equal(t, c.ID(), root.activeID(), "the remaining standalone tab becomes active")
	require.Contains(t, root.View().Content, "BODY gamma")
	root.Update(CloseTabMessage{TabID: c.ID()})
	require.Zero(t, root.tabs.Len())
	require.NotPanics(t, func() { root.View() })
}

func TestWorkspace_TabNavigationCyclesTabsWithoutChangingLayout(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	layout := root.workspace.Arrange(101, 33)
	for i, want := range []string{c.ID(), a.ID(), b.ID(), c.ID(), a.ID(), b.ID()} {
		workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
		require.Equal(t, want, root.activeID(), "Tab %d follows the displayed tab order", i+1)
		require.Equal(t, layout, root.workspace.Arrange(101, 33))
		require.True(t, root.workspace.Contains(a.ID()))
		require.True(t, root.workspace.Contains(b.ID()))
	}
	// Clicking the standalone tab already on screen must not assign it.
	root.navigateToTab(c.ID())
	root.switchToTab(c.ID())
	require.Equal(t, c.ID(), root.activeID())
	require.Equal(t, layout, root.workspace.Arrange(101, 33))
}

func TestWorkspace_TabDuringSearchStaysInSearch(t *testing.T) {
	t.Parallel()
	root, a, _, _ := workspaceRoot(t)
	a.searching = true
	root.tabs.SetActive(a.ID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, a.ID(), root.activeID(), "Tab while typing a search query must not switch tabs")
	require.Len(t, a.messages(), 1)
	require.IsType(t, tea.KeyPressMsg{}, a.messages()[0])
}

func TestWorkspace_SessionRestoresAssignmentsRatiosAndLogicalFocusWithoutZoom(t *testing.T) {
	t.Parallel()
	root, _, _, _ := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: '>', Text: ">"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'z', Text: "z"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF1})
	snapshot := root.TakeStateSnapshot()
	require.NotEmpty(t, snapshot.Layout)
	require.True(t, snapshot.Tabs[1].IsFocused, "temporary dialog blur is not logical loss of focus")
	var saved workspace.State
	require.NoError(t, json.Unmarshal(snapshot.Layout, &saved))
	require.Equal(t, 4500, saved.Root.Ratio)
	restored := newRestoreTestRoot(t)
	restored.Update(tea.WindowSizeMsg{Width: 101, Height: 33})
	restored.Update(persistedDataLoadedMessage{state: snapshot})
	t.Cleanup(func() {
		for _, model := range restored.tabs.All() {
			model.Close()
		}
	})
	require.Equal(t, "beta", restored.activeTab().Title())
	require.JSONEq(t, string(snapshot.Layout), string(restored.TakeStateSnapshot().Layout), "runtime UUIDs must remap to the saved tab records")
	require.NotContains(t, restored.View().Content, "-- Pane --")
	require.NotContains(t, restored.View().Content, "ZOOM")
	require.Contains(t, restored.View().Content, "> W1.2 beta")
}

func TestWorkspace_SessionRestoresAStandaloneTabWithoutPromotingItIntoTheLayout(t *testing.T) {
	t.Parallel()
	root, _, _, c := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, c.ID(), root.activeID())
	snapshot := root.TakeStateSnapshot()
	var saved workspace.State
	require.NoError(t, json.Unmarshal(snapshot.Layout, &saved))
	require.Equal(t, "0", saved.Focus, "layout focus remains an assigned pane, while TabState records the standalone tab")
	require.True(t, snapshot.Tabs[2].IsFocused)
	restored := newRestoreTestRoot(t)
	restored.Update(tea.WindowSizeMsg{Width: 101, Height: 33})
	restored.Update(persistedDataLoadedMessage{state: snapshot})
	t.Cleanup(func() {
		for _, model := range restored.tabs.All() {
			model.Close()
		}
	})
	require.Equal(t, "gamma", restored.activeTab().Title())
	require.False(t, restored.workspace.Contains(restored.activeID()), "restoration must not silently turn the standalone tab into a pane")
	require.Equal(t, 2, restored.workspace.Count())
	require.False(t, restored.usesWorkspace())
	workspaceKey(t, restored, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, "alpha", restored.activeTab().Title())
	require.True(t, restored.usesWorkspace(), "returning to an assigned tab restores the tiled workspace")
}

func TestWorkspace_LayoutRecoveryKeepsValidTabsAndDetachedSessionsIgnoreSavedLayout(t *testing.T) {
	for _, data := range []string{
		`"invalid metadata"`,
		`{"version":99,"root":{"tab":"0"}}`,
		`{"version":1,"root":{"axis":"columns","ratio":5000,"first":{"tab":"0"},"second":{"tab":"removed"}},"focus":"removed"}`,
	} {
		t.Run(data, func(t *testing.T) {
			t.Parallel()
			r := newRestoreTestRoot(t)
			r.Update(persistedDataLoadedMessage{state: save.AppState{
				Tabs:   []save.TabState{{Channel: "alpha", IdentityID: "acc"}, {Channel: "beta", IdentityID: "acc"}},
				Layout: json.RawMessage(data),
			}})
			t.Cleanup(func() {
				for _, model := range r.tabs.All() {
					model.Close()
				}
			})
			require.Equal(t, 2, r.tabs.Len())
			var state workspace.State
			require.NoError(t, json.Unmarshal(r.TakeStateSnapshot().Layout, &state))
			layout, _, repaired := workspace.Restore(state, nil)
			require.False(t, repaired)
			require.Equal(t, 1, layout.Count())
		})
	}
	t.Run("detached", func(t *testing.T) {
		t.Parallel()
		initial := save.AppState{Tabs: []save.TabState{
			{Kind: int(tab.LiveNotificationKind)},
			{Kind: int(tab.BroadcastKind), Channel: "alpha", IdentityID: "acc"},
		}, Layout: json.RawMessage(`{"version":1,"root":{"axis":"columns","ratio":5000,"first":{"tab":"0"},"second":{"tab":"1"}},"focus":"1"}`)}
		r := newTestRoot(t, []save.Account{{ID: "acc"}}, nil, &initial)
		r.Update(persistedDataLoadedMessage{state: initial})
		t.Cleanup(func() {
			for _, model := range r.tabs.All() {
				model.Close()
			}
		})
		require.True(t, r.IsDetached())
		var state workspace.State
		require.NoError(t, json.Unmarshal(r.TakeStateSnapshot().Layout, &state))
		layout, _, _ := workspace.Restore(state, nil)
		require.Equal(t, 1, layout.Count())
		_, cmd := r.Update(appStateSaveMessage{})
		require.Nil(t, cmd, "detached sessions must not schedule state writes")
	})
}

func TestWorkspace_CancelledAndStalePickerResultsCannotCommitToAnotherRequest(t *testing.T) {
	t.Parallel()
	root, _, b, _ := workspaceRoot(t)
	b.kind = tab.MentionKind
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	_, choose := root.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	oldResult := choose()
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 's', Text: "s"})
	root.Update(oldResult)
	require.NotZero(t, root.stack.Len(), "old result must not dismiss the newer picker")
	require.NotContains(t, root.View().Content, "BODY beta")
	_, choose = root.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	closedTabResult := choose()
	root.Update(CloseTabMessage{TabID: b.ID()})
	root.Update(closedTabResult)
	require.Zero(t, root.stack.Len())
	require.Contains(t, root.View().Content, "selected tab has closed")
	require.NotContains(t, root.View().Content, "BODY beta")
	require.Equal(t, "alpha", root.activeID())
}

func TestWorkspace_NewTabSplitUsesExistingJoinFlowAndTicketedCommit(t *testing.T) {
	t.Parallel()
	root, _, _, _ := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	_, selected := root.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// Supply the account-provider result through the actual modal message seam;
	// don't execute its external account/suggestion fetches in this UI test.
	root.Update(selected())
	root.Update(setJoinAccountsMessage{accounts: []save.Account{{ID: "main", DisplayName: "me"}}})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Zero(t, root.stack.Len())
	require.Equal(t, 4, root.tabs.Len())
	active, _ := root.tabs.Active()
	require.Equal(t, tab.LiveNotificationKind, active.Kind())
	require.Contains(t, root.View().Content, "BODY alpha")
	require.Contains(t, root.View().Content, "-- Pane --")
}

func TestWorkspace_NewTabSplitRevalidatesSpaceForWiderVerticalTabList(t *testing.T) {
	t.Parallel()
	root, _, _, _ := workspaceRoot(t)
	root.width = 90
	root.settings.VerticalTabList = true
	root.header = newVerticalTabHeader(10, 10, root.theme)
	root.handleResize()
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyDown})
	_, selectNew := root.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	root.Update(selectNew())
	root.Update(setJoinAccountsMessage{accounts: []save.Account{{ID: "main", DisplayName: "accountwithaverylongname"}}})
	root.Update(tea.PasteMsg{Content: "reallylongchannelname"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	t.Cleanup(func() {
		for _, model := range root.tabs.All() {
			model.Close()
		}
	})
	require.Equal(t, 3, root.tabs.Len(), "new tab must not consume a split that only fit the old sidebar width")
	require.Equal(t, "alpha", root.activeID())
	require.Contains(t, root.View().Content, "not enough space")
}

func TestWorkspace_GlobalDialogBlocksPaneInputAndRestoresControlState(t *testing.T) {
	t.Parallel()
	root, a, b, _ := workspaceRoot(t)
	b.kind = tab.MentionKind
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF1})
	require.Contains(t, root.View().Content, "Keybind Help")
	root.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	root.Update(tea.PasteMsg{Content: "never send this"})
	root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 6})
	root.Update(CloseTabMessage{TabID: b.ID()})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, a.ID(), root.activeID())
	require.Contains(t, root.View().Content, "-- Pane --")
	root.Update(tea.PasteMsg{Content: "also not chat"})
	require.Empty(t, a.messages())
	_, cmd := root.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	require.Nil(t, cmd, "printable quit aliases must not escape pane mode")
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	root.Update(tea.PasteMsg{Content: "only alpha"})
	require.Equal(t, []tea.Msg{tea.PasteMsg{Content: "only alpha"}}, a.messages())
	require.Empty(t, b.messages())
}

func workspacePoint(t *testing.T, root *Root, text string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(tab.StripANSI(root.View().Content), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return lipgloss.Width(line[:i]), y
		}
	}
	t.Fatalf("visible text %q not found", text)
	return 0, 0
}

func TestWorkspace_MouseUsesRenderedBoundsAndHoverScrollRetainsFocus(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		t.Run(map[bool]string{false: "horizontal tabs", true: "vertical tabs"}[vertical], func(t *testing.T) {
			t.Parallel()
			root, a, b, _ := workspaceRoot(t)
			root.width, root.height = 141, 41
			if vertical {
				root.settings.VerticalTabList = true
				root.header = newVerticalTabHeader(10, 10, root.theme)
			}
			root.handleResize()
			workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
			workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
			workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
			workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
			require.Equal(t, 141, lipgloss.Width(root.View().Content))
			require.Equal(t, 41, lipgloss.Height(root.View().Content))
			x, y := workspacePoint(t, root, "BODY alpha")
			root.tabs.Notify(a.ID())
			root.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: x, Y: y})
			require.Equal(t, b.ID(), root.activeID())
			require.Equal(t, [][2]int{{0, 0}}, a.wheels)
			require.Empty(t, b.wheels)
			require.True(t, root.tabs.Rows()[0].HasNotification)
			root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
			require.Equal(t, a.ID(), root.activeID())
			require.Equal(t, [][2]int{{0, 0}}, a.clicks)
			require.False(t, root.tabs.Rows()[0].HasNotification)
			bx, by := workspacePoint(t, root, "BODY beta")
			root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: bx, Y: by - 1})
			require.Equal(t, b.ID(), root.activeID(), "pane header focuses without selecting a chat row")
			require.Empty(t, b.clicks)
			root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + a.width, Y: y})
			root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: bx, Y: root.height - 1})
			require.Empty(t, b.clicks, "separators and status are not chat content")
			root.Update(tea.WindowSizeMsg{Width: 141, Height: 3})
			root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 5, Y: 2})
			require.Equal(t, b.ID(), root.activeID(), "a clipped header must not claim clicks on the shared status row")
			root.settings.Mouse = false
			root.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
			require.Equal(t, b.ID(), root.activeID())
		})
	}
}

func TestWorkspace_VerticalTabPaginationDoesNotMovePaneOriginsOnFocus(t *testing.T) {
	t.Parallel()
	root, _, _, _ := workspaceRoot(t)
	root.width, root.height = 75, 20
	root.settings.VerticalTabList = true
	root.header = newVerticalTabHeader(10, 10, root.theme)
	for _, id := range []string{"c3", "c4", "c5", "c6", "c7", "c8", "c9", "c10", "c11", "c12", "c13", "c14", "c15", "c16", "c17", "c18", "c19"} {
		root.tabs.Add(&workspaceTab{fakeTab: newFakeTab(id)})
	}
	root.handleResize()
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	before, _ := workspacePoint(t, root, "BODY alpha")
	for range 18 {
		workspaceKey(t, root, tea.KeyPressMsg{Code: ']', Text: "]"})
	}
	require.Equal(t, "c19", root.activeID())
	require.Contains(t, root.View().Content, "BODY c19", "unassigned navigation renders the standalone tab")
	workspaceKey(t, root, tea.KeyPressMsg{Code: ']', Text: "]"}) // wrap to alpha, restoring the workspace
	require.Equal(t, "alpha", root.activeID())
	after, _ := workspacePoint(t, root, "BODY alpha")
	require.Equal(t, before, after, "page-number digit changes must not move panes under the mouse")
}

func TestWorkspace_PaneFooterIsStyledCompactAndUsesUsefulHints(t *testing.T) {
	t.Parallel()
	root, _, _, _ := workspaceRoot(t)
	root.width, root.height = 160, 33
	root.handleResize()
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})

	raw := root.paneFooter(root.planPanes())
	view := tab.StripANSI(raw)
	require.Contains(t, raw, "\x1b[", "pane status must use theme styles")
	require.Contains(t, view, "-- Pane --")
	require.Contains(t, view, "hjkl focus")
	require.Contains(t, view, "HJKL move")
	require.Contains(t, view, "v/s split")
	require.Contains(t, view, "ctrl+t new")
	require.Contains(t, view, "x close")
	require.Contains(t, view, "z zoom")
	require.Contains(t, view, "<>-+ size")
	require.Contains(t, view, "tab cycle")
	require.Contains(t, view, "esc/f2 exit")
	require.LessOrEqual(t, lipgloss.Width(raw), 160)

	root.width = 60
	root.handleResize()
	narrow := tab.StripANSI(root.paneFooter(root.planPanes()))
	require.Contains(t, narrow, "-- Pane --")
	require.Contains(t, narrow, "hjkl focus")
	require.NotContains(t, narrow, "ctrl+t new", "narrow terminals keep only the core hints")
	require.LessOrEqual(t, lipgloss.Width(root.paneFooter(root.planPanes())), 60)
}

func TestWorkspace_RemapAndDisableControlsThroughKeymap(t *testing.T) {
	t.Parallel()
	root, a, b, _ := workspaceRoot(t)
	require.NoError(t, yaml.Unmarshal([]byte("pane_mode: [ctrl+g]\npane_split_columns: [c]\npane_focus_left: []\npane_close: [d]\n"), &root.keymap))
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	require.Zero(t, root.stack.Len(), "old default must not bypass an override")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'c', Text: "c"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, b.ID(), root.activeID())
	footer := tab.StripANSI(root.paneFooter(root.planPanes()))
	require.Contains(t, footer, "c/s split", "footer hints must follow the configured keymap")
	require.Contains(t, footer, "d close")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'h', Text: "h"})
	require.Equal(t, b.ID(), root.activeID(), "explicitly disabled actions must remain disabled")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'd', Text: "d"})
	require.False(t, b.closed)
	require.Equal(t, a.ID(), root.activeID())
}

func TestWorkspace_RenderingPreservesKittyPlaceholderAttributesAndLocalRows(t *testing.T) {
	t.Parallel()
	root, a, _, _ := workspaceRoot(t)
	placeholder := "\U0010EEEE\u030D\u0305"
	a.bodyOverride = "\x1b[38;2;0;0;1m" + placeholder + "\x1b[39m\nsecond graphic row " + placeholder
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	view := root.View().Content
	require.Contains(t, view, placeholder)
	require.Contains(t, view, "38;2;0;0;1", "image identity encoded in foreground color must survive composition")
	require.Contains(t, view, "BODY beta")
	require.Equal(t, 101, lipgloss.Width(view))
	require.Equal(t, 33, lipgloss.Height(view))
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'z', Text: "z"})
	require.NotContains(t, root.View().Content, placeholder, "zoom must not leave hidden pane content on the canvas")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'h', Text: "h"})
	require.Contains(t, root.View().Content, placeholder)
	require.NotContains(t, root.View().Content, "BODY beta")
}

func TestWorkspace_RestoreMapsOriginalRecordPositionsAfterSkippedAccounts(t *testing.T) {
	t.Parallel()
	root := newRestoreTestRoot(t)
	root.Update(persistedDataLoadedMessage{state: save.AppState{
		Tabs: []save.TabState{
			{Channel: "missing", IdentityID: "removed-account", IsFocused: true},
			{Channel: "beta", IdentityID: "acc"},
			{Kind: int(tab.LiveNotificationKind)},
		},
		Layout: json.RawMessage(`{"version":1,"root":{"axis":"columns","ratio":5000,"first":{"tab":"0"},"second":{"axis":"rows","ratio":6000,"first":{"tab":"1"},"second":{"tab":"2"}}},"focus":"0"}`),
	}})
	t.Cleanup(func() {
		for _, model := range root.tabs.All() {
			model.Close()
		}
	})
	require.Equal(t, 2, root.tabs.Len())
	require.Equal(t, "beta", root.activeTab().Title())
	var saved workspace.State
	require.NoError(t, json.Unmarshal(root.TakeStateSnapshot().Layout, &saved))
	require.Equal(t, workspace.Rows, saved.Root.Axis)
	require.Equal(t, 6000, saved.Root.Ratio)
	require.Equal(t, "0", saved.Root.First.Tab)
	require.Equal(t, "1", saved.Root.Second.Tab)
}

func TestWorkspace_TinyTerminalsStayBoundedAndDoNotDestroySavedLayout(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		t.Run(map[bool]string{false: "horizontal", true: "vertical"}[vertical], func(t *testing.T) {
			t.Parallel()
			root, a, b, _ := workspaceRoot(t)
			if vertical {
				root.settings.VerticalTabList = true
				root.header = newVerticalTabHeader(10, 10, root.theme)
				root.handleResize()
			}
			workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
			workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
			workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
			saved := root.TakeStateSnapshot().Layout
			check := func(width, height int) {
				t.Helper()
				require.NotPanics(t, func() {
					root.Update(tea.WindowSizeMsg{Width: width, Height: height})
					view := root.View().Content
					if width == 0 || height == 0 {
						require.Empty(t, view)
					} else {
						require.LessOrEqual(t, lipgloss.Width(view), width)
						require.LessOrEqual(t, lipgloss.Height(view), height)
					}
				})
				require.GreaterOrEqual(t, a.width, 0)
				require.GreaterOrEqual(t, b.height, 0)
			}
			for _, size := range [][2]int{{0, 0}, {0, 24}, {24, 0}, {1, 1}, {24, 8}, {49, 12}, {101, 33}} {
				check(size[0], size[1])
				require.JSONEq(t, string(saved), string(root.TakeStateSnapshot().Layout))
			}
			workspaceKey(t, root, tea.KeyPressMsg{Code: 'x', Text: "x"})
			workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
			for _, size := range [][2]int{{0, 0}, {1, 1}, {24, 8}, {101, 33}} {
				check(size[0], size[1])
			}
		})
	}
}

func TestWorkspace_MoveAtOuterEdgeRedocksAndChangesSplitOrientation(t *testing.T) {
	t.Parallel()
	root, a, b, _ := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.Equal(t, b.ID(), root.activeID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'h', Text: "h"})
	require.Equal(t, a.ID(), root.activeID())
	// No pane below alpha: moving down re-docks it as a full-width bottom pane.
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'J', Text: "J"})
	panes := root.workspace.Arrange(101, 33).Panes
	require.Len(t, panes, 2)
	require.Equal(t, b.ID(), panes[0].Tab)
	require.Equal(t, a.ID(), panes[1].Tab)
	require.Equal(t, 101, panes[1].Width)
	require.Equal(t, 16, panes[1].Height)
	// No pane left of alpha now: moving left re-docks it back into a column.
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'H', Text: "H"})
	panes = root.workspace.Arrange(101, 33).Panes
	require.Equal(t, a.ID(), panes[0].Tab)
	require.Equal(t, b.ID(), panes[1].Tab)
	require.Equal(t, 50, panes[0].Width)
	require.Equal(t, 33, panes[0].Height)
	require.Equal(t, a.ID(), root.activeID(), "focus follows the moved pane")
	require.Equal(t, []string{"alpha", "beta", "gamma"}, []string{root.tabs.Rows()[0].ID, root.tabs.Rows()[1].ID, root.tabs.Rows()[2].ID}, "moves never reorder registry storage order")
}

func TestWorkspace_ControlsNavigateSwapResizeZoomAndFallback(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 's', Text: "s"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'h', Text: "h"})
	require.Equal(t, a.ID(), root.activeID())
	oldHeight := a.height
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'L', Text: "L"})
	require.Equal(t, a.ID(), root.activeID(), "swap follows the moved tab")
	require.Less(t, a.height, oldHeight)
	workspaceKey(t, root, tea.KeyPressMsg{Code: '>', Text: ">"})
	require.Greater(t, a.width, b.width, "growing the right child moves its divider left")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'z', Text: "z"})
	require.Contains(t, root.View().Content, "BODY alpha")
	require.NotContains(t, root.View().Content, "BODY beta")
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Contains(t, root.View().Content, "STATUS alpha", "zoom keeps the focused tab's shared status line")
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, c.ID(), root.activeID())
	require.Contains(t, root.View().Content, "BODY gamma")
	require.NotContains(t, root.View().Content, "BODY alpha")
	root.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	require.Contains(t, root.View().Content, "SMALL TERMINAL")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'H', Text: "H"})
	require.Equal(t, c.ID(), root.activeID())
	require.Contains(t, root.View().Content, "SMALL TERMINAL")
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'z', Text: "z"})
	root.Update(tea.WindowSizeMsg{Width: 101, Height: 33})
	require.Contains(t, root.View().Content, "BODY alpha")
	require.Contains(t, root.View().Content, "BODY beta")
	require.Contains(t, root.View().Content, "BODY gamma")
}
