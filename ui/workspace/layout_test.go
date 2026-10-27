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

// A split along the container's own axis joins the container as a sibling and
// rebalances every child equally, like adding a window to an i3 container.
func TestLayout_SplitJoinsContainerAsEqualSiblings(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 99, 24))
	require.NoError(t, layout.Split("alpha", "gamma", workspace.Columns, 99, 24))

	arrangement := layout.Arrange(99, 24)
	require.True(t, arrangement.Fits)
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 32, Height: 24}},
		{Tab: "gamma", Rect: workspace.Rect{X: 33, Width: 32, Height: 24}},
		{Tab: "beta", Rect: workspace.Rect{X: 66, Width: 33, Height: 24}},
	}, arrangement.Panes, "the new pane is inserted after the focused one and all three share the row")

	// A split along the other axis wraps the focused leaf instead of flattening
	// the existing container.
	require.NoError(t, layout.Split("beta", "delta", workspace.Rows, 99, 49))
	rows := layout.Arrange(99, 49)
	require.Equal(t, "beta", rows.Panes[2].Tab)
	require.Equal(t, "delta", rows.Panes[3].Tab)
	require.Equal(t, 33, rows.Panes[2].Width, "the wrapped pane keeps the container's width")

	// Removing panes renormalizes the survivors' shares instead of resetting them.
	require.Equal(t, "beta", layout.Remove("delta"))
	require.Equal(t, "gamma", layout.Remove("alpha"))
	require.Equal(t, []workspace.Pane{
		{Tab: "gamma", Rect: workspace.Rect{Width: 49, Height: 24}},
		{Tab: "beta", Rect: workspace.Rect{X: 50, Width: 49, Height: 24}},
	}, layout.Arrange(99, 24).Panes)
}

func TestLayout_SplitAxisInheritsTheFocusedContainer(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.Equal(t, workspace.Columns, layout.SplitAxis("alpha"), "a lone pane splits to the right by default")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Rows, 101, 33))
	require.Equal(t, workspace.Rows, layout.SplitAxis("beta"))
	// Inheriting Rows inserts below the focused pane inside the same container.
	require.NoError(t, layout.Split("beta", "gamma", layout.SplitAxis("beta"), 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 101, Height: 10}},
		{Tab: "beta", Rect: workspace.Rect{Y: 11, Width: 101, Height: 10}},
		{Tab: "gamma", Rect: workspace.Rect{Y: 22, Width: 101, Height: 11}},
	}, layout.Arrange(101, 33).Panes)
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
	require.Empty(t, layout.Remove(""), "an empty target must not match an internal container")
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

func TestLayout_MoveSwapsNeighboursAndRedocksAtTheRootEdge(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 33))

	// Both panes are children of the root container: alpha is already at the
	// left edge of a matching axis, so that move must not rewrite the layout.
	before := layout.Arrange(101, 33)
	revision := layout.Revision()
	require.False(t, layout.Move("alpha", workspace.Left, 101, 33))
	require.Equal(t, before, layout.Arrange(101, 33))
	require.Equal(t, revision, layout.Revision())

	// There is no container above the root to lift into, so a vertical move
	// re-docks the pane as a new half of the root split and the row becomes a
	// stack.
	require.True(t, layout.Move("alpha", workspace.Down, 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "beta", Rect: workspace.Rect{Width: 101, Height: 16}},
		{Tab: "alpha", Rect: workspace.Rect{Y: 17, Width: 101, Height: 16}},
	}, layout.Arrange(101, 33).Panes)
	require.False(t, layout.Move("alpha", workspace.Down, 101, 33), "already at the requested root edge")

	// Re-docking from the bottom edge to the left restores a row layout.
	require.True(t, layout.Move("alpha", workspace.Left, 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 50, Height: 33}},
		{Tab: "beta", Rect: workspace.Rect{X: 51, Width: 50, Height: 33}},
	}, layout.Arrange(101, 33).Panes)

	// A pane with a neighbour keeps the current geometry and swaps contents.
	require.True(t, layout.Move("alpha", workspace.Right, 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "beta", Rect: workspace.Rect{Width: 50, Height: 33}},
		{Tab: "alpha", Rect: workspace.Rect{X: 51, Width: 50, Height: 33}},
	}, layout.Arrange(101, 33).Panes)
	require.False(t, layout.Move("alpha", workspace.Right, 101, 33))
}

// Moving the third pane of a row down re-docks it below the remaining row, the
// shape that a strict i3 root-edge refusal could not produce.
func TestLayout_MoveRedocksTheLastPaneOfARowBelowIt(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 161, 65))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Columns, 161, 65))
	require.True(t, layout.Move("gamma", workspace.Down, 161, 65))
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 80, Height: 32}},
		{Tab: "beta", Rect: workspace.Rect{X: 81, Width: 80, Height: 32}},
		{Tab: "gamma", Rect: workspace.Rect{Y: 33, Width: 161, Height: 32}},
	}, layout.Arrange(161, 65).Panes)
}

// At the edge of a nested container the pane leaves it and becomes a sibling of
// that container in the container above (one level up), instead of re-docking as
// a new split at the screen edge.
func TestLayout_MoveAtNestedEdgeLiftsPaneOneLevel(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 33))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Rows, 101, 33))

	// columns[alpha, rows[beta, gamma]]: beta has no pane above it, so it lifts
	// out of the rows container into the root columns container.
	require.True(t, layout.Move("beta", workspace.Up, 101, 33))
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 33, Height: 33}},
		{Tab: "beta", Rect: workspace.Rect{X: 34, Width: 33, Height: 33}},
		{Tab: "gamma", Rect: workspace.Rect{X: 68, Width: 33, Height: 33}},
	}, layout.Arrange(101, 33).Panes)
	require.Equal(t, workspace.Columns, layout.SplitAxis("gamma"), "the surviving pane now belongs to the root row")
}

func TestLayout_MovePreservesTheSurvivingSubtreeShares(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 65))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Rows, 101, 65))
	require.True(t, layout.Resize("beta", workspace.Rows, 1, 101, 65))
	before := layout.Arrange(101, 65)
	require.Greater(t, before.Panes[1].Height, before.Panes[2].Height)

	require.True(t, layout.Move("beta", workspace.Up, 101, 65))
	after := layout.Arrange(101, 65)
	require.Equal(t, []string{"alpha", "beta", "gamma"}, []string{after.Panes[0].Tab, after.Panes[1].Tab, after.Panes[2].Tab})
	for _, pane := range after.Panes {
		require.Equal(t, 65, pane.Height, "the lifted pane and the former container share the root row")
	}
	require.Equal(t, 101, after.Panes[0].Width+after.Panes[1].Width+after.Panes[2].Width+2)
}

// Lifting a pane out of a container with three or more children must leave the
// survivors' weights normalized: Resize and Restore both assume the weightTotal
// scale, and a stale scale silently turns one 5% resize step into a big jump and
// makes a freshly snapshotted layout restore as "repaired".
func TestLayout_MoveRenormalizesSurvivingContainerShares(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Rows, 161, 65))
	require.NoError(t, layout.Split("alpha", "gamma", workspace.Columns, 161, 65))
	require.NoError(t, layout.Split("alpha", "delta", workspace.Columns, 161, 65))

	// rows[columns[alpha, delta, gamma], beta]: alpha is at the left edge of
	// the nested columns container, so it lifts into the root rows container.
	require.True(t, layout.Move("alpha", workspace.Left, 161, 65))

	snapshot := layout.Snapshot("alpha", nil)
	_, _, repaired := workspace.Restore(snapshot, nil)
	require.False(t, repaired, "a move must not leave weights outside the snapshot scale")

	before := layout.Arrange(161, 65)
	// The surviving columns container sits beside beta at full width, so one
	// step moves its divider by 5% of 160 available cells, not by a share of a
	// stale 6666-scale weight.
	var gammaIndex int
	for i, pane := range before.Panes {
		if pane.Tab == "gamma" {
			gammaIndex = i
		}
	}
	require.True(t, layout.Resize("gamma", workspace.Columns, 1, 161, 65))
	after := layout.Arrange(161, 65)
	require.Equal(t, 8, after.Panes[gammaIndex].Width-before.Panes[gammaIndex].Width, "one 5% step of the container")
}

func TestLayout_MoveRejectsAProspectiveLayoutThatCannotFit(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 60, 33))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Rows, 60, 33))
	before := layout.Arrange(60, 33)
	revision := layout.Revision()
	// Lifting beta into the root row adds a divider and a pane minimum, which no
	// longer fits 60 columns.
	require.False(t, layout.Move("beta", workspace.Up, 60, 33))
	require.Equal(t, before, layout.Arrange(60, 33))
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

// Resizing moves the divider next to the focused pane, also when it sits in the
// middle of a container with more than two children.
func TestLayout_ResizeThreeSiblingsMovesTheFocusedDivider(t *testing.T) {
	t.Parallel()
	var layout workspace.Layout
	layout.Select("", "alpha")
	require.NoError(t, layout.Split("alpha", "beta", workspace.Columns, 101, 24))
	require.NoError(t, layout.Split("beta", "gamma", workspace.Columns, 101, 24))
	before := layout.Arrange(101, 24)
	require.True(t, layout.Resize("beta", workspace.Columns, 1, 101, 24))
	after := layout.Arrange(101, 24)
	require.Greater(t, after.Panes[1].Width, before.Panes[1].Width)
	require.Equal(t, before.Panes[0].Width, after.Panes[0].Width, "the divider after beta moves, not alpha's")
	require.Less(t, after.Panes[2].Width, before.Panes[2].Width)
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
	decoded.Root.Children[0].Tab = "modified"
	require.True(t, restored.Contains("new-alpha"), "restored layout must not alias input state")
	require.True(t, layout.Contains("alpha"), "snapshot must not expose the live tree")
}

// Version 1 layouts stored binary splits. Restoring one keeps the same ratios as
// a two-child container so old sessions reopen with their geometry intact.
func TestLayout_RestoreMigratesVersionOneBinarySplits(t *testing.T) {
	t.Parallel()
	tree := &workspace.Node{
		Axis: workspace.Columns, Ratio: 3000,
		First:  &workspace.Node{Tab: "alpha"},
		Second: &workspace.Node{Axis: workspace.Rows, Ratio: 7500, First: &workspace.Node{Tab: "beta"}, Second: &workspace.Node{Tab: "gamma"}},
	}
	layout, focus, repaired := workspace.Restore(workspace.State{Version: 1, Root: tree, Focus: "gamma"}, nil)
	require.False(t, repaired)
	require.Equal(t, "gamma", focus)
	require.Equal(t, 3, layout.Count())
	require.Equal(t, []workspace.Pane{
		{Tab: "alpha", Rect: workspace.Rect{Width: 30, Height: 33}},
		{Tab: "beta", Rect: workspace.Rect{X: 31, Width: 70, Height: 24}},
		{Tab: "gamma", Rect: workspace.Rect{X: 31, Y: 25, Width: 70, Height: 8}},
	}, layout.Arrange(101, 33).Panes)
	// Migration is one-way: the snapshot is written in the sibling-container
	// format and restores without repairs.
	state := layout.Snapshot(focus, nil)
	require.Equal(t, 2, state.Version)
	_, _, repaired = workspace.Restore(state, nil)
	require.False(t, repaired)
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
				// Vary the moved tab so the edge-lift and removal paths are
				// reachable; Move(layout.First(), Right, …) never leaves a
				// container.
				if panes := layout.PaneOrder(); len(panes) > 0 {
					layout.Swap(panes[int(op/8)%len(panes)], workspace.Direction(int(op/8)%4), 161, 65)
				}
			case 5:
				axis := workspace.Columns
				if op&1 != 0 {
					axis = workspace.Rows
				}
				layout.Resize(current, axis, 1-int(op%3), 49+int(op%113), 17+int(op%49))
			case 6:
				layout.Arrange(int(op%32), int(op%16))
			case 7:
				if panes := layout.PaneOrder(); len(panes) > 0 {
					layout.Move(panes[int(op/8)%len(panes)], workspace.Direction(int(op/8)%4), 49+int(op%113), 17+int(op%49))
				}
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
		tree = &workspace.Node{
			Axis: workspace.Rows, Weights: []int{5000, 5000},
			Children: []*workspace.Node{tree, {Tab: strconv.Itoa(i)}},
		}
	}
	layout, _, repaired := workspace.Restore(workspace.State{Version: 2, Root: tree, Focus: "0"}, nil)
	require.False(t, repaired, "a valid layout must not lose panes at a hard-coded nesting limit")
	require.Equal(t, 141, layout.Count())
	require.True(t, layout.Arrange(24, 1268).Fits)
	cycle := &workspace.Node{Axis: workspace.Columns, Weights: []int{5000, 5000}, Children: []*workspace.Node{nil, {Tab: "alpha"}}}
	cycle.Children[0] = cycle
	layout, _, repaired = workspace.Restore(workspace.State{Version: 2, Root: cycle, Focus: "alpha"}, nil)
	require.True(t, repaired)
	require.Equal(t, 1, layout.Count())
}

func TestLayout_RestoreRejectsInvalidMetadataWithoutDuplicatePanes(t *testing.T) {
	t.Parallel()
	for _, state := range []workspace.State{
		{Version: 100, Root: &workspace.Node{Tab: "alpha"}},
		{Version: 1, Root: &workspace.Node{Axis: "invalid", First: &workspace.Node{Tab: "alpha"}}},
		{Version: 1, Root: &workspace.Node{Axis: workspace.Columns, Ratio: -1, First: &workspace.Node{Tab: "alpha"}, Second: &workspace.Node{Tab: "alpha"}}},
		{Version: 2, Root: &workspace.Node{Axis: "invalid", Children: []*workspace.Node{{Tab: "alpha"}}}},
		{Version: 2, Root: &workspace.Node{Axis: workspace.Columns, Children: []*workspace.Node{{Tab: "alpha"}, {Tab: "alpha"}}}},
		{Version: 2, Root: &workspace.Node{Axis: workspace.Columns, Children: []*workspace.Node{{Tab: "alpha"}, {Tab: "beta"}}, Weights: []int{1}}},
	} {
		layout, _, repaired := workspace.Restore(state, nil)
		require.True(t, repaired)
		seen := map[string]bool{}
		for _, tab := range layout.PaneOrder() {
			require.False(t, seen[tab], "a tab must not be restored twice")
			seen[tab] = true
		}
	}
}
