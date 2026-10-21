package tab

import (
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
