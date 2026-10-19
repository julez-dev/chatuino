package nav_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeScreen struct {
	name  string
	route string
	// overlay screens keep the view below them visible
	overlay bool

	appeared    int
	disappeared int
	inits       int
	updates     []tea.Msg
	view        string
	next        nav.Screen // when set, Update swaps this screen in
}

func (f *fakeScreen) Init() tea.Cmd {
	f.inits++
	return nil
}

func (f *fakeScreen) Update(msg tea.Msg) (nav.Screen, tea.Cmd) {
	f.updates = append(f.updates, msg)

	if f.next != nil {
		return f.next, nil
	}

	return f, nil
}

func (f *fakeScreen) View() string { return f.view }

func (f *fakeScreen) Appeared() tea.Cmd {
	f.appeared++
	return nil
}

func (f *fakeScreen) Disappeared() { f.disappeared++ }

func (f *fakeScreen) Route() string { return f.route }

func (f *fakeScreen) OverlaysBackground() bool { return f.overlay }

func (f *fakeScreen) Resize(int, int) {}

func newStack() (*nav.Stack, *int) {
	var baseCalls int

	return nav.NewStack(func() string {
		baseCalls++
		return "base"
	}, func(fg, bg string) string {
		return fg + "|" + bg
	}), &baseCalls
}

func TestStack_EmptyShowsBase(t *testing.T) {
	s, _ := newStack()

	assert.Equal(t, 0, s.Len())
	assert.Equal(t, "", s.Route())
	assert.Equal(t, "base", s.View())

	_, ok := s.Top()
	assert.False(t, ok)
}

func TestStack_PushAppearsAndRenders(t *testing.T) {
	s, _ := newStack()
	join := &fakeScreen{name: "join", route: "join", overlay: true, view: "JOIN"}

	s.Push(join)

	assert.Equal(t, 1, join.appeared)
	assert.Equal(t, 0, join.disappeared, "lifecycle is preferred over Init")
	assert.Equal(t, 0, join.inits)
	assert.Equal(t, "join", s.Route())
	assert.Equal(t, "JOIN|base", s.View(), "overlays are composited over what is below")
}

// plainScreen deliberately does not implement Lifecycle.
type plainScreen struct {
	inits int
	view  string
}

func (p *plainScreen) Init() tea.Cmd { p.inits++; return nil }

func (p *plainScreen) Update(tea.Msg) (nav.Screen, tea.Cmd) { return p, nil }

func (p *plainScreen) View() string { return p.view }

func TestStack_PushWithoutLifecycleCallsInit(t *testing.T) {
	s, _ := newStack()
	help := &plainScreen{view: "HELP"}

	s.Push(help)

	assert.Equal(t, 1, help.inits)
	assert.Equal(t, "HELP", s.View(), "non-overlays replace the view below")
}

func TestStack_OnlyTopReceivesMessages(t *testing.T) {
	s, _ := newStack()
	below := &fakeScreen{route: "below", overlay: true, view: "BELOW"}
	top := &fakeScreen{route: "top", overlay: true, view: "TOP"}

	s.Push(below)
	s.Push(top)

	s.Update("msg")

	assert.Empty(t, below.updates, "a screen that is not on top must not receive input")
	assert.Equal(t, []tea.Msg{"msg"}, top.updates)

	assert.Equal(t, 1, below.disappeared, "pushing over a screen makes it disappear")
	assert.Equal(t, 1, top.appeared)
}

func TestStack_PopAppearsTheScreenBelow(t *testing.T) {
	s, _ := newStack()
	below := &fakeScreen{route: "below", overlay: true, view: "BELOW"}
	top := &fakeScreen{route: "top", overlay: true, view: "TOP"}

	s.Push(below)
	s.Push(top)
	s.Pop()

	assert.Equal(t, 1, top.disappeared)
	assert.Equal(t, 2, below.appeared, "appearing once on push and once when it returns to the top")
	assert.Equal(t, 1, s.Len())
	assert.Equal(t, "BELOW|base", s.View(), "the returning screen is still an overlay")
}

func TestStack_PopEmptyIsNoop(t *testing.T) {
	s, _ := newStack()

	assert.Nil(t, s.Pop())
	assert.Equal(t, "base", s.View())
}

func TestStack_UpdateWritesBackReplacement(t *testing.T) {
	s, _ := newStack()
	first := &fakeScreen{route: "first", view: "FIRST"}
	replacement := &fakeScreen{route: "replacement", view: "REPLACEMENT"}
	first.next = replacement

	s.Push(first)
	s.Update("msg")

	active, ok := s.Top()
	require.True(t, ok)
	assert.Same(t, replacement, active)
	assert.Equal(t, "REPLACEMENT", s.View())
}

func TestStack_Replace(t *testing.T) {
	s, _ := newStack()
	join := &fakeScreen{route: "join", overlay: true, view: "JOIN"}
	help := &fakeScreen{route: "help", view: "HELP"}

	s.Push(join)
	s.Replace(help)

	assert.Equal(t, 1, join.disappeared)
	assert.Equal(t, 1, help.appeared)
	assert.Equal(t, 1, s.Len())
	assert.Equal(t, "help", s.Route())
	assert.Equal(t, "HELP", s.View())
}

func TestStack_ResizeForwardsToScreens(t *testing.T) {
	s, _ := newStack()

	var got [2]int

	screen := &fakeScreen{route: "x", view: "x", overlay: true}
	screenResize := resizeFunc(func(w, h int) { got = [2]int{w, h} })

	s.Push(screen)
	s.Push(resizableScreen{fakeScreen: screen, resize: screenResize})
	s.Resize(80, 24)

	assert.Equal(t, [2]int{80, 24}, got)
}

type resizeFunc func(w, h int)

func (r resizeFunc) Resize(w, h int) { r(w, h) }

type resizableScreen struct {
	*fakeScreen
	resize resizeFunc
}

func (r resizableScreen) Resize(w, h int) { r.resize(w, h) }
