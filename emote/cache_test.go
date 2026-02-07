package emote_test

import (
	"context"
	"testing"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/mocks"
	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/ffz"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRefreshLocal(t *testing.T) {
	ttv := mocks.NewMockTwitchEmoteFetcher(t)
	seven := mocks.NewMockSevenTVEmoteFetcher(t)
	bttvService := mocks.NewMockBTTVEmoteFetcher(t)
	ffzService := mocks.NewMockFFZEmoteFetcher(t)

	ttv.EXPECT().GetChannelEmotes(mock.Anything, "test-channel").Once().Return(twitchapi.EmoteResponse{
		Data: []twitchapi.EmoteData{
			{
				ID:   "test",
				Name: "Kappa",
			},
		},
	}, nil)

	seven.EXPECT().GetChannelEmotes(mock.Anything, "test-channel").Once().Return(seventv.ChannelEmoteResponse{
		EmoteSet: struct {
			Emotes []seventv.Emote `json:"emotes"`
		}{
			Emotes: []seventv.Emote{
				{
					ID:   "seven-id",
					Name: "Kappa-seven",
					Data: seventv.EmoteData{
						Host: seventv.Host{
							Files: []seventv.Files{
								{
									Name: "test",
								},
							},
						},
					},
				},
			},
		},
	}, nil)

	bttvService.EXPECT().GetChannelEmotes(mock.Anything, "test-channel").Once().Return(bttv.UserResponse{
		ChannelEmotes: []bttv.Emote{
			{
				ID:   "test-bttv",
				Code: "BTTV-emote",
			},
		},
	}, nil)

	ffzService.EXPECT().GetChannelEmotes(mock.Anything, "test-channel").Once().Return([]ffz.Emote{
		{
			ID:   123,
			Name: "FFZ-emote",
			URLs: map[string]string{"1": "https://cdn.frankerfacez.com/emote/123/1"},
		},
	}, nil)

	store := emote.NewCache(
		zerolog.Nop(),
		ttv,
		seven,
		bttvService,
		ffzService,
	)

	// first call
	err := store.RefreshLocal(context.Background(), "test-channel")
	require.Nil(t, err)

	set := store.GetAllForChannel("test-channel")
	_, ok := set.GetByText("Kappa")
	require.True(t, ok)
	_, ok = set.GetByText("FFZ-emote")
	require.True(t, ok)

	// second call
	err = store.RefreshLocal(context.Background(), "test-channel")
	require.Nil(t, err)
}

func TestRefreshGlobal_3rdPartyFailureNonBlocking(t *testing.T) {
	t.Parallel()

	t.Run("7TV failure does not block", func(t *testing.T) {
		t.Parallel()

		ttv := mocks.NewMockTwitchEmoteFetcher(t)
		seven := mocks.NewMockSevenTVEmoteFetcher(t)
		bttvService := mocks.NewMockBTTVEmoteFetcher(t)
		ffzService := mocks.NewMockFFZEmoteFetcher(t)

		ttv.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(
			seventv.EmoteResponse{}, seventv.APIError{StatusCode: 500})

		bttvService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(bttv.GlobalEmoteResponse{
			{ID: "bttv-global", Code: "GlobalBTTVEmote"},
		}, nil)

		ffzService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return([]ffz.Emote{
			{ID: 1, Name: "GlobalFFZEmote"},
		}, nil)

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshGlobal(context.Background())
		require.NoError(t, err)

		set := store.GetAllForChannel("")
		_, ok := set.GetByText("GlobalTwitchEmote")
		require.True(t, ok)
		_, ok = set.GetByText("GlobalBTTVEmote")
		require.True(t, ok)
	})

	t.Run("BTTV failure does not block", func(t *testing.T) {
		t.Parallel()

		ttv := mocks.NewMockTwitchEmoteFetcher(t)
		seven := mocks.NewMockSevenTVEmoteFetcher(t)
		bttvService := mocks.NewMockBTTVEmoteFetcher(t)
		ffzService := mocks.NewMockFFZEmoteFetcher(t)

		ttv.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(seventv.EmoteResponse{
			Emotes: []seventv.Emote{
				{ID: "7tv-global", Name: "Global7TVEmote"},
			},
		}, nil)

		bttvService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(
			bttv.GlobalEmoteResponse{}, bttv.APIError{StatusCode: 503})

		ffzService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return([]ffz.Emote{
			{ID: 1, Name: "GlobalFFZEmote"},
		}, nil)

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshGlobal(context.Background())
		require.NoError(t, err)

		set := store.GetAllForChannel("")
		_, ok := set.GetByText("GlobalTwitchEmote")
		require.True(t, ok)
		_, ok = set.GetByText("Global7TVEmote")
		require.True(t, ok)
	})

	t.Run("FFZ failure does not block", func(t *testing.T) {
		t.Parallel()

		ttv := mocks.NewMockTwitchEmoteFetcher(t)
		seven := mocks.NewMockSevenTVEmoteFetcher(t)
		bttvService := mocks.NewMockBTTVEmoteFetcher(t)
		ffzService := mocks.NewMockFFZEmoteFetcher(t)

		ttv.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(seventv.EmoteResponse{
			Emotes: []seventv.Emote{
				{ID: "7tv-global", Name: "Global7TVEmote"},
			},
		}, nil)

		bttvService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(bttv.GlobalEmoteResponse{
			{ID: "bttv-global", Code: "GlobalBTTVEmote"},
		}, nil)

		ffzService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(
			[]ffz.Emote(nil), ffz.APIError{StatusCode: 500})

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshGlobal(context.Background())
		require.NoError(t, err)

		set := store.GetAllForChannel("")
		_, ok := set.GetByText("GlobalTwitchEmote")
		require.True(t, ok)
		_, ok = set.GetByText("Global7TVEmote")
		require.True(t, ok)
		_, ok = set.GetByText("GlobalBTTVEmote")
		require.True(t, ok)
	})

	t.Run("all 3rd parties fail, Twitch succeeds", func(t *testing.T) {
		t.Parallel()

		ttv := mocks.NewMockTwitchEmoteFetcher(t)
		seven := mocks.NewMockSevenTVEmoteFetcher(t)
		bttvService := mocks.NewMockBTTVEmoteFetcher(t)
		ffzService := mocks.NewMockFFZEmoteFetcher(t)

		ttv.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(
			seventv.EmoteResponse{}, seventv.APIError{StatusCode: 500})

		bttvService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(
			bttv.GlobalEmoteResponse{}, bttv.APIError{StatusCode: 500})

		ffzService.EXPECT().GetGlobalEmotes(mock.Anything).Once().Return(
			[]ffz.Emote(nil), ffz.APIError{StatusCode: 500})

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshGlobal(context.Background())
		require.NoError(t, err)

		set := store.GetAllForChannel("")
		_, ok := set.GetByText("GlobalTwitchEmote")
		require.True(t, ok)
	})
}
