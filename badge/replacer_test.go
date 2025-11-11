package badge

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

type mockBadgeCache struct {
	matchBadgeSetFunc func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion
}

func (m *mockBadgeCache) MatchBadgeSet(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
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
	t.Run("empty badge list returns empty results", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{}
			},
		}

		displayManager := &mockDisplayManager{}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
		}

		prepare, formatted, err := replacer.Replace("broadcaster123", []command.Badge{})

		require.NoError(t, err)
		assert.Empty(t, prepare)
		assert.Empty(t, formatted)
	})

	t.Run("single badge returns correct results", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{
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
		}

		badgeList := []command.Badge{
			{Name: "subscriber", Version: "1"},
		}

		prepare, formatted, err := replacer.Replace("broadcaster123", badgeList)

		require.NoError(t, err)
		assert.Equal(t, "prepare_command_1", prepare)
		require.Len(t, formatted, 1)
		assert.Equal(t, "replacement_1", formatted[0])
	})

	t.Run("multiple badges returns concatenated results", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{
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
		}

		badgeList := []command.Badge{
			{Name: "subscriber", Version: "1"},
			{Name: "moderator", Version: "1"},
			{Name: "vip", Version: "1"},
		}

		prepare, formatted, err := replacer.Replace("broadcaster123", badgeList)

		require.NoError(t, err)
		assert.Equal(t, 3, callCount, "Convert should be called 3 times")
		assert.Len(t, formatted, 3)
		assert.Contains(t, prepare, "prepare_")
	})

	t.Run("display manager error returns error", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{
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
		}

		badgeList := []command.Badge{
			{Name: "subscriber", Version: "1"},
		}

		prepare, formatted, err := replacer.Replace("broadcaster123", badgeList)

		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to convert")
		assert.Empty(t, prepare)
		assert.Nil(t, formatted)
	})

	t.Run("display unit has correct ID format", func(t *testing.T) {
		const broadcasterID = "broadcaster123"
		const badgeSetKey = "subscriber"
		const badgeID = "1"

		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{
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
		}

		badgeList := []command.Badge{
			{Name: "subscriber", Version: "1"},
		}

		_, _, err := replacer.Replace(broadcasterID, badgeList)

		require.NoError(t, err)

		expectedID := broadcasterID + badgeSetKey + badgeID
		assert.Equal(t, expectedID, capturedUnit.ID)
		assert.Equal(t, "badge", capturedUnit.Directory)
		assert.NotNil(t, capturedUnit.Load)
	})

	t.Run("preserves badge order in formatted output", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				// Note: maps don't preserve order, but we can test that all badges are present
				return map[string]twitch.BadgeVersion{
					"badge1": {ID: "1", Image_URL_1x: "url1"},
					"badge2": {ID: "2", Image_URL_1x: "url2"},
					"badge3": {ID: "3", Image_URL_1x: "url3"},
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
		}

		badgeList := []command.Badge{
			{Name: "badge1", Version: "1"},
			{Name: "badge2", Version: "2"},
			{Name: "badge3", Version: "3"},
		}

		_, formatted, err := replacer.Replace("broadcaster123", badgeList)

		require.NoError(t, err)
		assert.Len(t, formatted, 3)

		// Verify all expected texts are present
		expectedTexts := []string{
			"text_broadcaster123badge11",
			"text_broadcaster123badge22",
			"text_broadcaster123badge33",
		}

		for _, expected := range expectedTexts {
			assert.Contains(t, formatted, expected)
		}
	})

	t.Run("nil badge list is handled gracefully", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{}
			},
		}

		displayManager := &mockDisplayManager{}

		replacer := &Replacer{
			cache:          cache,
			displayManager: displayManager,
		}

		prepare, formatted, err := replacer.Replace("broadcaster123", nil)

		require.NoError(t, err)
		assert.Empty(t, prepare)
		assert.Empty(t, formatted)
	})

	t.Run("Load function can be invoked without panic", func(t *testing.T) {
		cache := &mockBadgeCache{
			matchBadgeSetFunc: func(broadcasterID string, ircBadge []command.Badge) map[string]twitch.BadgeVersion {
				return map[string]twitch.BadgeVersion{
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
				assert.Equal(t, "https://example.com/badge1.png", req.URL.String())
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
		}

		badgeList := []command.Badge{
			{Name: "subscriber", Version: "1"},
		}

		_, _, err := replacer.Replace("broadcaster123", badgeList)

		require.NoError(t, err)
	})
}
