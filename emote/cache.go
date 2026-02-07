package emote

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/ffz"
	"github.com/julez-dev/chatuino/twitch/twitchapi"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var ErrPartialFetch = errors.New("emote data could only be partially fetched")

type TwitchEmoteFetcher interface {
	GetGlobalEmotes(context.Context) (twitchapi.EmoteResponse, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) (twitchapi.EmoteResponse, error)
}

type SevenTVEmoteFetcher interface {
	GetGlobalEmotes(context.Context) (seventv.EmoteResponse, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) (seventv.ChannelEmoteResponse, error)
}

type BTTVEmoteFetcher interface {
	GetGlobalEmotes(context.Context) (bttv.GlobalEmoteResponse, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) (bttv.UserResponse, error)
}

type FFZEmoteFetcher interface {
	GetGlobalEmotes(context.Context) ([]ffz.Emote, error)
	GetChannelEmotes(ctx context.Context, broadcaster string) ([]ffz.Emote, error)
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
	ffzEmotes     FFZEmoteFetcher

	// supress duplicated calls
	single          *singleflight.Group
	channelsFetched map[string]struct{}
	globalFetched   bool
}

func NewCache(logger zerolog.Logger, twitchEmotes TwitchEmoteFetcher, sevenTVEmotes SevenTVEmoteFetcher, bttvEmotes BTTVEmoteFetcher, ffzEmotes FFZEmoteFetcher) *Cache {
	return &Cache{
		logger:          logger,
		m:               &sync.RWMutex{},
		channel:         map[string]EmoteSet{},
		twitchEmotes:    twitchEmotes,
		sevenTVEmotes:   sevenTVEmotes,
		bttvEmotes:      bttvEmotes,
		ffzEmotes:       ffzEmotes,
		single:          &singleflight.Group{},
		channelsFetched: map[string]struct{}{},
		user:            map[string]EmoteSet{},
		foreignEmotes:   map[string]Emote{},
	}
}

// RefreshLocal refreshes the local emote cache for a specific channel.
// When a 3rd party API fails, the cache will still be refreshed but a ErrPartialFetch will be returned.
func (s *Cache) RefreshLocal(ctx context.Context, channelID string) error {
	s.m.RLock()
	if _, isCached := s.channelsFetched[channelID]; isCached {
		s.m.RUnlock()
		return nil
	}
	s.m.RUnlock()

	set, err, _ := s.single.Do("channel"+channelID, func() (any, error) {
		var (
			ttvResp  twitchapi.EmoteResponse
			stvResp  seventv.ChannelEmoteResponse
			bttvResp bttv.UserResponse
			ffzResp  []ffz.Emote

			fetchErrs  error
			errSevenTV error // routine will not cancel when 3rd party fails
			errBTTV    error
			errFFZ     error
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
				errSevenTV = fmt.Errorf("could not fetch 7TV emotes: %w", err)
				return nil
			}

			stvResp = resp

			return nil
		})

		group.Go(func() error {
			resp, err := s.bttvEmotes.GetChannelEmotes(ctx, channelID)
			if err != nil {
				s.logger.Error().Str("channel_id", channelID).Err(err).Msg("could not fetch BTTV emotes")
				errBTTV = fmt.Errorf("could not fetch BTTV emotes: %w", err)
				return nil
			}

			bttvResp = resp

			return nil
		})

		group.Go(func() error {
			resp, err := s.ffzEmotes.GetChannelEmotes(ctx, channelID)
			if err != nil {
				s.logger.Error().Str("channel_id", channelID).Err(err).Msg("could not fetch FFZ emotes")
				errFFZ = fmt.Errorf("could not fetch FFZ emotes: %w", err)
				return nil
			}

			ffzResp = resp

			return nil
		})

		if err := group.Wait(); err != nil {
			return nil, err
		}

		fetchErrs = errors.Join(errSevenTV, errBTTV, errFFZ)

		emoteSet := make(EmoteSet, 0, len(ttvResp.Data)+len(stvResp.EmoteSet.Emotes)+len(bttvResp.ChannelEmotes)+len(ffzResp))

		for _, ttvEmote := range ttvResp.Data {
			animated := slices.Contains(ttvEmote.Format, "animated")
			emoteSet = append(emoteSet, Emote{
				ID:           ttvEmote.ID,
				Text:         ttvEmote.Name,
				Platform:     Twitch,
				URL:          twitchEmoteURL(ttvEmote.ID, animated),
				IsAnimated:   animated,
				TTVEmoteType: ttvEmote.EmoteType,
			})
		}

		for _, stvEmote := range stvResp.EmoteSet.Emotes {
			filename := pickSevenTVFile(stvEmote.Data.Animated, stvEmote.Data.Host.Files)
			url := fmt.Sprintf("%s/%s", stvEmote.Data.Host.URL, filename)
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
				URL:        bttvEmoteURL(bttvEmote.ID, bttvEmote.Animated),
			})
		}

		for _, bttvEmote := range bttvResp.SharedEmotes {
			emoteSet = append(emoteSet, Emote{
				ID:         bttvEmote.ID,
				Text:       bttvEmote.Code,
				IsAnimated: bttvEmote.Animated,
				Format:     bttvEmote.ImageType,
				Platform:   BTTV,
				URL:        bttvEmoteURL(bttvEmote.ID, bttvEmote.Animated),
			})
		}

		for _, ffzEmote := range ffzResp {
			if ffzEmote.Modifier {
				continue
			}

			emoteSet = append(emoteSet, Emote{
				ID:       strconv.Itoa(ffzEmote.ID),
				Text:     ffzEmote.Name,
				Platform: FFZ,
				URL:      ffzEmoteURL(ffzEmote),
			})
		}

		if fetchErrs != nil {
			return emoteSet, fmt.Errorf("%w: %w", ErrPartialFetch, fetchErrs)
		}

		return emoteSet, nil
	})

	s.m.Lock()
	defer s.m.Unlock()
	s.channelsFetched[channelID] = struct{}{}
	s.channel[channelID] = set.(EmoteSet)

	return err
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
			ttvResp  twitchapi.EmoteResponse
			stvResp  seventv.EmoteResponse
			bttvResp bttv.GlobalEmoteResponse
			ffzResp  []ffz.Emote
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
				s.logger.Error().Err(err).Msg("could not fetch 7TV global emotes")
				return nil
			}

			stvResp = resp
			return nil
		})

		group.Go(func() error {
			resp, err := s.bttvEmotes.GetGlobalEmotes(ctx)
			if err != nil {
				s.logger.Error().Err(err).Msg("could not fetch BTTV global emotes")
				return nil
			}

			bttvResp = resp
			return nil
		})

		group.Go(func() error {
			resp, err := s.ffzEmotes.GetGlobalEmotes(ctx)
			if err != nil {
				s.logger.Error().Err(err).Msg("could not fetch FFZ global emotes")
				return nil
			}

			ffzResp = resp
			return nil
		})

		if err := group.Wait(); err != nil {
			return nil, err
		}

		emoteSet := make(EmoteSet, 0, len(ttvResp.Data)+len(stvResp.Emotes)+len(bttvResp)+len(ffzResp))
		for _, ttvEmote := range ttvResp.Data {
			animated := slices.Contains(ttvEmote.Format, "animated")
			emoteSet = append(emoteSet, Emote{
				ID:           ttvEmote.ID,
				Text:         ttvEmote.Name,
				Platform:     Twitch,
				URL:          twitchEmoteURL(ttvEmote.ID, animated),
				IsAnimated:   animated,
				TTVEmoteType: ttvEmote.EmoteType,
			})
		}

		for _, stvEmote := range stvResp.Emotes {
			filename := pickSevenTVFile(stvEmote.Data.Animated, stvEmote.Data.Host.Files)
			url := fmt.Sprintf("%s/%s", stvEmote.Data.Host.URL, filename)
			url, _ = strings.CutPrefix(url, "//")
			url = "https://" + url

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
				URL:        bttvEmoteURL(bttvEmote.ID, bttvEmote.Animated),
			})
		}

		for _, ffzEmote := range ffzResp {
			if ffzEmote.Modifier {
				continue
			}

			emoteSet = append(emoteSet, Emote{
				ID:       strconv.Itoa(ffzEmote.ID),
				Text:     ffzEmote.Name,
				Platform: FFZ,
				URL:      ffzEmoteURL(ffzEmote),
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
	if e, ok := s.foreignEmotes[emoteText]; ok {
		s.m.RUnlock()
		return e
	}
	s.m.RUnlock()

	// fake new emote entry, since we can't ask the API for a single emote, but also can't infer
	// the channelID or the channel name of a sub emote.
	// Use animated format — Twitch CDN gracefully falls back to static if no animated version exists.
	e := Emote{
		ID:       emoteID,
		Text:     emoteText,
		Platform: Twitch, // only supported by twitch
		URL:      twitchEmoteURL(emoteID, true),
	}

	s.m.Lock()
	defer s.m.Unlock()
	s.foreignEmotes[emoteText] = e

	return e
}

// twitchEmoteURL returns the Twitch CDN URL for an emote.
// Animated emotes use the "animated" format, static emotes use "default".
func twitchEmoteURL(id string, animated bool) string {
	format := "default"
	if animated {
		format = "animated"
	}
	return fmt.Sprintf("https://static-cdn.jtvnw.net/emoticons/v2/%s/%s/light/1.0", id, format)
}

// bttvEmoteURL returns the BTTV CDN URL for an emote.
// Animated emotes use gif, static emotes use png.
func bttvEmoteURL(id string, animated bool) string {
	if animated {
		return fmt.Sprintf("https://cdn.betterttv.net/emote/%s/1x.gif", id)
	}
	return fmt.Sprintf("https://cdn.betterttv.net/emote/%s/1x.png", id)
}

// ffzEmoteURL returns the FFZ CDN URL for an emote.
// Prefers the URL from the API response (urls["1"]), falls back to CDN pattern.
func ffzEmoteURL(emote ffz.Emote) string {
	if url, ok := emote.URLs["1"]; ok {
		return url
	}
	return fmt.Sprintf("https://cdn.frankerfacez.com/emote/%d/1", emote.ID)
}

// pickSevenTVFile selects the best 1x file format from available files.
// For animated emotes: prefers gif > avif > webp
// For static emotes: prefers png > avif > webp
func pickSevenTVFile(animated bool, files []seventv.Files) string {
	var preferred, fallback1, fallback2 string
	if animated {
		preferred, fallback1, fallback2 = "1x.gif", "1x.avif", "1x.webp"
	} else {
		preferred, fallback1, fallback2 = "1x.png", "1x.avif", "1x.webp"
	}

	var hasFallback1, hasFallback2 bool
	for _, f := range files {
		switch f.Name {
		case preferred:
			return preferred
		case fallback1:
			hasFallback1 = true
		case fallback2:
			hasFallback2 = true
		}
	}

	if hasFallback1 {
		return fallback1
	}
	if hasFallback2 {
		return fallback2
	}
	// last resort: return first 1x file found
	for _, f := range files {
		if strings.HasPrefix(f.Name, "1x") {
			return f.Name
		}
	}
	return ""
}
