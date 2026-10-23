package badge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

type mockBadgeCache struct {
	matchBadgeSetFunc func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion
}

func (m *mockBadgeCache) MatchBadgeSet(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
	if m.matchBadgeSetFunc != nil {
		return m.matchBadgeSetFunc(broadcasterID, ircBadge)
	}
	return nil
}

type mockDisplayManager struct {
	convertFunc func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error)
}

func (m *mockDisplayManager) Convert(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
	if m.convertFunc != nil {
		return m.convertFunc(unit)
	}
	return kittyimg.KittyDisplayUnit{}, nil
}

func TestReplacer_Replace(t *testing.T) {
	t.Parallel()

	t.Run("empty badge list returns empty results", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{}
			},
		}

		displayManager := &mockDisplayManager{}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		prepare, formatted, err := replacer.Replace(t.Context(), "broadcaster123", []twitchirc.Badge{})

		require.NoError(t, err)
		require.Empty(t, prepare)
		require.Empty(t, formatted)
	})

	t.Run("single badge returns correct results", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{
					"subscriber": {
						ID:           "1",
						Image_URL_1x: "https://example.com/badge1.png",
						Title:        "Subscriber",
					},
				}
			},
		}

		displayManager := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "prepare_command_1",
					ReplacementText: "replacement_1",
				}, nil
			},
		}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		badgeList := []twitchirc.Badge{
			{Name: "subscriber", Version: "1"},
		}

		prepare, formatted, err := replacer.Replace(t.Context(), "broadcaster123", badgeList)

		require.NoError(t, err)
		require.Equal(t, "prepare_command_1", prepare)
		require.Len(t, formatted, 1)
		require.Equal(t, "replacement_1", formatted["subscriber"])
	})

	t.Run("multiple badges returns concatenated results", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{
					"subscriber": {
						ID:           "1",
						Image_URL_1x: "https://example.com/badge1.png",
						Title:        "Subscriber",
					},
					"moderator": {
						ID:           "1",
						Image_URL_1x: "https://example.com/badge2.png",
						Title:        "Moderator",
					},
					"vip": {
						ID:           "1",
						Image_URL_1x: "https://example.com/badge3.png",
						Title:        "VIP",
					},
				}
			},
		}

		callCount := 0
		displayManager := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				callCount++
				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "prepare_" + unit.ID,
					ReplacementText: "replacement_" + unit.ID,
				}, nil
			},
		}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		badgeList := []twitchirc.Badge{
			{Name: "subscriber", Version: "1"},
			{Name: "moderator", Version: "1"},
			{Name: "vip", Version: "1"},
		}

		prepare, formatted, err := replacer.Replace(t.Context(), "broadcaster123", badgeList)

		require.NoError(t, err)
		require.Equal(t, 3, callCount, "Convert should be called 3 times")
		require.Len(t, formatted, 3)
		require.Contains(t, prepare, "prepare_")
	})

	t.Run("display manager error returns error", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{
					"subscriber": {
						ID:           "1",
						Image_URL_1x: "https://example.com/badge1.png",
						Title:        "Subscriber",
					},
				}
			},
		}

		expectedErr := errors.New("display manager conversion failed")
		displayManager := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				return kittyimg.KittyDisplayUnit{}, expectedErr
			},
		}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		badgeList := []twitchirc.Badge{
			{Name: "subscriber", Version: "1"},
		}

		prepare, formatted, err := replacer.Replace(t.Context(), "broadcaster123", badgeList)

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to convert")
		require.Empty(t, prepare)
		require.Nil(t, formatted)
	})

	t.Run("display unit has correct ID format", func(t *testing.T) {
		const broadcasterID = "broadcaster123"
		const badgeSetKey = "subscriber"
		const badgeID = "1"

		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{
					badgeSetKey: {
						ID:           badgeID,
						Image_URL_1x: "https://example.com/badge1.png",
						Title:        "Subscriber",
					},
				}
			},
		}

		var capturedUnit kittyimg.DisplayUnit
		displayManager := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				capturedUnit = unit
				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "prepare",
					ReplacementText: "replacement",
				}, nil
			},
		}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		badgeList := []twitchirc.Badge{
			{Name: "subscriber", Version: "1"},
		}

		_, _, err := replacer.Replace(t.Context(), broadcasterID, badgeList)

		require.NoError(t, err)

		expectedID := broadcasterID + badgeSetKey + badgeID
		require.Equal(t, expectedID, capturedUnit.ID)
		require.Equal(t, "badge", capturedUnit.Directory)
		require.NotNil(t, capturedUnit.Load)
	})

	t.Run("preserves badge order in formatted output", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				// Note: maps don't preserve order, but we can test that all badges are present
				return map[string]twitchapi.BadgeVersion{
					"badge1": {ID: "1", Image_URL_1x: "url1", Title: "Badge1"},
					"badge2": {ID: "2", Image_URL_1x: "url2", Title: "Badge2"},
					"badge3": {ID: "3", Image_URL_1x: "url3", Title: "Badge3"},
				}
			},
		}

		displayManager := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "prep_" + unit.ID,
					ReplacementText: "text_" + unit.ID,
				}, nil
			},
		}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		badgeList := []twitchirc.Badge{
			{Name: "badge1", Version: "1"},
			{Name: "badge2", Version: "2"},
			{Name: "badge3", Version: "3"},
		}

		_, formatted, err := replacer.Replace(t.Context(), "broadcaster123", badgeList)

		require.NoError(t, err)
		require.Len(t, formatted, 3)

		// Verify all expected texts are present (map keys are badge set IDs)
		expectedBadges := map[string]string{
			"badge1": "text_broadcaster123badge11",
			"badge2": "text_broadcaster123badge22",
			"badge3": "text_broadcaster123badge33",
		}

		for setID, expectedText := range expectedBadges {
			require.Equal(t, expectedText, formatted[setID])
		}
	})

	t.Run("nil badge list is handled gracefully", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{}
			},
		}

		displayManager := &mockDisplayManager{}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			enableGraphics: true,
		}

		prepare, formatted, err := replacer.Replace(t.Context(), "broadcaster123", nil)

		require.NoError(t, err)
		require.Empty(t, prepare)
		require.Empty(t, formatted)
	})

	t.Run("Load function can be invoked without panic", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
				return map[string]twitchapi.BadgeVersion{
					"subscriber": {
						ID:           "1",
						Image_URL_1x: "https://example.com/badge1.png",
						Title:        "Subscriber",
					},
				}
			},
		}

		displayManager := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				_, contentType, err := unit.Load()

				require.NoError(t, err)
				require.Equal(t, "image/jpg", contentType)

				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "prepare",
					ReplacementText: "replacement",
				}, nil
			},
		}

		client := &http.Client{
			Transport: httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "https://example.com/badge1.png", req.URL.String())
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"image/jpg"}},
					Body:       io.NopCloser(bytes.NewReader([]byte{})),
				}, nil
			}),
		}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
			httpClient:     client,
			enableGraphics: true,
		}

		badgeList := []twitchirc.Badge{
			{Name: "subscriber", Version: "1"},
		}

		_, _, err := replacer.Replace(t.Context(), "broadcaster123", badgeList)

		require.NoError(t, err)
	})
}

func TestReplacer_DeadlineCancelsBadgeDownload(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if _, ok := req.Context().Deadline(); !ok {
			return nil, fmt.Errorf("download has no deadline")
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(time.Second):
			return nil, fmt.Errorf("download did not receive its caller's deadline")
		}
	})}
	cache := &mockBadgeCache{matchBadgeSetFunc: func(string, []twitchirc.Badge) map[string]twitchapi.BadgeVersion {
		return map[string]twitchapi.BadgeVersion{"subscriber": {ID: "1", Image_URL_1x: "https://example.com/badge.png"}}
	}}
	display := &mockDisplayManager{convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
		body, _, err := unit.Load()
		if body != nil {
			body.Close()
		}
		return kittyimg.KittyDisplayUnit{}, err
	}}
	replacer := NewReplacer(client, cache, true, save.BuildDefaultTheme(), display)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	_, replacements, err := replacer.Replace(ctx, "channel", []twitchirc.Badge{{Name: "subscriber", Version: "1"}})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Empty(t, replacements)
}

func TestReplacer_CancelledContributorPreparationDoesNotMutateBadges(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	replacer := NewReplacer(nil, nil, false, save.BuildDefaultTheme(), nil)
	badges := map[string]string{}
	_, err := replacer.InjectContributorBadge(ctx, "julez", badges)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, badges)
}
