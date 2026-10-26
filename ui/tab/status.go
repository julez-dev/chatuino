package tab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// humanizeDuration converts a duration to a human-readable string like "5 minutes" or "1 day 2 hours"
func humanizeDuration(d time.Duration) string {
	if d < time.Second {
		return "0 seconds"
	}

	var parts []string

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 day")
		} else {
			parts = append(parts, fmt.Sprintf("%d days", days))
		}
	}

	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hour")
		} else {
			parts = append(parts, fmt.Sprintf("%d hours", hours))
		}
	}

	if minutes > 0 && days == 0 { // Only show minutes if less than a day
		if minutes == 1 {
			parts = append(parts, "1 minute")
		} else {
			parts = append(parts, fmt.Sprintf("%d minutes", minutes))
		}
	}

	if seconds > 0 && hours == 0 && days == 0 { // Only show seconds if less than an hour
		if seconds == 1 {
			parts = append(parts, "1 second")
		} else {
			parts = append(parts, fmt.Sprintf("%d seconds", seconds))
		}
	}

	if len(parts) == 0 {
		return "0 seconds"
	}

	return strings.Join(parts, " ")
}

type setSteamStatusDataMessage struct {
	target   string
	err      error
	settings twitchapi.ChatSettingData
}

// settingsFetchFallbackMessage asks the status bar to request the settings it is
// still missing, because no ROOMSTATE arrived for the channel.
type settingsFetchFallbackMessage struct {
	target string
	seq    int
}

// settingsFetchDelay is how long a tab waits for the channel's ROOMSTATE before
// falling back to the REST endpoint. Twitch sends ROOMSTATE with the join, so the
// wait is normally never used: the fallback only covers a channel whose room
// state never arrived.
const settingsFetchDelay = 3 * time.Second

type streamStatus struct {
	width, height int
	accountID     string
	channelID     string
	tab           *Broadcast
	api           APIClient

	// chatSettings is shared with every other tab of the channel, so a channel is
	// fetched at most once.
	chatSettings *chatstate.Cache

	// fallbackSeq invalidates a scheduled fallback: applying a ROOMSTATE bumps it,
	// which makes the pending fetch a no-op.
	fallbackSeq int

	spinner spinner.Model

	settings      twitchapi.ChatSettingData
	err           error
	isDataFetched bool

	// pre-created styles to avoid allocations in View() (called every frame)
	maxWidthStyle   lipgloss.Style // for padded rendering; Width set at render time
	statusHighlight lipgloss.Style // bold + status color for slow/follower mode values
	updateHighlight lipgloss.Style // splash highlight for update notification
	rightAlignStyle lipgloss.Style // right-aligned layout; Width set at render time
}

func newStreamStatus(width, height int, tab *Broadcast, accountID, channelID string, theme save.Theme, api APIClient, chatSettings *chatstate.Cache) *streamStatus {
	return &streamStatus{
		api:          api,
		chatSettings: chatSettings,
		tab:          tab,
		accountID:    accountID,
		width:        width,
		height:       height,
		channelID:    channelID,
		spinner:      spinner.New(spinner.WithSpinner(LoadingSpinner)),

		maxWidthStyle:   lipgloss.NewStyle(), // Width set at render time
		statusHighlight: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(theme.StatusColor)),
		updateHighlight: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.SplashHighlightColor)),
		rightAlignStyle: lipgloss.NewStyle().AlignHorizontal(lipgloss.Right), // Width set at render time
	}
}

// Init renders the status bar from what is already known. The startup loader
// fetches the settings of every restored channel, and ROOMSTATE carries them for
// every later join, so a request is only made when neither happened.
func (s *streamStatus) Init() tea.Cmd {
	if settings, ok := s.chatSettings.Get(s.channelID); ok {
		s.settings = settings
		s.err = nil
		s.isDataFetched = true

		return nil
	}

	return tea.Batch(s.spinner.Tick, s.scheduleFallback())
}

// ApplyRoomState updates the status bar from the ROOMSTATE received on join.
// ROOMSTATE already carries every setting this bar renders, so the tab does not
// re-request them over REST.
func (s *streamStatus) ApplyRoomState(state *twitchirc.RoomState) {
	s.settings = s.chatSettings.ApplyRoomState(s.channelID, state)
	s.err = nil
	s.isDataFetched = true

	// A pending fallback would only repeat what this message just told us.
	s.fallbackSeq++
}

// ApplySettings replaces the rendered settings with a complete state, either from
// a chat settings write that this tab made or from a channel.chat_settings.update
// event. Unlike ROOMSTATE the state is complete, so it is stored as-is.
func (s *streamStatus) ApplySettings(settings twitchapi.ChatSettingData) {
	s.settings = settings
	s.err = nil
	s.isDataFetched = true
	s.fallbackSeq++

	if s.chatSettings != nil {
		s.chatSettings.Put(s.channelID, settings)
	}
}

// scheduleFallback arms the fallback request for a channel that has not reported
// its room state yet.
// scheduleFallback arms the fallback request for a channel that has not reported
// its room state yet.
func (s *streamStatus) scheduleFallback() tea.Cmd {
	s.fallbackSeq++

	seq, target := s.fallbackSeq, s.tab.id

	return tea.Tick(settingsFetchDelay, func(time.Time) tea.Msg {
		return settingsFetchFallbackMessage{target: target, seq: seq}
	})
}

// fetchSettings requests the channel's settings through the cache, so tabs that
// show the same channel (or the startup prefetch) share one request.
func (s *streamStatus) fetchSettings() tea.Cmd {
	if s.api == nil {
		return nil
	}

	api, cache := s.api, s.chatSettings
	target, channelID, login := s.tab.id, s.channelID, s.tab.channelLogin

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		settings, err := cache.Fetch(ctx, channelID, func(ctx context.Context) (twitchapi.ChatSettingData, error) {
			resp, err := api.GetChatSettings(ctx, channelID, "")
			if err != nil {
				return twitchapi.ChatSettingData{}, err
			}

			settings, err := chatstate.SettingsFromResponse(resp)
			if err != nil {
				return twitchapi.ChatSettingData{}, fmt.Errorf("%w: %s", err, login)
			}

			return settings, nil
		})

		return setSteamStatusDataMessage{
			target:   target,
			settings: settings,
			err:      err,
		}
	}
}

func (s *streamStatus) Update(msg tea.Msg) (*streamStatus, tea.Cmd) {
	switch msg := msg.(type) {
	case settingsFetchFallbackMessage:
		if msg.target != s.tab.id || msg.seq != s.fallbackSeq || s.isDataFetched {
			// Another tab's fallback, a superseded one, or a ROOMSTATE made it moot.
			return s, nil
		}

		return s, s.fetchSettings()
	case setSteamStatusDataMessage:
		if msg.target != s.tab.id {
			return s, nil
		}

		s.err = msg.err
		s.settings = msg.settings

		s.isDataFetched = true

		return s, nil
	}

	if !s.isDataFetched {
		var cmd tea.Cmd
		s.spinner, cmd = s.spinner.Update(msg)
		return s, cmd
	}

	return s, nil
}

func (s *streamStatus) View() string {
	padded := s.maxWidthStyle.MaxWidth(s.width).Render

	if !s.isDataFetched {
		return padded(s.spinner.View() + " Fetching chat settings")
	}

	if s.err != nil {
		return padded(s.err.Error())
	}

	state := s.tab.state.String()
	if s.tab.chatWindow.state == searchChatWindowState {
		state = "Search"
	}

	if s.tab.state == userInspectMode && s.tab.userInspect.chatWindow.state == searchChatWindowState {
		state = "Inspect / Search"
	}

	stateStr := fmt.Sprintf("-- %s --", state)

	settingsBuilder := strings.Builder{}

	if s.tab.isUserMod && !s.tab.isModStatusAssumed {
		settingsBuilder.WriteString("Mod")
	}

	if s.settings.SlowMode {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}

		dur := humanizeDuration(time.Duration(s.settings.SlowModeWaitTime) * time.Second)
		settingsBuilder.WriteString("Slow Mode: ")
		settingsBuilder.WriteString(s.statusHighlight.Render(dur))
	}

	if s.settings.FollowerMode {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}

		dur := humanizeDuration(time.Duration(s.settings.FollowerModeDuration) * time.Minute)
		settingsBuilder.WriteString("Follow Only: ")
		settingsBuilder.WriteString(s.statusHighlight.Render(dur))
	}

	if s.settings.SubscriberMode {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}

		settingsBuilder.WriteString("Sub Only")
	}

	if s.tab.isLocalSub {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}
		settingsBuilder.WriteString("Local Sub Only")
	}

	if s.tab.isUniqueOnlyChat {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}
		settingsBuilder.WriteString("Unique Only")
	}

	if s.settings.EmoteMode {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}
		settingsBuilder.WriteString("Emote Only")
	}

	if s.settings.UniqueChatMode {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}
		settingsBuilder.WriteString("Unique Only")
	}

	if s.tab.updateAvailable != "" {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}
		settingsBuilder.WriteString(s.updateHighlight.Render("New update available: " + s.tab.updateAvailable))
	}

	return padded(stateStr + s.rightAlignStyle.Width(s.width-lipgloss.Width(stateStr)).Render(settingsBuilder.String()))
}
