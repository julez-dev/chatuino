//go:build !unix && !darwin

package main

import (
	"errors"
)

var errUnsupported = errors.New("image support not available for this platform")

func detectImageSupport() graphicsSupport {
	return graphicsSupport{
		Terminal: "Windows",
		Reason:   "no Windows terminal renders kitty Unicode placeholders (Windows Terminal has sixel only)",
		Hint:     "disable graphic emotes, or run Chatuino inside WSL with kitty, Ghostty or iTerm2",
	}
}

func getTermCellWidthHeight() (float32, float32, error) {
	return 0, 0, errUnsupported
}
