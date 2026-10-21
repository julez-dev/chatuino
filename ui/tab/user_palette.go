package tab

import (
	"sort"

	"charm.land/lipgloss/v2"
	"github.com/rs/zerolog/log"
)

// userPalette owns the per-user color render functions. Rendered chat messages,
// the user inspect window and the composer suggestions all read from the same
// palette, so a user keeps the same color everywhere.
//
// The zero value is not usable; construct one with newUserPalette.
type userPalette struct {
	renders map[string]func(...string) string
}

func newUserPalette() *userPalette {
	return &userPalette{renders: map[string]func(...string) string{}}
}

// Renderer returns the render function for name, creating and caching one from
// colorHex when the user is seen for the first time. An empty colorHex falls
// back to a random color.
func (p *userPalette) Renderer(name, colorHex string) func(...string) string {
	if render, ok := p.renders[name]; ok {
		return render
	}

	if colorHex == "" {
		colorHex = randomHexColor()
	}

	render := lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex)).Render
	p.renders[name] = render

	return render
}

// Lookup returns the cached render function for name.
func (p *userPalette) Lookup(name string) (func(...string) string, bool) {
	render, ok := p.renders[name]

	return render, ok
}

// Prune drops every user not in keep. Logins are compared case-insensitively
// because Twitch logins are.
func (p *userPalette) Prune(keep map[string]struct{}) {
	for name := range p.renders {
		if _, ok := keep[name]; ok {
			continue
		}

		log.Logger.Info().Str("user", name).Msg("delete user from cache")
		delete(p.renders, name)
	}
}

// Names returns the cached user names in sorted order, for diagnostics.
func (p *userPalette) Names() []string {
	names := make([]string, 0, len(p.renders))
	for name := range p.renders {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
