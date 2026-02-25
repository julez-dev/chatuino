package mainui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
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

type streamStatus struct {
	width, height int
	accountID     string
	channelID     string
	tab           *broadcastTab
	deps          *DependencyContainer

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

func newStreamStatus(width, height int, tab *broadcastTab, accountID, channelID string, deps *DependencyContainer) *streamStatus {
	return &streamStatus{
		deps:      deps,
		tab:       tab,
		accountID: accountID,
		width:     width,
		height:    height,
		channelID: channelID,
		spinner:   spinner.New(spinner.WithSpinner(loadingSpinner)),

		maxWidthStyle:   lipgloss.NewStyle(), // Width set at render time
		statusHighlight: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(deps.UserConfig.Theme.StatusColor)),
		updateHighlight: lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.SplashHighlightColor)),
		rightAlignStyle: lipgloss.NewStyle().AlignHorizontal(lipgloss.Right), // Width set at render time
	}
}

func (s *streamStatus) Init() tea.Cmd {
	return tea.Batch(s.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		settingsResp, err := s.deps.APIUserClients[s.accountID].GetChatSettings(ctx, s.channelID, "")
		if err != nil {
			return setSteamStatusDataMessage{
				target: s.tab.id,
				err:    err,
			}
		}

		if len(settingsResp.Data) == 0 {
			return setSteamStatusDataMessage{
				target: s.tab.id,
				err:    fmt.Errorf("no chat status settings found for channel: %s", s.tab.channelLogin),
			}
		}

		return setSteamStatusDataMessage{
			target:   s.tab.id,
			settings: settingsResp.Data[0],
			err:      err,
		}
	})
}

func (s *streamStatus) Update(msg tea.Msg) (*streamStatus, tea.Cmd) {
	switch msg := msg.(type) {
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

	if s.tab.updateInfo != nil && s.tab.updateInfo.HasUpdate {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}
		settingsBuilder.WriteString(s.updateHighlight.Render("New update available: " + s.tab.updateInfo.LatestVersion))
	}

	return padded(stateStr + s.rightAlignStyle.Width(s.width-lipgloss.Width(stateStr)).Render(settingsBuilder.String()))
}
