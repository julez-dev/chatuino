package emote

import "slices"

type Platform int

const (
	Unknown Platform = iota
	Twitch
	SevenTV
	BTTV
	FFZ
)

func (p Platform) String() string {
	switch p {
	case Twitch:
		return "Twitch"
	case SevenTV:
		return "SevenTV"
	case BTTV:
		return "BTTV"
	case FFZ:
		return "FFZ"
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

	// For channel specific twitch emotes
	// bitstier, follower, subscriptions
	TTVEmoteType string
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
