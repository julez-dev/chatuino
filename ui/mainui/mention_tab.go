package mainui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/rs/zerolog"
)

type setMentionTabData struct {
	err       error
	usernames []string
}

type mentionTab struct {
	id       string
	keymap   save.KeyMap
	logger   zerolog.Logger
	provider AccountProvider

	focused bool

	state         tabState
	width, height int

	usernames     []string
	hasDataLoaded bool

	chatWindow *chatWindow
}

func newMentionTab(id string, logger zerolog.Logger, keymap save.KeyMap, provider AccountProvider, emoteStore EmoteStore, width, height int) *mentionTab {
	return &mentionTab{
		id:         id,
		logger:     logger,
		keymap:     keymap,
		provider:   provider,
		state:      inChatWindow,
		width:      width,
		height:     height,
		chatWindow: newChatWindow(logger, width, height, emoteStore, keymap),
	}
}

func (m *mentionTab) Init() tea.Cmd {
	return func() tea.Msg {
		// fetch all of users account names
		accounts, err := m.provider.GetAllAccounts()

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

func (m *mentionTab) InitWithUserData(twitch.UserData) tea.Cmd {
	return m.Init()
}

func (m *mentionTab) Update(msg tea.Msg) (tab, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case setMentionTabData:
		m.hasDataLoaded = true
		m.usernames = msg.usernames

		if msg.err != nil {
			msg := fmt.Sprintf("Failed to load user accounts: %s", msg.err.Error())
			m.chatWindow.handleMessage(chatEventMessage{
				message: &command.Notice{
					FakeTimestamp: time.Now(),
					MsgID:         command.MsgID(uuid.NewString()),
					Message:       msg,
				},
				isFakeEvent:                 true,
				messageContentEmoteOverride: msg,
			})

			return m, nil
		}

		notice := fmt.Sprintf("Displaying mentions of: %s", strings.Join(m.usernames, ", "))
		m.chatWindow.handleMessage(chatEventMessage{
			message: &command.Notice{
				FakeTimestamp: time.Now(),
				MsgID:         command.MsgID(uuid.NewString()),
				Message:       notice,
			},
			isFakeEvent:                 true,
			messageContentEmoteOverride: notice,
		})

		return m, nil
	}

	if event, ok := msg.(chatEventMessage); ok {
		if privMsg, ok := event.message.(*command.PrivateMessage); ok {
			var mentioned bool

			for iu := range m.usernames {
				if messageContainsCaseInsensitive(privMsg, m.usernames[iu]) {
					privMsg.Message = fmt.Sprintf("%s [mentioned in %s]", privMsg.Message, privMsg.ChannelUserName)
					mentioned = true
					break
				}
			}

			if !mentioned {
				return m, nil
			}

			var cmds []tea.Cmd
			m.chatWindow, cmd = m.chatWindow.Update(msg)
			cmds = append(cmds, cmd)
			cmds = append(cmds, func() tea.Msg {
				return requestNotificationIconMessage{
					tabID: m.id,
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

func (m *mentionTab) State() tabState {
	return m.state
}

func (m *mentionTab) IsDataLoaded() bool {
	return m.hasDataLoaded
}

func (m *mentionTab) ID() string {
	return m.id
}

func (m *mentionTab) Focused() bool {
	return m.focused
}

func (m *mentionTab) ChannelID() string {
	return ""
}

func (m *mentionTab) HandleResize() {
	m.chatWindow.width = m.width
	m.chatWindow.height = m.height
	m.chatWindow.recalculateLines()
}

func (m *mentionTab) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *mentionTab) Kind() tabKind {
	return mentionTabKind
}
