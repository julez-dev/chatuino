package emote

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/julez-dev/chatuino/httputil"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/rs/zerolog/log"

	"github.com/stretchr/testify/assert"
)

func TestReplacer_Replace(t *testing.T) {
	t.Parallel()

	t.Run("cached-graphics", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"Kappa": {
					Text:     "Kappa",
					URL:      "https://example.com/kappa.png",
					Platform: Twitch,
				},
			},
		}

		replacer := NewReplacer(nil, store, true, 10, 10, save.Theme{})
		replacer.createEncodedImage = func(buff []byte, e Emote, offset int) (string, error) {
			t.Log("should not call createEncodedImage")
			return "", nil
		}
		replacer.openCached = func(e Emote) (DecodedEmote, bool, error) {
			assert.Equal(t, "Kappa", e.Text)
			return DecodedEmote{
				Cols: 2,
				Images: []DecodedImage{
					{
						Width:       10,
						Height:      10,
						EncodedPath: "/path/to/kappa.png",
					},
				},
			}, true, nil
		}
		replacer.saveCached = func(e Emote, decoded DecodedEmote) error {
			t.Log("should not call saveCached")
			return nil
		}

		command, replacedText, err := replacer.Replace("", "Test Message with Kappa emote", nil)
		assert.Nil(t, err)
		assert.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/path/to/kappa.png\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\", command)
		assert.Equal(t, "Test Message with \x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m emote", replacedText)
	})

	t.Run("color-mode", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"Kappa": {
					Text:     "Kappa",
					URL:      "https://example.com/kappa.png",
					Platform: Twitch,
				},
			},
		}

		replacer := NewReplacer(nil, store, false, 0, 0, save.Theme{})
		replacer.createEncodedImage = func(buff []byte, e Emote, offset int) (string, error) {
			t.Log("should not call createEncodedImage")
			return "", nil
		}
		replacer.openCached = func(e Emote) (DecodedEmote, bool, error) {
			t.Log("should not call openCached")
			return DecodedEmote{}, false, nil
		}
		replacer.saveCached = func(e Emote, decoded DecodedEmote) error {
			t.Log("should not call saveCached")
			return nil
		}

		command, replacedText, err := replacer.Replace("", "Test Message with Kappa emote", nil)
		assert.Nil(t, err)
		assert.Equal(t, "", command)
		assert.Equal(t, "Test Message with Kappa emote", replacedText)
	})

	t.Run("fetch-emote", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"Kappa": {
					Text:     "Kappa",
					URL:      "https://example.com/kappa.png",
					Platform: Twitch,
				},
			},
		}

		emoteData, err := os.ReadFile("./testdata/pepeLaugh.webp")
		if err != nil {
			t.Log(err)
			t.FailNow()
		}

		client := &http.Client{
			Transport: httputil.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "https://example.com/kappa.png", req.URL.String())
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewReader(emoteData)),
				}, nil
			}),
		}

		replacer := NewReplacer(client, store, true, 400, 400, save.Theme{})
		replacer.createEncodedImage = func(buff []byte, e Emote, offset int) (string, error) {
			assert.NotEmpty(t, buff)
			assert.Equal(t, "Kappa", e.Text)
			assert.Equal(t, 0, offset)
			return "/path/to/kappa.png", nil
		}
		replacer.openCached = func(e Emote) (DecodedEmote, bool, error) {
			return DecodedEmote{}, false, nil
		}
		var cached bool
		replacer.saveCached = func(e Emote, decoded DecodedEmote) error {
			assert.Equal(t, "Kappa", e.Text)
			assert.Len(t, decoded.Images, 1)
			assert.Equal(t, 28, decoded.Images[0].Width)
			assert.Equal(t, 28, decoded.Images[0].Height)
			cached = true

			return nil
		}

		command, replacedText, err := replacer.Replace("", "Test Message with Kappa emote", nil)
		assert.True(t, cached, "should call saveCached")
		assert.Nil(t, err)
		assert.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=28,v=28;L3BhdGgvdG8va2FwcGEucG5n\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=1\x1b\\", command)
		assert.Equal(t, "Test Message with \x1b[38;2;0;0;1m\U0010eeee\x1b[39m emote", replacedText)
	})

	t.Run("badge-list", func(t *testing.T) {
		store := &mockEmoteStore{
			emotes: map[string]Emote{
				"KappaCustomID": {
					Text:     "Kappa",
					URL:      "https://example.com/kappa.png",
					Platform: Twitch,
				},
			},
		}

		replacer := NewReplacer(nil, store, true, 10, 10, save.Theme{})
		replacer.createEncodedImage = func(buff []byte, e Emote, offset int) (string, error) {
			t.Log("should not call createEncodedImage")
			return "", nil
		}
		replacer.openCached = func(e Emote) (DecodedEmote, bool, error) {
			assert.Equal(t, "Kappa", e.Text)
			return DecodedEmote{
				Cols: 2,
				Images: []DecodedImage{
					{
						Width:       10,
						Height:      10,
						EncodedPath: "/path/to/kappa.png",
					},
				},
			}, true, nil
		}
		replacer.saveCached = func(e Emote, decoded DecodedEmote) error {
			t.Log("should not call saveCached")
			return nil
		}

		command, replacedText, err := replacer.Replace("123", "Test Message with Kappa emote", []command.Emote{
			{
				ID: "KappaCustomID",
				Positions: []command.EmotePosition{
					{
						Start: 18,
						End:   22,
					},
				},
			},
		})
		assert.Nil(t, err)
		assert.Equal(t, "\x1b_Gf=32,i=1,t=f,q=2,s=10,v=10;/path/to/kappa.png\x1b\\\x1b_Ga=p,i=1,p=1,q=2,U=1,r=1,c=2\x1b\\", command)
		assert.Equal(t, "Test Message with \x1b[38;2;0;0;1m\U0010eeee\U0010eeee\x1b[39m emote", replacedText)
	})
}

func Test24BitID(t *testing.T) {
	t.Parallel()

	t.Run("256", func(t *testing.T) {
		emote := DecodedEmote{
			ID:   256,
			Cols: 2,
		}

		assert.Equal(t, "\x1b[38;2;0;1;0m\U0010eeee\U0010eeee\x1b[39m", emote.DisplayUnicodePlaceholder())
	})

	t.Run("8bit", func(t *testing.T) {
		emote := DecodedEmote{
			ID:   255,
			Cols: 2,
		}

		assert.Equal(t, "\x1b[38;2;0;0;255m\U0010eeee\U0010eeee\x1b[39m", emote.DisplayUnicodePlaceholder())
	})

	t.Run("24bit", func(t *testing.T) {
		emote := DecodedEmote{
			ID:   8235331,
			Cols: 2,
		}

		assert.Equal(t, "\x1b[38;2;125;169;67m\U0010eeee\U0010eeee\x1b[39m", emote.DisplayUnicodePlaceholder())
	})
}

type mockEmoteStore struct {
	emotes map[string]Emote
}

func (m *mockEmoteStore) GetByTextAllChannels(text string) (Emote, bool) {
	emote, ok := m.emotes[text]
	return emote, ok
}

func (m *mockEmoteStore) GetByText(_ string, text string) (Emote, bool) {
	return m.GetByTextAllChannels(text)
}

func (m *mockEmoteStore) LoadSetForeignEmote(id, text string) Emote {
	log.Logger.Info().Str("id", id).Str("text", text).Send()
	return m.emotes[id]
}
