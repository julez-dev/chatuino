package emote

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/muesli/termenv"
	"github.com/rs/zerolog/log"

	"github.com/stretchr/testify/require"
)

func init() {
	lipgloss.SetColorProfile(termenv.TrueColor)
}

func TestReplacer_Replace_GraphicsEnabled(t *testing.T) {
	t.Parallel()

	t.Run("cached-graphics", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"Kappa": {
					ID:       "kappa-id",
					Text:     "Kappa",
					URL:      "https://example.com/kappa.png",
					Platform: Twitch,
				},
			},
		}

		mockDisplay := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				require.Equal(t, "emote", unit.Directory)
				require.Equal(t, "twitch.kappa-id", unit.ID)
				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/path/to/kappa.png\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\",
					ReplacementText: "\x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m",
				}, nil
			},
		}

		replacer := NewReplacer(nil, store, true, save.Theme{}, mockDisplay)

		command, replacement, err := replacer.Replace("", "Test Message with Kappa emote", nil)
		require.NoError(t, err)
		require.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/path/to/kappa.png\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\", command)
		require.Equal(t, map[string]string{"Kappa": "\x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m"}, replacement)
	})

	t.Run("fetch-emote", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"Kappa": {
					ID:       "kappa-id",
					Text:     "Kappa",
					URL:      "https://example.com/kappa.png",
					Platform: Twitch,
				},
			},
		}

		emoteData, err := os.ReadFile("./testdata/pepeLaugh.webp")
		require.NoError(t, err)

		client := &http.Client{
			Transport: httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, "https://example.com/kappa.png", req.URL.String())
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"image/webp"}},
					Body:       io.NopCloser(bytes.NewReader(emoteData)),
				}, nil
			}),
		}

		var loadCalled bool
		mockDisplay := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				require.Equal(t, "emote", unit.Directory)
				require.Equal(t, "twitch.kappa-id", unit.ID)
				require.False(t, unit.IsAnimated)

				// Test that Load function works
				body, contentType, err := unit.Load()
				require.NoError(t, err)
				require.Equal(t, "image/webp", contentType)
				defer body.Close()

				data, err := io.ReadAll(body)
				require.NoError(t, err)
				require.NotEmpty(t, data)
				loadCalled = true

				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "\x1b_Gf=32,i=1,t=f,q=2,s=28,v=28;L3BhdGgvdG8va2FwcGEucG5n\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=1\x1b\\",
					ReplacementText: "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m",
				}, nil
			},
		}

		replacer := NewReplacer(client, store, true, save.Theme{}, mockDisplay)

		command, replacement, err := replacer.Replace("", "Test Message with Kappa emote", nil)
		require.NoError(t, err)
		require.True(t, loadCalled, "Load function should be called")
		require.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=28,v=28;L3BhdGgvdG8va2FwcGEucG5n\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=1\x1b\\", command)
		require.Equal(t, map[string]string{"Kappa": "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m"}, replacement)
	})

	t.Run("animated-emote", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"PogChamp": {
					ID:         "pogchamp-id",
					Text:       "PogChamp",
					URL:        "https://example.com/pogchamp.avif",
					Platform:   SevenTV,
					IsAnimated: true,
				},
			},
		}

		mockDisplay := &mockDisplayManager{
			convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
				require.Equal(t, "emote", unit.Directory)
				require.Equal(t, "seventv.pogchamp-id", unit.ID)
				require.True(t, unit.IsAnimated)

				return kittyimg.KittyDisplayUnit{
					PrepareCommand:  "\x1b_Gf=32,i=1,t=f,q=2,s=28,v=28;frame1\x1b\\\x1b_Ga=a,i=1,r=1,z=100,q=2;\x1b\\",
					ReplacementText: "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m",
				}, nil
			},
		}

		replacer := NewReplacer(nil, store, true, save.Theme{}, mockDisplay)

		command, replacedText, err := replacer.Replace("", "PogChamp", nil)
		require.NoError(t, err)
		require.NotEmpty(t, command)
		require.Contains(t, replacedText["PogChamp"], "\U0010eeee")
	})
}

func TestReplacer_Replace_ColorMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		platform  Platform
		emoteText string
		theme     save.Theme
		expected  map[string]string
	}{
		{
			name:      "twitch-emote",
			platform:  Twitch,
			emoteText: "Kappa",
			theme: save.Theme{
				TwitchTVEmoteColor: "#9147FF",
			},
			expected: map[string]string{
				"Kappa": "\x1b[38;2;145;71;255mKappa\x1b[0m",
			},
		},
		{
			name:      "7tv-emote",
			platform:  SevenTV,
			emoteText: "pepeD",
			theme: save.Theme{
				SevenTVEmoteColor: "#00A8FC",
			},
			expected: map[string]string{
				"pepeD": "\x1b[38;2;0;168;252mpepeD\x1b[0m",
			},
		},
		{
			name:      "bttv-emote",
			platform:  BTTV,
			emoteText: "monkaS",
			theme: save.Theme{
				BetterTTVEmoteColor: "#D50014",
			},
			expected: map[string]string{
				"monkaS": "\x1b[38;2;213;0;20mmonkaS\x1b[0m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockEmoteStore{
				emotes: map[string]Emote{
					tt.emoteText: {
						Text:     tt.emoteText,
						URL:      "https://example.com/emote.png",
						Platform: tt.platform,
					},
				},
			}

			replacer := NewReplacer(nil, store, false, tt.theme, nil)

			command, replacement, err := replacer.Replace("", "Test Message with "+tt.emoteText+" emote", nil)
			require.NoError(t, err)
			require.Empty(t, command, "should not generate graphics commands in color mode")
			require.Equal(t, tt.expected, replacement)
		})
	}
}

func TestReplacer_Replace_WithBadgeList(t *testing.T) {
	t.Parallel()

	store := &mockEmoteStore{
		emotes: map[string]Emote{
			"KappaCustomID": {
				ID:       "kappa-custom-id",
				Text:     "Kappa",
				URL:      "https://example.com/kappa.png",
				Platform: Twitch,
			},
		},
	}

	mockDisplay := &mockDisplayManager{
		convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
			return kittyimg.KittyDisplayUnit{
				PrepareCommand:  "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/path/to/kappa.png\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\",
				ReplacementText: "\x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m",
			}, nil
		},
	}

	replacer := NewReplacer(nil, store, true, save.Theme{}, mockDisplay)

	command, replacement, err := replacer.Replace("123", "Test Message with Kappa emote", []twitchirc.Emote{
		{
			ID: "KappaCustomID",
			Positions: []twitchirc.EmotePosition{
				{
					Start: 18,
					End:   22,
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/path/to/kappa.png\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\", command)
	require.Equal(t, map[string]string{"Kappa": "\x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m"}, replacement)
}

func TestReplacer_Replace_ForeignEmote(t *testing.T) {
	t.Parallel()

	store := &mockEmoteStore{
		emotes: map[string]Emote{},
		foreignEmotes: map[string]Emote{
			"ForeignEmote": {
				ID:       "ForeignEmoteID",
				Text:     "ForeignEmote",
				URL:      "https://example.com/foreign.png",
				Platform: Twitch,
			},
		},
	}

	mockDisplay := &mockDisplayManager{
		convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
			return kittyimg.KittyDisplayUnit{
				PrepareCommand:  "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/foreign\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\",
				ReplacementText: "\x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m",
			}, nil
		},
	}

	replacer := NewReplacer(nil, store, true, save.Theme{}, mockDisplay)

	command, replacement, err := replacer.Replace("channel123", "Check out ForeignEmote here", []twitchirc.Emote{
		{
			ID: "ForeignEmoteID",
			Positions: []twitchirc.EmotePosition{
				{
					Start: 10,
					End:   21,
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, command)
	require.Equal(t, map[string]string{"ForeignEmote": "\x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m"}, replacement)
}

func TestReplacer_Replace_MultipleEmotes(t *testing.T) {
	t.Parallel()

	store := &mockEmoteStore{
		emotes: map[string]Emote{
			"Kappa": {
				ID:       "kappa-id",
				Text:     "Kappa",
				URL:      "https://example.com/kappa.png",
				Platform: Twitch,
			},
			"PogChamp": {
				ID:       "pogchamp-id",
				Text:     "PogChamp",
				URL:      "https://example.com/pogchamp.png",
				Platform: Twitch,
			},
		},
	}

	callCount := 0
	mockDisplay := &mockDisplayManager{
		convertFunc: func(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error) {
			callCount++
			return kittyimg.KittyDisplayUnit{
				PrepareCommand:  "\x1b_Gf=32,i=" + string(rune('0'+callCount)) + ",t=f,q=2,s=10,v=10;/path\x1b\\",
				ReplacementText: "\x1b[38;2;0;0;" + string(rune('0'+callCount)) + "m\U0010eeee\x1b[39m",
			}, nil
		},
	}

	replacer := NewReplacer(nil, store, true, save.Theme{}, mockDisplay)

	command, replacement, err := replacer.Replace("", "Kappa and PogChamp", nil)
	require.NoError(t, err)
	require.NotEmpty(t, command)
	require.Equal(t, map[string]string{"Kappa": "\x1b[38;2;0;0;1m\U0010eeee\x1b[39m", "PogChamp": "\x1b[38;2;0;0;2m\U0010eeee\x1b[39m"}, replacement)
	require.Equal(t, 2, callCount, "should convert 2 emotes")
}

type mockEmoteStore struct {
	emotes        map[string]Emote
	foreignEmotes map[string]Emote
}

func (m *mockEmoteStore) GetByTextAllChannels(text string) (Emote, bool) {
	emote, ok := m.emotes[text]
	return emote, ok
}

func (m *mockEmoteStore) GetByText(_ string, text string) (Emote, bool) {
	return m.GetByTextAllChannels(text)
}

func (m *mockEmoteStore) LoadSetForeignEmote(id, text string) Emote {
	log.Logger.Info().Str("id", id).Str("text", text).Msg("loading foreign emote")
	if emote, ok := m.foreignEmotes[text]; ok {
		return emote
	}
	return Emote{}
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
