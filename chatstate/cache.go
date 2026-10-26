// Package chatstate keeps the chat-room settings: slow mode, follower mode,
// emote-only and the rest of the state a tab renders in its status bar.
//
// The settings have two sources. IRC ROOMSTATE messages are free but partial:
// Twitch only sends the tags it wants to report, so an absent tag means "no news",
// not "off". The Twitch Get Chat Settings endpoint returns the full state but
// costs one request per broadcaster and cannot be batched. The cache merges both
// and deduplicates requests, so a channel is fetched at most once and every tab
// showing that channel renders the same state.
package chatstate

import (
	"context"
	"errors"
	"sync"

	"resenje.org/singleflight"

	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// ErrNoSettings reports that Get Chat Settings answered without an entry for the
// broadcaster, which means the response cannot be used as settings.
var ErrNoSettings = errors.New("no chat settings returned for channel")

// SettingsFromResponse unwraps the single entry Get Chat Settings returns. Both the
// startup prefetch and the tab fallback read settings this way, so the "empty answer"
// rule lives here instead of in each caller.
func SettingsFromResponse(resp twitchapi.GetChatSettingsResponse) (twitchapi.ChatSettingData, error) {
	if len(resp.Data) == 0 {
		return twitchapi.ChatSettingData{}, ErrNoSettings
	}

	return resp.Data[0], nil
}

// Fetcher loads the complete chat settings of one broadcaster.
type Fetcher func(ctx context.Context) (twitchapi.ChatSettingData, error)

// Cache holds the known chat settings per broadcaster ID. Settings belong to the
// channel, not to the account reading them, so tabs of different accounts share
// an entry.
type Cache struct {
	mu       sync.RWMutex
	settings map[string]twitchapi.ChatSettingData

	group *singleflight.Group[string, twitchapi.ChatSettingData]
}

func NewCache() *Cache {
	return &Cache{
		settings: make(map[string]twitchapi.ChatSettingData),
		group:    &singleflight.Group[string, twitchapi.ChatSettingData]{},
	}
}

// Get returns the settings known for a channel without making a request.
func (c *Cache) Get(broadcasterID string) (twitchapi.ChatSettingData, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	settings, ok := c.settings[broadcasterID]

	return settings, ok
}

// Put records settings for a channel. Empty IDs are ignored: they carry no
// channel identity and would share one map entry across unrelated tabs.
func (c *Cache) Put(broadcasterID string, settings twitchapi.ChatSettingData) {
	if broadcasterID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.settings[broadcasterID] = settings
}

// Fetch returns the settings of a channel and only calls fetch when nothing is
// cached. Concurrent callers for the same channel share one request. Failures are
// not cached, so a later call can retry.
func (c *Cache) Fetch(ctx context.Context, broadcasterID string, fetch Fetcher) (twitchapi.ChatSettingData, error) {
	if settings, ok := c.Get(broadcasterID); ok {
		return settings, nil
	}

	settings, _, err := c.group.Do(ctx, broadcasterID, func(ctx context.Context) (twitchapi.ChatSettingData, error) {
		// Another caller may have filled the cache while this call was queued.
		if settings, ok := c.Get(broadcasterID); ok {
			return settings, nil
		}

		settings, err := fetch(ctx)
		if err != nil {
			return twitchapi.ChatSettingData{}, err
		}

		c.Put(broadcasterID, settings)

		return settings, nil
	})

	return settings, err
}

// ApplyRoomState merges a ROOMSTATE message into the settings known for the
// channel and returns the result. Because the message is partial, a full state
// fetched earlier survives an update that only mentions one setting.
func (c *Cache) ApplyRoomState(broadcasterID string, state *twitchirc.RoomState) twitchapi.ChatSettingData {
	base, _ := c.Get(broadcasterID)

	settings := merge(base, state)
	if settings.BroadcasterID == "" {
		settings.BroadcasterID = broadcasterID
	}

	c.Put(broadcasterID, settings)

	return settings
}

// merge applies the fields a ROOMSTATE message carried on top of settings. A nil
// field means Twitch did not send that tag, so the known setting is kept.
func merge(settings twitchapi.ChatSettingData, state *twitchirc.RoomState) twitchapi.ChatSettingData {
	if state == nil {
		return settings
	}

	if state.EmoteOnly != nil {
		settings.EmoteMode = *state.EmoteOnly
	}

	if state.SubsOnly != nil {
		settings.SubscriberMode = *state.SubsOnly
	}

	// r9k is Twitch's IRC name for unique chat.
	if state.R9K != nil {
		settings.UniqueChatMode = *state.R9K
	}

	if state.Slow != nil {
		settings.SlowMode = *state.Slow > 0
		settings.SlowModeWaitTime = max(*state.Slow, 0)
	}

	if state.FollowersOnly != nil {
		// -1 disables the restriction. Every other value is the required follow
		// time in minutes, where 0 means "any follower".
		settings.FollowerMode = *state.FollowersOnly >= 0
		settings.FollowerModeDuration = max(*state.FollowersOnly, 0)
	}

	return settings
}
