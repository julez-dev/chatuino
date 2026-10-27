//go:build !unix

package main

func detectImageSupport() graphicsSupport {
	return unsupported("Windows", "no Windows terminal renders kitty Unicode placeholders (Windows Terminal has sixel only)")
}
