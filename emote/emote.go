package emote

import "slices"

type Platform int

const (
	Unknown Platform = iota
	Twitch
	SevenTV
	BTTV
)

func (p Platform) String() string {
	switch p {
	case 1:
		return "Twitch"
	case 2:
		return "SevenTV"
	case 3:
		return "BTTV"
	}

	return "Unknown"
}

type Emote struct {
	ID         string
	Text       string
	Platform   Platform
	URL        string
	IsAnimated bool
	Format     string
}

type EmoteSet []Emote

func (set EmoteSet) GetByText(text string) (Emote, bool) {
	for e := range slices.Values(set) {
		if e.Text == text {
			return e, true
		}
	}

	return Emote{}, false
}
