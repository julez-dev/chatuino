package twitchapi

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
	EmoteResponse struct {
		Data     []EmoteData `json:"data"`
		Template string      `json:"template"`
	}

	EmoteData struct {
		ID        string     `json:"id"`
		EmoteType string     `json:"emote_type"`
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

	// ChatSettingsUpdateData is the PATCH body of Update Chat Settings. Every field
	// is optional: nil means "leave this setting alone", which is why the fields are
	// pointers instead of zero values. A false boolean is a real update.
	ChatSettingsUpdateData struct {
		SlowMode                      *bool `json:"slow_mode,omitempty"`
		SlowModeWaitTime              *int  `json:"slow_mode_wait_time,omitempty"`
		FollowerMode                  *bool `json:"follower_mode,omitempty"`
		FollowerModeDuration          *int  `json:"follower_mode_duration,omitempty"`
		SubscriberMode                *bool `json:"subscriber_mode,omitempty"`
		EmoteMode                     *bool `json:"emote_mode,omitempty"`
		UniqueChatMode                *bool `json:"unique_chat_mode,omitempty"`
		NonModeratorChatDelay         *bool `json:"non_moderator_chat_delay,omitempty"`
		NonModeratorChatDelayDuration *int  `json:"non_moderator_chat_delay_duration,omitempty"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-moderators
// https://dev.twitch.tv/docs/api/reference/#get-vips
type (
	ChatRoleUsersResponse struct {
		Data       []ChatRoleUser `json:"data"`
		Pagination Pagination     `json:"pagination"`
	}

	ChatRoleUser struct {
		UserID    string `json:"user_id"`
		UserLogin string `json:"user_login"`
		UserName  string `json:"user_name"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#warn-chat-user
type (
	WarnChatUserRequest struct {
		Data WarnChatUserData `json:"data"`
	}

	WarnChatUserData struct {
		UserID string `json:"user_id"`
		Reason string `json:"reason"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#start-a-raid
type (
	StartRaidResponse struct {
		Data []RaidData `json:"data"`
	}

	RaidData struct {
		CreatedAt time.Time `json:"created_at"`
		IsMature  bool      `json:"is_mature"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#manage-held-automod-messages
type (
	ManageAutoModMessageRequest struct {
		UserID string `json:"user_id"`
		MsgID  string `json:"msg_id"`
		Action string `json:"action"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#snooze-next-ad
type (
	SnoozeNextAdResponse struct {
		Data []AdSnoozeData `json:"data"`
	}

	AdSnoozeData struct {
		NextAdAt        string `json:"next_ad_at"`
		SnoozeRefreshAt string `json:"snooze_refresh_at"`
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

// https://dev.twitch.tv/docs/api/reference/#get-followed-channels
type (
	GetFollowedChannelsResponse struct {
		Total      int               `json:"total"`
		Data       []FollowedChannel `json:"data"`
		Pagination Pagination        `json:"pagination"`
	}

	FollowedChannel struct {
		BroadcasterID    string    `json:"broadcaster_id"`
		BroadcasterLogin string    `json:"broadcaster_login"`
		BroadcasterName  string    `json:"broadcaster_name"`
		FollowedAt       time.Time `json:"followed_at"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-eventsub-subscriptions
type (
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
	CreateEventSubSubscriptionRequest struct {
		Type      string                   `json:"type"`
		Version   string                   `json:"version"`
		Condition map[string]string        `json:"condition"`
		Transport EventSubTransportRequest `json:"transport"`
	}

	EventSubTransportRequest struct {
		Method    string `json:"method"`
		Callback  string `json:"callback"`
		ConduitID string `json:"conduit_id"`
		Secret    string `json:"secret"`
		SessionID string `json:"session_id"`
	}

	CreateEventSubSubscriptionResponse struct {
		Data         []EventSubData `json:"data"`
		Total        int            `json:"total"`
		TotalCost    int            `json:"total_cost"`
		MaxTotalCost int            `json:"max_total_cost"`
	}

	EventSubTransport struct {
		Method    string `json:"method"`
		ConduitID string `json:"conduit_id"`
	}

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
	CreateClipResponse struct {
		Data []CreatedClip `json:"data"`
	}
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
	CreateStreamMarkerRequest struct {
		UserID string `json:"user_id"`
		// optional
		Description string `json:"description,omitempty"`
	}
	CreateStreamMarkerResponse struct {
		Data []StreamMarker `json:"data"`
	}
	StreamMarker struct {
		ID              string    `json:"id"`
		CreatedAt       time.Time `json:"created_at"`
		PositionSeconds int       `json:"position_seconds"`
		Description     string    `json:"description"`
	}
)

// https://dev.twitch.tv/docs/api/reference/#get-user-emotes
type (
	GetUserEmotesResponse struct {
		Data       []UserEmoteImage `json:"data"`
		Template   string           `json:"template"`
		Pagination Pagination       `json:"pagination"`
	}
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
	GetUserChatColorResponse struct {
		Data []UserChatColor `json:"data"`
	}
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
	GetChannelChatBadgesResp struct {
		Data []BadgeSet `json:"data"`
	}
	GetGlobalBadgesResp struct {
		Data []BadgeSet `json:"data"`
	}
	BadgeSet struct {
		ID       string         `json:"set_id"`
		Versions []BadgeVersion `json:"versions"`
	}
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
