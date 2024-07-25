package mainui

import (
	"context"
	"fmt"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/rs/zerolog/log"
)

type followedFetcher interface {
	FetchUserFollowedChannels(ctx context.Context, userID string, broadcasterID string) ([]twitch.FollowedChannel, error)
}

type currentJoinInput int

const (
	channelInput currentJoinInput = iota
	accountSelect
)

type listItem struct {
	title string
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return i.title }

type joinChannelMessage struct {
	channel string
	account save.Account
}

type setAccountsMessage struct {
	accounts    []save.Account
	suggestions []string
}

type join struct {
	focused          bool
	width, height    int
	input            *component.SuggestionTextInput
	list             list.Model
	selectedInput    currentJoinInput
	accounts         []save.Account
	keymap           save.KeyMap
	provider         AccountProvider
	followedFetchers map[string]followedFetcher
	hasLoaded        bool
}

func newJoin(provider AccountProvider, clients map[string]APIClient, width, height int, keymap save.KeyMap) *join {
	emptyUserMap := map[string]func(...string) string{}

	input := component.NewSuggestionTextInput(emptyUserMap)
	input.InputModel.CharLimit = 25
	input.InputModel.Prompt = " "
	input.InputModel.Placeholder = "Channel"
	input.InputModel.Validate = func(s string) error {
		for _, r := range s {
			if unicode.IsSpace(r) {
				return fmt.Errorf("white space not allowed")
			}
		}
		return nil
	}
	input.IncludeCommandSuggestions = false
	input.InputModel.Cursor.BlinkSpeed = time.Millisecond * 750
	input.SetWidth(width)

	channelList := list.New(nil, list.NewDefaultDelegate(), width, height/2)

	channelList.Select(0)
	channelList.SetShowHelp(false)
	channelList.SetShowPagination(false)
	channelList.SetShowTitle(false)
	channelList.DisableQuitKeybindings()
	channelList.SetShowStatusBar(false)
	channelList.SetStatusBarItemName("account", "accounts")

	followedFetchers := map[string]followedFetcher{}
	for id, client := range clients {
		if c, ok := client.(followedFetcher); ok {
			followedFetchers[id] = c
		}
	}

	return &join{
		width:            width,
		height:           height,
		input:            input,
		provider:         provider,
		list:             channelList,
		keymap:           keymap,
		followedFetchers: followedFetchers,
	}
}

func (j *join) Init() tea.Cmd {
	return tea.Batch(func() tea.Msg {
		accounts, err := j.provider.GetAllAccounts()
		if err != nil {
			return nil
		}

		for i, a := range accounts {
			if a.IsAnonymous {
				accounts[i].DisplayName = "Anonymous"
			}
		}

		uniqueChannels := map[string]struct{}{}
		for id, fetcher := range j.followedFetchers {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			followed, err := fetcher.FetchUserFollowedChannels(ctx, id, "")

			// suggestions are not important enough to fail the whole join command
			// just skip if the call fails
			if err != nil {
				log.Logger.Err(err).Str("account-id", id).Msg("could not fetch followed channels")
				continue
			}

			for _, f := range followed {
				uniqueChannels[f.BroadcasterLogin] = struct{}{}
			}
		}

		channelSuggestions := make([]string, 0, len(uniqueChannels))
		for c := range uniqueChannels {
			channelSuggestions = append(channelSuggestions, c)
		}

		return setAccountsMessage{accounts: accounts, suggestions: channelSuggestions}
	}, j.input.InputModel.Cursor.BlinkCmd())
}

func (j *join) Update(msg tea.Msg) (*join, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if msg, ok := msg.(setAccountsMessage); ok {
		j.accounts = msg.accounts
		listItems := make([]list.Item, 0, len(j.accounts))

		var index int
		for i, a := range j.accounts {
			listItems = append(listItems, listItem{title: a.DisplayName})

			if a.IsMain {
				index = i
			}
		}

		j.list.SetItems(listItems)
		j.list.Select(index)

		j.input.SetSuggestions(msg.suggestions)
		j.hasLoaded = true

		return j, nil
	}

	if j.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if !j.hasLoaded {
				return j, nil
			}

			if key.Matches(msg, j.keymap.NextFilter, j.keymap.PrevFilter) {
				if j.selectedInput == channelInput {
					j.selectedInput = accountSelect
				} else {
					j.selectedInput = channelInput
					cmd = j.input.InputModel.Cursor.BlinkCmd()
				}

				return j, cmd
			}

			if key.Matches(msg, j.keymap.Confirm) && j.input.Value() != "" {
				return j, func() tea.Msg {
					return joinChannelMessage{
						channel: j.input.Value(),
						account: j.accounts[j.list.Cursor()],
					}
				}
			}
		}
	}

	if j.selectedInput == channelInput {
		j.input, cmd = j.input.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		j.list, cmd = j.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return j, tea.Batch(cmds...)
}

func (j *join) View() string {
	style := lipgloss.NewStyle().
		Width(j.width - 2). // - border width
		MaxWidth(j.width).
		Height(j.height - 2). // - border height
		MaxHeight(j.height).
		Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("135")).
		AlignHorizontal(lipgloss.Center)
	// AlignVertical(lipgloss.Center)

	labelStyle := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render
	labelIdentityStyle := lipgloss.NewStyle().MarginBottom(2).MarginTop(2).Foreground(lipgloss.Color("135")).Render

	var (
		label         string
		labelIdentity string
	)

	if j.selectedInput == channelInput {
		label = labelStyle("> Enter a channel to join")
		labelIdentity = labelIdentityStyle("Choose an identity")
	} else {
		label = labelStyle("Enter a channel to join")
		labelIdentity = labelIdentityStyle("> Choose an identity")
	}

	return style.Render(label + "\n" + j.input.View() + "\n" + labelIdentity + "\n" + j.list.View())
}

func (c *join) focus() {
	c.focused = true
	c.input.Focus()
}

func (c *join) blur() {
	c.focused = false
	c.input.Blur()
}
