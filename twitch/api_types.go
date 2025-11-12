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

// https://dev.twitch.tv/docs/api/reference/#send-chat-message
type (
	SendChatMessageRequest struct {
		BroadcasterID string `json:"broadcaster_id"`
		SenderID      string `json:"sender_id"`
		Message       string `json:"message"`

		ReplyMessageID string `json:"reply_parent_message_id"`
		ForSourceOnly  bool   `json:"for_source_only,omitempty"`
	}
	SendChatMessageResponse struct {
		Data []SendChatMessageData `json:"data"`
	}
	SendChatMessageData struct {
		MessageID  string     `json:"message_id"`
		IsSent     bool       `json:"is_sent"`
		DropReason DropReason `json:"drop_reason"`
	}
	DropReason struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
)

// https://api.twitch.tv/helix/chat/emotes/global
type (
	//easyjson:json
	EmoteResponse struct {
		Data     []EmoteData `json:"data"`
		Template string      `json:"template"`
	}

	//easyjson:json
	EmoteData struct {
		ID        string     `json:"id"`
		Name      string     `json:"name"`
		Images    EmoteImage `json:"images"`
		Format    []string   `json:"format"`
		Scale     []string   `json:"scale"`
		ThemeMode []string   `json:"theme_mode"`
	}

	//easyjson:json
	EmoteImage struct {
		URL1X string `json:"url_1x"`
		URL2X string `json:"url_2x"`
		URL4X string `json:"url_4x"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-users
type (
	//easyjson:json
	UserResponse struct {
		Data []UserData `json:"data"`
	}

	//easyjson:json
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
	//easyjson:json
	GetStreamsResponse struct {
		Data       []StreamData `json:"data"`
		Pagination Pagination   `json:"pagination"`
	}

	//easyjson:json
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

	//easyjson:json
	Pagination struct {
		Cursor string `json:"cursor"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-chat-settings
type (
	//easyjson:json
	GetChatSettingsResponse struct {
		Data []ChatSettingData `json:"data"`
	}

	//easyjson:json
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
	//easyjson:json
	BanUserRequest struct {
		Data BanUserData `json:"data"`
	}

	//easyjson:json
	BanUserData struct {
		UserID            string `json:"user_id"`
		DurationInSeconds int    `json:"duration,omitempty"`
		Reason            string `json:"reason"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-unban-requests
type (
	//easyjson:json
	GetUnbanRequestsResponse struct {
		Data       []UnbanRequest `json:"data"`
		Pagination Pagination     `json:"pagination"`
	}

	//easyjson:json
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

// https://dev.twitch.tv/docs/api/reference/#get-followed-channels
type (
	//easyjson:json
	GetFollowedChannelsResponse struct {
		Total      int               `json:"total"`
		Data       []FollowedChannel `json:"data"`
		Pagination Pagination        `json:"pagination"`
	}

	//easyjson:json
	FollowedChannel struct {
		BroadcasterID    string    `json:"broadcaster_id"`
		BroadcasterLogin string    `json:"broadcaster_login"`
		BroadcasterName  string    `json:"broadcaster_name"`
		FollowedAt       time.Time `json:"followed_at"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-eventsub-subscriptions
type (
	//easyjson:json
	GetEventSubSubscriptionsResponse struct {
		Total        int            `json:"total"`
		TotalCost    int            `json:"total_cost"`
		MaxTotalCost int            `json:"max_total_cost"`
		Pagination   Pagination     `json:"pagination"`
		Data         []EventSubData `json:"data"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#create-eventsub-subscription
type (
	//easyjson:json
	CreateEventSubSubscriptionRequest struct {
		Type      string                   `json:"type"`
		Version   string                   `json:"version"`
		Condition map[string]string        `json:"condition"`
		Transport EventSubTransportRequest `json:"transport"`
	}

	//easyjson:json
	EventSubTransportRequest struct {
		Method    string `json:"method"`
		Callback  string `json:"callback"`
		ConduitID string `json:"conduit_id"`
		Secret    string `json:"secret"`
		SessionID string `json:"session_id"`
	}

	//easyjson:json
	CreateEventSubSubscriptionResponse struct {
		Data         []EventSubData `json:"data"`
		Total        int            `json:"total"`
		TotalCost    int            `json:"total_cost"`
		MaxTotalCost int            `json:"max_total_cost"`
	}

	//easyjson:json
	EventSubTransport struct {
		Method    string `json:"method"`
		ConduitID string `json:"conduit_id"`
	}

	//easyjson:json
	EventSubData struct {
		ID        string            `json:"id"`
		Status    string            `json:"status"`
		Type      string            `json:"type"`
		Version   string            `json:"version"`
		Condition map[string]string `json:"condition"`
		CreatedAt time.Time         `json:"created_at"`
		Transport EventSubTransport `json:"transport"`
		Cost      int               `json:"cost"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#create-clip
type (
	//easyjson:json
	CreateClipResponse struct {
		Data []CreatedClip `json:"data"`
	}
	//easyjson:json
	CreatedClip struct {
		ID      string `json:"id"`
		EditURL string `json:"edit_url"`
	}
)

type ChatAnnouncementColor string

const (
	ChatAnnouncementColorBlue    ChatAnnouncementColor = "blue"
	ChatAnnouncementColorGreen   ChatAnnouncementColor = "green"
	ChatAnnouncementColorOrange  ChatAnnouncementColor = "orange"
	ChatAnnouncementColorPurple  ChatAnnouncementColor = "purple"
	ChatAnnouncementColorPrimary ChatAnnouncementColor = "primary"
)

// https://dev.twitch.tv/docs/api/reference/#send-chat-announcement
type (
	CreateChatAnnouncementRequest struct {
		Message string                `json:"message"`
		Color   ChatAnnouncementColor `json:"color"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#create-stream-marker
type (
	//easyjson:json
	CreateStreamMarkerRequest struct {
		UserID string `json:"user_id"`
		// optional
		Description string `json:"description,omitempty"`
	}
	//easyjson:json
	CreateStreamMarkerResponse struct {
		Data []StreamMarker `json:"data"`
	}
	//easyjson:json
	StreamMarker struct {
		ID              string    `json:"id"`
		CreatedAt       time.Time `json:"created_at"`
		PositionSeconds int       `json:"position_seconds"`
		Description     string    `json:"description"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-user-emotes
type (
	//easyjson:json
	GetUserEmotesResponse struct {
		Data       []UserEmoteImage `json:"data"`
		Template   string           `json:"template"`
		Pagination Pagination       `json:"pagination"`
	}
	//easyjson:json
	UserEmoteImage struct {
		ID         string   `json:"id"`
		EmoteSetID string   `json:"emote_set_id"`
		EmoteType  string   `json:"emote_type"`
		Format     []string `json:"format"`
		Name       string   `json:"name"`
		OwnerID    string   `json:"owner_id"`
		Scale      []string `json:"scale"`
		ThemeMode  []string `json:"theme_mode"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-user-chat-color
type (
	//easyjson:json
	GetUserChatColorResponse struct {
		Data []UserChatColor `json:"data"`
	}
	//easyjson:json
	UserChatColor struct {
		Color     string `json:"color"`
		UserName  string `json:"user_name"`
		UserLogin string `json:"user_login"`
		UserID    string `json:"user_id"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-channel-chat-badges
// https://dev.twitch.tv/docs/api/reference#get-global-emotes
type (
	//easyjson:json
	GetChannelChatBadgesResp struct {
		Data []BadgeSet `json:"data"`
	}
	//easyjson:json
	GetGlobalBadgesResp struct {
		Data []BadgeSet `json:"data"`
	}
	//easyjson:json
	BadgeSet struct {
		ID       string         `json:"set_id"`
		Versions []BadgeVersion `json:"versions"`
	}
	//easyjson:json
	BadgeVersion struct {
		ID           string `json:"id"`
		Image_URL_1x string `json:"image_url_1x"`
		Image_URL_2x string `json:"image_url_2x"`
		Image_URL_4x string `json:"image_url_4x"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		ClickAction  string `json:"click_action"`
		ClickURL     string `json:"click_url"`
	}
)
