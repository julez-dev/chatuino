package emote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/julez-dev/chatuino/twitch/bttv"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type TwitchEmoteFetcher interface {
	GetGlobalEmotes(context.Context) (twitch.EmoteResponse, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) (twitch.EmoteResponse, error)
}

type SevenTVEmoteFetcher interface {
	GetGlobalEmotes(context.Context) (seventv.EmoteResponse, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) (seventv.ChannelEmoteResponse, error)
}

type BTTVEmoteFetcher interface {
	GetGlobalEmotes(context.Context) (bttv.GlobalEmoteResponse, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) (bttv.UserResponse, error)
}

type Cache struct {
	logger zerolog.Logger
	m      *sync.RWMutex

	global  EmoteSet
	channel map[string]EmoteSet
	user    map[string]EmoteSet // emoteset usable by a specific twitch ID (for exapmle subs.)

	// Emotes that were included in the emotes tag inside a twitch irc message but which are not included in the broadcasters EmoteSet.
	// This can be sub emotes from other channels for example. This is only supported for twitch.
	foreignEmotes map[string]Emote

	twitchEmotes  TwitchEmoteFetcher
	sevenTVEmotes SevenTVEmoteFetcher
	bttvEmotes    BTTVEmoteFetcher

	// supress duplicated calls
	single          *singleflight.Group
	channelsFetched map[string]struct{}
	globalFetched   bool
}

func NewCache(logger zerolog.Logger, twitchEmotes TwitchEmoteFetcher, sevenTVEmotes SevenTVEmoteFetcher, bttvEmotes BTTVEmoteFetcher) *Cache {
	return &Cache{
		logger:          logger,
		m:               &sync.RWMutex{},
		channel:         map[string]EmoteSet{},
		twitchEmotes:    twitchEmotes,
		sevenTVEmotes:   sevenTVEmotes,
		bttvEmotes:      bttvEmotes,
		single:          &singleflight.Group{},
		channelsFetched: map[string]struct{}{},
		user:            map[string]EmoteSet{},
		foreignEmotes:   map[string]Emote{},
	}
}

func (s *Cache) RefreshLocal(ctx context.Context, channelID string) error {
	s.m.RLock()
	if _, isCached := s.channelsFetched[channelID]; isCached {
		s.m.RUnlock()
		return nil
	}
	s.m.RUnlock()

	set, err, _ := s.single.Do("channel"+channelID, func() (any, error) {
		var (
			ttvResp  twitch.EmoteResponse
			stvResp  seventv.ChannelEmoteResponse
			bttvResp bttv.UserResponse
		)

		group, ctx := errgroup.WithContext(ctx)

		group.Go(func() error {
			resp, err := s.twitchEmotes.GetChannelEmotes(ctx, channelID)
			if err != nil {
				return err
			}

			ttvResp = resp

			return nil
		})

		group.Go(func() error {
			resp, err := s.sevenTVEmotes.GetChannelEmotes(ctx, channelID)
			if err != nil {
				s.logger.Error().Str("channel_id", channelID).Err(err).Msg("could not fetch 7TV emotes")

				var apiErr seventv.APIError
				if errors.As(err, &apiErr) {
					if apiErr.StatusCode == http.StatusNotFound {
						return nil
					}
				}

				return err
			}

			stvResp = resp

			return nil
		})

		group.Go(func() error {
			resp, err := s.bttvEmotes.GetChannelEmotes(ctx, channelID)
			if err != nil {

				var apiErr bttv.APIError
				if errors.As(err, &apiErr) {
					if apiErr.StatusCode == http.StatusNotFound {
						return nil
					}
				}

				s.logger.Error().Str("channel_id", channelID).Err(err).Msg("could not fetch BTTV emotes")

				return err
			}

			bttvResp = resp

			return nil
		})

		if err := group.Wait(); err != nil {
			return nil, err
		}

		emoteSet := make(EmoteSet, 0, len(ttvResp.Data)+len(stvResp.EmoteSet.Emotes)+len(bttvResp.ChannelEmotes))

		for _, ttvEmote := range ttvResp.Data {
			emoteSet = append(emoteSet, Emote{
				ID:       ttvEmote.ID,
				Text:     ttvEmote.Name,
				Platform: Twitch,
				URL:      ttvEmote.Images.URL1X,
			})
		}

		for _, stvEmote := range stvResp.EmoteSet.Emotes {
			var url string
			if stvEmote.Data.Animated {
				url = fmt.Sprintf("%s/1x.avif", stvEmote.Data.Host.URL)
			} else {
				url = fmt.Sprintf("%s/1x.webp", stvEmote.Data.Host.URL)
			}
			url, _ = strings.CutPrefix(url, "//")
			url = "https://" + url

			emoteSet = append(emoteSet, Emote{
				ID:         stvEmote.ID,
				Text:       stvEmote.Name,
				Platform:   SevenTV,
				IsAnimated: stvEmote.Data.Animated,
				URL:        url,
			})
		}

		for _, bttvEmote := range bttvResp.ChannelEmotes {
			emoteSet = append(emoteSet, Emote{
				ID:         bttvEmote.ID,
				Text:       bttvEmote.Code,
				IsAnimated: bttvEmote.Animated,
				Format:     bttvEmote.ImageType,
				Platform:   BTTV,
				URL:        fmt.Sprintf("https://cdn.betterttv.net/emote/%s/1x", bttvEmote.ID),
			})
		}

		return emoteSet, nil
	})

	if err != nil {
		return err
	}

	s.m.Lock()
	defer s.m.Unlock()
	s.channelsFetched[channelID] = struct{}{}
	s.channel[channelID] = set.(EmoteSet)

	return nil
}

func (s *Cache) RefreshGlobal(ctx context.Context) error {
	s.m.RLock()
	if s.globalFetched {
		s.m.RUnlock()
		return nil
	}
	s.m.RUnlock()

	set, err, shared := s.single.Do("global", func() (any, error) {
		group, ctx := errgroup.WithContext(ctx)

		var (
			ttvResp  twitch.EmoteResponse
			stvResp  seventv.EmoteResponse
			bttvResp bttv.GlobalEmoteResponse
		)

		group.Go(func() error {
			resp, err := s.twitchEmotes.GetGlobalEmotes(ctx)
			if err != nil {
				return err
			}

			ttvResp = resp
			return nil
		})

		group.Go(func() error {
			resp, err := s.sevenTVEmotes.GetGlobalEmotes(ctx)
			if err != nil {
				return err
			}

			stvResp = resp
			return nil
		})

		group.Go(func() error {
			resp, err := s.bttvEmotes.GetGlobalEmotes(ctx)
			if err != nil {
				return err
			}

			bttvResp = resp
			return nil
		})

		if err := group.Wait(); err != nil {
			return nil, err
		}

		emoteSet := make(EmoteSet, 0, len(ttvResp.Data)+len(stvResp.Emotes)+len(bttvResp))
		for _, ttvEmote := range ttvResp.Data {
			emoteSet = append(emoteSet, Emote{
				ID:       ttvEmote.ID,
				Text:     ttvEmote.Name,
				Platform: Twitch,
				URL:      ttvEmote.Images.URL1X,
			})
		}

		for _, stvEmote := range stvResp.Emotes {
			var url string
			if stvEmote.Data.Animated {
				url = fmt.Sprintf("%s/1x.avif", stvEmote.Data.Host.URL)
			} else {
				url = fmt.Sprintf("%s/1x.webp", stvEmote.Data.Host.URL)
			}
			url, _ = strings.CutPrefix(url, "//")
			url = "https://" + url

			log.Logger.Info().Str("text", stvEmote.Name).Send()
			emoteSet = append(emoteSet, Emote{
				ID:         stvEmote.ID,
				Text:       stvEmote.Name,
				IsAnimated: stvEmote.Data.Animated,
				Platform:   SevenTV,
				URL:        url,
			})
		}

		for _, bttvEmote := range bttvResp {
			emoteSet = append(emoteSet, Emote{
				ID:         bttvEmote.ID,
				Text:       bttvEmote.Code,
				Platform:   BTTV,
				IsAnimated: bttvEmote.Animated,
				Format:     bttvEmote.ImageType,
				URL:        fmt.Sprintf("https://cdn.betterttv.net/emote/%s/1x", bttvEmote.ID),
			})
		}

		return emoteSet, nil
	})

	if err != nil {
		return err
	}

	log.Logger.Info().Bool("shared", shared).Msg("refreshed global emote set channel")

	s.m.Lock()
	defer s.m.Unlock()
	s.globalFetched = true
	s.global = set.(EmoteSet)

	return nil
}

// GetAllForChannel retrieves all emotes for a specific user.
func (s *Cache) GetAllForChannel(id string) EmoteSet {
	s.m.RLock()
	defer s.m.RUnlock()

	userEmotes := s.channel[id]
	data := make(EmoteSet, 0, len(s.global)+len(userEmotes))

	data = append(data, userEmotes...)
	data = append(data, s.global...)

	return data
}

func (s *Cache) GetAll() EmoteSet {
	s.m.RLock()
	defer s.m.RUnlock()

	var lenUserEmotes int
	for _, lc := range s.channel {
		lenUserEmotes += len(lc)
	}

	fmtEmoteKey := func(e Emote) string {
		return fmt.Sprintf("%s.%s", e.Platform.String(), e.ID)
	}

	unique := make(map[string]Emote, len(s.global)+lenUserEmotes)

	for _, emotes := range s.channel {
		for _, e := range emotes {
			unique[fmtEmoteKey(e)] = e
		}
	}

	for _, e := range s.global {
		unique[fmtEmoteKey(e)] = e
	}

	set := make(EmoteSet, 0, len(unique))

	for _, e := range unique {
		set = append(set, e)
	}

	return set
}

func (s *Cache) GetByTextAllChannels(text string) (Emote, bool) {
	s.m.RLock()
	defer s.m.RUnlock()

	if emote, ok := s.global.GetByText(text); ok {
		return emote, true
	}

	for _, channelSet := range s.channel {
		if emote, ok := channelSet.GetByText(text); ok {
			return emote, true
		}
	}

	for _, userSet := range s.user {
		if emote, ok := userSet.GetByText(text); ok {
			return emote, true
		}
	}

	if emote, ok := s.foreignEmotes[text]; ok {
		return emote, true
	}

	return Emote{}, false
}

func (s *Cache) GetByText(channelID, text string) (Emote, bool) {
	s.m.RLock()
	defer s.m.RUnlock()

	if emote, ok := s.global.GetByText(text); ok {
		return emote, true
	}

	for _, userSet := range s.user {
		if emote, ok := userSet.GetByText(text); ok {
			return emote, true
		}
	}

	channelSet, ok := s.channel[channelID]

	if !ok {
		return Emote{}, false
	}

	if emote, ok := channelSet.GetByText(text); ok {
		return emote, true
	}

	return Emote{}, false
}

func (s *Cache) AllEmotesUsableByUser(userID string) []Emote {
	s.m.RLock()
	defer s.m.RUnlock()

	copied := make([]Emote, len(s.user[userID])+len(s.global))
	copy(copied, s.user[userID])

	for _, e := range s.global {
		if e.Platform != Twitch {
			copied = append(copied, e)
		}
	}
	return copied
}

func (s *Cache) RemoveEmoteSetForChannel(channelID string) {
	s.m.Lock()
	defer s.m.Unlock()

	delete(s.channel, channelID)
	delete(s.channelsFetched, channelID)
}

func (s *Cache) AddUserEmotes(userID string, emotes []Emote) {
	s.m.Lock()
	defer s.m.Unlock()

	log.Logger.Info().Str("user-id", userID).Msg("added emote for user to storage")
	s.user[userID] = append(s.user[userID], emotes...)
}

func (s *Cache) LoadSetForeignEmote(emoteID, emoteText string) Emote {
	s.m.RLock()

	// emote was already added, reuse
	if e, ok := s.foreignEmotes[emoteID]; ok {
		s.m.RUnlock()
		return e
	}
	s.m.RUnlock()

	// fake new emote entry, since we can't ask the API for a single emote, but also can't infer
	// the channelID or the channel name of a sub emote
	e := Emote{
		ID:       emoteID,
		Text:     emoteText,
		Platform: Twitch, // only supported by twitch
		URL:      fmt.Sprintf("https://static-cdn.jtvnw.net/emoticons/v2/%s/default/light/1.0", emoteID),
	}

	s.m.Lock()
	defer s.m.Unlock()
	s.foreignEmotes[emoteID] = e

	return e
}
