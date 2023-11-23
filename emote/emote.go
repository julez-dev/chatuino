package emote

type Platform int

func (p Platform) String() string {
	switch p {
	case 1:
		return "Twitch"
	case 2:
		return "SevenTV"
	}

	return "Unknown"
}

const (
	Unknown Platform = iota
	Twitch
	SevenTV
)

// func Decode(r io.Reader) (image.Image, error) {
// 	img, format, err := image.Decode(r)
// 	if err != nil {
// 		return nil, err
// 	}

// 	fmt.Println(format)

// 	return img, nil
// }

// func ImageToString(img image.Image) (string, error) {
// 	var (
// 		bounds = img.Bounds()
// 		width  = bounds.Max.X
// 		height = bounds.Max.Y
// 		out    = strings.Builder{}
// 	)

// 	for y := 0; y < height; y += 2 {
// 		for x := 0; x < width; x++ {
// 			imgColor, hasColor := colorful.MakeColor(img.At(x, y))

// 			if hasColor {
// 				style := lipgloss.NewStyle().
// 					Foreground(lipgloss.Color(imgColor.Hex())).
// 					Background(lipgloss.Color(imgColor.Hex()))
// 				out.WriteString(style.Render("\u2585"))

// 			} else {
// 				out.WriteString("")
// 			}

// 		}

// 		out.WriteString("\n")
// 	}

// 	return out.String(), nil
// }

// func ImageToString(img image.Image) (string, error) {
// 	out := strings.Builder{}

// 	err := kittyimg.Fprint(&out, img)
// 	if err != nil {
// 		return "", err
// 	}

// 	return out.String(), nil
// }

type EmoteSet []Emote

func (set EmoteSet) GetByText(text string) (Emote, bool) {
	for _, e := range set {
		if e.Text == text {
			return e, true
		}
	}

	return Emote{}, false
}

type Emote struct {
	ID       string
	Text     string
	Platform Platform
	URL      string
}
