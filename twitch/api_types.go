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

// https://api.twitch.tv/helix/streams
type (
	GetStreamsResponse struct {
		Data       []StreamData `json:"data"`
		Pagination Pagination   `json:"pagination"`
	}
	StreamData struct {
		ID           string    `json:"id"`
		UserID       string    `json:"user_id"`
		UserLogin    string    `json:"user_login"`
		UserName     string    `json:"user_name"`
		GameID       string    `json:"game_id"`
		GameName     string    `json:"game_name"`
		Type         string    `json:"type"`
		Title        string    `json:"title"`
		Tags         []string  `json:"tags"`
		ViewerCount  int       `json:"viewer_count"`
		StartedAt    time.Time `json:"started_at"`
		Language     string    `json:"language"`
		ThumbnailURL string    `json:"thumbnail_url"`
		TagIds       []any     `json:"tag_ids"`
		IsMature     bool      `json:"is_mature"`
	}
	Pagination struct {
		Cursor string `json:"cursor"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-chat-settings
type (
	GetChatSettingsResponse struct {
		Data []ChatSettingData `json:"data"`
	}
	ChatSettingData struct {
		BroadcasterID                 string `json:"broadcaster_id"`
		SlowMode                      bool   `json:"slow_mode"`
		SlowModeWaitTime              int    `json:"slow_mode_wait_time"` // in seconds
		FollowerMode                  bool   `json:"follower_mode"`
		FollowerModeDuration          int    `json:"follower_mode_duration"` // in minutes
		SubscriberMode                bool   `json:"subscriber_mode"`
		EmoteMode                     bool   `json:"emote_mode"`
		UniqueChatMode                bool   `json:"unique_chat_mode"`
		NonModeratorChatDelay         bool   `json:"non_moderator_chat_delay"`
		NonModeratorChatDelayDuration int    `json:"non_moderator_chat_delay_duration"` // in seconds
	}
)

// https://dev.twitch.tv/docs/api/reference/#ban-user
type (
	BanUserRequest struct {
		Data BanUserData `json:"data"`
	}
	BanUserData struct {
		UserID            string `json:"user_id"`
		DurationInSeconds int    `json:"duration,omitempty"`
		Reason            string `json:"reason"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-unban-requests
type (
	GetUnbanRequestsResponse struct {
		Data       []UnbanRequest `json:"data"`
		Pagination Pagination     `json:"pagination"`
	}
	UnbanRequest struct {
		ID               string    `json:"id"`
		BroadcasterName  string    `json:"broadcaster_name"`
		BroadcasterLogin string    `json:"broadcaster_login"`
		BroadcasterID    string    `json:"broadcaster_id"`
		ModeratorID      string    `json:"moderator_id"`
		ModeratorLogin   string    `json:"moderator_login"`
		ModeratorName    string    `json:"moderator_name"`
		UserID           string    `json:"user_id"`
		UserLogin        string    `json:"user_login"`
		UserName         string    `json:"user_name"`
		Text             string    `json:"text"`
		Status           string    `json:"status"`
		CreatedAt        time.Time `json:"created_at"`
		ResolvedAt       time.Time `json:"resolved_at"`
		ResolutionText   string    `json:"resolution_text"`
	}
)
