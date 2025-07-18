package mainui

import (
	"bytes"
	"fmt"
	"iter"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"

	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/rivo/uniseg"
)

var (
	duplicateBypass   = rune(917504)
	ansiRegex         = regexp.MustCompile(`(\x9B|\x1B\[)[0-?]*[ -\/]*[@-~]`)
	accountStartRegex = regexp.MustCompile(`^[^a-zA-Z0-9_-]+`)
	accountEndRegex   = regexp.MustCompile(`[^a-zA-Z0-9_-]+$`)
)

func filter[S ~[]E, E any](x S, f func(e E) bool) iter.Seq[E] {
	return func(yield func(E) bool) {
		for v := range slices.Values(x) {
			if !f(v) {
				continue
			}

			if !yield(v) {
				return
			}
		}
	}
}

func stripAnsi(str string) string {
	return ansiRegex.ReplaceAllString(str, "")
}

func stripDisplayNameEdges(str string) string {
	return accountEndRegex.ReplaceAllString(accountStartRegex.ReplaceAllString(str, ""), "")
}

func clamp(v, low, high int) int {
	return min(max(v, low), high)
}

func selectWordAtIndex(sentence string, index int) string {
	if index > len(sentence) || sentence == "" {
		return ""
	}

	before, after := sentence[:index], sentence[index:]

	spaceIndexBefore := strings.LastIndex(before, " ")

	if spaceIndexBefore == -1 {
		spaceIndexBefore = 0
	} else {
		spaceIndexBefore++
	}

	spaceIndexAfter := strings.Index(after, " ")

	if spaceIndexAfter == -1 {
		spaceIndexAfter = index + len(after)
	} else {
		spaceIndexAfter = index + spaceIndexAfter
	}

	return sentence[spaceIndexBefore:spaceIndexAfter]
}

// centerTextGraphemeAware centers text in a string, grapheme clusters aware.
// certain emojis break lipgloss's centering, this function works around that.
func centerTextGraphemeAware(width int, s string) string {
	var b bytes.Buffer
	n := (width - uniseg.StringWidth(s)) / 2
	if n < 0 {
		_, _ = b.WriteString(s)
		return b.String()
	}

	fmt.Fprintf(&b, "%s%s", strings.Repeat("\u0020", n), s)
	return b.String()
}

func messageContainsCaseInsensitive(msg *command.PrivateMessage, sub string) bool {
	return strings.Contains(strings.ToLower(msg.Message), strings.ToLower(sub))
}

// // hexToLuminance converts a r,g,b to its luminance.
// func hexToLuminance(r, g, b uint32) float64 {
// 	return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 65535
// }

func randomHexColor() string {
	red := rand.Int32N(256)
	green := rand.Int32N(256)
	blue := rand.Int32N(256)

	return fmt.Sprintf("#%02x%02x%02x", red, green, blue)
}
