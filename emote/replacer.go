package emote

import (
	"context"
	"fmt"

	"io"
	"net/http"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog/log"
)

type EmoteStore interface {
	GetByTextAllChannels(text string) (Emote, bool)
	GetByText(channelID, text string) (Emote, bool)
	LoadSetForeignEmote(emoteID, emoteText string) Emote
}

type DisplayManager interface {
	Convert(unit kittyimg.DisplayUnit) (kittyimg.KittyDisplayUnit, error)
}

type Replacer struct {
	store          EmoteStore
	httpClient     *http.Client
	enableGraphics bool
	displayManager DisplayManager

	stvStyle  lipgloss.Style
	ttvStyle  lipgloss.Style
	bttvStyle lipgloss.Style
	ffzStyle  lipgloss.Style
}

func NewReplacer(httpClient *http.Client, store EmoteStore, enableGraphics bool, theme save.Theme, displayManager DisplayManager) *Replacer {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Replacer{
		enableGraphics: enableGraphics,
		store:          store,
		httpClient:     httpClient,
		displayManager: displayManager,

		stvStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SevenTVEmoteColor)),
		ttvStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TwitchTVEmoteColor)),
		bttvStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.BetterTTVEmoteColor)),
		ffzStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(theme.FFZEmoteColor)),
	}
}

func (i *Replacer) Replace(channelID, content string, emoteList []twitchirc.Emote) (string, map[string]string, error) {
	// twitch sends us a list of emotes used in the message, even emotes from other channels (sub emotes)
	// parse the emote text with the index and replace it from the global store, since its guaranteed
	// the user has access to the emote
	emotesFromIRCTag := map[string]string{} // emoteText:emoteID
	for _, e := range emoteList {
		c := strings.TrimPrefix(content, "\x01ACTION ")
		r := []rune(c) // convert to runes for multi byte handling
		emoteText := string(r[e.Positions[0].Start : e.Positions[0].End+1])

		emotesFromIRCTag[emoteText] = e.ID
	}

	words := strings.Split(content, " ")
	replacements := map[string]string{}

	var cmd strings.Builder
	for _, word := range words {
		var (
			emote   Emote
			isEmote bool
		)

		if channelID == "" {
			emote, isEmote = i.store.GetByTextAllChannels(word)
		} else {
			emote, isEmote = i.store.GetByText(channelID, word)

			// current word is emote from tag, not yet cached and not native to channelID
			if emoteID, ok := emotesFromIRCTag[word]; !isEmote && ok {
				emote = i.store.LoadSetForeignEmote(emoteID, word)
				isEmote = true // always true
				log.Info().Str("word", word).Str("channel", channelID).Str("url", emote.URL).Msg("replaced foreign emote")
			}
		}

		if !isEmote {
			continue
		}

		//log.Info().Str("word", word).Str("channel", channelID).Bool("is-in-cache", isEmote).Msg("replaced emote")

		// graphics not enabled, replace with colored emote
		if !i.enableGraphics {
			replacements[word] = i.replaceEmoteColored(emote)
			continue
		}

		unit, err := i.displayManager.Convert(kittyimg.DisplayUnit{
			Directory:  "emote",
			ID:         strings.ToLower(fmt.Sprintf("%s.%s", emote.Platform.String(), emote.ID)),
			IsAnimated: emote.IsAnimated,
			Load: func() (io.ReadCloser, string, error) {
				return i.fetchEmote(context.Background(), emote.URL)
			},
		})

		if err != nil {
			continue
		}

		_, _ = cmd.WriteString(unit.PrepareCommand)
		replacements[word] = unit.ReplacementText
	}

	return cmd.String(), replacements, nil
}

func (i *Replacer) fetchEmote(ctx context.Context, reqURL string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code, got: %d", resp.StatusCode)
	}

	return resp.Body, resp.Header.Get("content-type"), nil
}

func (i *Replacer) replaceEmoteColored(emote Emote) string {
	switch emote.Platform {
	case Twitch:
		return i.ttvStyle.Render(emote.Text)
	case SevenTV:
		return i.stvStyle.Render(emote.Text)
	case BTTV:
		return i.bttvStyle.Render(emote.Text)
	case FFZ:
		return i.ffzStyle.Render(emote.Text)
	}

	return emote.Text
}
