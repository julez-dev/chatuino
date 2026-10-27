//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/julez-dev/chatuino/kittyimg"
)

func getTermCellWidthHeight() (kittyimg.CellSize, error) {
	f, err := os.OpenFile("/dev/tty", unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NDELAY|unix.O_RDWR, 0666)
	if err != nil {
		return kittyimg.CellSize{}, fmt.Errorf("open /dev/tty: %w", err)
	}
	defer f.Close()

	sz, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return kittyimg.CellSize{}, fmt.Errorf("read terminal size: %w", err)
	}

	if sz.Col == 0 || sz.Row == 0 {
		return kittyimg.CellSize{}, errors.New("the terminal reported no size")
	}
	if sz.Xpixel == 0 || sz.Ypixel == 0 {
		return kittyimg.CellSize{}, errors.New("the terminal does not report its cell pixel size")
	}

	return kittyimg.CellSize{
		Width:  float32(sz.Xpixel) / float32(sz.Col),
		Height: float32(sz.Ypixel) / float32(sz.Row),
	}, nil
}
