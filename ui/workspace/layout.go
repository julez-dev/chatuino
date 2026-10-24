// Package workspace implements a tab-referencing split tree. It owns geometry,
// not tab models or keyboard focus; callers supply the active tab identity.
package workspace

import (
	"cmp"
	"errors"
	"math"
	"slices"
)

const (
	MinWidth  = 24
	MinHeight = 8
)

// Direction is a spatial focus/swap direction.
type Direction int

const (
	Left Direction = iota
	Down
	Up
	Right
)

// Axis names describe the arrangement, not the direction of the divider.
type Axis string

const (
	Columns Axis = "columns"
	Rows    Axis = "rows"
)

// Rect is a terminal-cell rectangle with an exclusive right and bottom edge.
type Rect struct {
	X, Y, Width, Height int
}

func (r Rect) Contains(x, y int) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.Width && y < r.Y+r.Height
}

type Pane struct {
	Tab string
	Rect
}

// Arrangement uses virtual minimum-sized bounds when Fits is false. This lets
// callers navigate the whole layout while drawing only their focused pane.
type Arrangement struct {
	Panes    []Pane
	Dividers []Rect
	Fits     bool
}

// Node is a serialized tree node. Snapshot/Restore copy nodes, so changing a
// serialized tree never mutates the live layout. Ratios use ten-thousandths.
type Node struct {
	Tab    string `json:"tab,omitempty"`
	Axis   Axis   `json:"axis,omitempty"`
	Ratio  int    `json:"ratio,omitempty"`
	First  *Node  `json:"first,omitempty"`
	Second *Node  `json:"second,omitempty"`
}

// Layout's zero value is an empty workspace. It deliberately has no active-tab
// field: the tab registry remains the single source of truth for activation.
type Layout struct {
	root     *Node
	revision uint64
}

// State refers to the caller's serialized tab records, not ephemeral UI IDs.
// Unknown versions are discarded by Restore without affecting those records.
type State struct {
	Version int    `json:"version"`
	Root    *Node  `json:"root"`
	Focus   string `json:"focus"`
}

func remap(tab string, refs map[string]string) string {
	if refs == nil {
		return tab
	}
	return refs[tab]
}

func (l Layout) Snapshot(focus string, refs map[string]string) State {
	var clone func(*Node) *Node
	clone = func(n *Node) *Node {
		if n == nil {
			return nil
		}
		if n.Tab != "" {
			return &Node{Tab: remap(n.Tab, refs)}
		}
		return &Node{Axis: n.Axis, Ratio: n.Ratio, First: clone(n.First), Second: clone(n.Second)}
	}
	return State{Version: 1, Root: clone(l.root), Focus: remap(focus, refs)}
}

// Restore validates and copies a serialized tree, pruning missing/duplicate tab
// references and collapsing empty branches. It never changes the caller's tabs.
// Cyclic/aliased node graphs are pruned without imposing a pane/depth cap.
func Restore(state State, refs map[string]string) (layout Layout, focus string, repaired bool) {
	if state.Version != 1 {
		return layout, "", true
	}
	if state.Root == nil {
		// An empty workspace is valid and persists with a nil root.
		return layout, "", false
	}
	seen := make(map[string]bool)
	visited := make(map[*Node]bool)
	var restore func(*Node) *Node
	restore = func(n *Node) *Node {
		if n == nil || visited[n] {
			repaired = true
			return nil
		}
		visited[n] = true
		if n.Tab != "" {
			tab := remap(n.Tab, refs)
			if tab == "" || seen[tab] {
				repaired = true
				return nil
			}
			seen[tab] = true
			if n.Axis != "" || n.Ratio != 0 || n.First != nil || n.Second != nil {
				repaired = true
			}
			return &Node{Tab: tab}
		}
		if n.Axis != Columns && n.Axis != Rows {
			repaired = true
			return nil
		}
		a, b := restore(n.First), restore(n.Second)
		if a == nil {
			return b
		}
		if b == nil {
			return a
		}
		ratio := n.Ratio
		if ratio <= 0 || ratio >= 10000 {
			ratio, repaired = 5000, true
		}
		return &Node{Axis: n.Axis, Ratio: ratio, First: a, Second: b}
	}
	layout.root = restore(state.Root)
	layout.revision = 1
	focus = remap(state.Focus, refs)
	if !layout.Contains(focus) {
		focus, repaired = layout.First(), true
	}
	return layout, focus, repaired
}

func (l Layout) Revision() uint64 { return l.revision }

func (l Layout) Count() int {
	var count func(*Node) int
	count = func(n *Node) int {
		if n == nil {
			return 0
		}
		if n.Tab != "" {
			return 1
		}
		return count(n.First) + count(n.Second)
	}
	return count(l.root)
}

func find(n *Node, tab string) *Node {
	if n == nil || tab == "" {
		return nil
	}
	if n.Tab == tab {
		return n
	}
	if found := find(n.First, tab); found != nil {
		return found
	}
	return find(n.Second, tab)
}

func (l Layout) Contains(tab string) bool { return find(l.root, tab) != nil }

func readingOrder(panes []Pane) {
	slices.SortFunc(panes, func(a, b Pane) int {
		if a.Y != b.Y {
			return cmp.Compare(a.Y, b.Y)
		}
		return cmp.Compare(a.X, b.X)
	})
}

func (l Layout) Cycle(tab string, step, width, height int) string {
	panes := l.Arrange(width, height).Panes
	readingOrder(panes)
	for i, pane := range panes {
		if pane.Tab == tab {
			return panes[((i+step)%len(panes)+len(panes))%len(panes)].Tab
		}
	}
	return ""
}

// Neighbour prefers the closest edge sharing the requested direction, then the
// closest perpendicular centre. Reading order is the deterministic tie-breaker.
func (l Layout) Neighbour(tab string, direction Direction, width, height int) string {
	panes := l.Arrange(width, height).Panes
	readingOrder(panes)
	var current Rect
	for _, pane := range panes {
		if pane.Tab == tab {
			current = pane.Rect
		}
	}
	if current.Width == 0 || current.Height == 0 {
		return ""
	}
	bestGap, bestDistance := math.MaxInt, math.MaxInt
	var target string
	for _, pane := range panes {
		if pane.Tab == tab {
			continue
		}
		var gap, distance, overlap int
		switch direction {
		case Left, Right:
			overlap = min(current.Y+current.Height, pane.Y+pane.Height) - max(current.Y, pane.Y)
			distance = abs(2*current.Y + current.Height - 2*pane.Y - pane.Height)
			if direction == Left {
				gap = current.X - pane.X - pane.Width
			} else {
				gap = pane.X - current.X - current.Width
			}
		case Up, Down:
			overlap = min(current.X+current.Width, pane.X+pane.Width) - max(current.X, pane.X)
			distance = abs(2*current.X + current.Width - 2*pane.X - pane.Width)
			if direction == Up {
				gap = current.Y - pane.Y - pane.Height
			} else {
				gap = pane.Y - current.Y - current.Height
			}
		default:
			return ""
		}
		if gap >= 0 && overlap > 0 && (gap < bestGap || (gap == bestGap && distance < bestDistance)) {
			target, bestGap, bestDistance = pane.Tab, gap, distance
		}
	}
	return target
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (l *Layout) Swap(tab string, direction Direction, width, height int) bool {
	target := l.Neighbour(tab, direction, width, height)
	if target == "" {
		return false
	}
	a, b := find(l.root, tab), find(l.root, target)
	a.Tab, b.Tab = b.Tab, a.Tab
	l.revision++
	return true
}

func cloneNode(n *Node) *Node {
	if n == nil {
		return nil
	}
	if n.Tab != "" {
		return &Node{Tab: n.Tab}
	}
	return &Node{Axis: n.Axis, Ratio: n.Ratio, First: cloneNode(n.First), Second: cloneNode(n.Second)}
}

// detach removes tab from an already cloned tree, collapsing its parent. The
// returned tree keeps every other pane and preferred proportion.
func detach(n *Node, tab string) (*Node, bool) {
	if n == nil || n.Tab == tab {
		return nil, n != nil
	}
	if n.Tab != "" {
		return n, false
	}
	if first, found := detach(n.First, tab); found {
		if first == nil {
			return n.Second, true
		}
		n.First = first
		return n, true
	}
	if second, found := detach(n.Second, tab); found {
		if second == nil {
			return n.First, true
		}
		n.Second = second
		return n, true
	}
	return n, false
}

// Move is i3-like directional movement. A pane swaps positions with its nearest
// neighbour when one exists. At an outer edge it is detached and re-docked as a
// new 50/50 root split on that edge, which can turn a row of panes into a stack
// (or the reverse). The focused tab stays active; only layout geometry changes.
func (l *Layout) Move(tab string, direction Direction, width, height int) bool {
	if direction < Left || direction > Right || l.Count() < 2 || !l.Contains(tab) {
		return false
	}
	if l.Neighbour(tab, direction, width, height) != "" {
		return l.Swap(tab, direction, width, height)
	}

	axis := Columns
	first := direction == Left || direction == Up
	if direction == Up || direction == Down {
		axis = Rows
	}
	// Re-docking a pane that already occupies the requested root edge would
	// only reset its preferred ratio to 50/50.
	if l.root.Axis == axis {
		if first && find(l.root.First, tab) != nil {
			return false
		}
		if !first && find(l.root.Second, tab) != nil {
			return false
		}
	}

	remaining, found := detach(cloneNode(l.root), tab)
	if !found || remaining == nil {
		return false
	}
	moved := &Node{Tab: tab}
	root := &Node{Axis: axis, Ratio: 5000}
	if first {
		root.First, root.Second = moved, remaining
	} else {
		root.First, root.Second = remaining, moved
	}
	minimumWidth, minimumHeight := minimum(root)
	if width < minimumWidth || height < minimumHeight {
		return false
	}
	l.root = root
	l.revision++
	return true
}

func first(n *Node) string {
	if n == nil {
		return ""
	}
	if n.Tab != "" {
		return n.Tab
	}
	return first(n.First)
}

func (l Layout) First() string { return first(l.root) }

// PaneOrder returns assigned tabs in the same tree order as PaneNumber/Arrange.
func (l Layout) PaneOrder() []string {
	out := make([]string, 0, l.Count())
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Tab != "" {
			out = append(out, n.Tab)
			return
		}
		walk(n.First)
		walk(n.Second)
	}
	walk(l.root)
	return out
}

// PaneNumber returns the 1-based tree/reading order index used by both tab
// badges and pane headers, or 0 when the tab is not assigned.
func (l Layout) PaneNumber(tab string) int {
	var walk func(*Node, *int) int
	walk = func(n *Node, count *int) int {
		if n == nil {
			return 0
		}
		if n.Tab != "" {
			*count = *count + 1
			if n.Tab == tab {
				return *count
			}
			return 0
		}
		if found := walk(n.First, count); found != 0 {
			return found
		}
		return walk(n.Second, count)
	}
	count := 0
	return walk(l.root, &count)
}

// Remove collapses the parent split and returns the first tab in its surviving
// sibling. The caller uses that target only when it removed the active pane.
func (l *Layout) Remove(tab string) string {
	if tab == "" {
		return ""
	}
	var remove func(**Node) (string, bool)
	remove = func(slot **Node) (string, bool) {
		n := *slot
		if n == nil {
			return "", false
		}
		if n.Tab == tab {
			*slot = nil
			return "", true
		}
		if n.Tab != "" {
			return "", false
		}
		for _, child := range []**Node{&n.First, &n.Second} {
			if target, found := remove(child); found {
				if n.First == nil {
					*slot = n.Second
					return first(n.Second), true
				}
				if n.Second == nil {
					*slot = n.First
					return first(n.First), true
				}
				return target, true
			}
		}
		return "", false
	}
	target, changed := remove(&l.root)
	if changed {
		l.revision++
	}
	return target
}

// Select assigns an unassigned tab to the target pane, or creates the initial
// pane. An already assigned tab is left alone; its activation is the caller's job.
func (l *Layout) Select(target, tab string) bool {
	if tab == "" || l.Contains(tab) {
		return false
	}
	if l.root == nil {
		l.root = &Node{Tab: tab}
	} else if n := find(l.root, target); n != nil {
		n.Tab = tab
	} else {
		return false
	}
	l.revision++
	return true
}

func minimum(n *Node) (int, int) { return splitMinimum(n, "", "") }

func splitMinimum(n *Node, target string, axis Axis) (int, int) {
	if n == nil {
		return 0, 0
	}
	if n.Tab != "" {
		if n.Tab == target {
			if axis == Columns {
				return 2*MinWidth + 1, MinHeight
			}
			return MinWidth, 2*MinHeight + 1
		}
		return MinWidth, MinHeight
	}
	aw, ah := splitMinimum(n.First, target, axis)
	bw, bh := splitMinimum(n.Second, target, axis)
	if n.Axis == Columns {
		return aw + 1 + bw, max(ah, bh)
	}
	return max(aw, bw), ah + 1 + bh
}

// CanSplit checks a prospective split before asking the user to select a tab.
func (l Layout) CanSplit(target string, axis Axis, width, height int) error {
	if !l.Contains(target) || (axis != Columns && axis != Rows) {
		return errors.New("the split target is no longer available")
	}
	w, h := splitMinimum(l.root, target, axis)
	if width < w || height < h {
		return errors.New("not enough space: panes need at least 24 columns by 8 rows")
	}
	return nil
}

// Split is atomic, including fit validation. It never creates an empty pane.
func (l *Layout) Split(target, tab string, axis Axis, width, height int) error {
	n := find(l.root, target)
	if n == nil || tab == "" || l.Contains(tab) || (axis != Columns && axis != Rows) {
		return errors.New("the split target or selected tab is no longer available")
	}
	if err := l.CanSplit(target, axis, width, height); err != nil {
		return err
	}
	*n = Node{Axis: axis, Ratio: 5000, First: &Node{Tab: target}, Second: &Node{Tab: tab}}
	l.revision++
	return nil
}

// Arrange allocates every cell exactly once, including one-cell dividers.
// Terminal resizing never rewrites preferred proportions.
func (l Layout) Arrange(width, height int) Arrangement {
	out, _ := l.arrange(width, height)
	return out
}

func (l Layout) arrange(width, height int) (Arrangement, map[*Node]Rect) {
	width, height = max(0, width), max(0, height)
	mw, mh := minimum(l.root)
	out := Arrangement{Fits: l.Count() <= 1 || (width >= mw && height >= mh)}
	if !out.Fits {
		width, height = max(width, mw), max(height, mh)
	}
	bounds := make(map[*Node]Rect)
	var place func(*Node, Rect)
	place = func(n *Node, r Rect) {
		if n == nil {
			return
		}
		bounds[n] = r
		if n.Tab != "" {
			out.Panes = append(out.Panes, Pane{Tab: n.Tab, Rect: r})
			return
		}
		aw, ah := minimum(n.First)
		bw, bh := minimum(n.Second)
		if n.Axis == Columns {
			available := r.Width - 1
			first := max(aw, min(available-bw, available*n.Ratio/10000))
			place(n.First, Rect{r.X, r.Y, first, r.Height})
			out.Dividers = append(out.Dividers, Rect{r.X + first, r.Y, 1, r.Height})
			place(n.Second, Rect{r.X + first + 1, r.Y, available - first, r.Height})
		} else {
			available := r.Height - 1
			first := max(ah, min(available-bh, available*n.Ratio/10000))
			place(n.First, Rect{r.X, r.Y, r.Width, first})
			out.Dividers = append(out.Dividers, Rect{r.X, r.Y + first, r.Width, 1})
			place(n.Second, Rect{r.X, r.Y + first + 1, r.Width, available - first})
		}
	}
	place(l.root, Rect{Width: width, Height: height})
	return out, bounds
}

// Resize adjusts the deepest split on the requested axis. Positive steps grow
// the focused region, even when it is the second child of that split.
func (l *Layout) Resize(tab string, axis Axis, steps, width, height int) bool {
	arrangement, bounds := l.arrange(width, height)
	if !arrangement.Fits || steps == 0 {
		return false
	}
	var path []*Node
	var descend func(*Node) bool
	descend = func(n *Node) bool {
		if n == nil {
			return false
		}
		if n.Tab == tab {
			return true
		}
		if descend(n.First) || descend(n.Second) {
			path = append(path, n)
			return true
		}
		return false
	}
	if !descend(l.root) {
		return false
	}
	for _, n := range path {
		if n.Axis != axis {
			continue
		}
		if find(n.Second, tab) != nil {
			steps = -steps
		}
		aw, ah := minimum(n.First)
		bw, bh := minimum(n.Second)
		total, low, high := bounds[n].Width-1, aw, bounds[n].Width-1-bw
		if axis == Rows {
			total, low, high = bounds[n].Height-1, ah, bounds[n].Height-1-bh
		}
		minimumRatio := (low*10000 + total - 1) / total
		maximumRatio := (high*10000 + total - 1) / total
		// Terminal-size clamping preserves the preferred ratio. An explicit
		// resize, however, must start at the visible divider; otherwise a pane
		// pinned at its minimum could be impossible to grow after shrinking.
		effective := max(minimumRatio, min(maximumRatio, n.Ratio))
		next := max(minimumRatio, min(maximumRatio, effective+steps*500))
		oldCells := max(low, min(high, total*n.Ratio/10000))
		if oldCells == max(low, min(high, total*next/10000)) {
			return false
		}
		n.Ratio = next
		l.revision++
		return true
	}
	return false
}
