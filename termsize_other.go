//go:build !unix

package main

import (
	"errors"

	"github.com/julez-dev/chatuino/kittyimg"
)

var errUnsupported = errors.New("image support not available for this platform")

func getTermCellWidthHeight() (kittyimg.CellSize, error) {
	return kittyimg.CellSize{}, errUnsupported
}
