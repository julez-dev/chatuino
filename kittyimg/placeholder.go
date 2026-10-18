package kittyimg

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

// MaxRows is the maximum number of terminal rows an image may span.
const MaxRows = 4

// placeholderRune is the kitty graphics protocol unicode placeholder.
const placeholderRune = '\U0010EEEE'

// rowColumnDiacritics are the first entries of the kitty row/column diacritics table.
// Index n encodes row (or column) n.
// See https://sw.kovidgoyal.net/kitty/_downloads/f0a0de9ec8d9ff4456206db8e0814937/rowcolumn-diacritics.txt
var rowColumnDiacritics = [MaxRows]rune{'\u0305', '\u030D', '\u030E', '\u0310'}

// PlaceholderRowsAbove returns the rows-1 terminal lines that must be rendered above line, so
// multi-row images in line (which contains their bottom row) are displayed in full.
// Every placeholder cell in line is repeated at the same column with the matching row diacritic;
// all other cells become spaces. Returns nil if rows <= 1 or line contains no placeholders.
func PlaceholderRowsAbove(line string, rows int) []string {
	rows = min(rows, MaxRows)
	if rows <= 1 || !strings.ContainsRune(line, placeholderRune) {
		return nil
	}

	above := make([]string, rows-1)

	for row := range rows - 1 {
		above[row] = buildPlaceholderRow(line, row)
	}

	return above
}

func buildPlaceholderRow(line string, row int) string {
	var (
		b          strings.Builder
		currentFg  string // foreground SGR params active in source line (encodes image ID)
		writtenFg  string // foreground currently set in output
		pendingGap int    // spaces not yet written; trailing spaces are dropped
	)

	b.Grow(len(line))

	for len(line) > 0 {
		if line[0] == '\x1b' {
			seq, params, isSGR := nextEscapeSequence(line)
			line = line[len(seq):]

			if isSGR {
				currentFg = foregroundFromSGR(params, currentFg)
			}
			continue
		}

		// consume text up to next escape sequence grapheme by grapheme
		end := strings.IndexByte(line, '\x1b')
		if end == -1 {
			end = len(line)
		}
		text := line[:end]
		line = line[end:]

		state := -1
		for len(text) > 0 {
			var (
				cluster string
				width   int
			)
			cluster, text, width, state = uniseg.FirstGraphemeClusterInString(text, state)

			r, size := utf8.DecodeRuneInString(cluster)
			if r != placeholderRune {
				pendingGap += width
				continue
			}

			if pendingGap > 0 {
				if writtenFg != "" {
					b.WriteString("\x1b[39m")
					writtenFg = ""
				}
				b.WriteString(strings.Repeat(" ", pendingGap))
				pendingGap = 0
			}

			if writtenFg != currentFg {
				b.WriteString("\x1b[" + currentFg + "m")
				writtenFg = currentFg
			}

			b.WriteRune(placeholderRune)

			// first diacritic is the row; replace it, keep the column diacritic
			if diacritics := cluster[size:]; diacritics != "" {
				_, rowSize := utf8.DecodeRuneInString(diacritics)
				b.WriteRune(rowColumnDiacritics[row])
				b.WriteString(diacritics[rowSize:])
			}
		}
	}

	if writtenFg != "" {
		b.WriteString("\x1b[39m")
	}

	return b.String()
}

// nextEscapeSequence returns the escape sequence at the start of s. For CSI SGR sequences
// it also returns the parameter string.
func nextEscapeSequence(s string) (seq string, sgrParams string, isSGR bool) {
	if len(s) < 2 {
		return s, "", false
	}

	switch s[1] {
	case '[': // CSI: parameters until final byte 0x40-0x7E
		for i := 2; i < len(s); i++ {
			if s[i] >= 0x40 && s[i] <= 0x7E {
				return s[:i+1], s[2:i], s[i] == 'm'
			}
		}
		return s, "", false
	case ']', '_', 'P', '^', 'X': // OSC/APC/DCS/PM/SOS: until BEL or ST
		for i := 2; i < len(s); i++ {
			if s[i] == '\a' {
				return s[:i+1], "", false
			}
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
				return s[:i+2], "", false
			}
		}
		return s, "", false
	}

	return s[:2], "", false
}

// foregroundFromSGR returns the foreground SGR params after applying params to current.
// Only 24-bit foreground colors are tracked since those encode the image ID.
func foregroundFromSGR(params, current string) string {
	if params == "" {
		return ""
	}

	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "0", "39":
			current = ""
		case "38":
			if i+4 < len(parts) && parts[i+1] == "2" {
				current = "38;2;" + strings.Join(parts[i+2:i+5], ";")
				i += 4
			} else if i+2 < len(parts) && parts[i+1] == "5" {
				current = "38;5;" + parts[i+2]
				i += 2
			}
		case "48", "58": // skip background/underline color arguments
			if i+1 < len(parts) && parts[i+1] == "2" {
				i += 4
			} else if i+1 < len(parts) && parts[i+1] == "5" {
				i += 2
			}
		default:
			n, err := strconv.Atoi(parts[i])
			if err == nil && ((n >= 30 && n <= 37) || (n >= 90 && n <= 97)) {
				current = parts[i]
			}
		}
	}

	return current
}
