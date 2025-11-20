package mainui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
)

type liveNotificationTab struct {
	id   string
	deps *DependencyContainer

	focused bool

	state         broadcastTabState
	width, height int

	streamerLive map[string]bool
	chatWindow   *chatWindow
}

func newLiveNotificationTab(id string, width, height int, deps *DependencyContainer) *liveNotificationTab {
	return &liveNotificationTab{
		id:           id,
		deps:         deps,
		state:        inChatWindow,
		width:        width,
		height:       height,
		chatWindow:   newChatWindow(width, height, deps),
		streamerLive: map[string]bool{},
	}
}

func (l *liveNotificationTab) Init() tea.Cmd {
	return nil
}

func (l *liveNotificationTab) InitWithUserData(twitchapi.UserData) tea.Cmd {
	return nil
}

func (l *liveNotificationTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	var cmd tea.Cmd

	if info, ok := msg.(setStreamInfoMessage); ok {
		// If broadcaster already exists in open streamer map, see if prevoiusly was live/offline, then notify user and save new state
		// Else add broadcaster
		wasAlreadyLive, alreadyMonitored := l.streamerLive[info.target]

		if !alreadyMonitored {
			l.streamerLive[info.target] = info.isLive
			return l, cmd
		}

		// status did not change
		if wasAlreadyLive == info.isLive {
			return l, cmd
		}

		// status did change
		l.streamerLive[info.target] = info.isLive

		var msg string

		if info.isLive {
			msg = fmt.Sprintf("%s is now live: %q!", info.username, info.title)
			cmd = func() tea.Msg {
				id := l.id
				return requestNotificationIconMessage{
					tabID: id,
				}
			}
		} else {
			msg = fmt.Sprintf("%s is now offline!", info.username)
		}

		l.chatWindow.handleMessage(chatEventMessage{
			message: &command.Notice{
				FakeTimestamp: time.Now(),
				MsgID:         command.MsgID(uuid.NewString()),
				Message:       msg,
			},
			isFakeEvent:                 true,
			messageContentEmoteOverride: msg,
		})

		return l, cmd
	}

	if _, ok := msg.(chatEventMessage); ok {
		return l, cmd
	}

	l.chatWindow, cmd = l.chatWindow.Update(msg)
	return l, cmd
}

func (l *liveNotificationTab) View() string {
	return l.chatWindow.View()
}

func (l *liveNotificationTab) Focus() {
	l.focused = true
	l.chatWindow.Focus()
}

func (l *liveNotificationTab) Blur() {
	l.focused = false
	l.chatWindow.Blur()
}

func (l *liveNotificationTab) AccountID() string {
	return ""
}

func (l *liveNotificationTab) Channel() string {
	return ""
}

func (l *liveNotificationTab) State() broadcastTabState {
	return l.state
}

func (l *liveNotificationTab) IsDataLoaded() bool {
	return true
}

func (l *liveNotificationTab) ID() string {
	return l.id
}

func (l *liveNotificationTab) Focused() bool {
	return l.focused
}

func (l *liveNotificationTab) ChannelID() string {
	return ""
}

func (l *liveNotificationTab) HandleResize() {
	l.chatWindow.width = l.width
	l.chatWindow.height = l.height
	l.chatWindow.recalculateLines()
}

func (l *liveNotificationTab) SetSize(width, height int) {
	l.width = width
	l.height = height
}

func (l *liveNotificationTab) Kind() tabKind {
	return liveNotificationTabKind
}
