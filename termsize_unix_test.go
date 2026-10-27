//go:build unix || darwin

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// environmentKeys are all variables detectImageSupport reads. Tests clear them
// before applying a case so the developer's own terminal does not leak in.
var environmentKeys = []string{
	"KITTY_WINDOW_ID",
	"TERM",
	"TERM_PROGRAM",
	"TERM_PROGRAM_VERSION",
	"LC_TERMINAL",
	"LC_TERMINAL_VERSION",
	"GHOSTTY_RESOURCES_DIR",
	"GHOSTTY_BIN_DIR",
	"WEZTERM_EXECUTABLE",
	"KONSOLE_VERSION",
	"VTE_VERSION",
	"TMUX",
	"STY",
}

const (
	clientTermname   = "display-message -p #{client_termname}"
	allowPassthrough = "show -Apv allow-passthrough"
)

func TestDetectImageSupport(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		tmux        map[string]string // tmux args -> output; unknown args fail
		available   bool
		terminal    string
		passthrough bool
		reason      string
		hint        string
	}{
		{
			name:      "kitty",
			env:       map[string]string{"KITTY_WINDOW_ID": "1"},
			available: true,
			terminal:  "kitty",
		},
		{
			name:      "kitty by TERM",
			env:       map[string]string{"TERM": "xterm-kitty"},
			available: true,
			terminal:  "kitty",
		},
		{
			name:      "ghostty",
			env:       map[string]string{"TERM": "xterm-ghostty"},
			available: true,
			terminal:  "Ghostty",
		},
		{
			name:      "ghostty resources directory",
			env:       map[string]string{"GHOSTTY_RESOURCES_DIR": "/usr/share/ghostty"},
			available: true,
			terminal:  "Ghostty",
		},
		{
			name:      "iterm2 new enough",
			env:       map[string]string{"LC_TERMINAL": "iTerm2", "LC_TERMINAL_VERSION": "3.6.0"},
			available: true,
			terminal:  "iTerm2 3.6.0",
		},
		{
			name:     "iterm2 too old",
			env:      map[string]string{"LC_TERMINAL": "iTerm2", "LC_TERMINAL_VERSION": "3.4.19"},
			terminal: "iTerm2 3.4.19",
			reason:   "cannot display kitty graphics",
			hint:     "update iTerm2",
		},
		{
			name:     "wezterm has no placeholders",
			env:      map[string]string{"TERM_PROGRAM": "WezTerm"},
			terminal: "WezTerm",
			reason:   "Unicode placeholders",
		},
		{
			name:     "konsole has no placeholders",
			env:      map[string]string{"KONSOLE_VERSION": "220400"},
			terminal: "Konsole",
			reason:   "placeholders",
		},
		{
			name:     "foot is sixel only",
			env:      map[string]string{"TERM": "foot"},
			terminal: "foot",
			reason:   "sixel",
		},
		{
			name:     "alacritty has no images",
			env:      map[string]string{"TERM": "alacritty"},
			terminal: "Alacritty",
			reason:   "no inline image protocol",
		},
		{
			name:     "vte has no images",
			env:      map[string]string{"VTE_VERSION": "8200"},
			terminal: "VTE / GNOME Terminal",
			reason:   "no inline image support",
		},
		{
			name:     "apple terminal has no images",
			env:      map[string]string{"TERM_PROGRAM": "Apple_Terminal"},
			terminal: "Apple Terminal",
			reason:   "no inline image protocol",
		},
		{
			name:     "unknown terminal",
			terminal: `unrecognised terminal (TERM="", TERM_PROGRAM="")`,
			reason:   "no terminal with kitty Unicode placeholder support was detected",
		},
		{
			name:     "screen",
			env:      map[string]string{"STY": "1234.pts-0.host"},
			terminal: "screen",
			reason:   "screen does not forward kitty graphics",
			hint:     "run Chatuino in tmux or outside screen",
		},
		{
			name:        "tmux with kitty and passthrough on",
			env:         map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			tmux:        map[string]string{clientTermname: "xterm-kitty", allowPassthrough: "on"},
			available:   true,
			terminal:    "kitty inside tmux",
			passthrough: true,
		},
		{
			name:        "tmux with ghostty and passthrough all",
			env:         map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			tmux:        map[string]string{clientTermname: "xterm-ghostty", allowPassthrough: "all"},
			available:   true,
			terminal:    "Ghostty inside tmux",
			passthrough: true,
		},
		{
			name:        "tmux with iTerm2 detected from surviving env",
			env:         map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0", "LC_TERMINAL": "iTerm2", "LC_TERMINAL_VERSION": "3.6.0"},
			tmux:        map[string]string{clientTermname: "xterm-256color", allowPassthrough: "on"},
			available:   true,
			terminal:    "iTerm2 3.6.0 inside tmux",
			passthrough: true,
		},
		{
			name:     "tmux does not forward images",
			env:      map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			tmux:     map[string]string{clientTermname: "xterm-kitty", allowPassthrough: "off"},
			terminal: "kitty inside tmux",
			reason:   "allow-passthrough is off",
			hint:     "allow-passthrough on",
		},
		{
			name:     "tmux option cannot be read",
			env:      map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			tmux:     map[string]string{clientTermname: "xterm-kitty"},
			terminal: "kitty inside tmux",
			reason:   "could not read tmux's allow-passthrough setting",
			hint:     "allow-passthrough on",
		},
		{
			name:     "tmux with unknown outer terminal",
			env:      map[string]string{"TMUX": "/tmp/tmux-1000/default,1,0"},
			tmux:     map[string]string{clientTermname: "tmux-256color", allowPassthrough: "on"},
			terminal: `unrecognised terminal (TERM="", TERM_PROGRAM="")`,
			reason:   "no terminal with kitty Unicode placeholder support was detected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range environmentKeys {
				t.Setenv(key, "")
			}
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			previous := tmuxOutput
			tmuxOutput = func(args ...string) (string, error) {
				if output, ok := tc.tmux[strings.Join(args, " ")]; ok {
					return output, nil
				}
				return "", errors.New("no tmux server")
			}
			t.Cleanup(func() { tmuxOutput = previous })

			got := detectImageSupport()
			require.Equal(t, tc.available, got.Available())
			require.Equal(t, tc.terminal, got.Terminal)
			require.Equal(t, tc.passthrough, got.Passthrough)
			if tc.reason != "" {
				require.Contains(t, got.Reason, tc.reason)
			}
			if tc.hint != "" {
				require.Contains(t, got.Hint, tc.hint)
			}
			if !tc.available {
				message := got.unavailableError("/tmp/settings.yaml").Error()
				require.Contains(t, message, "kitty >= 0.28, Ghostty, iTerm2 >= 3.5.4")
				require.Contains(t, message, "/tmp/settings.yaml")
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"3.5.4", true},
		{"3.5.4beta2", true},
		{"3.5.10", true},
		{"3.6.0", true},
		{"4.0", true},
		{"3.4.19", false},
		{"3.5", false},
		{"3.5.3", false},
		{"2.9.9", false},
		{"", true},     // unparseable: do not block the user
		{"beta", true}, // unparseable: do not block the user
		{"3.5.4.1", true},
	} {
		require.Equal(t, tc.want, versionAtLeast(tc.version, 3, 5, 4), tc.version)
	}
}
