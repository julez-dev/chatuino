package mainui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"golang.org/x/sync/errgroup"

	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/rs/zerolog/log"
)

// startupUsers is the part of the Twitch API the startup loader needs: looking up
// the channels of all restored tabs in one batch.
type startupUsers interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error)
}

// startupChatSettings is the Twitch API surface the loader needs to warm the
// chat-settings cache of the restored channels.
type startupChatSettings interface {
	GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitchapi.GetChatSettingsResponse, error)
}

// chatSettingsFetchLimit bounds how many chat-settings requests the startup
// prefetch keeps in flight. The endpoint takes one broadcaster per request, so a
// workspace full of tabs would otherwise burst its whole channel list at once.
const chatSettingsFetchLimit = 6

// startupEmotes is the emote-cache surface the loader fills.
type startupEmotes interface {
	RefreshGlobal(ctx context.Context) error
	AddUserEmotes(userID string, emotes []emote.Emote)
}

// startupBadges is the badge-cache surface the loader fills.
type startupBadges interface {
	RefreshGlobal(ctx context.Context) error
}

// startupLoader performs the fetching Root needs before it can restore tabs. It
// owns the best-effort global cache refreshes, the per-account user emote fetch,
// the batched channel lookup and the suggestion seeding.
//
// It reads collaborators only: model-owned state is applied by Root in Update.
type startupLoader struct {
	accounts        []save.Account
	clients         map[string]tab.ClientCapabilities
	serverAPI       startupUsers
	emoteCache      startupEmotes
	badgeCache      startupBadges
	appStateManager AppStateManager
	channelHistory  ChannelHistory
	chatSettings    *chatstate.Cache
	initialState    *save.AppState
}

func newStartupLoader(root *Root) *startupLoader {
	return &startupLoader{
		accounts:        root.accounts,
		clients:         root.clients,
		serverAPI:       root.serverAPI,
		emoteCache:      root.emoteCache,
		badgeCache:      root.badgeCache,
		appStateManager: root.appStateManager,
		channelHistory:  root.channelHistory,
		chatSettings:    root.chatSettings,
		initialState:    root.initialState,
	}
}

// loadCmd runs the loader in a command, so the fetching happens off the update
// loop.
func (l *startupLoader) loadCmd() tea.Cmd {
	return func() tea.Msg {
		return l.load()
	}
}

// load fetches the startup data. Fatal errors are reported in the result's err;
// cache refreshes and suggestions are best effort and only logged.
func (l *startupLoader) load() persistedDataLoadedMessage {
	var (
		state save.AppState
		err   error
	)

	if l.initialState != nil {
		state = *l.initialState
	} else if l.appStateManager != nil {
		state, err = l.appStateManager.LoadAppState()
		if err != nil {
			return persistedDataLoadedMessage{
				err: fmt.Errorf("failed to load save state: %w", err),
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	group, ctx := errgroup.WithContext(ctx)

	l.refreshGlobalAssets(ctx, group)
	l.fetchUserEmotes(ctx, group)

	users := l.fetchTabUsers(ctx, group, state)
	l.prefetchChatSettings(ctx, group, state, users)

	var channelSuggestions []string
	group.Go(func() error {
		channelSuggestions = l.loadChannelSuggestions(ctx)
		return nil
	})

	err = group.Wait()

	return persistedDataLoadedMessage{
		state:              state,
		ttvUsers:           users.byLogin,
		channelSuggestions: channelSuggestions,
		err:                err,
	}
}

// refreshGlobalAssets refreshes the global badge and emote sets. Failures are
// logged but do not prevent the session from starting.
func (l *startupLoader) refreshGlobalAssets(ctx context.Context, group *errgroup.Group) {
	if l.badgeCache != nil {
		group.Go(func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			if err := l.badgeCache.RefreshGlobal(ctx); err != nil {
				log.Logger.Error().Err(err).Msg("could not fetch global badges")
			}

			return nil
		})
	}

	if l.emoteCache != nil {
		group.Go(func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Second*5)
			defer cancel()

			if err := l.emoteCache.RefreshGlobal(ctx); err != nil {
				log.Logger.Error().Err(err).Msg("could not fetch global emotes")
			}

			return nil
		})
	}
}

// fetchUserEmotes loads every authenticated account's usable emote set into the
// cache. Failures are logged per account.
func (l *startupLoader) fetchUserEmotes(ctx context.Context, group *errgroup.Group) {
	if l.emoteCache == nil {
		return
	}

	for _, acc := range l.accounts {
		if acc.IsAnonymous {
			continue
		}

		client, has := l.clients[acc.ID]
		if !has {
			continue
		}

		fetcher := client.Emotes
		if fetcher == nil {
			log.Logger.Error().Msg("failed to parse user emote client")
			continue
		}

		group.Go(func() error {
			set, template, err := fetcher.FetchAllUserEmotes(ctx, acc.ID, "")
			if err != nil {
				log.Logger.Error().Str("user_id", acc.ID).Err(err).Msg("could not fetch user emotes")
				return nil
			}

			l.emoteCache.AddUserEmotes(acc.ID, buildUserEmoteSet(set, template))

			return nil
		})
	}
}

// tabUsers is the startup user lookup: the channels of the restored tabs, plus the
// signal that the map is complete. Only one goroutine fills the map, so it may only
// be read after ready is closed.
type tabUsers struct {
	byLogin map[string]twitchapi.UserData
	ready   <-chan struct{}
}

// fetchTabUsers looks up the channels of all restored broadcast tabs in one call,
// which saves one request per tab. A failure here is fatal for the session: Root
// cannot restore tabs without knowing their channel IDs. The lookup runs in the
// background, so a caller that needs the channel IDs (the chat-settings prefetch)
// waits on ready instead of blocking the rest of the startup.
func (l *startupLoader) fetchTabUsers(ctx context.Context, group *errgroup.Group, state save.AppState) tabUsers {
	users := make(map[string]twitchapi.UserData, len(state.Tabs))
	ready := make(chan struct{})

	loginsUnique := make(map[string]struct{}, len(state.Tabs))
	for _, spec := range state.Tabs {
		if spec.Kind != int(tab.BroadcastKind) {
			continue
		}

		loginsUnique[spec.Channel] = struct{}{}
	}

	logins := slices.Collect(maps.Keys(loginsUnique))

	if len(logins) == 0 || l.serverAPI == nil {
		close(ready)

		return tabUsers{byLogin: users, ready: ready}
	}

	group.Go(func() error {
		defer close(ready)

		resp, err := l.serverAPI.GetUsers(ctx, logins, nil)
		if err != nil {
			log.Logger.Error().Err(err).Msg("could not fetch users for tabs")
			return fmt.Errorf("could not fetch users for tabs: %w", err)
		}

		// Only this goroutine writes users; the caller reads it after Wait.
		for _, data := range resp.Data {
			users[data.Login] = data
		}

		return nil
	})

	return tabUsers{byLogin: users, ready: ready}
}

// prefetchChatSettings warms the chat-settings cache of every restored channel.
// Get Chat Settings accepts one broadcaster, so this is still one request per
// channel, but only for the unique channels of a session and bounded in
// concurrency. The tabs then render the status bar from the cache instead of each
// requesting its own channel, and failures are best effort: the tab falls back to
// fetching on demand.
func (l *startupLoader) prefetchChatSettings(ctx context.Context, group *errgroup.Group, state save.AppState, users tabUsers) {
	fetcher := l.chatSettingsFetcher()
	if l.chatSettings == nil || fetcher == nil {
		return
	}

	group.Go(func() error {
		select {
		case <-users.ready:
		case <-ctx.Done():
			return nil
		}

		channelIDs := make([]string, 0, len(state.Tabs))
		seen := make(map[string]struct{}, len(state.Tabs))
		for _, spec := range state.Tabs {
			if spec.Kind != int(tab.BroadcastKind) {
				continue
			}

			id := users.byLogin[spec.Channel].ID
			if id == "" {
				continue
			}

			if _, dup := seen[id]; dup {
				continue
			}

			if _, cached := l.chatSettings.Get(id); cached {
				continue
			}

			seen[id] = struct{}{}
			channelIDs = append(channelIDs, id)
		}

		if len(channelIDs) == 0 {
			return nil
		}

		// Errors are logged per channel: one unavailable channel must not keep the
		// settings of the others out of the cache, so the group only bounds how many
		// requests are in flight.
		var fetches errgroup.Group
		fetches.SetLimit(chatSettingsFetchLimit)

		for _, channelID := range channelIDs {
			fetches.Go(func() error {
				// One slow channel must not hold up the restore of every tab.
				callCtx, cancel := context.WithTimeout(ctx, time.Second*5)
				defer cancel()

				if _, err := l.chatSettings.Fetch(callCtx, channelID, chatSettingsCall(fetcher, channelID)); err != nil {
					log.Logger.Warn().Err(err).Str("channel_id", channelID).Msg("could not prefetch chat settings")
				}

				return nil
			})
		}

		fetches.Wait()

		return nil
	})
}

// chatSettingsFetcher returns a client that can read chat settings. A signed-in
// account's own client is preferred because it talks to Twitch directly; the
// server proxy is the fallback for anonymous sessions.
func (l *startupLoader) chatSettingsFetcher() startupChatSettings {
	for _, account := range l.accounts {
		if account.IsAnonymous {
			continue
		}

		if fetcher, ok := l.clients[account.ID].API.(startupChatSettings); ok {
			return fetcher
		}
	}

	fetcher, _ := l.serverAPI.(startupChatSettings)

	return fetcher
}

// chatSettingsCall adapts one client call to the cache's fetcher shape.
func chatSettingsCall(fetcher startupChatSettings, channelID string) chatstate.Fetcher {
	return func(ctx context.Context) (twitchapi.ChatSettingData, error) {
		resp, err := fetcher.GetChatSettings(ctx, channelID, "")
		if err != nil {
			return twitchapi.ChatSettingData{}, err
		}

		settings, err := chatstate.SettingsFromResponse(resp)
		if err != nil {
			return twitchapi.ChatSettingData{}, fmt.Errorf("%w: %s", err, channelID)
		}

		return settings, nil
	}
}

// loadChannelSuggestions seeds the /join autocomplete with history plus the
// channels every account follows, deduplicated by login.
func (l *startupLoader) loadChannelSuggestions(ctx context.Context) []string {
	suggestions := make([]string, 0)
	seen := make(map[string]struct{})

	if l.channelHistory != nil {
		entries, err := l.channelHistory.LoadHistory()
		if err != nil {
			log.Logger.Err(err).Msg("could not load channel history for suggestions")
		} else {
			for _, e := range entries {
				if _, ok := seen[e.ChannelLogin]; !ok {
					seen[e.ChannelLogin] = struct{}{}
					suggestions = append(suggestions, e.ChannelLogin)
				}
			}
		}
	}

	for accountID, client := range l.clients {
		fc := client.Follows
		if fc == nil {
			continue
		}

		followed, err := fc.FetchUserFollowedChannels(ctx, accountID, "")
		if err != nil {
			log.Logger.Err(err).Str("account-id", accountID).Msg("could not fetch followed channels for suggestions")
			continue
		}

		for _, f := range followed {
			login := strings.ToLower(f.BroadcasterLogin)
			if _, ok := seen[login]; !ok {
				seen[login] = struct{}{}
				suggestions = append(suggestions, login)
			}
		}
	}

	return suggestions
}

// buildUserEmoteSet converts the API response into the cache representation.
func buildUserEmoteSet(set []twitchapi.UserEmoteImage, template string) emote.EmoteSet {
	emotes := make(emote.EmoteSet, 0, len(set))
	for _, e := range set {
		animated := slices.Contains(e.Format, "animated")

		format := "static"
		if animated {
			format = "animated"
		}

		url := strings.ReplaceAll(template, "{{id}}", e.ID)
		url = strings.ReplaceAll(url, "{{format}}", format)
		url = strings.ReplaceAll(url, "{{theme_mode}}", "light")
		url = strings.ReplaceAll(url, "{{scale}}", "1.0")

		emotes = append(emotes, emote.Emote{
			ID:         e.ID,
			Text:       e.Name,
			Platform:   emote.Twitch,
			IsAnimated: animated,
			URL:        url,
		})
	}

	return emotes
}
