package tab

import (
	"fmt"
	"testing"

	"github.com/julez-dev/chatuino/save"
	"github.com/stretchr/testify/require"
)

func TestUserPalette_ComposerSeesUpdatesThroughReadOnlyInterface(t *testing.T) {
	t.Parallel()

	palette := newUserPalette()
	input := newChatInput(palette, nil, save.BuildDefaultTheme(), nil)
	input.SetWidth(80)
	input.SetValue("@al")
	require.NotContains(t, input.View(), "@alice")

	// A user discovered by message rendering becomes available to the existing
	// composer, without sharing a mutable map or rebuilding the composer.
	render := palette.Renderer("alice", "#ff0000")
	input.SetValue("@al")
	require.Contains(t, input.View(), render("@alice"))

	names := palette.Names()
	names[0] = "changed"
	_, ok := palette.Lookup("alice")
	require.True(t, ok, "enumerating names must not expose mutable cache state")

	palette.Prune(nil)
	input.SetValue("@al")
	require.NotContains(t, input.View(), "@alice")
}

func TestUserPalette(t *testing.T) {
	t.Parallel()

	p := newUserPalette()

	render, ok := p.Lookup("alice")
	require.False(t, ok)
	require.Nil(t, render)

	first := p.Renderer("alice", "#123456")
	second := p.Renderer("alice", "#abcdef")
	require.NotNil(t, first)

	require.Equal(t, fmt.Sprintf("%p", first), fmt.Sprintf("%p", second), "the cached renderer is reused")

	p.Renderer("bob", "")

	require.ElementsMatch(t, []string{"alice", "bob"}, p.Names())

	p.Prune(map[string]struct{}{"alice": {}})
	require.ElementsMatch(t, []string{"alice"}, p.Names())
}
