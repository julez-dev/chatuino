package emote

import (
	"context"
	"errors"
	"fmt"
	"github.com/julez-dev/chatuino/bttv"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/julez-dev/chatuino/seventv"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
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

type Store struct {
	logger  zerolog.Logger
	m       *sync.RWMutex
	global  EmoteSet
	channel map[string]EmoteSet

	twitchEmotes  TwitchEmoteFetcher
	sevenTVEmotes SevenTVEmoteFetcher
	bttvEmotes    BTTVEmoteFetcher
}

func NewStore(logger zerolog.Logger, twitchEmotes TwitchEmoteFetcher, sevenTVEmotes SevenTVEmoteFetcher, bttvEmotes BTTVEmoteFetcher) Store {
	return Store{
		logger:        logger,
		m:             &sync.RWMutex{},
		channel:       map[string]EmoteSet{},
		twitchEmotes:  twitchEmotes,
		sevenTVEmotes: sevenTVEmotes,
		bttvEmotes:    bttvEmotes,
	}
}

func (s *Store) GetAllForUser(id string) EmoteSet {
	s.m.RLock()
	defer s.m.RUnlock()

	userEmotes := s.channel[id]
	data := make(EmoteSet, 0, len(s.global)+len(userEmotes))

	data = append(data, userEmotes...)
	data = append(data, s.global...)

	return data
}

func (s *Store) GetByText(channelID, text string) (Emote, bool) {
	s.m.RLock()
	defer s.m.RUnlock()

	if emote, ok := s.global.GetByText(text); ok {
		return emote, true
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

func (s *Store) RefreshLocal(ctx context.Context, channelID string) error {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.channel, channelID)

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
			s.logger.Error().Str("channel_id", channelID).Err(err).Msg("could not fetch BTTV emotes")

			var apiErr bttv.APIError
			if errors.As(err, &apiErr) {
				if apiErr.StatusCode == http.StatusNotFound {
					return nil
				}
			}

			return err
		}

		bttvResp = resp

		return nil
	})

	if err := group.Wait(); err != nil {
		return err
	}

	s.channel[channelID] = make(EmoteSet, 0, len(ttvResp.Data)+len(stvResp.EmoteSet.Emotes)+len(bttvResp.ChannelEmotes))

	for _, ttvEmote := range ttvResp.Data {
		s.channel[channelID] = append(s.channel[channelID], Emote{
			ID:       ttvEmote.ID,
			Text:     ttvEmote.Name,
			Platform: Twitch,
			URL:      ttvEmote.Images.URL1X,
		})
	}

	for _, bttvEmote := range bttvResp.ChannelEmotes {
		s.channel[channelID] = append(s.channel[channelID], Emote{
			ID:       bttvEmote.ID,
			Text:     bttvEmote.Code,
			Platform: BTTV,
			URL:      fmt.Sprintf("https://betterttv.com/emotes/%s", bttvEmote.ID),
		})
	}

	for _, stvEmote := range stvResp.EmoteSet.Emotes {
		url := fmt.Sprintf("%s/%s", stvEmote.Data.Host.URL, stvEmote.Data.Host.Files[0].Name)
		url, _ = strings.CutPrefix(url, "//")
		url = "https://" + url

		s.channel[channelID] = append(s.channel[channelID], Emote{
			ID:       stvEmote.ID,
			Text:     stvEmote.Name,
			Platform: SevenTV,
			URL:      url,
		})
	}

	return nil
}

func (s *Store) RefreshGlobal(ctx context.Context) error {
	s.m.Lock()
	defer s.m.Unlock()

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
		return err
	}

	s.global = nil
	s.global = make(EmoteSet, 0, len(ttvResp.Data)+len(stvResp.Emotes))

	for _, ttvEmote := range ttvResp.Data {
		s.global = append(s.global, Emote{
			ID:       ttvEmote.ID,
			Text:     ttvEmote.Name,
			Platform: Twitch,
			URL:      ttvEmote.Images.URL1X,
		})
	}

	for _, bttvEmote := range bttvResp {
		s.global = append(s.global, Emote{
			ID:       bttvEmote.ID,
			Text:     bttvEmote.Code,
			Platform: BTTV,
			URL:      fmt.Sprintf("https://betterttv.com/emotes/%s", bttvEmote.ID),
		})
	}

	for _, stvEmote := range stvResp.Emotes {
		url := fmt.Sprintf("%s/%s", stvEmote.Data.Host.URL, stvEmote.Data.Host.Files[0].Name)
		url, _ = strings.CutPrefix(url, "//")
		url = "https://" + url

		s.global = append(s.global, Emote{
			ID:       stvEmote.ID,
			Text:     stvEmote.Name,
			Platform: SevenTV,
			URL:      url,
		})
	}

	return nil
}
