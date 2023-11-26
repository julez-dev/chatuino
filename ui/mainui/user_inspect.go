package mainui

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/ivr"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog"
)

type setUserInspectData struct {
	target   string
	err      error
	ivrResp  ivr.SubAgeResponse
	userData twitch.UserData
}

type userInspect struct {
	subAge        ivr.SubAgeResponse
	userData      twitch.UserData
	err           error
	isDataFetched bool

	width, height int
	tabID         string // used to identify the tab, can be used here too since a tab only ever has one user inspect at once
	user          string // the chatter
	channel       string // the streamer

	ivr        *ivr.API
	ttvAPI     apiClient
	chatWindow *chatWindow
}

func newUserInspect(logger zerolog.Logger, ttvAPI apiClient, tabID string, width, height int, user, channel string, channelID string, emoteStore EmoteStore) *userInspect {
	return &userInspect{
		tabID:   tabID,
		channel: channel,
		user:    user,
		ivr:     ivr.NewAPI(http.DefaultClient),
		ttvAPI:  ttvAPI,
		// start chat window in full size, will be resized once data is fetched
		chatWindow: newChatWindow(logger, tabID, width, height, channel, channelID, emoteStore),
	}
}

func (u *userInspect) Init() tea.Cmd {
	var cmds []tea.Cmd

	cmds = append(cmds, u.chatWindow.Init())
	cmds = append(cmds, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		ivrResp, err := u.ivr.GetSubAge(ctx, u.user, u.channel)
		if err != nil {
			return setUserInspectData{
				target: u.tabID,
				err:    err,
			}
		}

		ctx, cancel = context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		ttvResp, err := u.ttvAPI.GetUsers(ctx, nil, []string{ivrResp.User.ID})
		if err != nil {
			return setUserInspectData{
				target: u.tabID,
				err:    err,
			}
		}

		if len(ttvResp.Data) != 1 {
			return setUserInspectData{
				target: u.tabID,
				err:    fmt.Errorf("could not return user data for: %s", ivrResp.User.ID),
			}
		}

		return setUserInspectData{
			target:   u.tabID,
			err:      err,
			ivrResp:  ivrResp,
			userData: ttvResp.Data[0],
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

		u.handleResize()
		return u, nil
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
	style := lipgloss.NewStyle().
		Padding(1).
		Border(lipgloss.BlockBorder(), true).
		BorderForeground(lipgloss.Color("135"))
		// Width(u.width)

	styleCentered := style.Copy().MaxWidth(u.width).AlignHorizontal(lipgloss.Center)

	if !u.isDataFetched {
		return styleCentered.Render("Fetching data...")
	}

	if u.err != nil {
		return styleCentered.Render(fmt.Sprintf("Error while fetching data: %s", u.err.Error()))
	}

	b := &strings.Builder{}
	fmt.Fprintf(b, "User %s (%s)\n", u.subAge.User.DisplayName, u.subAge.User.ID)
	fmt.Fprintf(b, "Account created at: %s\n", u.userData.CreatedAt.Format("02.01.2006 15:04:05"))
	if !u.subAge.FollowedAt.IsZero() {
		fmt.Fprintf(b, "Following since: %s\n", u.subAge.FollowedAt.Format("02.01.2006 15:04:05"))
	} else {
		b.WriteString("User does not follow the channel\n")
	}

	if u.subAge.Cumulative.Months > 0 {
		fmt.Fprintf(b, "Subscribed for %d Months\n", u.subAge.Cumulative.Months)
	}

	if u.subAge.Streak.Months > 0 {
		fmt.Fprintf(b, "%d Month Sub Streak!", u.subAge.Streak.Months)
	}

	return style.Render(strings.TrimSuffix(b.String(), "\n"))
}
