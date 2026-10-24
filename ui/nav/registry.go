// Package nav holds routing and identity primitives for the UI.
//
// A [Registry] owns a set of addressable items (tabs, panes) together with which
// one is active, so no other component has to keep an index, a copy of the item
// list, or an "is selected" flag of its own.
package nav

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// Item is anything the registry can activate. Implementations decide what Focus
// and Blur mean: moving keyboard focus into a child, picking up a border style,
// or nothing at all.
type Item interface {
	ID() string
	Focus()
	Blur()
}

// Row is the rendering projection of an item. Headers and sidebars render rows
// and never look at the items themselves.
type Row struct {
	ID              string
	Title           string
	Identity        string
	Active          bool
	HasNotification bool
	// Workspace is the short workspace label (for example W1) owning this tab,
	// empty for a standalone tab. WorkspacePane is the 1-based pane within that
	// workspace. Headers render both as a membership badge.
	Workspace     string
	WorkspacePane int
}

// Registry is the single source of truth for the set of open items and which of
// them is active. Focus changes only ever happen here, so the invariant "exactly
// one item is focused" holds by construction rather than by convention.
type Registry[T Item] struct {
	project func(T) Row

	items  []T
	active string
	notify map[string]struct{}

	// rows caches the projection. It is rebuilt only when membership, the active
	// item or a notification flag changes, so rendering stays allocation free.
	rows  []Row
	dirty bool
}

// NewRegistry returns an empty registry. project turns an item into its
// rendering row and must be safe to call with any item in the registry.
func NewRegistry[T Item](project func(T) Row) *Registry[T] {
	return &Registry[T]{
		project: project,
		notify:  make(map[string]struct{}),
		dirty:   true,
	}
}

// Len reports the number of items.
func (r *Registry[T]) Len() int {
	return len(r.items)
}

// All returns the items in display order. The slice must not be mutated.
func (r *Registry[T]) All() []T {
	return r.items
}

// Active returns the active item. ok is false when the registry is empty.
func (r *Registry[T]) Active() (T, bool) {
	i := r.index(r.active)
	if i < 0 {
		var zero T
		return zero, false
	}

	return r.items[i], true
}

// Any reports whether pred matches at least one item.
func (r *Registry[T]) Any(pred func(T) bool) bool {
	return slices.ContainsFunc(r.items, pred)
}

// Add appends item and makes it active when the registry was empty. Adding does
// not steal the active slot, so callers that open a tab explicitly decide.
func (r *Registry[T]) Add(item T) {
	r.items = append(r.items, item)
	r.dirty = true

	if r.active == "" {
		r.SetActive(item.ID())
	}
}

// Remove drops the item with the given id and blurs it. When it was the active
// item, the neighbour that takes its place becomes active (wrapping to the first
// item when the last one was closed).
func (r *Registry[T]) Remove(id string) (T, bool) {
	i := r.index(id)
	if i < 0 {
		var zero T
		return zero, false
	}

	removed := r.items[i]
	wasActive := r.active == id

	removed.Blur()
	r.items = slices.Delete(r.items, i, i+1)
	delete(r.notify, id)
	r.dirty = true

	if !wasActive {
		return removed, true
	}

	r.active = ""

	if len(r.items) == 0 {
		return removed, true
	}

	target := i
	if target >= len(r.items) {
		target = 0
	}

	r.SetActive(r.items[target].ID())

	return removed, true
}

// SetActive blurs the previous active item, focuses id and clears its
// notification flag. It reports whether the active item changed.
func (r *Registry[T]) SetActive(id string) bool {
	if id == r.active {
		return false
	}

	// Resolve the target before blurring anything: an unknown id must leave the
	// current focus exactly as it was.
	target := r.index(id)
	if target < 0 {
		return false
	}

	if i := r.index(r.active); i >= 0 {
		r.items[i].Blur()
	}

	r.active = id
	delete(r.notify, id)
	r.dirty = true
	r.items[target].Focus()

	return true
}

// ClearActive blurs the active item and leaves the registry with no active
// item. It is used when switching to a workspace whose visible tabs are empty.
func (r *Registry[T]) ClearActive() {
	if i := r.index(r.active); i >= 0 {
		r.items[i].Blur()
	}
	r.active = ""
	r.dirty = true
}

// Next activates the item after the active one, wrapping around.
func (r *Registry[T]) Next() {
	r.step(1)
}

// Prev activates the item before the active one, wrapping around.
func (r *Registry[T]) Prev() {
	r.step(-1)
}

// FocusActive focuses the active item without changing which one is active. It
// is the counterpart of BlurActive, used when a modal takes over input and later
// hands it back to the very same item.
func (r *Registry[T]) FocusActive() {
	if i := r.index(r.active); i >= 0 {
		r.items[i].Focus()
	}
}

// BlurActive blurs the active item without forgetting which one it is.
func (r *Registry[T]) BlurActive() {
	if i := r.index(r.active); i >= 0 {
		r.items[i].Blur()
	}
}

// Notify flags the item for attention. Notifying the active item is a no-op:
// the user is already looking at it.
func (r *Registry[T]) Notify(id string) {
	if id == r.active {
		return
	}

	if r.index(id) < 0 {
		return
	}

	r.notify[id] = struct{}{}
	r.dirty = true
}

// InvalidateRows drops the cached projection after state outside the registry
// (such as workspace pane membership) changes what a row should render.
func (r *Registry[T]) InvalidateRows() {
	r.dirty = true
}

// Rows returns the cached projection of all items. The slice must not be mutated.
func (r *Registry[T]) Rows() []Row {
	if !r.dirty && r.rows != nil {
		return r.rows
	}

	rows := make([]Row, len(r.items))

	for i, item := range r.items {
		id := item.ID()
		row := r.project(item)
		row.ID = id
		row.Active = id == r.active
		_, row.HasNotification = r.notify[id]
		rows[i] = row
	}

	r.rows = rows
	r.dirty = false

	return rows
}

// UpdateAll hands msg to every item and writes the updated items back.
func (r *Registry[T]) UpdateAll(msg tea.Msg, update func(T, tea.Msg) (T, tea.Cmd)) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(r.items))

	for i, item := range r.items {
		var cmd tea.Cmd
		r.items[i], cmd = update(item, msg)
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// UpdateActive hands msg to the active item only and writes it back. ok is false
// when the registry is empty.
func (r *Registry[T]) UpdateActive(msg tea.Msg, update func(T, tea.Msg) (T, tea.Cmd)) (tea.Cmd, bool) {
	i := r.index(r.active)
	if i < 0 {
		return nil, false
	}

	var cmd tea.Cmd
	r.items[i], cmd = update(r.items[i], msg)

	return cmd, true
}

// UpdateOne hands msg to the item with the given id only and writes it back.
func (r *Registry[T]) UpdateOne(id string, msg tea.Msg, update func(T, tea.Msg) (T, tea.Cmd)) (tea.Cmd, bool) {
	i := r.index(id)
	if i < 0 {
		return nil, false
	}

	var cmd tea.Cmd
	r.items[i], cmd = update(r.items[i], msg)

	return cmd, true
}

func (r *Registry[T]) step(delta int) {
	n := len(r.items)
	if n == 0 {
		return
	}

	i := r.index(r.active)
	if i < 0 {
		r.SetActive(r.items[0].ID())
		return
	}

	r.SetActive(r.items[((i+delta)%n+n)%n].ID())
}

func (r *Registry[T]) index(id string) int {
	if id == "" {
		return -1
	}

	for i, item := range r.items {
		if item.ID() == id {
			return i
		}
	}

	return -1
}
