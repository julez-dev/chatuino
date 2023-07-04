package twitch

import (
	"fmt"
	"time"
)

// error response
type (
	APIError struct {
		ErrorText string `json:"error"`
		Status    int    `json:"status"`
		Message   string `json:"message"`
	}
)

func (a APIError) Error() string {
	return fmt.Sprintf("%s (%d): %s", a.ErrorText, a.Status, a.Message)
}

// https://api.twitch.tv/helix/chat/emotes/global
type (
	EmoteResponse struct {
		Data     []EmoteData `json:"data"`
		Template string      `json:"template"`
	}

	EmoteData struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		Images    EmoteImage `json:"images"`
		Format    []string   `json:"format"`
		Scale     []string   `json:"scale"`
		ThemeMode []string   `json:"theme_mode"`
	}

	EmoteImage struct {
		URL1X string `json:"url_1x"`
		URL2X string `json:"url_2x"`
		URL4X string `json:"url_4x"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-users
type (
	UserResponse struct {
		Data []UserData `json:"data"`
	}
	UserData struct {
		ID              string    `json:"id"`
		Login           string    `json:"login"`
		DisplayName     string    `json:"display_name"`
		Type            string    `json:"type"`
		BroadcasterType string    `json:"broadcaster_type"`
		Description     string    `json:"description"`
		ProfileImageURL string    `json:"profile_image_url"`
		OfflineImageURL string    `json:"offline_image_url"`
		ViewCount       int       `json:"view_count"`
		Email           string    `json:"email"`
		CreatedAt       time.Time `json:"created_at"`
	}
)
