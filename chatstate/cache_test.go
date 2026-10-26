package chatstate_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func ptr[T any](v T) *T { return &v }

// TestCache_FetchRequestsOncePerBroadcaster covers the reason the cache exists:
// the same channel opened in several tabs, or by the startup prefetch and a tab,
// results in one request.
func TestCache_FetchRequestsOncePerBroadcaster(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	full := twitchapi.ChatSettingData{BroadcasterID: "channel-id", SlowMode: true, SlowModeWaitTime: 30}

	var (
		calls   atomic.Int64
		release = make(chan struct{})
		wg      sync.WaitGroup
	)

	fetch := func(context.Context) (twitchapi.ChatSettingData, error) {
		calls.Add(1)
		<-release

		return full, nil
	}

	results := make([]twitchapi.ChatSettingData, 8)
	for i := range results {
		wg.Add(1)

		go func() {
			defer wg.Done()

			settings, err := cache.Fetch(t.Context(), "channel-id", fetch)
			require.NoError(t, err)

			results[i] = settings
		}()
	}

	// Let every caller join the in-flight request before it completes.
	require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond)
	close(release)
	wg.Wait()

	require.EqualValues(t, 1, calls.Load(), "concurrent callers share one request")
	for _, settings := range results {
		require.Equal(t, full, settings)
	}

	cached, ok := cache.Get("channel-id")
	require.True(t, ok)
	require.Equal(t, full, cached)

	_, err := cache.Fetch(t.Context(), "channel-id", func(context.Context) (twitchapi.ChatSettingData, error) {
		t.Fatal("cached settings must not be fetched again")

		return twitchapi.ChatSettingData{}, nil
	})
	require.NoError(t, err)
}

// TestCache_FetchDoesNotCacheFailures keeps a failed request retryable instead of
// pinning the channel to an empty state for the session.
func TestCache_FetchDoesNotCacheFailures(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()

	_, err := cache.Fetch(t.Context(), "channel-id", func(context.Context) (twitchapi.ChatSettingData, error) {
		return twitchapi.ChatSettingData{}, errors.New("twitch down")
	})
	require.ErrorContains(t, err, "twitch down")

	_, ok := cache.Get("channel-id")
	require.False(t, ok, "errors are not cached")

	settings, err := cache.Fetch(t.Context(), "channel-id", func(context.Context) (twitchapi.ChatSettingData, error) {
		return twitchapi.ChatSettingData{SlowMode: true}, nil
	})
	require.NoError(t, err)
	require.True(t, settings.SlowMode)
}

// TestCache_ApplyRoomStateKeepsUnmentionedSettings covers the partial nature of
// ROOMSTATE: it must update the settings it mentions and leave the rest alone.
func TestCache_ApplyRoomStateKeepsUnmentionedSettings(t *testing.T) {
	t.Parallel()

	cache := chatstate.NewCache()
	cache.Put("channel-id", twitchapi.ChatSettingData{
		FollowerMode:         true,
		FollowerModeDuration: 30,
		EmoteMode:            true,
	})

	// The join message only reports slow mode.
	settings := cache.ApplyRoomState("channel-id", &twitchirc.RoomState{
		RoomID: "channel-id",
		Slow:   ptr(10),
	})

	require.True(t, settings.SlowMode)
	require.Equal(t, 10, settings.SlowModeWaitTime)
	require.True(t, settings.FollowerMode, "an unmentioned setting keeps its known value")
	require.Equal(t, 30, settings.FollowerModeDuration)
	require.True(t, settings.EmoteMode)

	cached, ok := cache.Get("channel-id")
	require.True(t, ok)
	require.Equal(t, settings, cached)

	// A later message that disables follower mode is applied.
	settings = cache.ApplyRoomState("channel-id", &twitchirc.RoomState{FollowersOnly: ptr(-1)})
	require.False(t, settings.FollowerMode)
	require.Zero(t, settings.FollowerModeDuration)
	require.True(t, settings.SlowMode, "still the slow mode from the previous message")
}

// applyRoomState merges a ROOMSTATE into settings the channel is already known to
// have, the way a tab does when the join tags arrive.
func applyRoomState(known twitchapi.ChatSettingData, state *twitchirc.RoomState) twitchapi.ChatSettingData {
	cache := chatstate.NewCache()
	cache.Put("chan-id", known)

	return cache.ApplyRoomState("chan-id", state)
}

// TestCache_ApplyRoomStateOnlyAppliesSentTags pins the mapping between IRC tags and
// the API's settings fields, including the "off" values.
func TestMerge_OnlyAppliesSentTags(t *testing.T) {
	t.Parallel()

	t.Run("empty room state changes nothing", func(t *testing.T) {
		t.Parallel()

		known := twitchapi.ChatSettingData{
			BroadcasterID: "chan-id",
			SlowMode:      true, SlowModeWaitTime: 5,
			FollowerMode: true, FollowerModeDuration: 15,
			SubscriberMode: true, EmoteMode: true, UniqueChatMode: true,
		}

		require.Equal(t, known, applyRoomState(known, &twitchirc.RoomState{}))
	})

	t.Run("every tag is mapped", func(t *testing.T) {
		t.Parallel()

		settings := applyRoomState(twitchapi.ChatSettingData{}, &twitchirc.RoomState{
			EmoteOnly:     ptr(true),
			SubsOnly:      ptr(true),
			R9K:           ptr(true),
			Slow:          ptr(120),
			FollowersOnly: ptr(30),
		})

		require.True(t, settings.EmoteMode)
		require.True(t, settings.SubscriberMode)
		require.True(t, settings.UniqueChatMode)
		require.True(t, settings.SlowMode)
		require.Equal(t, 120, settings.SlowModeWaitTime)
		require.True(t, settings.FollowerMode)
		require.Equal(t, 30, settings.FollowerModeDuration)
	})

	t.Run("disabled values that are not zero", func(t *testing.T) {
		t.Parallel()

		settings := applyRoomState(
			twitchapi.ChatSettingData{SlowMode: true, SlowModeWaitTime: 30, FollowerMode: true, FollowerModeDuration: 10},
			&twitchirc.RoomState{Slow: ptr(0), FollowersOnly: ptr(-1)},
		)

		require.False(t, settings.SlowMode)
		require.Zero(t, settings.SlowModeWaitTime)
		require.False(t, settings.FollowerMode)
		require.Zero(t, settings.FollowerModeDuration)
	})

	t.Run("any follower", func(t *testing.T) {
		t.Parallel()

		settings := applyRoomState(twitchapi.ChatSettingData{}, &twitchirc.RoomState{FollowersOnly: ptr(0)})
		require.True(t, settings.FollowerMode)
		require.Zero(t, settings.FollowerModeDuration)
	})

	t.Run("nil room state", func(t *testing.T) {
		t.Parallel()

		known := twitchapi.ChatSettingData{BroadcasterID: "chan-id", SlowMode: true}
		require.Equal(t, known, applyRoomState(known, nil))
	})
}
