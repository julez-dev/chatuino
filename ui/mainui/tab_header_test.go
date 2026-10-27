package mainui

import (
	"strings"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/require"
)

func workspaceHeaderRows(active int) []nav.Row {
	return []nav.Row{
		{ID: "alpha", Title: "alpha", Identity: "me", Active: active == 0, Workspace: "W1", WorkspacePane: 1},
		{ID: "beta", Title: "beta", Identity: "me", Active: active == 1, Workspace: "W1", WorkspacePane: 2},
		{ID: "gamma", Title: "gamma", Identity: "me", Active: active == 2, Workspace: "W2", WorkspacePane: 1},
	}
}

func TestTabHeader_WorkspaceBadgesAndTiledSummary(t *testing.T) {
	t.Parallel()

	rows := workspaceHeaderRows(0)
	h := newHorizontalTabHeader(80, save.BuildDefaultTheme())
	view := chat.StripANSI(h.View(rows))

	require.Contains(t, view, "[ Tabs 2 tiled ]")
	require.Contains(t, view, "▸ W1.1 alpha (me)")
	require.Contains(t, view, "W1.2 beta (me)")
	require.Contains(t, view, "W2.1 gamma (me)")

	line := strings.Split(view, "\n")[1]
	x := strings.Index(line, "beta")
	require.Positive(t, x)
	id, ok := h.TabAt(rows, x, 1)
	require.True(t, ok, "badges must be included in tab hit-testing")
	require.Equal(t, "beta", id)
}

func TestTabHeader_ShowsMembershipForOnePaneWorkspace(t *testing.T) {
	t.Parallel()

	h := newHorizontalTabHeader(80, save.BuildDefaultTheme())
	rows := []nav.Row{
		{ID: "alpha", Title: "alpha", Identity: "me", Active: true, Workspace: "W1", WorkspacePane: 1},
		{ID: "gamma", Title: "gamma", Identity: "me"},
	}
	view := chat.StripANSI(h.View(rows))
	require.Contains(t, view, "[ Tabs ]")
	require.Contains(t, view, "▸ W1.1 alpha (me)")
	require.Contains(t, view, "· gamma (me)")
}

func TestTabHeader_NoBadgesWhenAllTabsAreStandalone(t *testing.T) {
	t.Parallel()

	h := newHorizontalTabHeader(80, save.BuildDefaultTheme())
	rows := []nav.Row{
		{ID: "alpha", Title: "alpha", Identity: "me", Active: true},
		{ID: "gamma", Title: "gamma", Identity: "me"},
	}
	view := chat.StripANSI(h.View(rows))
	require.Contains(t, view, "[ Tabs ]")
	require.Contains(t, view, "alpha (me)")
	require.NotContains(t, view, "W1")
	require.NotContains(t, view, "· gamma")
}

func TestTabHeader_StandaloneActiveStillShowsOtherWorkspaceMembership(t *testing.T) {
	t.Parallel()

	h := newHorizontalTabHeader(80, save.BuildDefaultTheme())
	rows := []nav.Row{
		{ID: "alpha", Title: "alpha", Identity: "me", Workspace: "W1", WorkspacePane: 1},
		{ID: "gamma", Title: "gamma", Identity: "me", Active: true},
	}
	view := chat.StripANSI(h.View(rows))
	require.Contains(t, view, "[ Tabs standalone ]")
	require.Contains(t, view, "W1.1 alpha (me)")
	require.Contains(t, view, "▸ · gamma (me)")
}

func TestVerticalTabHeader_WorkspaceBadgesAndSummary(t *testing.T) {
	t.Parallel()

	v := newVerticalTabHeader(40, 10, save.BuildDefaultTheme())
	view := chat.StripANSI(v.View(workspaceHeaderRows(0)))
	require.Contains(t, view, "[ Channels 2 tiled ]")
	require.Contains(t, view, "▸ W1.1 alpha (me)")
	require.Contains(t, view, "W1.2 beta (me)")
	require.Contains(t, view, "W2.1 gamma (me)")
}
