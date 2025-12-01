package emote_test

import (
	"context"
	"testing"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/mocks"
	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRefreshLocal(t *testing.T) {
	ttv := mocks.NewMockTwitchEmoteFetcher(t)
	seven := mocks.NewMockSevenTVEmoteFetcher(t)
	bttvService := mocks.NewMockBTTVEmoteFetcher(t)

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

	store := emote.NewCache(
		zerolog.Nop(),
		ttv,
		seven,
		bttvService,
	)

	// first call
	err := store.RefreshLocal(context.Background(), "test-channel")
	assert.Nil(t, err)

	set := store.GetAllForChannel("test-channel")
	_, ok := set.GetByText("Kappa")
	assert.True(t, ok)

	// second call
	err = store.RefreshLocal(context.Background(), "test-channel")
	assert.Nil(t, err)
}
