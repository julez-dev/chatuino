package main

import (
	"errors"
	"fmt"
	"strings"
)

// graphicsSupport describes whether graphic emotes can be displayed and how.
// Chatuino composes its chat as text, so only terminals that render kitty
// Unicode placeholders (grid-resident images) can show them. See
// doc/SETTINGS.md for the supported terminals and tmux setup.
type graphicsSupport struct {
	Terminal    string // human-readable terminal name, for messages
	Passthrough bool   // kitty commands must be wrapped for tmux allow-passthrough
	Reason      string // why images cannot be shown; empty when they can
	Hint        string // concrete fix for the user
}

func (s graphicsSupport) Available() bool { return s.Reason == "" }

// unsupported builds a refusal for a terminal that cannot render placeholders.
func unsupported(terminal, reason string) graphicsSupport {
	return graphicsSupport{Terminal: terminal, Reason: reason}
}

// unsupportedWithHint adds a concrete fix to the refusal: a tmux option, a
// version update, or a different terminal to run in.
func unsupportedWithHint(terminal, reason, hint string) graphicsSupport {
	return graphicsSupport{Terminal: terminal, Reason: reason, Hint: hint}
}

// unavailableError is the startup failure when graphic emotes are enabled on a
// setup that cannot show them. The feature is opt-in, so the two ways out are
// changing the terminal/tmux or turning the feature off.
func (s graphicsSupport) unavailableError(settingsPath string) error {
	var b strings.Builder
	b.WriteString("graphic emotes are enabled, but this setup cannot display inline images.\n")
	if s.Terminal != "" {
		fmt.Fprintf(&b, "  terminal:   %s\n", s.Terminal)
	}
	fmt.Fprintf(&b, "  problem:    %s\n", s.Reason)
	if s.Hint != "" {
		fmt.Fprintf(&b, "  fix:        %s\n", s.Hint)
	}
	b.WriteString("  supported:  kitty >= 0.28, Ghostty, iTerm2 >= 3.6.7 (kitty Unicode placeholders)\n")
	fmt.Fprintf(&b, "  or disable: chat.graphic_emotes and chat.graphic_badges in %s", settingsPath)

	return errors.New(b.String())
}
