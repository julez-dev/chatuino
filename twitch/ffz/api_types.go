package ffz

import (
	"fmt"
	"net/http"
)

type APIError struct {
	StatusCode int    `json:"-"`
	Status     string `json:"-"`
	Message    string `json:"message"`
}

func (a APIError) Error() string {
	return fmt.Sprintf("%s (%d): %s", a.Status, a.StatusCode, a.Message)
}

// IsNotFound reports a channel that has no emotes on this provider. That is a
// normal answer, not a failure.
func (a APIError) IsNotFound() bool {
	return a.StatusCode == http.StatusNotFound
}

type (
	// globalResponse is the raw API response from /v1/set/global.
	// DefaultSets lists the set IDs that contain global emotes.
	globalResponse struct {
		DefaultSets []int               `json:"default_sets"`
		Sets        map[string]emoteSet `json:"sets"`
	}

	// channelResponse is the raw API response from /v1/room/id/{twitch_id}.
	channelResponse struct {
		Room Room                `json:"room"`
		Sets map[string]emoteSet `json:"sets"`
	}

	Room struct {
		TwitchID int `json:"twitch_id"`
		Set      int `json:"set"`
	}

	emoteSet struct {
		ID        int     `json:"id"`
		Emoticons []Emote `json:"emoticons"`
	}

	Emote struct {
		ID       int               `json:"id"`
		Name     string            `json:"name"`
		Height   int               `json:"height"`
		Width    int               `json:"width"`
		Modifier bool              `json:"modifier"`
		URLs     map[string]string `json:"urls"`
	}
)
