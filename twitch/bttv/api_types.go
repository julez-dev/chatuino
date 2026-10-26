package bttv

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
	UserResponse struct {
		ID            string        `json:"id"`
		Bots          []string      `json:"bots"`
		Avatar        string        `json:"avatar"`
		ChannelEmotes []Emote       `json:"channelEmotes"`
		SharedEmotes  []SharedEmote `json:"sharedEmotes"`
	}

	GlobalEmoteResponse []Emote

	Emote struct {
		ID        string `json:"id"`
		Code      string `json:"code"`
		ImageType string `json:"imageType"`
		Animated  bool   `json:"animated"`
		UserId    string `json:"userId"`
	}

	SharedEmote struct {
		ID        string          `json:"id"`
		Code      string          `json:"code"`
		ImageType string          `json:"imageType"`
		Animated  bool            `json:"animated"`
		User      SharedEmoteUser `json:"user"`
	}

	SharedEmoteUser struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		ProviderId  string `json:"providerId"`
	}
)
