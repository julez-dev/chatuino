package workspace_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/julez-dev/chatuino/ui/workspace"
	"github.com/stretchr/testify/require"
)

func TestLayout_SplitKeepsExistingTabAndRejectsDuplicateViews(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	require.True(t, layout.Select("", "alpha"))
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 81, 24))
	arrangement := layout.Arrange(81, 24)
	require.True(t, arrangement.Fits)
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 40, Height: 24}},
		{Tab: "beta", Rect: workspace.Rect{X: 41, Width: 40, Height: 24}},
	}, arrangement.Panes)
	require.Equal(t, []workspace.Rect{{X: 40, Width: 1, Height: 24}}, arrangement.Dividers)
	require.Error(t, layout.Split("alpha", "beta", workspace.Rows, 81, 24))
	require.False(t, layout.Select("alpha", "beta"), "selecting an assigned tab must not duplicate it")
	require.Equal(t, arrangement, layout.Arrange(81, 24))
}

func TestLayout_CloseCollapsesSiblingAndPreservesItsSubtree(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 33))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Rows, 101, 33))
	before := layout.Arrange(101, 33)
	require.Equal(t, "beta", layout.Remove("alpha"))
	require.False(t, layout.Contains("alpha"))
	require.Equal(t, 2, layout.Count())
	after := layout.Arrange(101, 33)
	require.Equal(t, []workspace.Pane{
		{Tab: "beta", Rect: workspace.Rect{Width: 101, Height: 16}},
		{Tab: "gamma", Rect: workspace.Rect{Y: 17, Width: 101, Height: 16}},
	}, after.Panes)
	require.Equal(t, before.Panes[1].Height, after.Panes[0].Height)
	revision := layout.Revision()
	require.Empty(t, layout.Remove("missing"))
	require.Empty(t, layout.Remove(""), "an empty target must not match an internal split node")
	require.Equal(t, 2, layout.Count())
	require.Equal(t, revision, layout.Revision())
	require.Equal(t, "gamma", layout.Remove("beta"))
	require.Equal(t, "gamma", layout.First())
}

func TestLayout_NavigationAndSwapsUseSpatialOrderWithoutWrapping(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 33))
	require.NoError(t, layout.Split("alpha", "gamma", workspace.Rows, 101, 33))
	require.Equal(t, "beta", layout.Cycle("alpha", 1, 101, 33), "reading order is not subtree traversal order")
	require.Equal(t, "gamma", layout.Cycle("alpha", -1, 101, 33))
	require.Equal(t, "alpha", layout.Cycle("gamma", 1, 101, 33))
	require.Empty(t, layout.Neighbour("alpha", workspace.Left, 101, 33))
	require.Equal(t, "gamma", layout.Neighbour("alpha", workspace.Down, 101, 33))
	require.Equal(t, "beta", layout.Neighbour("alpha", workspace.Right, 0, 0), "navigation must still work during fallback")
	require.True(t, layout.Swap("alpha", workspace.Right, 101, 33))
	panes := layout.Arrange(101, 33).Panes
	require.Equal(t, "beta", panes[0].Tab)
	require.Equal(t, "gamma", panes[1].Tab)
	require.Equal(t, "alpha", panes[2].Tab)
	require.False(t, layout.Swap("alpha", workspace.Right, 101, 33))
}

func TestLayout_MoveSwapsNeighboursAndRedocksAtOuterEdges(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 33))

	// A pane with a neighbour keeps the current geometry and swaps contents.
	require.True(t, layout.Move("alpha", workspace.Right, 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "beta", Rect: workspace.Rect{Width: 50, Height: 33}},
		{Tab: "alpha", Rect: workspace.Rect{X: 51, Width: 50, Height: 33}},
	}, layout.Arrange(101, 33).Panes)

	// At an outer edge, move re-docks the pane and changes the split orientation.
	require.True(t, layout.Move("alpha", workspace.Down, 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "beta", Rect: workspace.Rect{Width: 101, Height: 16}},
		{Tab: "alpha", Rect: workspace.Rect{Y: 17, Width: 101, Height: 16}},
	}, layout.Arrange(101, 33).Panes)
	require.False(t, layout.Move("alpha", workspace.Down, 101, 33), "already at the requested outer edge")

	// A move at the new edge can switch back to columns.
	require.True(t, layout.Move("alpha", workspace.Left, 101, 33))
	columns := layout.Arrange(101, 33)
	require.Equal(t, "alpha", columns.Panes[0].Tab)
	require.Equal(t, "beta", columns.Panes[1].Tab)
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 50, Height: 33}},
		{Tab: "beta", Rect: workspace.Rect{X: 51, Width: 50, Height: 33}},
	}, columns.Panes, "moving from the bottom edge back to the left restores a row layout")
}

func TestLayout_MovePreservesTheRemainingSubtreeAndPreferredRatios(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 65))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Rows, 101, 65))
	require.True(t, layout.Resize("beta", workspace.Rows, 1, 101, 65))
	require.True(t, layout.Move("alpha", workspace.Down, 101, 65))
	after := layout.Arrange(101, 65)
	require.Equal(t, "alpha", after.Panes[2].Tab)
	require.Equal(t, 101, after.Panes[2].Width, "the moved pane becomes a full-width bottom pane")
	require.Equal(t, "beta", after.Panes[0].Tab)
	require.Equal(t, "gamma", after.Panes[1].Tab)
	require.Greater(t, after.Panes[0].Height, after.Panes[1].Height, "the surviving nested split keeps its preferred ratio")
	require.Equal(t, 31, after.Panes[0].Height+after.Panes[1].Height, "the surviving subtree keeps the top half, excluding its divider")
	require.Equal(t, 32, after.Panes[2].Height)
}

func TestLayout_MoveRejectsAProspectiveLayoutThatCannotFit(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 49, 12))
	before := layout.Arrange(49, 12)
	revision := layout.Revision()
	require.False(t, layout.Move("alpha", workspace.Down, 49, 12), "a stacked layout needs 17 rows")
	require.Equal(t, before, layout.Arrange(49, 12))
	require.Equal(t, revision, layout.Revision())
}

func TestLayout_ResizeClampsAndTerminalChangesDoNotRewriteProportions(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 141, 24))
	require.True(t, layout.Resize("alpha", workspace.Columns, 1, 141, 24))
	require.Equal(t, 77, layout.Arrange(141, 24).Panes[0].Width)
	require.Equal(t, 63, layout.Arrange(141, 24).Panes[1].Width)
	for range 100 {
		layout.Resize("alpha", workspace.Columns, 1, 141, 24)
	}
	before := layout.Arrange(141, 24)
	require.Equal(t, 116, before.Panes[0].Width)
	require.Equal(t, 24, before.Panes[1].Width)
	revision := layout.Revision()
	require.False(t, layout.Resize("alpha", workspace.Columns, 1, 141, 24))
	require.Equal(t, revision, layout.Revision())
	for range 100 {
		require.False(t, layout.Arrange(20, 3).Fits)
		require.True(t, layout.Arrange(49, 8).Fits)
		require.Equal(t, before, layout.Arrange(141, 24))
	}
	require.False(t, layout.Resize("alpha", workspace.Columns, -1, 20, 3), "fallback must not allow unseen edits")
	require.False(t, layout.Resize("alpha", workspace.Rows, 1, 141, 24), "no matching divider")
}

func TestLayout_ResizeStartsFromVisibleDividerAfterTerminalClamping(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 141, 24))
	for range 20 {
		layout.Resize("alpha", workspace.Columns, 1, 141, 24)
	}
	require.Equal(t, 24, layout.Arrange(70, 24).Panes[1].Width)
	require.True(t, layout.Resize("beta", workspace.Columns, 1, 70, 24), "a clamped pane must be growable without first enlarging the terminal")
	require.Greater(t, layout.Arrange(70, 24).Panes[1].Width, 24)
}

func TestLayout_SnapshotRestoresNewIdentitiesAndRepairsMissingTabs(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 33))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Rows, 101, 33))
	require.True(t, layout.Resize("alpha", workspace.Columns, 1, 101, 33))
	state := layout.Snapshot("gamma", map[string]string{"alpha": "0", "beta": "1", "gamma": "2"})
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	var decoded workspace.State
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	restored, focus, repaired := workspace.Restore(decoded, map[string]string{"0": "new-alpha", "1": "new-beta", "2": "new-gamma"})
	require.False(t, repaired)
	require.Equal(t, "new-gamma", focus)
	require.Equal(t, layout.Arrange(101, 33).Panes[0].Rect, restored.Arrange(101, 33).Panes[0].Rect)
	repairedLayout, focus, repaired := workspace.Restore(decoded, map[string]string{"0": "new-alpha", "1": "new-beta"})
	require.True(t, repaired)
	require.Equal(t, "new-alpha", focus)
	require.Equal(t, 2, repairedLayout.Count())
	require.Equal(t, 33, repairedLayout.Arrange(101, 33).Panes[1].Height)
	decoded.Root.First.Tab = "modified"
	require.True(t, restored.Contains("new-alpha"), "restored layout must not alias input state")
	require.True(t, layout.Contains("alpha"), "snapshot must not expose the live tree")
}

func TestLayout_AllocatesCellsExactlyOnceAndRejectsTooSmallSplits(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	revision := layout.Revision()
	require.Error(t, layout.CanSplit("alpha", workspace.Columns, 48, 8))
	require.NoError(t, layout.CanSplit("alpha", workspace.Columns, 49, 8))
	require.Error(t, layout.Split("alpha", "beta", workspace.Columns, 48, 8))
	require.Equal(t, revision, layout.Revision())
	require.Equal(t, 1, layout.Count())
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 161, 49))
	require.NoError(t, layout.Split("alpha", "gamma", workspace.Rows, 161, 49))
	require.NoError(t, layout.Split("beta", "delta", workspace.Columns, 161, 49))
	for width := 0; width < 150; width += 13 {
		for height := 0; height < 60; height += 7 {
			arrangement := layout.Arrange(width, height)
			occupied := map[[2]int]bool{}
			var maxX, maxY int
			occupy := func(r workspace.Rect) {
				for y := r.Y; y < r.Y+r.Height; y++ {
					for x := r.X; x < r.X+r.Width; x++ {
						cell := [2]int{x, y}
						require.False(t, occupied[cell], "overlapping allocation at %v", cell)
						occupied[cell] = true
					}
				}
				maxX, maxY = max(maxX, r.X+r.Width), max(maxY, r.Y+r.Height)
			}
			for _, pane := range arrangement.Panes {
				require.GreaterOrEqual(t, pane.Width, 24)
				require.GreaterOrEqual(t, pane.Height, 8)
				occupy(pane.Rect)
			}
			for _, divider := range arrangement.Dividers {
				occupy(divider)
			}
			require.Len(t, occupied, maxX*maxY, "unallocated cells")
			if arrangement.Fits {
				require.Equal(t, width, maxX)
				require.Equal(t, height, maxY)
			}
		}
	}
}

func TestLayout_ResizeGrowsFocusedChildOnBothAxes(t *testing.T) {
	for _, axis := range []workspace.Axis{workspace.Columns, workspace.Rows} {
		for _, tab := range []string{"alpha", "beta"} {
			t.Run(string(axis)+"/"+tab, func(t *testing.T) {
				t.Parallel()
				var layout workspace.Layout
				layout.Select("", "alpha")
				require.NoError(t, layout.Split("alpha", "beta", axis, 161, 81))
				before := layout.Arrange(161, 81)
				require.True(t, layout.Resize(tab, axis, 1, 161, 81))
				after := layout.Arrange(161, 81)
				for i, pane := range before.Panes {
					if pane.Tab != tab {
						continue
					}
					if axis == workspace.Columns {
						require.Greater(t, after.Panes[i].Width, pane.Width)
						require.Equal(t, pane.Height, after.Panes[i].Height)
					} else {
						require.Greater(t, after.Panes[i].Height, pane.Height)
						require.Equal(t, pane.Width, after.Panes[i].Width)
					}
				}
			})
		}
	}
}

func FuzzLayout_OperationsPreserveAssignmentsAndRoundTrip(f *testing.F) {
	f.Add([]byte{0, 1, 7, 20, 45, 64, 255, 128, 2, 3, 4, 5})
	f.Add([]byte{3, 3, 3, 17, 18, 19, 41, 42})
	f.Fuzz(func(t *testing.T, operations []byte) {
		if len(operations) > 64 {
			return
		}
		var layout workspace.Layout
		layout.Select("", "alpha")
		tabs := []string{"", "alpha", "beta", "gamma", "delta", "epsilon"}
		for _, op := range operations {
			current, target := layout.First(), tabs[int(op)%len(tabs)]
			switch op % 8 {
			case 0:
				layout.Split(current, target, workspace.Columns, 161, 65)
			case 1:
				layout.Split(current, target, workspace.Rows, 161, 65)
			case 2:
				if layout.Count() > 1 {
					layout.Remove(target)
				}
			case 3:
				layout.Select(current, target)
			case 4:
				layout.Swap(current, workspace.Direction(op%4), 161, 65)
			case 5:
				axis := workspace.Columns
				if op&1 != 0 {
					axis = workspace.Rows
				}
				layout.Resize(current, axis, 1-int(op%3), 49+int(op%113), 17+int(op%49))
			case 6:
				layout.Arrange(int(op%32), int(op%16))
			case 7:
				layout.Move(current, workspace.Direction(op%4), 49+int(op%113), 17+int(op%49))
			}
			arrangement := layout.Arrange(161, 65)
			require.True(t, arrangement.Fits)
			seen := map[string]bool{}
			for _, pane := range arrangement.Panes {
				require.NotEmpty(t, pane.Tab)
				require.False(t, seen[pane.Tab], "a tab was duplicated")
				seen[pane.Tab] = true
				require.GreaterOrEqual(t, pane.Width, 0)
				require.GreaterOrEqual(t, pane.Height, 0)
			}
			require.Equal(t, layout.Count(), len(seen))
			state := layout.Snapshot(layout.First(), nil)
			clone, _, repaired := workspace.Restore(state, nil)
			require.False(t, repaired)
			require.Equal(t, arrangement, clone.Arrange(161, 65))
		}
	})
}

func TestLayout_RestoreHasNoArbitraryDepthLimitButRejectsCycles(t *testing.T) {
	t.Parallel()
	tree := &workspace.Node{Tab: "0"}
	for i := 1; i <= 140; i++ {
		tree = &workspace.Node{Axis: workspace.Rows, Ratio: 5000, First: tree, Second: &workspace.Node{Tab: strconv.Itoa(i)}}
	}
	layout, _, repaired := workspace.Restore(workspace.State{Version: 1, Root: tree, Focus: "0"}, nil)
	require.False(t, repaired, "a valid layout must not lose panes at a hard-coded nesting limit")
	require.Equal(t, 141, layout.Count())
	require.True(t, layout.Arrange(24, 1268).Fits)
	cycle := &workspace.Node{Axis: workspace.Columns, Ratio: 5000, Second: &workspace.Node{Tab: "alpha"}}
	cycle.First = cycle
	layout, _, repaired = workspace.Restore(workspace.State{Version: 1, Root: cycle, Focus: "alpha"}, nil)
	require.True(t, repaired)
	require.Equal(t, 1, layout.Count())
}

func TestLayout_RestoreRejectsInvalidMetadataWithoutDuplicatePanes(t *testing.T) {
	t.Parallel()
	for _, state := range []workspace.State{
		{Version: 100, Root: &workspace.Node{Tab: "alpha"}},
		{Version: 1, Root: &workspace.Node{Axis: "invalid", First: &workspace.Node{Tab: "alpha"}}},
		{Version: 1, Root: &workspace.Node{Axis: workspace.Columns, Ratio: -1, First: &workspace.Node{Tab: "alpha"}, Second: &workspace.Node{Tab: "alpha"}}},
	} {
		layout, _, repaired := workspace.Restore(state, nil)
		require.True(t, repaired)
		require.LessOrEqual(t, layout.Count(), 1)
		require.True(t, layout.Arrange(0, 0).Fits)
	}
}
