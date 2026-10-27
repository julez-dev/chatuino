package tab

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/ivr"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/rs/zerolog/log"
)

type setUserInspectData struct {
	target         string
	err            error
	ivrResp        ivr.SubAgeResponse
	userData       twitchapi.UserData
	initialEvents  []chat.ChatEventMessage
	prepareCommand string // Kitty graphics prepare command
}

type userInspect struct {
	subAge        ivr.SubAgeResponse
	userData      twitchapi.UserData
	err           error
	isDataFetched bool

	width, height   int
	infoHeight      int    // rows the info banner above the history occupies
	tabID           string // used to identify the tab, can be used here too since a tab only ever has one user inspect at once
	user            string // the chatter
	channel         string // the streamer
	accountID       string // account id from chatuino user
	badges          []twitchirc.Badge
	formattedBadges chat.WordReplacement

	ivr           *ivr.API
	settings      save.Settings
	theme         save.Theme
	api           APIClient
	messageLogger MessageLogger
	emoteReplacer EmoteReplacer
	badgeReplacer BadgeReplacer
	spinner       spinner.Model

	chatWindow *chatWindow
}

func newUserInspect(
	tabID string,
	width, height int,
	user, channel string,
	accountID string,
	settings save.Settings,
	theme save.Theme,
	keymap save.KeyMap,
	api APIClient,
	messageLogger MessageLogger,
	emoteReplacer EmoteReplacer,
	badgeReplacer BadgeReplacer,
) *userInspect {
	c := newChatWindow(width, height, settings, theme, keymap)
	c.SetTimeFormat(settings.Chat.UserInspectTimeFormat)

	return &userInspect{
		tabID:         tabID,
		channel:       channel,
		accountID:     accountID,
		user:          user,
		ivr:           ivr.NewAPI(http.DefaultClient),
		settings:      settings,
		theme:         theme,
		api:           api,
		messageLogger: messageLogger,
		emoteReplacer: emoteReplacer,
		badgeReplacer: badgeReplacer,
		spinner:       spinner.New(spinner.WithSpinner(LoadingSpinner)),
		// start chat window in full size, will be resized once data is fetched
		chatWindow: c,
	}
}

func (u *userInspect) Init() tea.Cmd {
	return u.init(nil)
}

func (u *userInspect) init(initialEvents []chat.ChatEventMessage) tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, u.chatWindow.Init(), u.spinner.Tick)
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

		ttvResp, err := u.api.GetUsers(ctx, []string{ivrResp.User.Login}, nil)
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

		var prepareCmd strings.Builder

		fakeInitialEvent := make([]chat.ChatEventMessage, 0, len(loggedEntries))
		for loggedEntry := range slices.Values(loggedEntries) {
			// remove duplicate messages
			isAlreadyStored := slices.ContainsFunc(initialEvents, func(e chat.ChatEventMessage) bool {
				privMSG, ok := e.Message.(*twitchirc.PrivateMessage)

				if !ok {
					return false
				}

				return privMSG.ID == loggedEntry.PrivateMessage.ID
			})

			if isAlreadyStored {
				continue
			}

			prepare, contentOverwrite, _ := u.emoteReplacer.Replace(ctx, ttvResp.Data[0].ID, loggedEntry.PrivateMessage.Message, loggedEntry.PrivateMessage.Emotes)
			prepareCmd.WriteString(prepare)

			prepare, badgeOverwrite, _ := u.badgeReplacer.Replace(ctx, ttvResp.Data[0].ID, loggedEntry.PrivateMessage.Badges)
			prepareCmd.WriteString(prepare)

			if badgeOverwrite == nil {
				badgeOverwrite = make(chat.WordReplacement)
			}

			contributorPrepare, err := u.badgeReplacer.InjectContributorBadge(ctx, loggedEntry.PrivateMessage.LoginName, badgeOverwrite)
			if err != nil {
				log.Logger.Info().Err(err).Str("login", loggedEntry.PrivateMessage.LoginName).Msg("failed to inject contributor badge")
			}
			prepareCmd.WriteString(contributorPrepare)

			fakeInitialEvent = append(fakeInitialEvent, chat.ChatEventMessage{
				IsFakeEvent: true,
				Message:     loggedEntry.PrivateMessage,
				DisplayModifier: chat.MessageContentModifier{
					WordReplacements: contentOverwrite,
					BadgeReplacement: badgeOverwrite,
				},
			})
		}

		// prepend all messages
		initialEvents = append(fakeInitialEvent, initialEvents...)

		// sort all messages by timestamp
		slices.SortFunc(initialEvents, func(e1, e2 chat.ChatEventMessage) int {
			var (
				t1 time.Time
				t2 time.Time
			)

			switch msg := e1.Message.(type) {
			case *twitchirc.PrivateMessage:
				t1 = msg.TMISentTS
			case *twitchirc.ClearChat:
				t1 = msg.TMISentTS
			}

			switch msg := e2.Message.(type) {
			case *twitchirc.PrivateMessage:
				t2 = msg.TMISentTS
			case *twitchirc.ClearChat:
				t2 = msg.TMISentTS
			}

			return t1.Compare(t2)
		})

		return setUserInspectData{
			target:         u.tabID,
			err:            err,
			ivrResp:        ivrResp,
			userData:       ttvResp.Data[0],
			initialEvents:  initialEvents,
			prepareCommand: prepareCmd.String(),
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

		if msg.prepareCommand != "" {
			cmds = append(cmds, tea.Raw(msg.prepareCommand))
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

	// update spinner while loading
	if !u.isDataFetched {
		u.spinner, cmd = u.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	chatEvent, ok := msg.(chat.ChatEventMessage)

	// we don't need to intervene if the message is not a chat event, the window can handle it
	if !ok {
		u.chatWindow, cmd = u.chatWindow.Update(msg)
		cmds = append(cmds, cmd)
		return u, tea.Batch(cmds...)
	}

	switch msg := chatEvent.Message.(type) {
	case *twitchirc.PrivateMessage:
		// user inspect user is not sender and message does not contain current user
		if !strings.EqualFold(msg.LoginName, u.user) && !messageContainsCaseInsensitive(msg, u.user) {
			return u, nil
		}
	case *twitchirc.ClearChat:
		// let all clear chat messages through if affect user inspect user or sender
		// of message in user inspect chat window
		var affectsUserInChat bool

		if msg.UserName == nil {
			return u, nil
		}

		for _, e := range u.chatWindow.Entries() {
			if priv, ok := e.Event.Message.(*twitchirc.PrivateMessage); ok && strings.EqualFold(priv.LoginName, *msg.UserName) {
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
	if msg, ok := chatEvent.Message.(*twitchirc.PrivateMessage); ok && strings.EqualFold(msg.LoginName, u.user) {
		u.badges = msg.Badges
		u.formattedBadges = chatEvent.DisplayModifier.BadgeReplacement
	}

	u.chatWindow, cmd = u.chatWindow.Update(msg)
	cmds = append(cmds, cmd)

	return u, tea.Batch(cmds...)
}

func (u *userInspect) View() string {
	if u.height <= 0 {
		return ""
	}

	uiView := u.renderUserInfo()
	if lipgloss.Height(uiView) > u.height {
		return lipgloss.NewStyle().MaxHeight(u.height).Render(uiView)
	}

	chatView := u.chatWindow.View()
	if uiView == "" {
		return chatView
	}
	if u.chatWindow.height == 0 {
		return uiView
	}

	// An empty one-row viewport renders as "", but still owns its row.
	return uiView + "\n" + chatView
}

// handleResize gives the info banner its rows and the chat window the rest. The
// newline joining the two blocks terminates a row instead of adding one, so no
// extra row is reserved for it.
func (u *userInspect) handleResize() {
	chatHeight := u.height
	u.infoHeight = 0

	if uiView := u.renderUserInfo(); uiView != "" {
		u.infoHeight = lipgloss.Height(uiView)
		chatHeight = u.height - u.infoHeight
	}

	u.chatWindow.Resize(u.width, max(0, chatHeight))
}

// chatSpan is the part of the inspect window that shows history, relative to the
// window itself. The info banner above it is not clickable.
func (u *userInspect) chatSpan() layoutSpan {
	return layoutSpan{top: u.infoHeight, height: u.chatWindow.height}
}

func (u *userInspect) renderUserInfo() string {
	border := lipgloss.Border{
		Top:    "+",
		Bottom: "+",
	}

	style := lipgloss.NewStyle().
		Padding(0).
		Border(border, true).
		BorderForeground(lipgloss.Color(u.theme.InspectBorderColor)).
		Width(u.width - 2)

	styleCentered := style.MaxWidth(u.width).AlignHorizontal(lipgloss.Center)

	if !u.isDataFetched {
		return styleCentered.Render("\n", u.spinner.View()+" Fetching user data", "\n")
	}

	if u.err != nil {
		return styleCentered.Render(fmt.Sprintf("Error while fetching data: %s", u.err.Error()))
	}

	b := &strings.Builder{}
	_, _ = fmt.Fprintf(b, "User %s (%s)", u.subAge.User.DisplayName, u.subAge.User.ID)
	if len(u.formattedBadges) > 0 {
		_, _ = fmt.Fprintf(b, " - %s\n", chat.FormatBadgeReplacement(u.settings, u.formattedBadges))
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
