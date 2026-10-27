package chat

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// duplicateBypass is the invisible rune the tab inserts to keep Twitch from
// collapsing an intentionally repeated message. It must never be rendered.
const duplicateBypass = rune(917504)

var ansiRegex = regexp.MustCompile(`(\x9B|\x1B\[)[0-?]*[ -\/]*[@-~]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func clamp(v, low, high int) int {
	return min(max(v, low), high)
}

func randomHexColor() string {
	red := rand.Int32N(256)
	green := rand.Int32N(256)
	blue := rand.Int32N(256)

	return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
}

var (
	accountStartRegex = regexp.MustCompile(`^[^a-zA-Z0-9_-]+`)
	accountEndRegex   = regexp.MustCompile(`[^a-zA-Z0-9_-]+$`)
)

func stripDisplayNameEdges(str string) string {
	return accountEndRegex.ReplaceAllString(accountStartRegex.ReplaceAllString(str, ""), "")
}

// Snippet normalizes a message into one line for a reply indicator or quote.
func Snippet(message string) string {
	message = strings.ReplaceAll(message, string(duplicateBypass), "")

	return strings.Join(strings.Fields(message), " ")
}

// TruncateToWidth shortens s so it renders in at most width cells.
func TruncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(s) <= width {
		return s
	}

	var b strings.Builder
	used := 0

	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > width-1 {
			break
		}

		b.WriteRune(r)
		used += rw
	}

	return b.String() + "…"
}

func humanizeDuration(d time.Duration) string {
	if d < time.Second {
		return "0 seconds"
	}

	var parts []string

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 day")
		} else {
			parts = append(parts, fmt.Sprintf("%d days", days))
		}
	}

	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", hours))
		}
	}

	if minutes > 0 && days == 0 {
		if minutes == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", minutes))
		}
	}

	if seconds > 0 && hours == 0 && days == 0 {
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	if len(parts) == 0 {
		return "0 seconds"
	}

	return strings.Join(parts, " ")
}
