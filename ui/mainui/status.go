package mainui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setSteamStatusData struct {
	target   string
	err      error
	settings twitch.ChatSettingData
}

type streamStatus struct {
	logger            zerolog.Logger
	ttvAPI            APIClient
	tab               *broadcastTab
	width, height     int
	userID, channelID string
	userConfig        UserConfiguration

	settings      twitch.ChatSettingData
	err           error
	isDataFetched bool
}

func newStreamStatus(logger zerolog.Logger, ttvAPI APIClient, tab *broadcastTab, width, height int, userID, channelID string, userConfig UserConfiguration) *streamStatus {
	return &streamStatus{
		logger:     logger,
		ttvAPI:     ttvAPI,
		tab:        tab,
		width:      width,
		height:     height,
		userID:     userID,
		channelID:  channelID,
		userConfig: userConfig,
	}
}

func (s *streamStatus) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		settingsResp, err := s.ttvAPI.GetChatSettings(ctx, s.channelID, "")
		if err != nil {
			return setSteamStatusData{
				target: s.tab.id,
				err:    err,
			}
		}

		if len(settingsResp.Data) == 0 {
			return setSteamStatusData{
				target: s.tab.id,
				err:    fmt.Errorf("no settings found for channel %s", s.userID),
			}
		}

		return setSteamStatusData{
			target:   s.tab.id,
			settings: settingsResp.Data[0],
			err:      err,
		}
	}
}

func (s *streamStatus) Update(msg tea.Msg) (*streamStatus, tea.Cmd) {
	switch msg := msg.(type) {
	case setSteamStatusData:
		if msg.target != s.tab.id {
			return s, nil
		}

		s.err = msg.err
		s.settings = msg.settings

		s.isDataFetched = true

		return s, nil
	}

	return s, nil
}

func (s *streamStatus) View() string {
	padded := lipgloss.NewStyle().MaxWidth(s.width).Render

	if !s.isDataFetched {
		return padded("Fetching chat settings...")
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

	stateStr := fmt.Sprintf("-- %s --", lipgloss.NewStyle().Foreground(lipgloss.Color(s.userConfig.Theme.StatusColor)).Render(state))

	settingsBuilder := strings.Builder{}

	if s.settings.SlowMode {
		dur := time.Duration(s.settings.SlowModeWaitTime * 1e9).String()
		settingsBuilder.WriteString("Slow Mode: ")
		settingsBuilder.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(s.userConfig.Theme.StatusColor)).Render(dur))
	}

	if s.settings.FollowerMode {
		if settingsBuilder.Len() > 0 {
			settingsBuilder.WriteString(" | ")
		}

		dur := time.Duration(s.settings.FollowerModeDuration * 6e+10).String()
		settingsBuilder.WriteString("Follow Only: ")
		settingsBuilder.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(s.userConfig.Theme.StatusColor)).Render(dur))
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

	return padded(stateStr + lipgloss.NewStyle().AlignHorizontal(lipgloss.Right).Width(s.width-lipgloss.Width(stateStr)).Render(settingsBuilder.String()))
}
