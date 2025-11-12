package mainui

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/twitch/command"
	"github.com/julez-dev/chatuino/twitch/ivr"
	"github.com/rs/zerolog"
)

type setUserInspectData struct {
	target        string
	err           error
	ivrResp       ivr.SubAgeResponse
	userData      twitch.UserData
	initialEvents []chatEventMessage
}

type userInspect struct {
	subAge            ivr.SubAgeResponse
	userData          twitch.UserData
	err               error
	isDataFetched     bool
	userConfiguration UserConfiguration

	width, height   int
	tabID           string // used to identify the tab, can be used here too since a tab only ever has one user inspect at once
	user            string // the chatter
	channel         string // the streamer
	badges          []command.Badge
	formattedBadges []string

	ivr           *ivr.API
	ttvAPI        APIClient
	emoteReplacer EmoteReplacer
	badgeReplacer BadgeReplacer
	messageLogger MessageLogger

	chatWindow *chatWindow
}

func newUserInspect(logger zerolog.Logger, ttvAPI APIClient, tabID string, width, height int, user, channel string, keymap save.KeyMap, emoteReplacer EmoteReplacer, messageLogger MessageLogger, userConfiguration UserConfiguration, badgeReplacer BadgeReplacer) *userInspect {
	c := newChatWindow(logger, width, height, keymap, userConfiguration)
	c.timeFormatFunc = func(t time.Time) string {
		return t.Local().Format("2006-01-02 15:04:05")
	}

	return &userInspect{
		tabID:   tabID,
		channel: channel,
		user:    user,
		ivr:     ivr.NewAPI(http.DefaultClient),
		ttvAPI:  ttvAPI,
		// start chat window in full size, will be resized once data is fetched
		chatWindow:        c,
		userConfiguration: userConfiguration,
		emoteReplacer:     emoteReplacer,
		messageLogger:     messageLogger,
		badgeReplacer:     badgeReplacer,
	}
}

func (u *userInspect) Init() tea.Cmd {
	return u.init(nil)
}

func (u *userInspect) init(initialEvents []chatEventMessage) tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, u.chatWindow.Init())
	cmds = append(cmds, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		ivrResp, err := u.ivr.GetSubAge(ctx, u.user, u.channel)
		if err != nil {
			return setUserInspectData{
				target: u.tabID,
				err:    fmt.Errorf("failed to fetch user data for: %w", err),
			}
		}

		ctx, cancel = context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		ttvResp, err := u.ttvAPI.GetUsers(ctx, []string{ivrResp.User.Login}, nil)
		if err != nil {
			return setUserInspectData{
				target: u.tabID,
				err:    fmt.Errorf("failed to fetch user data for: %s: %w", ivrResp.User.ID, err),
			}
		}

		if len(ttvResp.Data) != 1 {
			return setUserInspectData{
				target: u.tabID,
				err:    fmt.Errorf("failed to fetch user data for: %s", ivrResp.User.ID),
			}
		}

		// get all recent messages for user
		loggedEntries, err := u.messageLogger.MessagesFromUserInChannel(u.user, u.channel)
		if err != nil {
			return setUserInspectData{
				target: u.tabID,
				err:    fmt.Errorf("failed to fetch user logs: %w", err),
			}
		}

		fakeInitialEvent := make([]chatEventMessage, 0, len(loggedEntries))
		for loggedEntry := range slices.Values(loggedEntries) {
			// remove duplicate messages
			isAlreadyStored := slices.ContainsFunc(initialEvents, func(e chatEventMessage) bool {
				privMSG, ok := e.message.(*command.PrivateMessage)

				if !ok {
					return false
				}

				return privMSG.ID == loggedEntry.PrivateMessage.ID
			})

			if isAlreadyStored {
				continue
			}

			prepare, contentOverwrite, _ := u.emoteReplacer.Replace(ttvResp.Data[0].ID, loggedEntry.PrivateMessage.Message, loggedEntry.PrivateMessage.Emotes)
			io.WriteString(os.Stdout, prepare)

			prepare, badgeOverwrite, _ := u.badgeReplacer.Replace(ttvResp.Data[0].ID, loggedEntry.PrivateMessage.Badges)
			io.WriteString(os.Stdout, prepare)

			fakeInitialEvent = append(fakeInitialEvent, chatEventMessage{
				isFakeEvent:                 true,
				message:                     loggedEntry.PrivateMessage,
				messageContentEmoteOverride: contentOverwrite,
				badgeReplacement:            badgeOverwrite,
			})
		}

		// prepend all messages
		initialEvents = append(fakeInitialEvent, initialEvents...)

		// sort all messages by timestamp
		slices.SortFunc(initialEvents, func(e1, e2 chatEventMessage) int {
			var (
				t1 time.Time
				t2 time.Time
			)

			switch msg := e1.message.(type) {
			case *command.PrivateMessage:
				t1 = msg.TMISentTS
			case *command.ClearChat:
				t1 = msg.TMISentTS
			}

			switch msg := e2.message.(type) {
			case *command.PrivateMessage:
				t2 = msg.TMISentTS
			case *command.ClearChat:
				t2 = msg.TMISentTS
			}

			return t1.Compare(t2)
		})

		return setUserInspectData{
			target:        u.tabID,
			err:           err,
			ivrResp:       ivrResp,
			userData:      ttvResp.Data[0],
			initialEvents: initialEvents,
		}
	})
	return tea.Batch(cmds...)
}

func (u *userInspect) Update(msg tea.Msg) (*userInspect, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case setUserInspectData:
		if msg.target != u.tabID {
			return u, nil
		}

		u.err = msg.err
		u.subAge = msg.ivrResp
		u.userData = msg.userData
		u.isDataFetched = true

		for event := range slices.Values(msg.initialEvents) {
			u, cmd = u.Update(event)
			cmds = append(cmds, cmd)
		}

		u.handleResize()
		u.chatWindow.moveToBottom()
		return u, tea.Batch(cmds...)
	}

	chatEvent, ok := msg.(chatEventMessage)

	// we don't need to intervene if the message is not a chat event, the window can handle it
	if !ok {
		u.chatWindow, cmd = u.chatWindow.Update(msg)
		cmds = append(cmds, cmd)
		return u, tea.Batch(cmds...)
	}

	switch msg := chatEvent.message.(type) {
	case *command.PrivateMessage:
		// user inspect user is not sender and message does not contain current user
		if !strings.EqualFold(msg.DisplayName, u.user) && !messageContainsCaseInsensitive(msg, u.user) {
			return u, nil
		}
	case *command.ClearChat:
		// let all clear chat messages through if affect user inspect user or sender
		// of message in user inspect chat window
		var affectsUserInChat bool

		if msg.UserName == nil {
			return u, nil
		}

		for _, e := range u.chatWindow.entries {
			if priv, ok := e.Event.message.(*command.PrivateMessage); ok && strings.EqualFold(priv.DisplayName, *msg.UserName) {
				affectsUserInChat = true
				break
			}
		}

		if !strings.EqualFold(*msg.UserName, u.user) && !affectsUserInChat {
			return u, nil
		}

	default: // exit early if not a private message or clear chat message (timeout/ban)
		return u, nil
	}

	// set badges, update for each message
	// update badges if user inspect user is sender
	if msg, ok := chatEvent.message.(*command.PrivateMessage); ok && strings.EqualFold(msg.DisplayName, u.user) {
		u.badges = msg.Badges
		u.formattedBadges = chatEvent.badgeReplacement
	}

	u.chatWindow, cmd = u.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return u, tea.Batch(cmds...)
}

func (u *userInspect) View() string {
	uiView := u.renderUserInfo()

	if uiView != "" {
		return uiView + "\n" + u.chatWindow.View()
	}

	return u.chatWindow.View()
}

func (u *userInspect) handleResize() {
	uiView := u.renderUserInfo()
	var uiViewHeight int

	if uiView != "" {
		uiViewHeight = lipgloss.Height(uiView)
	}

	u.chatWindow.height = u.height - uiViewHeight
	u.chatWindow.width = u.width
	u.chatWindow.recalculateLines()
}

func (u *userInspect) renderUserInfo() string {
	border := lipgloss.Border{
		Top:    "+",
		Bottom: "+",
	}

	style := lipgloss.NewStyle().
		Padding(0).
		Border(border, true).
		BorderForeground(lipgloss.Color(u.userConfiguration.Theme.InspectBorderColor)).
		Width(u.width - 2)

	styleCentered := style.MaxWidth(u.width).AlignHorizontal(lipgloss.Center)

	if !u.isDataFetched {
		// render with some new lines to look a little bit better once all data is available
		return styleCentered.Render("\n", "Fetching data...", "\n")
	}

	if u.err != nil {
		return styleCentered.Render(fmt.Sprintf("Error while fetching data: %s", u.err.Error()))
	}

	b := &strings.Builder{}
	_, _ = fmt.Fprintf(b, "User %s (%s)", u.subAge.User.DisplayName, u.subAge.User.ID)
	if len(u.formattedBadges) > 0 {
		_, _ = fmt.Fprintf(b, " - %s\n", formatBadgeReplacement(u.userConfiguration.Settings, u.formattedBadges))
	} else {
		_, _ = fmt.Fprintf(b, "\n")
	}

	_, _ = fmt.Fprintf(b, "Account created at: %s", u.userData.CreatedAt.Format("02.01.2006 15:04:05"))

	if !u.subAge.FollowedAt.IsZero() {
		_, _ = fmt.Fprintf(b, " - Following since: %s\n", u.subAge.FollowedAt.Format("02.01.2006 15:04:05"))
	} else {
		b.WriteString(" - User does not follow the channel\n")
	}

	if u.subAge.Cumulative.Months > 0 {
		_, _ = fmt.Fprintf(b, "Subscribed for %d Months", u.subAge.Cumulative.Months)
	}

	if u.subAge.Streak.Months > 0 {
		_, _ = fmt.Fprintf(b, " - %d Month Sub Streak!", u.subAge.Streak.Months)
	}

	return style.Render(strings.TrimSuffix(b.String(), "\n"))
}
