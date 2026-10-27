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

	// weightTotal is the resolution of a container's child shares. Weights are
	// relative, so only their ratio matters; a container of equal siblings uses
	// equalWeights.
	weightTotal = 10000

	// currentVersion is the layout format Snapshot writes. Version 1 stored
	// binary splits (ratio/first/second); Restore still migrates those.
	currentVersion = 2
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

// Node is a serialized tree node: either a leaf (Tab) or a container (Axis with
// Children, one weight per child). Like i3, a container is a list of siblings, so
// adding a pane next to an existing one never deepens the tree.
//
// Ratio, First and Second are the version 1 binary-split fields. They are only
// read to migrate old sessions and are never written.
type Node struct {
	Tab string `json:"tab,omitempty"`

	Axis     Axis    `json:"axis,omitempty"`
	Children []*Node `json:"children,omitempty"`
	Weights  []int   `json:"weights,omitempty"`

	// Deprecated: version 1 layout fields, decode-only.
	Ratio  int   `json:"ratio,omitempty"`
	First  *Node `json:"first,omitempty"`
	Second *Node `json:"second,omitempty"`
}

func (n *Node) leaf() bool { return n != nil && n.Tab != "" }

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

// equalWeights returns shares that split a container as evenly as integer math
// allows, which is how i3 treats the children of one container.
func equalWeights(n int) []int {
	if n <= 0 {
		return nil
	}
	weights := make([]int, n)
	base, remainder := weightTotal/n, weightTotal%n
	for i := range weights {
		weights[i] = base
	}
	for i := 0; i < remainder; i++ {
		weights[i]++
	}
	return weights
}

// normalizeWeights scales arbitrary positive weights to weightTotal, keeping
// their ratio. Rounding leftovers go to the largest shares.
func normalizeWeights(weights []int) []int {
	n := len(weights)
	if n == 0 {
		return nil
	}
	total := 0
	for _, weight := range weights {
		total += max(1, weight)
	}
	out := make([]int, n)
	remainder := make([]int, n)
	assigned := 0
	for i, weight := range weights {
		weight = max(1, weight)
		out[i] = max(1, weight*weightTotal/total)
		remainder[i] = weight * weightTotal % total
		assigned += out[i]
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(remainder[b], remainder[a]) })
	for k := 0; assigned < weightTotal; k++ {
		out[order[k%n]]++
		assigned++
	}
	for k := 0; assigned > weightTotal && k < n; k++ {
		index := order[n-1-k]
		if out[index] > 1 {
			out[index]--
			assigned--
		}
	}
	return out
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
		out := &Node{Axis: n.Axis, Children: make([]*Node, len(n.Children)), Weights: slices.Clone(n.Weights)}
		for i, child := range n.Children {
			out.Children[i] = clone(child)
		}
		return out
	}
	return State{Version: currentVersion, Root: clone(l.root), Focus: remap(focus, refs)}
}

// Restore validates and copies a serialized tree, pruning missing/duplicate tab
// references and collapsing empty branches. It never changes the caller's tabs.
// Cyclic/aliased node graphs are pruned without imposing a pane/depth cap.
// Version 1 binary splits are migrated to sibling containers with the same ratio.
func Restore(state State, refs map[string]string) (layout Layout, focus string, repaired bool) {
	if state.Version != 1 && state.Version != currentVersion {
		return layout, "", true
	}
	if state.Root == nil {
		// An empty workspace is valid and persists with a nil root.
		return layout, "", false
	}
	seen := make(map[string]bool)
	visited := make(map[*Node]bool)

	restoreLeaf := func(n *Node) *Node {
		tab := remap(n.Tab, refs)
		if tab == "" || seen[tab] {
			repaired = true
			return nil
		}
		seen[tab] = true
		return &Node{Tab: tab}
	}

	var restoreV1 func(*Node) *Node
	restoreV1 = func(n *Node) *Node {
		if n == nil || visited[n] {
			repaired = true
			return nil
		}
		visited[n] = true
		if n.Tab != "" {
			if n.Axis != "" || n.Ratio != 0 || n.First != nil || n.Second != nil {
				repaired = true
			}
			return restoreLeaf(n)
		}
		if n.Axis != Columns && n.Axis != Rows {
			repaired = true
			return nil
		}
		a, b := restoreV1(n.First), restoreV1(n.Second)
		switch {
		case a == nil:
			return b
		case b == nil:
			return a
		}
		ratio := n.Ratio
		if ratio <= 0 || ratio >= weightTotal {
			ratio, repaired = 5000, true
		}
		return &Node{Axis: n.Axis, Children: []*Node{a, b}, Weights: []int{ratio, weightTotal - ratio}}
	}

	var restoreV2 func(*Node) *Node
	restoreV2 = func(n *Node) *Node {
		if n == nil || visited[n] {
			repaired = true
			return nil
		}
		visited[n] = true
		if n.Tab != "" {
			if n.Axis != "" || len(n.Children) != 0 {
				repaired = true
			}
			if n.Ratio != 0 || n.First != nil || n.Second != nil {
				repaired = true
			}
			return restoreLeaf(n)
		}
		if n.Axis != Columns && n.Axis != Rows || len(n.Children) == 0 {
			repaired = true
			return nil
		}
		if n.Ratio != 0 || n.First != nil || n.Second != nil {
			repaired = true
		}
		weightsMatch := len(n.Weights) == len(n.Children)
		if !weightsMatch {
			repaired = true
		}
		children := make([]*Node, 0, len(n.Children))
		weights := make([]int, 0, len(n.Children))
		for i, child := range n.Children {
			restored := restoreV2(child)
			if restored == nil {
				continue
			}
			children = append(children, restored)
			weight := 1
			if weightsMatch {
				weight = n.Weights[i]
				if weight <= 0 {
					weight, repaired = 1, true
				}
			}
			weights = append(weights, weight)
		}
		switch len(children) {
		case 0:
			return nil
		case 1:
			repaired = true
			return children[0]
		}
		normalized := normalizeWeights(weights)
		if !slices.Equal(normalized, weights) {
			repaired = true
		}
		return &Node{Axis: n.Axis, Children: children, Weights: normalized}
	}

	if state.Version == 1 {
		layout.root = restoreV1(state.Root)
	} else {
		layout.root = restoreV2(state.Root)
	}
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
		total := 0
		for _, child := range n.Children {
			total += count(child)
		}
		return total
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
	for _, child := range n.Children {
		if found := find(child, tab); found != nil {
			return found
		}
	}
	return nil
}

func (l Layout) Contains(tab string) bool { return find(l.root, tab) != nil }

// parentOf returns the container directly holding tab, and its child index. A
// tab that is the whole layout (a root leaf) has no parent.
func parentOf(n *Node, tab string) (*Node, int) {
	if n == nil || n.Tab != "" {
		return nil, -1
	}
	for i, child := range n.Children {
		if child.Tab == tab {
			return n, i
		}
		if parent, index := parentOf(child, tab); parent != nil {
			return parent, index
		}
	}
	return nil, -1
}

// parentOfNode returns the container directly holding target, and its index.
func parentOfNode(n, target *Node) (*Node, int) {
	if n == nil || n.Tab != "" {
		return nil, -1
	}
	for i, child := range n.Children {
		if child == target {
			return n, i
		}
		if parent, index := parentOfNode(child, target); parent != nil {
			return parent, index
		}
	}
	return nil, -1
}

// SplitAxis reports the axis a new pane inherits from the layout: the focused
// pane's container orientation, or Columns for a pane that is alone.
func (l Layout) SplitAxis(tab string) Axis {
	parent, _ := parentOf(l.root, tab)
	if parent == nil {
		return Columns
	}
	return parent.Axis
}

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
	out := &Node{Axis: n.Axis, Children: make([]*Node, len(n.Children)), Weights: slices.Clone(n.Weights)}
	for i, child := range n.Children {
		out.Children[i] = cloneNode(child)
	}
	return out
}

// Move is i3-like directional movement. A pane first swaps contents with its
// nearest neighbour. At an outer edge it moves one level up the tree instead of
// re-docking at the screen edge: it leaves its container and becomes a sibling of
// that container in the container above, on the requested side.
func (l *Layout) Move(tab string, direction Direction, width, height int) bool {
	if direction < Left || direction > Right || l.Count() < 2 || !l.Contains(tab) {
		return false
	}
	if l.Neighbour(tab, direction, width, height) != "" {
		return l.Swap(tab, direction, width, height)
	}

	axis := Columns
	if direction == Up || direction == Down {
		axis = Rows
	}
	parent, _ := parentOf(l.root, tab)
	if parent == nil || parent.Axis != axis {
		// The pane is not at an edge of a container that splits along the
		// requested direction, or it is the whole layout.
		return false
	}
	if parent == l.root {
		return false
	}

	root := cloneNode(l.root)
	container, index := parentOf(root, tab)
	leaf := container.Children[index]
	container.Children = slices.Delete(container.Children, index, index+1)
	container.Weights = slices.Delete(container.Weights, index, index+1)
	if len(container.Children) == 1 {
		// The container had just this pane and one sibling; the sibling takes
		// its place and keeps its own subtree.
		only := container.Children[0]
		*container = *only
	}

	grandparent, grandIndex := parentOfNode(root, container)
	if grandparent == nil {
		return false
	}
	insertAt := grandIndex
	if direction == Right || direction == Down {
		insertAt++
	}
	grandparent.Children = slices.Insert(grandparent.Children, insertAt, leaf)
	grandparent.Weights = equalWeights(len(grandparent.Children))

	if minimumWidth, minimumHeight := minimum(root); width < minimumWidth || height < minimumHeight {
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
	for _, child := range n.Children {
		if found := first(child); found != "" {
			return found
		}
	}
	return ""
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
		for _, child := range n.Children {
			walk(child)
		}
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
		for _, child := range n.Children {
			if found := walk(child, count); found != 0 {
				return found
			}
		}
		return 0
	}
	count := 0
	return walk(l.root, &count)
}

// Remove drops the pane and collapses a container that is left with one child.
// It returns the tab that should take focus, which the caller uses only when it
// removed the focused pane.
func (l *Layout) Remove(tab string) string {
	if tab == "" {
		return ""
	}
	var remove func(*Node) (*Node, string, bool)
	remove = func(n *Node) (*Node, string, bool) {
		if n == nil {
			return nil, "", false
		}
		if n.Tab == tab {
			return nil, "", true
		}
		if n.Tab != "" {
			return n, "", false
		}
		for i, child := range n.Children {
			restored, focus, found := remove(child)
			if !found {
				continue
			}
			if restored == nil {
				n.Children = slices.Delete(n.Children, i, i+1)
				n.Weights = slices.Delete(n.Weights, i, i+1)
			} else {
				n.Children[i] = restored
			}
			switch len(n.Children) {
			case 0:
				return nil, "", true
			case 1:
				only := n.Children[0]
				*n = *only
				if focus == "" {
					focus = first(n)
				}
				return n, focus, true
			}
			n.Weights = normalizeWeights(n.Weights)
			if focus == "" {
				focus = first(n.Children[min(i, len(n.Children)-1)])
			}
			return n, focus, true
		}
		return n, "", false
	}
	root, focus, removed := remove(l.root)
	if removed {
		l.root = root
		l.revision++
	}
	return focus
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

func minimum(n *Node) (int, int) {
	if n == nil {
		return 0, 0
	}
	if n.Tab != "" {
		return MinWidth, MinHeight
	}
	width, height := 0, 0
	for i, child := range n.Children {
		childWidth, childHeight := minimum(child)
		if n.Axis == Columns {
			width += childWidth
			height = max(height, childHeight)
		} else {
			width = max(width, childWidth)
			height += childHeight
		}
		if i > 0 {
			if n.Axis == Columns {
				width++
			} else {
				height++
			}
		}
	}
	return width, height
}

// splitPlaceholder lets CanSplit measure a prospective insertion without a tab.
const splitPlaceholder = "\x00split"

// insertSplit adds tab next to target. When target's container already splits
// along axis the new pane becomes a sibling there and the container's shares are
// rebalanced equally, so all its panes grow into a row/stack of equal parts.
// Otherwise target is wrapped in a new two-pane container. Shares of unrelated
// containers are untouched.
func insertSplit(n *Node, target string, axis Axis, tab string) bool {
	if n == nil || n.Tab != "" {
		return false
	}
	for i, child := range n.Children {
		if child.Tab == target {
			if n.Axis == axis {
				n.Children = slices.Insert(n.Children, i+1, &Node{Tab: tab})
				n.Weights = equalWeights(len(n.Children))
			} else {
				n.Children[i] = &Node{
					Axis:     axis,
					Children: []*Node{{Tab: target}, {Tab: tab}},
					Weights:  equalWeights(2),
				}
			}
			return true
		}
		if insertSplit(child, target, axis, tab) {
			return true
		}
	}
	return false
}

func prospectiveSplit(root *Node, target string, axis Axis, tab string) (*Node, bool) {
	if root == nil || root.Tab != "" {
		if root == nil || root.Tab != target {
			return root, false
		}
		return &Node{
			Axis:     axis,
			Children: []*Node{{Tab: target}, {Tab: tab}},
			Weights:  equalWeights(2),
		}, true
	}
	if !insertSplit(root, target, axis, tab) {
		return root, false
	}
	return root, true
}

// CanSplit checks a prospective split before asking the user to select a tab.
func (l Layout) CanSplit(target string, axis Axis, width, height int) error {
	if !l.Contains(target) || (axis != Columns && axis != Rows) {
		return errors.New("the split target is no longer available")
	}
	root, ok := prospectiveSplit(cloneNode(l.root), target, axis, splitPlaceholder)
	if !ok {
		return errors.New("the split target is no longer available")
	}
	if w, h := minimum(root); width < w || height < h {
		return errors.New("not enough space: panes need at least 24 columns by 8 rows")
	}
	return nil
}

// Split is atomic, including fit validation. It never creates an empty pane.
// A pane whose container already splits along axis joins it as an equal sibling
// (i3's container list); otherwise the target is wrapped in a new container.
func (l *Layout) Split(target, tab string, axis Axis, width, height int) error {
	if !l.Contains(target) || tab == "" || l.Contains(tab) || (axis != Columns && axis != Rows) {
		return errors.New("the split target or selected tab is no longer available")
	}
	root, ok := prospectiveSplit(cloneNode(l.root), target, axis, tab)
	if !ok {
		return errors.New("the split target or selected tab is no longer available")
	}
	if w, h := minimum(root); width < w || height < h {
		return errors.New("not enough space: panes need at least 24 columns by 8 rows")
	}
	l.root = root
	l.revision++
	return nil
}

// allocate distributes avail cells among children by weight. Children never drop
// below their minimum; the minimum takes the cells from the largest sibling.
// Proportional floor allocation keeps equal siblings within one cell of each
// other and always hands out exactly avail cells.
func allocate(avail int, weights, mins []int) []int {
	n := len(weights)
	sizes := make([]int, n)
	total := 0
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 {
		total = 1
	}
	cumulative, previous := 0, 0
	for i, weight := range weights {
		cumulative += weight
		end := cumulative * max(0, avail) / total
		sizes[i] = end - previous
		previous = end
	}
	for i := range sizes {
		for sizes[i] < mins[i] {
			donor := largestAbove(sizes, mins)
			if donor < 0 {
				break
			}
			sizes[donor]--
			sizes[i]++
		}
	}
	return sizes
}

func largestAbove(sizes, mins []int) int {
	best := -1
	for i := range sizes {
		if sizes[i] > mins[i] && (best < 0 || sizes[i] > sizes[best]) {
			best = i
		}
	}
	return best
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
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
		count := len(n.Children)
		weights := n.Weights
		if len(weights) != count {
			weights = equalWeights(count)
		}
		mins := make([]int, count)
		for i, child := range n.Children {
			childWidth, childHeight := minimum(child)
			if n.Axis == Columns {
				mins[i] = childWidth
			} else {
				mins[i] = childHeight
			}
		}
		available := max(0, r.Width-(count-1))
		if n.Axis == Rows {
			available = max(0, r.Height-(count-1))
		}
		sizes := allocate(available, weights, mins)
		offset := 0
		for i, child := range n.Children {
			if n.Axis == Columns {
				place(child, Rect{r.X + offset, r.Y, sizes[i], r.Height})
			} else {
				place(child, Rect{r.X, r.Y + offset, r.Width, sizes[i]})
			}
			offset += sizes[i]
			if i < count-1 {
				if n.Axis == Columns {
					out.Dividers = append(out.Dividers, Rect{r.X + offset, r.Y, 1, r.Height})
				} else {
					out.Dividers = append(out.Dividers, Rect{r.X, r.Y + offset, r.Width, 1})
				}
				offset++
			}
		}
	}
	place(l.root, Rect{Width: width, Height: height})
	return out, bounds
}

// Resize moves the divider next to the focused pane by five-percent steps.
// Positive steps grow the focused child, even when it is not the first child of
// the container. It edits the deepest matching container on the pane's path.
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
		for _, child := range n.Children {
			if descend(child) {
				path = append(path, n)
				return true
			}
		}
		return false
	}
	if !descend(l.root) {
		return false
	}
	for _, n := range path {
		if n.Axis != axis || len(n.Children) < 2 {
			continue
		}
		index := slices.IndexFunc(n.Children, func(child *Node) bool { return find(child, tab) != nil })
		if index < 0 {
			continue
		}
		// The divider that moves is the one next to the focused child: between
		// it and the following sibling, or the preceding one when it is last.
		first := min(index, len(n.Children)-2)
		second := first + 1
		direction := steps
		if index != first {
			direction = -direction
		}
		available := bounds[n].Width - (len(n.Children) - 1)
		if axis == Rows {
			available = bounds[n].Height - (len(n.Children) - 1)
		}
		if available <= 0 {
			continue
		}
		mins := make([]int, len(n.Children))
		for i, child := range n.Children {
			childWidth, childHeight := minimum(child)
			if axis == Rows {
				mins[i] = childHeight
			} else {
				mins[i] = childWidth
			}
		}
		weights := n.Weights
		if len(weights) != len(n.Children) {
			weights = equalWeights(len(n.Children))
		}
		sizes := allocate(available, weights, mins)
		low := mins[first]
		high := sizes[first] + sizes[second] - mins[second]
		if high <= low {
			return false
		}
		lowRatio := ceilDiv(low*weightTotal, available)
		highRatio := ceilDiv(high*weightTotal, available)
		effective := max(lowRatio, min(highRatio, weights[first]))
		next := max(lowRatio, min(highRatio, effective+direction*500))
		// Compare visible cells, not ratios: a pane pinned at its minimum must
		// stay growable after the terminal shrinks.
		oldCells := max(low, min(high, available*weights[first]/weightTotal))
		newCells := max(low, min(high, available*next/weightTotal))
		if oldCells == newCells {
			return false
		}
		sum := weights[first] + weights[second]
		if next < 1 || sum-next < 1 {
			return false
		}
		if len(n.Weights) != len(n.Children) {
			n.Weights = slices.Clone(weights)
		}
		n.Weights[first] = next
		n.Weights[second] = sum - next
		l.revision++
		return true
	}
	return false
}
