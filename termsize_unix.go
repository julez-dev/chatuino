//go:build unix || darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

func hasImageSupport() bool {
	_, isKitty := os.LookupEnv("KITTY_WINDOW_ID") // always defined by kitty
	term := os.Getenv("TERM")

	return isKitty || term == "xterm-ghostty"
}

func getTermCellWidthHeight() (float32, float32, error) {
	f, err := os.OpenFile("/dev/tty", unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NDELAY|unix.O_RDWR, 0666)
	if err != nil {
		return 0, 0, err
	}

	sz, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)

	if err != nil {
		return 0, 0, err
	}

	cellWidth := float32(sz.Xpixel) / float32(sz.Col)
	cellHeight := float32(sz.Ypixel) / float32(sz.Row)

	return cellWidth, cellHeight, nil
}
