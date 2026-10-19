package nav_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeItem struct {
	id      string
	title   string
	focused bool
	focuses int
	blurs   int
	value   string
}

func (f *fakeItem) ID() string { return f.id }

func (f *fakeItem) Focus() {
	f.focused = true
	f.focuses++
}

func (f *fakeItem) Blur() {
	f.focused = false
	f.blurs++
}

func (f fakeItem) row() nav.Row {
	return nav.Row{Title: f.title, Identity: "all"}
}

func newRegistry(items ...*fakeItem) (*nav.Registry[*fakeItem], []*fakeItem) {
	reg := nav.NewRegistry(func(f *fakeItem) nav.Row { return f.row() })

	for _, item := range items {
		reg.Add(item)
	}

	return reg, items
}

// focusedCount is the invariant the whole registry exists to guarantee.
func focusedCount(items []*fakeItem) int {
	var n int
	for _, item := range items {
		if item.focused {
			n++
		}
	}

	return n
}

func TestRegistry_AddActivatesFirstItemOnly(t *testing.T) {
	reg, _ := newRegistry()

	assert.Equal(t, 0, reg.Len())

	a := &fakeItem{id: "a", title: "chan-a"}
	b := &fakeItem{id: "b", title: "chan-b"}

	reg.Add(a)
	assert.True(t, a.focused, "first item becomes active")
	assert.Equal(t, 1, focusedCount(reg.All()))

	reg.Add(b)
	assert.True(t, a.focused, "adding does not steal the active slot")
	assert.False(t, b.focused)
	assert.Equal(t, 1, focusedCount(reg.All()))

	active, ok := reg.Active()
	require.True(t, ok)
	assert.Equal(t, "a", active.ID())
}

func TestRegistry_SetActiveMovesFocusAndClearsNotification(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})
	a, b := items[0], items[1]

	reg.Notify("b")
	require.True(t, reg.Rows()[1].HasNotification)

	assert.True(t, reg.SetActive("b"))
	assert.False(t, a.focused)
	assert.True(t, b.focused)
	assert.Equal(t, 1, focusedCount(items))
	assert.False(t, reg.Rows()[1].HasNotification, "activating clears the notification")
}

func TestRegistry_SetActiveSameItemIsNoop(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})
	a := items[0]

	focusesBefore := a.focuses

	assert.False(t, reg.SetActive("a"))
	assert.Equal(t, focusesBefore, a.focuses)
}

func TestRegistry_SetActiveUnknownLeavesStateUntouched(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"})
	a := items[0]

	assert.False(t, reg.SetActive("nope"))
	assert.True(t, a.focused, "blurring the old item before rejecting the new one would lose focus")

	active, ok := reg.Active()
	require.True(t, ok)
	assert.Equal(t, "a", active.ID())
}

func TestRegistry_RemoveInactiveKeepsActive(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"}, &fakeItem{id: "c"})
	b := items[1]

	reg.SetActive("b")
	removed, ok := reg.Remove("a")

	require.True(t, ok)
	assert.Equal(t, "a", removed.ID())
	assert.True(t, b.focused)
	assert.Equal(t, 1, focusedCount(reg.All()))
	assert.Equal(t, 2, reg.Len())
}

func TestRegistry_RemoveActiveActivatesNeighbour(t *testing.T) {
	t.Run("middle", func(t *testing.T) {
		reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"}, &fakeItem{id: "c"})
		a, b, c := items[0], items[1], items[2]

		reg.SetActive("b")
		removed, ok := reg.Remove("b")

		require.True(t, ok)
		assert.Equal(t, "b", removed.ID())
		assert.True(t, c.focused, "the tab that took the removed slot becomes active")
		assert.False(t, a.focused)
		assert.False(t, b.focused, "the removed item is blurred")
		assert.Equal(t, 1, focusedCount(reg.All()))

		active, ok := reg.Active()
		require.True(t, ok)
		assert.Equal(t, "c", active.ID())
	})

	t.Run("last wraps to first", func(t *testing.T) {
		reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})
		a := items[0]

		reg.SetActive("b")
		_, ok := reg.Remove("b")

		require.True(t, ok)
		assert.True(t, a.focused)
		assert.Equal(t, 1, focusedCount(reg.All()))
	})

	t.Run("only item leaves registry empty", func(t *testing.T) {
		reg, _ := newRegistry(&fakeItem{id: "a"})

		_, ok := reg.Remove("a")
		require.True(t, ok)
		assert.Equal(t, 0, reg.Len())

		_, ok = reg.Active()
		assert.False(t, ok)
	})
}

func TestRegistry_NextPrevWrap(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"}, &fakeItem{id: "c"})

	reg.Next()
	active, _ := reg.Active()
	assert.Equal(t, "b", active.ID(), "next from first")

	reg.Next()
	active, _ = reg.Active()
	assert.Equal(t, "c", active.ID())

	reg.Next()
	active, _ = reg.Active()
	assert.Equal(t, "a", active.ID(), "next wraps")

	reg.Prev()
	active, _ = reg.Active()
	assert.Equal(t, "c", active.ID(), "prev wraps")

	assert.Equal(t, 1, focusedCount(items))
}

func TestRegistry_NextOnEmptyRegistry(t *testing.T) {
	reg, _ := newRegistry()

	reg.Next()
	reg.Prev()

	_, ok := reg.Active()
	assert.False(t, ok)
}

func TestRegistry_Notify(t *testing.T) {
	reg, _ := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})

	reg.Notify("a")
	assert.False(t, reg.Rows()[0].HasNotification, "notifying the active item is a no-op")

	reg.Notify("b")
	assert.True(t, reg.Rows()[1].HasNotification)

	reg.Notify("unknown")
	assert.False(t, reg.Rows()[0].HasNotification)
	assert.True(t, reg.Rows()[1].HasNotification)

	_, ok := reg.Remove("b")
	require.True(t, ok)

	reg.Add(&fakeItem{id: "c"})
	assert.False(t, reg.Rows()[1].HasNotification, "flags do not survive removal")
}

func TestRegistry_RowsAreCachedUntilSomethingChanges(t *testing.T) {
	reg, _ := newRegistry(&fakeItem{id: "a", title: "chan-a"}, &fakeItem{id: "b", title: "chan-b"})

	first := reg.Rows()
	second := reg.Rows()
	assert.Same(t, &first[0], &second[0], "rows are reused while nothing changed")

	require.Len(t, first, 2)
	assert.Equal(t, "chan-a", first[0].Title)
	assert.Equal(t, "a", first[0].ID)
	assert.True(t, first[0].Active)
	assert.False(t, first[1].Active)

	reg.SetActive("b")

	third := reg.Rows()
	assert.False(t, third[0].Active)
	assert.True(t, third[1].Active)
}

func TestRegistry_FocusActiveRoundTrips(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})
	a := items[0]

	reg.SetActive("a")
	reg.BlurActive()
	assert.False(t, a.focused)

	active, ok := reg.Active()
	require.True(t, ok)
	assert.Equal(t, "a", active.ID(), "blurring does not forget which item is active")

	reg.FocusActive()
	assert.True(t, a.focused)
	assert.Equal(t, 1, focusedCount(items))
}

func TestRegistry_UpdateAllWritesBack(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})

	cmd := reg.UpdateAll("ping", func(f *fakeItem, msg tea.Msg) (*fakeItem, tea.Cmd) {
		assert.Equal(t, "ping", msg)
		f.value = "touched"
		return f, nil
	})

	assert.Nil(t, cmd)
	assert.Equal(t, "touched", items[0].value)
	assert.Equal(t, "touched", items[1].value)
	assert.Equal(t, "touched", reg.All()[1].value)
}

func TestRegistry_UpdateActiveAndUpdateOne(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})
	a, b := items[0], items[1]

	appendValue := func(f *fakeItem, msg tea.Msg) (*fakeItem, tea.Cmd) {
		f.value += msg.(string)
		return f, nil
	}

	_, ok := reg.UpdateActive("1", appendValue)
	require.True(t, ok)
	assert.Equal(t, "1", a.value)
	assert.Empty(t, b.value)

	_, ok = reg.UpdateOne("b", "2", appendValue)
	require.True(t, ok)
	assert.Equal(t, "2", b.value)

	_, ok = reg.UpdateOne("missing", "3", appendValue)
	assert.False(t, ok)
}

func TestRegistry_EmptyActiveIsNotFound(t *testing.T) {
	reg, _ := newRegistry()

	_, ok := reg.Active()
	assert.False(t, ok)

	cmd, ok := reg.UpdateActive("x", func(f *fakeItem, _ tea.Msg) (*fakeItem, tea.Cmd) {
		t.Fatal("update must not be called on an empty registry")
		return f, nil
	})
	assert.False(t, ok)
	assert.Nil(t, cmd)
}

func TestRegistry_Any(t *testing.T) {
	reg, _ := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})

	assert.True(t, reg.Any(func(f *fakeItem) bool { return f.id == "b" }))
	assert.False(t, reg.Any(func(f *fakeItem) bool { return f.id == "c" }))
}

func TestRegistry_Dispatch(t *testing.T) {
	reg, items := newRegistry(&fakeItem{id: "a"}, &fakeItem{id: "b"})
	a, b := items[0], items[1]

	set := func(f *fakeItem, msg tea.Msg) (*fakeItem, tea.Cmd) {
		f.value = msg.(string)
		return f, nil
	}

	cmd, ok := reg.Dispatch(nav.Address("b", "for-b"), set)
	require.True(t, ok)
	assert.Nil(t, cmd)
	assert.Equal(t, "for-b", b.value)
	assert.Empty(t, a.value, "an addressed message must not reach other items")

	_, ok = reg.Dispatch(nav.Address("missing", "dropped"), set)
	assert.False(t, ok, "an unknown target is reported so the caller can keep handling it")

	_, ok = reg.Dispatch("unaddressed", set)
	assert.False(t, ok)
	assert.Empty(t, a.value)
}

func TestAddress_ProducesEnvelope(t *testing.T) {
	msg := nav.To("tab-1", "payload")()

	envelope, ok := msg.(nav.Envelope)
	require.True(t, ok)
	assert.Equal(t, "tab-1", envelope.To)
	assert.Equal(t, "payload", envelope.Msg)
}
