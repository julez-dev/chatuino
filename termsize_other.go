//go:build !unix && !darwin

package main

import (
	"errors"
)

var errUnsupported = errors.New("emote support not available for this platform")

func hasEmoteSupport() bool {
	return false
}

func getTermCellWidthHeight() (float32, float32, error) {
	return 0, 0, errUnsupported
}
