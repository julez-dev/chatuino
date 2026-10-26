package tab

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

type liveNotificationTab struct {
	id string

	settings save.Settings
	theme    save.Theme
	keymap   save.KeyMap

	focused bool

	state         broadcastTabState
	width, height int

	streamerLive map[string]bool
	chatWindow   *chatWindow
}

func NewLiveNotification(
	id string,
	width, height int,
	settings save.Settings,
	theme save.Theme,
	keymap save.KeyMap,
) *liveNotificationTab {
	return &liveNotificationTab{
		id:           id,
		settings:     settings,
		theme:        theme,
		keymap:       keymap,
		state:        inChatWindow,
		width:        width,
		height:       height,
		chatWindow:   newChatWindow(width, height, settings, theme, keymap),
		streamerLive: map[string]bool{},
	}
}

func (l *liveNotificationTab) Init() tea.Cmd {
	return nil
}

func (l *liveNotificationTab) InitWithUserData(twitchapi.UserData) tea.Cmd {
	return nil
}

func (l *liveNotificationTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmd tea.Cmd

	if info, ok := msg.(SetStreamInfoMessage); ok {
		// If broadcaster already exists in open streamer map, see if prevoiusly was live/offline, then notify user and save new state
		// Else add broadcaster
		wasAlreadyLive, alreadyMonitored := l.streamerLive[info.Target]

		if !alreadyMonitored {
			l.streamerLive[info.Target] = info.IsLive
			return l, cmd
		}

		// status did not change
		if wasAlreadyLive == info.IsLive {
			return l, cmd
		}

		// status did change
		l.streamerLive[info.Target] = info.IsLive

		var msg string

		if info.IsLive {
			msg = fmt.Sprintf("%s is now live: %q!", info.Username, info.Title)
			cmd = func() tea.Msg {
				id := l.id
				return RequestNotificationIconMessage{
					TabID: id,
				}
			}
		} else {
			msg = fmt.Sprintf("%s is now offline!", info.Username)
		}

		l.chatWindow.handleMessage(ChatEventMessage{
			Message: &twitchirc.Notice{
				FakeTimestamp: time.Now(),
				MsgID:         twitchirc.MsgID(uuid.NewString()),
				Message:       msg,
			},
			IsFakeEvent: true,
		})

		return l, cmd
	}

	if _, ok := msg.(ChatEventMessage); ok {
		return l, cmd
	}

	l.chatWindow, cmd = l.chatWindow.Update(msg)
	return l, cmd
}

func (l *liveNotificationTab) View() string {
	return l.chatWindow.View()
}

func (l *liveNotificationTab) ViewWithoutStatusBar() string {
	return l.chatWindow.View() // live notification tab has no status bar
}

func (l *liveNotificationTab) StatusBarView() string {
	return "" // live notification tab has no status bar
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

// IsComposing reports whether a text input inside the tab is capturing keys.
func (l *liveNotificationTab) IsComposing() bool {
	return l.state.isComposing()
}

// Close releases resources; these tabs start nothing that needs releasing.
func (l *liveNotificationTab) Close() {}

func (l *liveNotificationTab) IsSearching() bool {
	return l.chatWindow.state == searchChatWindowState
}

func (l *liveNotificationTab) IsDataLoaded() bool {
	return true
}

func (l *liveNotificationTab) ID() string {
	return l.id
}

func (l *liveNotificationTab) Title() string {
	title, _ := Caption(LiveNotificationKind, save.Account{}, "")
	return title
}

func (l *liveNotificationTab) Identity() string {
	_, identity := Caption(LiveNotificationKind, save.Account{}, "")
	return identity
}

func (l *liveNotificationTab) Focused() bool {
	return l.focused
}

func (l *liveNotificationTab) ChannelID() string {
	return ""
}

func (l *liveNotificationTab) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button == tea.MouseLeft {
		if l.chatWindow.NewMessageBadgeHit(y, x) {
			l.chatWindow.jumpToNewest()

			return nil
		}

		l.chatWindow.selectAtViewportRow(y)
	}

	return nil
}

func (l *liveNotificationTab) HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd {
	if delta := WheelDelta(msg); delta != 0 {
		l.chatWindow.scrollByLines(delta)
		l.chatWindow.snapScroll()
	}

	return nil
}

func (l *liveNotificationTab) HandleResize() {
	l.chatWindow.Resize(l.width, l.height)
}

func (l *liveNotificationTab) SetSize(width, height int) {
	l.width = width
	l.height = height
}

func (l *liveNotificationTab) SetFullWidth(_ int) {
	// No-op for live notification tab (no status bar)
}

func (l *liveNotificationTab) Kind() TabKind {
	return LiveNotificationKind
}
