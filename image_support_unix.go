//go:build unix

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// tmuxTimeout bounds the tmux queries done once at startup.
const tmuxTimeout = 2 * time.Second

// tmuxOutput runs a tmux client command and returns its trimmed stdout. It is a
// variable so the detection tests can run without a tmux server.
var tmuxOutput = func(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// detectImageSupport decides whether kitty Unicode placeholders can reach the
// real terminal. Inside tmux that additionally needs allow-passthrough, because
// released tmux has no kitty image backend of its own: it only forwards the
// wrapped escape sequences a pane emits.
func detectImageSupport() graphicsSupport {
	if os.Getenv("STY") != "" {
		return unsupportedWithHint("screen", "screen does not forward kitty graphics", "run Chatuino in tmux or outside screen")
	}

	if os.Getenv("TMUX") == "" {
		return terminalSupport()
	}

	return tmuxSupport()
}

func tmuxSupport() graphicsSupport {
	outer := tmuxOuterTerminalSupport()
	if !outer.Available() {
		return outer
	}
	outer.Terminal += " inside tmux"

	value, err := tmuxOutput("show", "-Apv", "allow-passthrough")
	if err != nil {
		return unsupportedWithHint(outer.Terminal,
			"could not read tmux's allow-passthrough setting",
			"tmux set -g allow-passthrough on, or run Chatuino outside tmux")
	}

	switch value {
	case "on", "all":
		outer.Passthrough = true
		return outer
	case "off":
		return unsupportedWithHint(outer.Terminal,
			"tmux is not forwarding images (allow-passthrough is off)",
			"tmux set -g allow-passthrough on   (set 'all' to keep images in invisible panes)")
	default:
		return unsupportedWithHint(outer.Terminal,
			fmt.Sprintf("tmux reported an unexpected allow-passthrough value %q", value),
			"tmux set -g allow-passthrough on")
	}
}

// tmuxOuterTerminalSupport asks the tmux client which terminal it is attached
// to before trusting the pane environment. A tmux server started elsewhere does
// not have KITTY_WINDOW_ID, but the client TERM still identifies the outer
// terminal.
func tmuxOuterTerminalSupport() graphicsSupport {
	if termname, err := tmuxOutput("display-message", "-p", "#{client_termname}"); err == nil {
		switch {
		case strings.Contains(termname, "kitty"):
			return graphicsSupport{Terminal: "kitty"}
		case strings.Contains(termname, "ghostty"):
			return graphicsSupport{Terminal: "Ghostty"}
		}
	}

	return terminalSupport()
}

// terminalSupport identifies the attached terminal from the environment. tmux
// overwrites TERM_PROGRAM in panes, so only variables that survive into a pane
// count here; tmuxSupport asks the tmux client for its outer TERM as well.
func terminalSupport() graphicsSupport {
	term := os.Getenv("TERM")
	program := os.Getenv("TERM_PROGRAM")

	switch {
	case os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(term, "kitty"):
		return graphicsSupport{Terminal: "kitty"}
	case program == "ghostty" || strings.Contains(term, "ghostty") ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" || os.Getenv("GHOSTTY_BIN_DIR") != "":
		return graphicsSupport{Terminal: "Ghostty"}
	case os.Getenv("LC_TERMINAL") == "iTerm2" || program == "iTerm.app":
		return itermSupport()
	case program == "WezTerm" || os.Getenv("WEZTERM_EXECUTABLE") != "":
		return unsupported("WezTerm", "WezTerm speaks kitty graphics but does not render Unicode placeholders yet")
	case program == "WarpTerminal":
		return unsupported("Warp", "Warp does not render kitty Unicode placeholders")
	case os.Getenv("KONSOLE_VERSION") != "":
		return unsupported("Konsole", "Konsole supports kitty image placements, but not Unicode placeholders")
	case strings.HasPrefix(term, "foot"):
		return unsupported("foot", "foot supports sixel, not kitty Unicode placeholders")
	case os.Getenv("VTE_VERSION") != "":
		return unsupported("VTE / GNOME Terminal", "VTE has no inline image support")
	case program == "Apple_Terminal":
		return unsupported("Apple Terminal", "Apple Terminal has no inline image protocol")
	case strings.Contains(term, "alacritty"):
		return unsupported("Alacritty", "Alacritty has no inline image protocol")
	}

	return unsupported(
		fmt.Sprintf("unrecognised terminal (TERM=%q, TERM_PROGRAM=%q)", term, program),
		"no terminal with kitty Unicode placeholder support was detected",
	)
}

// itermPlaceholderVersion is the first stable iTerm2 release that renders
// Unicode placeholders correctly (kitty graphics itself landed earlier, but
// placeholder lookups returned the wrong image until 3.6.7).
const itermPlaceholderVersion = "3.6.7"

func itermSupport() graphicsSupport {
	// LC_TERMINAL_VERSION survives into tmux panes. TERM_PROGRAM_VERSION is only
	// trustworthy outside tmux, which overwrites it with its own version.
	version := os.Getenv("LC_TERMINAL_VERSION")
	if version == "" && os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		version = os.Getenv("TERM_PROGRAM_VERSION")
	}

	if version == "" {
		return unsupportedWithHint("iTerm2", "could not determine the iTerm2 version", "update iTerm2 to "+itermPlaceholderVersion+" or newer")
	}

	name := "iTerm2 " + version
	if !versionAtLeast(version, 3, 6, 7) {
		return unsupportedWithHint(name, "this iTerm2 version does not render Unicode placeholders correctly", "update iTerm2 to "+itermPlaceholderVersion+" or newer")
	}

	return graphicsSupport{Terminal: name}
}

// versionAtLeast compares the leading dotted numeric components of a terminal
// version ("3.6.7", "3.6.7beta1"). Unknown or unparseable versions fail closed:
// graphic emotes are opt-in, so an unverifiable terminal gets an actionable
// error instead of images that may render incorrectly.
func versionAtLeast(version string, want ...int) bool {
	parts := strings.SplitN(version, ".", len(want)+1)
	got := make([]int, len(want))

	for i := range want {
		if i >= len(parts) {
			continue
		}

		digits := parts[i]
		end := 0
		for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
			end++
		}
		if end == 0 {
			return false
		}

		number, err := strconv.Atoi(digits[:end])
		if err != nil {
			return false
		}
		got[i] = number
	}

	for i, minimum := range want {
		if got[i] != minimum {
			return got[i] > minimum
		}
	}

	return true
}
