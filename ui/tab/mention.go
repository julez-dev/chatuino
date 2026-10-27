package tab

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
)

type setMentionTabData struct {
	err       error
	usernames []string
}

type mentionTab struct {
	id string

	settings        save.Settings
	theme           save.Theme
	keymap          save.KeyMap
	accountProvider AccountProvider

	focused bool

	state         broadcastTabState
	width, height int

	usernames     []string
	hasDataLoaded bool

	chatWindow *chatWindow
}

func NewMention(
	id string,
	width, height int,
	settings save.Settings,
	theme save.Theme,
	keymap save.KeyMap,
	accountProvider AccountProvider,
) *mentionTab {
	return &mentionTab{
		id:              id,
		state:           inChatWindow,
		width:           width,
		height:          height,
		settings:        settings,
		theme:           theme,
		keymap:          keymap,
		accountProvider: accountProvider,
		chatWindow:      newChatWindow(width, height, settings, theme, keymap),
	}
}

func (m *mentionTab) Init() tea.Cmd {
	return func() tea.Msg {
		// fetch all of users account names
		accounts, err := m.accountProvider.GetAllAccounts()
		if err != nil {
			return setMentionTabData{
				err: err,
			}
		}

		usernames := []string{}

		for _, account := range accounts {
			if account.IsAnonymous {
				continue
			}

			usernames = append(usernames, account.DisplayName)
		}

		return setMentionTabData{
			usernames: usernames,
		}
	}
}

func (m *mentionTab) InitWithUserData(twitchapi.UserData) tea.Cmd {
	return m.Init()
}

func (m *mentionTab) Update(msg tea.Msg) (Tab, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case setMentionTabData:
		m.hasDataLoaded = true
		m.usernames = msg.usernames

		if msg.err != nil {
			msg := fmt.Sprintf("Failed to load user accounts: %s", msg.err.Error())
			m.chatWindow.handleMessage(chat.ChatEventMessage{
				Message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					MsgID:         twitchirc.MsgID(uuid.NewString()),
					Message:       msg,
				},
				IsFakeEvent: true,
			})

			return m, nil
		}

		notice := fmt.Sprintf("Displaying mentions of: %s", strings.Join(m.usernames, ", "))
		m.chatWindow.handleMessage(chat.ChatEventMessage{
			Message: &twitchirc.Notice{
				FakeTimestamp: time.Now(),
				MsgID:         twitchirc.MsgID(uuid.NewString()),
				Message:       notice,
			},
			IsFakeEvent: true,
		})

		return m, nil
	}

	if event, ok := msg.(chat.ChatEventMessage); ok {
		if privMsg, ok := event.Message.(*twitchirc.PrivateMessage); ok {
			var mentioned bool

			for iu := range m.usernames {
				if messageContainsCaseInsensitive(privMsg, m.usernames[iu]) {
					event.DisplayModifier.MessageSuffix = fmt.Sprintf(" (mentioned in %s)", privMsg.ChannelUserName)
					mentioned = true
					break
				}
			}

			if !mentioned || messageMatchesBlocked(event.Message, m.settings.BlockSettings) {
				return m, nil
			}

			var cmds []tea.Cmd
			m.chatWindow, cmd = m.chatWindow.Update(event)
			cmds = append(cmds, cmd)
			cmds = append(cmds, func() tea.Msg {
				return RequestNotificationIconMessage{
					TabID: m.id,
				}
			})

			return m, tea.Batch(cmds...)
		}

		return m, nil
	}

	m.chatWindow, cmd = m.chatWindow.Update(msg)

	return m, cmd
}

func (m *mentionTab) View() string {
	return m.chatWindow.View()
}

func (m *mentionTab) ViewWithoutStatusBar() string {
	return m.chatWindow.View() // mention tab has no status bar
}

func (m *mentionTab) StatusBarView() string {
	return "" // mention tab has no status bar
}

func (m *mentionTab) Focus() {
	m.focused = true
	m.chatWindow.Focus()
}

func (m *mentionTab) Blur() {
	m.focused = false
	m.chatWindow.Blur()
}

func (m *mentionTab) AccountID() string {
	return ""
}

func (m *mentionTab) Channel() string {
	return ""
}

// IsComposing reports whether a text input inside the tab is capturing keys.
func (m *mentionTab) IsComposing() bool {
	return m.state.isComposing()
}

// Close releases resources; these tabs start nothing that needs releasing.
func (m *mentionTab) Close() {}

func (m *mentionTab) IsSearching() bool {
	return m.chatWindow.state == searchChatWindowState
}

func (m *mentionTab) IsDataLoaded() bool {
	return m.hasDataLoaded
}

func (m *mentionTab) ID() string {
	return m.id
}

func (m *mentionTab) Title() string {
	title, _ := Caption(MentionKind, save.Account{}, "")
	return title
}

func (m *mentionTab) Identity() string {
	_, identity := Caption(MentionKind, save.Account{}, "")
	return identity
}

func (m *mentionTab) Focused() bool {
	return m.focused
}

func (m *mentionTab) ChannelID() string {
	return ""
}

func (m *mentionTab) HandleMouseClick(msg tea.MouseClickMsg, x, y int) tea.Cmd {
	if msg.Button == tea.MouseLeft {
		if m.chatWindow.handleChatClick(y, x) {
			return nil
		}

		m.chatWindow.selectAtViewportRow(y)
	}

	return nil
}

func (m *mentionTab) HandleMouseWheel(msg tea.MouseWheelMsg, x, y int) tea.Cmd {
	if delta := WheelDelta(msg); delta != 0 {
		m.chatWindow.scrollByLines(delta)
		m.chatWindow.SnapScroll()
	}

	return nil
}

func (m *mentionTab) HandleResize() {
	m.chatWindow.Resize(m.width, m.height)
}

func (m *mentionTab) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *mentionTab) SetFullWidth(_ int) {
	// No-op for mention tab (no status bar)
}

func (m *mentionTab) Kind() TabKind {
	return MentionKind
}
