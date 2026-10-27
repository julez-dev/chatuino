//go:build unix || darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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
		return graphicsSupport{
			Terminal: "screen",
			Reason:   "screen does not forward kitty graphics",
			Hint:     "run Chatuino in tmux or outside screen",
		}
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
		return graphicsSupport{
			Terminal: outer.Terminal,
			Reason:   "could not read tmux's allow-passthrough setting",
			Hint:     "tmux set -g allow-passthrough on, or run Chatuino outside tmux",
		}
	}

	switch value {
	case "on", "all":
		outer.Passthrough = true
		return outer
	case "off":
		return graphicsSupport{
			Terminal: outer.Terminal,
			Reason:   "tmux is not forwarding images (allow-passthrough is off)",
			Hint:     "tmux set -g allow-passthrough on   (set 'all' to keep images in invisible panes)",
		}
	default:
		return graphicsSupport{
			Terminal: outer.Terminal,
			Reason:   fmt.Sprintf("tmux reported an unexpected allow-passthrough value %q", value),
			Hint:     "tmux set -g allow-passthrough on",
		}
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
		return graphicsSupport{
			Terminal: "WezTerm",
			Reason:   "WezTerm speaks kitty graphics but does not render Unicode placeholders yet",
		}
	case program == "WarpTerminal":
		return graphicsSupport{
			Terminal: "Warp",
			Reason:   "Warp does not render kitty Unicode placeholders",
		}
	case os.Getenv("KONSOLE_VERSION") != "":
		return graphicsSupport{
			Terminal: "Konsole",
			Reason:   "Konsole supports kitty image placements, but not Unicode placeholders",
		}
	case strings.HasPrefix(term, "foot"):
		return graphicsSupport{
			Terminal: "foot",
			Reason:   "foot supports sixel, not kitty Unicode placeholders",
		}
	case os.Getenv("VTE_VERSION") != "":
		return graphicsSupport{
			Terminal: "VTE / GNOME Terminal",
			Reason:   "VTE has no inline image support",
		}
	case program == "Apple_Terminal":
		return graphicsSupport{
			Terminal: "Apple Terminal",
			Reason:   "Apple Terminal has no inline image protocol",
		}
	case strings.Contains(term, "alacritty"):
		return graphicsSupport{
			Terminal: "Alacritty",
			Reason:   "Alacritty has no inline image protocol",
		}
	}

	return graphicsSupport{
		Terminal: fmt.Sprintf("unrecognised terminal (TERM=%q, TERM_PROGRAM=%q)", term, program),
		Reason:   "no terminal with kitty Unicode placeholder support was detected",
	}
}

// itermSupport checks the iTerm2 version: the kitty graphics protocol landed in
// 3.5.4, and Unicode placeholders were fixed in later builds.
func itermSupport() graphicsSupport {
	version := os.Getenv("LC_TERMINAL_VERSION")
	if version == "" {
		version = os.Getenv("TERM_PROGRAM_VERSION")
	}

	name := "iTerm2"
	if version != "" {
		name += " " + version
		if !versionAtLeast(version, 3, 5, 4) {
			return graphicsSupport{
				Terminal: name,
				Reason:   "this iTerm2 version cannot display kitty graphics",
				Hint:     "update iTerm2 to 3.5.4 or newer",
			}
		}
	}

	return graphicsSupport{Terminal: name}
}

// versionAtLeast compares the leading dotted numeric components of a terminal
// version ("3.5.4", "3.5.4beta2"). An unparseable version counts as recent
// enough: blocking a user because a vendor changed its version format is worse
// than showing a static image on an older build.
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
			return true
		}

		number, err := strconv.Atoi(digits[:end])
		if err != nil {
			return true
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

func getTermCellWidthHeight() (float32, float32, error) {
	f, err := os.OpenFile("/dev/tty", unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NDELAY|unix.O_RDWR, 0666)
	if err != nil {
		return 0, 0, fmt.Errorf("open /dev/tty: %w", err)
	}
	defer f.Close()

	sz, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, fmt.Errorf("read terminal size: %w", err)
	}

	if sz.Col == 0 || sz.Row == 0 {
		return 0, 0, errors.New("the terminal reported no size")
	}
	if sz.Xpixel == 0 || sz.Ypixel == 0 {
		return 0, 0, errors.New("the terminal does not report its cell pixel size")
	}

	cellWidth := float32(sz.Xpixel) / float32(sz.Col)
	cellHeight := float32(sz.Ypixel) / float32(sz.Row)

	return cellWidth, cellHeight, nil
}
