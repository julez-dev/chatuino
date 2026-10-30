package emote_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/twitch/bttv"
	"github.com/julez-dev/chatuino/twitch/ffz"
	"github.com/julez-dev/chatuino/twitch/seventv"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// stubFetcher stands in for one emote provider. The cache only needs providers
// to answer with a fixed value, so a struct with function fields replaces the
// generated mocks; call counters keep the "one fetch per refresh" contract.
type stubFetcher[G, C any] struct {
	globalEmotes  func(context.Context) (G, error)
	channelEmotes func(context.Context, string) (C, error)

	globalCalls  atomic.Int32
	channelCalls atomic.Int32
}

func (s *stubFetcher[G, C]) GetGlobalEmotes(ctx context.Context) (G, error) {
	s.globalCalls.Add(1)
	if s.globalEmotes == nil {
		var zero G
		return zero, nil
	}

	return s.globalEmotes(ctx)
}

func (s *stubFetcher[G, C]) GetChannelEmotes(ctx context.Context, broadcaster string) (C, error) {
	s.channelCalls.Add(1)
	if s.channelEmotes == nil {
		var zero C
		return zero, nil
	}

	return s.channelEmotes(ctx, broadcaster)
}

func newStubs() (
	*stubFetcher[twitchapi.EmoteResponse, twitchapi.EmoteResponse],
	*stubFetcher[seventv.EmoteResponse, seventv.ChannelEmoteResponse],
	*stubFetcher[bttv.GlobalEmoteResponse, bttv.UserResponse],
	*stubFetcher[[]ffz.Emote, []ffz.Emote],
) {
	return &stubFetcher[twitchapi.EmoteResponse, twitchapi.EmoteResponse]{},
		&stubFetcher[seventv.EmoteResponse, seventv.ChannelEmoteResponse]{},
		&stubFetcher[bttv.GlobalEmoteResponse, bttv.UserResponse]{},
		&stubFetcher[[]ffz.Emote, []ffz.Emote]{}
}

// answerGlobal and answerChannel ignore the context and always answer the same.
func answerGlobal[T any](v T, err error) func(context.Context) (T, error) {
	return func(context.Context) (T, error) { return v, err }
}

func answerChannel[T any](v T, err error) func(context.Context, string) (T, error) {
	return func(context.Context, string) (T, error) { return v, err }
}

func TestCache_RefreshCancelledWaiterDoesNotCancelSharedFetch(t *testing.T) {
	t.Parallel()

	for _, scope := range []string{"channel", "global"} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()

			store, started, release, providerCalls := newBlockedEmoteRefresh(t, scope)
			refresh := func(ctx context.Context) error {
				if scope == "global" {
					return store.RefreshGlobal(ctx)
				}
				return store.RefreshLocal(ctx, "shared-channel")
			}
			leader := make(chan error, 1)
			go func() { leader <- refresh(t.Context()) }()
			var sharedCtx context.Context
			select {
			case sharedCtx = <-started:
			case <-time.After(time.Second):
				t.Fatal("shared refresh did not start")
			}

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			follower := make(chan error, 1)
			go func() { follower <- refresh(ctx) }()
			returned := false
			select {
			case err := <-follower:
				returned = true
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(time.Second):
				t.Error("cancelled waiter remained blocked on another caller's refresh")
			}
			select {
			case <-sharedCtx.Done():
				t.Error("cancelling a waiter cancelled the shared fetch")
			default:
			}

			release()
			select {
			case err := <-leader:
				require.NoError(t, err, "the original caller must still receive its emotes")
			case <-time.After(time.Second):
				t.Fatal("original refresh did not finish")
			}
			if !returned {
				select {
				case <-follower:
				case <-time.After(time.Second):
					t.Fatal("waiter did not finish after releasing the shared fetch")
				}
			}
			_, ok := store.GetAllForChannel("shared-channel").GetByText("SharedEmote")
			require.True(t, ok, "the successful refresh must populate the cache")
			require.EqualValues(t, 1, providerCalls.Load(), "coalesced refreshes must reach the provider once")
		})
	}
}

// The fetchers are external API boundaries; the cache and its coalescing remain real.
func newBlockedEmoteRefresh(t *testing.T, scope string) (*emote.Cache, <-chan context.Context, func(), *atomic.Int32) {
	t.Helper()

	ttv, seven, bttvService, ffzService := newStubs()
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	releaseRefresh := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseRefresh)

	var providerCalls atomic.Int32
	fetch := func(ctx context.Context) (twitchapi.EmoteResponse, error) {
		providerCalls.Add(1)
		started <- ctx
		select {
		case <-release:
			return twitchapi.EmoteResponse{Data: []twitchapi.EmoteData{{ID: "shared", Name: "SharedEmote"}}}, nil
		case <-ctx.Done():
			return twitchapi.EmoteResponse{}, ctx.Err()
		}
	}

	if scope == "global" {
		ttv.globalEmotes = fetch
	} else {
		ttv.channelEmotes = func(ctx context.Context, _ string) (twitchapi.EmoteResponse, error) {
			return fetch(ctx)
		}
	}

	return emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService), started, releaseRefresh, &providerCalls
}

func TestRefreshLocal(t *testing.T) {
	ttv, seven, bttvService, ffzService := newStubs()

	ttv.channelEmotes = answerChannel(twitchapi.EmoteResponse{
		Data: []twitchapi.EmoteData{
			{
				ID:   "test",
				Name: "Kappa",
			},
		},
	}, nil)

	seven.channelEmotes = answerChannel(seventv.ChannelEmoteResponse{
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

	bttvService.channelEmotes = answerChannel(bttv.UserResponse{
		ChannelEmotes: []bttv.Emote{
			{
				ID:   "test-bttv",
				Code: "BTTV-emote",
			},
		},
	}, nil)

	ffzService.channelEmotes = answerChannel([]ffz.Emote{
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

	require.EqualValues(t, 1, ttv.channelCalls.Load(), "a cached channel is not fetched twice")
	require.EqualValues(t, 1, ffzService.channelCalls.Load())
}

func TestRefreshGlobal_3rdPartyFailureNonBlocking(t *testing.T) {
	t.Parallel()

	t.Run("7TV failure does not block", func(t *testing.T) {
		t.Parallel()

		ttv, seven, bttvService, ffzService := newStubs()

		ttv.globalEmotes = answerGlobal(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.globalEmotes = answerGlobal(seventv.EmoteResponse{}, seventv.APIError{StatusCode: 500})

		bttvService.globalEmotes = answerGlobal(bttv.GlobalEmoteResponse{
			{ID: "bttv-global", Code: "GlobalBTTVEmote"},
		}, nil)

		ffzService.globalEmotes = answerGlobal([]ffz.Emote{
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

		ttv, seven, bttvService, ffzService := newStubs()

		ttv.globalEmotes = answerGlobal(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.globalEmotes = answerGlobal(seventv.EmoteResponse{
			Emotes: []seventv.Emote{
				{ID: "7tv-global", Name: "Global7TVEmote"},
			},
		}, nil)

		bttvService.globalEmotes = answerGlobal(bttv.GlobalEmoteResponse{}, bttv.APIError{StatusCode: 503})

		ffzService.globalEmotes = answerGlobal([]ffz.Emote{
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

		ttv, seven, bttvService, ffzService := newStubs()

		ttv.globalEmotes = answerGlobal(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.globalEmotes = answerGlobal(seventv.EmoteResponse{
			Emotes: []seventv.Emote{
				{ID: "7tv-global", Name: "Global7TVEmote"},
			},
		}, nil)

		bttvService.globalEmotes = answerGlobal(bttv.GlobalEmoteResponse{
			{ID: "bttv-global", Code: "GlobalBTTVEmote"},
		}, nil)

		ffzService.globalEmotes = answerGlobal([]ffz.Emote(nil), ffz.APIError{StatusCode: 500})

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

		ttv, seven, bttvService, ffzService := newStubs()

		ttv.globalEmotes = answerGlobal(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{
				{ID: "ttv-global", Name: "GlobalTwitchEmote"},
			},
		}, nil)

		seven.globalEmotes = answerGlobal(seventv.EmoteResponse{}, seventv.APIError{StatusCode: 500})

		bttvService.globalEmotes = answerGlobal(bttv.GlobalEmoteResponse{}, bttv.APIError{StatusCode: 500})

		ffzService.globalEmotes = answerGlobal([]ffz.Emote(nil), ffz.APIError{StatusCode: 500})

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshGlobal(context.Background())
		require.NoError(t, err)

		set := store.GetAllForChannel("")
		_, ok := set.GetByText("GlobalTwitchEmote")
		require.True(t, ok)
	})
}

// TestRefreshLocal_FailuresArePartial keeps emote data cosmetic: a provider that
// answers with an error, and a refresh that runs out of time, must both be reported
// as a partial fetch so a tab degrades instead of showing an error for its chat.
func TestRefreshLocal_FailuresArePartial(t *testing.T) {
	t.Parallel()

	t.Run("missing channel emotes are not a failure", func(t *testing.T) {
		t.Parallel()

		ttv, seven, bttvService, ffzService := newStubs()

		ttv.channelEmotes = answerChannel(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{{ID: "ttv", Name: "TwitchEmote"}},
		}, nil)
		// A channel without BTTV/FFZ emotes answers with 404, which is normal.
		bttvService.channelEmotes = answerChannel(bttv.UserResponse{}, bttv.APIError{StatusCode: 404})
		ffzService.channelEmotes = answerChannel([]ffz.Emote(nil), ffz.APIError{StatusCode: 404})

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshLocal(t.Context(), "no-emotes-channel")
		require.NoError(t, err, "a provider without emotes for the channel is not a partial fetch")

		_, ok := store.GetAllForChannel("no-emotes-channel").GetByText("TwitchEmote")
		require.True(t, ok)
	})

	t.Run("provider error keeps the other providers", func(t *testing.T) {
		t.Parallel()

		ttv, seven, bttvService, ffzService := newStubs()

		ttv.channelEmotes = answerChannel(twitchapi.EmoteResponse{
			Data: []twitchapi.EmoteData{{ID: "ttv", Name: "TwitchEmote"}},
		}, nil)
		bttvService.channelEmotes = answerChannel(bttv.UserResponse{}, bttv.APIError{StatusCode: 500})

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		err := store.RefreshLocal(t.Context(), "partial-channel")
		require.ErrorIs(t, err, emote.ErrPartialFetch)

		_, ok := store.GetAllForChannel("partial-channel").GetByText("TwitchEmote")
		require.True(t, ok, "the providers that answered are still cached")
	})

	t.Run("a refresh that ran out of time is partial and caches nothing", func(t *testing.T) {
		t.Parallel()

		ttv, seven, bttvService, ffzService := newStubs()

		// The authoritative provider never answers, so the refresh as a whole fails.
		// Whatever the others returned in the meantime was never confirmed, so it must
		// not be cached under this channel: the next open retries the whole thing.
		ttv.channelEmotes = func(ctx context.Context, _ string) (twitchapi.EmoteResponse, error) {
			<-ctx.Done()

			return twitchapi.EmoteResponse{}, ctx.Err()
		}
		bttvService.channelEmotes = answerChannel(bttv.UserResponse{
			ChannelEmotes: []bttv.Emote{{ID: "bttv", Code: "BttvEmote"}},
		}, nil)

		store := emote.NewCache(zerolog.Nop(), ttv, seven, bttvService, ffzService)

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()

		err := store.RefreshLocal(ctx, "slow-channel")
		require.ErrorIs(t, err, emote.ErrPartialFetch, "a timed out refresh must not read as a fatal error")
		require.ErrorIs(t, err, context.DeadlineExceeded)

		_, ok := store.GetAllForChannel("slow-channel").GetByText("BttvEmote")
		require.False(t, ok, "a refresh that never finished caches nothing")
	})
}
