package chat

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserPalette(t *testing.T) {
	t.Parallel()

	p := NewPalette()

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
