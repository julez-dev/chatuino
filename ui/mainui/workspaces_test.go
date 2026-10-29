package mainui

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/workspace"
	"github.com/stretchr/testify/require"
)

func TestWorkspaces_PaneModeCreatesAWorkspaceAndKeepsEveryTabVisible(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)

	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	require.Equal(t, "workspace-1", root.activeWorkspaceID)
	require.True(t, root.workspace.Contains(a.ID()))
	require.Len(t, root.visibleRows(), 3, "the tab collection is never filtered by workspace")

	rows := root.tabs.Rows()
	require.Equal(t, "W1", rows[0].Workspace)
	require.Equal(t, 1, rows[0].WorkspacePane)
	require.Empty(t, rows[1].Workspace, "beta stays standalone until used in a pane")
	require.Empty(t, rows[2].Workspace)

	view := root.View().Content
	require.Contains(t, view, "W1.1 alpha")
	require.Contains(t, view, "· beta")
	require.Contains(t, view, "· gamma")
	require.True(t, root.workspace.Contains(a.ID()))
	require.False(t, root.workspace.Contains(b.ID()))
	require.False(t, root.workspace.Contains(c.ID()))
}

func TestWorkspaces_TabSwitchingActivatesTheOwningWorkspaceAutomatically(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)

	// W1: alpha and beta.
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.True(t, root.workspace.Contains(a.ID()))
	require.True(t, root.workspace.Contains(b.ID()))

	// W2 is created automatically when gamma enters pane mode.
	root.navigateToTab(c.ID())
	require.False(t, root.workspace.Contains(c.ID()))
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	require.Equal(t, "workspace-2", root.activeWorkspaceID)
	require.True(t, root.workspace.Contains(c.ID()))
	require.False(t, root.workspace.Contains(a.ID()))

	require.Equal(t, "W1", root.tabRow(root.tabByID(a.ID())).Workspace)
	require.Equal(t, "W2", root.tabRow(root.tabByID(c.ID())).Workspace)
	require.Equal(t, []string{a.ID(), b.ID(), c.ID()}, rowIDs(root.visibleRows()),
		"workspaces and their panes determine header order")

	// Selecting alpha restores W1 without a workspace picker.
	root.navigateToTab(a.ID())
	require.Equal(t, "workspace-1", root.activeWorkspaceID)
	require.True(t, root.workspace.Contains(a.ID()))
	require.True(t, root.workspace.Contains(b.ID()))
	require.Equal(t, 2, root.workspace.Count())

	// Selecting gamma switches back to W2.
	root.navigateToTab(c.ID())
	require.Equal(t, "workspace-2", root.activeWorkspaceID)
	require.Equal(t, 1, root.workspace.Count())
	require.True(t, root.workspace.Contains(c.ID()))

	// A tab-list click follows the same automatic rule.
	root.switchToTab(a.ID())
	require.Equal(t, "workspace-1", root.activeWorkspaceID)
	require.Equal(t, 2, root.workspace.Count())
}

func TestWorkspaces_HeaderOrderFollowsPaneReadingOrder(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)

	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.Equal(t, []string{a.ID(), b.ID(), c.ID()}, rowIDs(root.visibleRows()))

	// beta is on the right of alpha; moving it left swaps their pane positions.
	root.navigateToTab(b.ID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'H', Text: "H"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})

	require.Equal(t, []string{b.ID(), a.ID(), c.ID()}, rowIDs(root.visibleRows()),
		"header order follows pane reading order and keeps standalone tabs last")
	require.Equal(t, []string{a.ID(), b.ID(), c.ID()}, rowIDs(root.tabs.Rows()),
		"the registry keeps creation order as its storage order")

	// Tab navigation follows what the header displays.
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, a.ID(), root.activeID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, c.ID(), root.activeID())
}

func rowIDs(rows []nav.Row) []string {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

func TestWorkspaces_StandaloneCreationDoesNotJoinTheActiveWorkspace(t *testing.T) {
	t.Parallel()
	root, a, _, c := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	require.Equal(t, "workspace-1", root.activeWorkspaceID)

	extra := &workspaceTab{fakeTab: newFakeTab("extra")}
	root.tabs.Add(extra)
	root.navigateToTab(extra.ID())
	require.Nil(t, root.workspaceForTab(extra.ID()))
	require.Equal(t, a.ID(), root.workspace.First())
	require.False(t, root.workspace.Contains(extra.ID()))

	// Leaving pane mode is not required to keep a standalone tab standalone;
	// entering pane mode gives the new tab its own workspace.
	root.navigateToTab(c.ID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	require.Equal(t, "workspace-2", root.activeWorkspaceID)
	require.True(t, root.workspace.Contains(c.ID()))
	require.False(t, root.workspace.Contains(extra.ID()))
}

func TestWorkspaces_SplitPickerOnlyOffersStandaloneTabs(t *testing.T) {
	t.Parallel()
	root, a, b, c := workspaceRoot(t)

	// alpha belongs to W1; gamma starts W2. beta remains standalone.
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	root.navigateToTab(c.ID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})

	top, ok := root.stack.Top()
	require.True(t, ok)
	picker, ok := top.(*panePicker)
	require.True(t, ok)

	var offered []string
	for _, item := range picker.list.Items() {
		offered = append(offered, item.(paneItem).label)
	}
	require.Equal(t, []string{"beta ()", "Open a new tab…"}, offered,
		"tabs owned by other workspaces must never be duplicated into this layout")
	require.NotContains(t, offered, "alpha ()")
	require.NotContains(t, offered, "gamma ()")
	require.True(t, root.workspace.Contains(c.ID()))
	require.False(t, root.workspace.Contains(a.ID()))
	require.False(t, root.workspace.Contains(b.ID()))
}

func TestWorkspaces_PaneModeCreateAddsANewPaneToTheCurrentWorkspace(t *testing.T) {
	t.Parallel()
	root, a, _, _ := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	_, _ = root.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})

	require.Equal(t, joinRoute, root.stack.Route())
	require.NotNil(t, root.pendingSplit)
	require.Equal(t, a.ID(), root.pendingSplit.target)
	require.Equal(t, workspace.Columns, root.pendingSplit.axis)

	// Outside pane mode the same binding remains the standalone creation flow.
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.False(t, root.paneMode)
	_, _ = root.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	require.Equal(t, joinRoute, root.stack.Route())
	require.Nil(t, root.pendingSplit)
}

func TestWorkspaces_ClosingTheLastPaneLeavesAReusableEmptyWorkspace(t *testing.T) {
	t.Parallel()
	root, a, b, _ := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	root.Update(CloseTabMessage{TabID: a.ID()})
	require.Equal(t, b.ID(), root.activeID())
	require.Nil(t, root.workspaceForTab(b.ID()))
	require.Zero(t, root.workspace.Count())

	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	require.Equal(t, "workspace-1", root.activeWorkspaceID)
	require.True(t, root.workspace.Contains(b.ID()))
	require.Len(t, root.workspaces, 1, "empty screens are reused instead of accumulating")
}

func TestWorkspaces_MultipleLayoutsRoundTripWithFocusedTab(t *testing.T) {
	t.Parallel()
	root, _, _, c := workspaceRoot(t)
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})
	workspaceKey(t, root, tea.KeyPressMsg{Code: 'v', Text: "v"})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEnter})
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyEscape})
	root.navigateToTab(c.ID())
	workspaceKey(t, root, tea.KeyPressMsg{Code: tea.KeyF2})

	snapshot := root.TakeStateSnapshot()
	var set savedWorkspaceSet
	require.NoError(t, json.Unmarshal(snapshot.Workspaces, &set))
	require.Len(t, set.Workspaces, 2)
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
	require.Equal(t, "workspace-2", restored.activeWorkspaceID)
	require.Equal(t, 1, restored.workspace.Count())
	require.Len(t, restored.visibleRows(), 3)

	alphaID, betaID := restored.tabs.All()[0].ID(), restored.tabs.All()[1].ID()
	restored.navigateToTab(alphaID)
	require.Equal(t, "workspace-1", restored.activeWorkspaceID)
	require.Equal(t, 2, restored.workspace.Count())
	require.True(t, restored.workspace.Contains(betaID))
}

func TestWorkspaces_RestoreRepairsDuplicateOwnershipAndBadMetadata(t *testing.T) {
	t.Parallel()
	layout := workspace.State{Version: 1, Root: &workspace.Node{Tab: "0"}, Focus: "0"}
	multi, err := json.Marshal(savedWorkspaceSet{
		Version: 1,
		Workspaces: []savedWorkspace{
			{ID: "workspace-1", Name: "One", Layout: layout},
			{ID: "workspace-2", Name: "Two", Layout: layout},
		},
	})
	require.NoError(t, err)

	root := newRestoreTestRoot(t)
	root.Update(persistedDataLoadedMessage{state: save.AppState{
		Tabs: []save.TabState{
			{Channel: "alpha", IdentityID: "acc", IsFocused: true},
			{Channel: "beta", IdentityID: "acc"},
		},
		Workspaces: multi,
	}})
	t.Cleanup(func() {
		for _, model := range root.tabs.All() {
			model.Close()
		}
	})
	require.Equal(t, 2, root.tabs.Len())
	require.Len(t, root.workspaces, 2)
	assigned := 0
	for _, ws := range root.workspaces {
		assigned += ws.layout.Count()
	}
	require.Equal(t, 1, assigned, "a tab may only survive in one workspace")

	// Invalid metadata must not prevent valid tabs from loading.
	broken := newRestoreTestRoot(t)
	broken.Update(persistedDataLoadedMessage{state: save.AppState{
		Tabs:       []save.TabState{{Channel: "alpha", IdentityID: "acc"}},
		Workspaces: jsontext.Value(`{"version":`),
	}})
	t.Cleanup(func() {
		for _, model := range broken.tabs.All() {
			model.Close()
		}
	})
	require.Equal(t, 1, broken.tabs.Len())
	require.NotEmpty(t, broken.workspaces)
}
