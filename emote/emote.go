package emote

import (
	"fmt"
	"image"
	"io"
	"strings"

	_ "image/gif"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	_ "golang.org/x/image/webp"
)

func Decode(r io.Reader) (image.Image, error) {
	img, format, err := image.Decode(r)

	if err != nil {
		return nil, err
	}

	fmt.Println(format)

	return img, nil
}

func ImageToString(img image.Image) (string, error) {
	var (
		bounds = img.Bounds()
		width  = bounds.Max.X
		height = bounds.Max.Y
		out    = strings.Builder{}
	)

	for y := 0; y < height; y += 2 {
		for x := 0; x < width; x++ {
			imgColor, hasColor := colorful.MakeColor(img.At(x, y))

			if hasColor {
				style := lipgloss.NewStyle().
					Foreground(lipgloss.Color(imgColor.Hex())).
					Background(lipgloss.Color(imgColor.Hex()))
				out.WriteString(style.Render("\u2585"))

			} else {
				out.WriteString("")
			}

		}

		out.WriteString("\n")
	}

	return out.String(), nil
}
