package mainui

import (
	"context"
	"fmt"
	"strings"
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
	tabSelect
)

type listItem struct {
	title string
	kind  tabKind
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return i.title }

type joinChannelMessage struct {
	tabKind tabKind
	channel string
	account save.Account
}

type setJoinAccountsMessage struct {
	accounts []save.Account
}

type setJoinSuggestionMessage struct {
	suggestions []string
}

type join struct {
	focused          bool
	width, height    int
	input            *component.SuggestionTextInput
	tabKindList      list.Model
	accountList      list.Model
	selectedInput    currentJoinInput
	accounts         []save.Account
	keymap           save.KeyMap
	provider         AccountProvider
	followedFetchers map[string]followedFetcher
	hasLoaded        bool
}

func createDefaultList(width, height int) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = lipgloss.NewStyle().AlignHorizontal(lipgloss.Center)
	delegate.Styles.SelectedTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color("135"))
	delegate.ShowDescription = false
	delegate.SetSpacing(0)

	newList := list.New(nil, delegate, 20, height/2)

	newList.Select(0)
	newList.SetShowHelp(false)
	newList.SetShowPagination(false)
	newList.SetShowTitle(false)
	newList.DisableQuitKeybindings()
	newList.SetShowStatusBar(false)
	newList.Styles = list.Styles{}

	return newList
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

	tabKindList := createDefaultList(width, height)
	tabKindList.SetStatusBarItemName("kind", "kinds")
	tabKindList.SetItems([]list.Item{
		listItem{
			title: broadcastTabKind.String(),
			kind:  broadcastTabKind,
		},
		listItem{
			title: mentionTabKind.String(),
			kind:  mentionTabKind,
		},
	})
	tabKindList.Select(0)
	tabKindList.SetHeight(3)

	channelList := createDefaultList(width, height)
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
		accountList:      channelList,
		tabKindList:      tabKindList,
		keymap:           keymap,
		followedFetchers: followedFetchers,
	}
}

// Init loads initial data in batch
// - The accounts for the account selection
// - The suggestions for the text input
// - Text blinking
// All done concurrently because fetching suggestions will most likely take the most time
// So the user does not have to wait if they can type faster
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

		return setJoinAccountsMessage{accounts: accounts}
	},
		func() tea.Msg {
			accounts, err := j.provider.GetAllAccounts()
			if err != nil {
				return nil
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

			for _, a := range accounts {
				if a.IsAnonymous {
					continue
				}

				uniqueChannels[a.DisplayName] = struct{}{}
			}

			channelSuggestions := make([]string, 0, len(uniqueChannels))
			for c := range uniqueChannels {
				channelSuggestions = append(channelSuggestions, c)
			}

			return setJoinSuggestionMessage{suggestions: channelSuggestions}
		},
		j.input.InputModel.Cursor.BlinkCmd(),
	)
}

func (j *join) Update(msg tea.Msg) (*join, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if msg, ok := msg.(setJoinAccountsMessage); ok {
		j.accounts = msg.accounts
		listItems := make([]list.Item, 0, len(j.accounts))

		var index int
		for i, a := range j.accounts {
			listItems = append(listItems, listItem{title: a.DisplayName})

			if a.IsMain {
				index = i
			}
		}

		j.accountList.SetItems(listItems)
		j.accountList.Select(index)

		j.hasLoaded = true

		return j, nil
	}

	if msg, ok := msg.(setJoinSuggestionMessage); ok {
		j.input.SetSuggestions(msg.suggestions)
		return j, nil
	}

	if j.focused {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if !j.hasLoaded {
				return j, nil
			}

			if key.Matches(msg, j.keymap.NextInput) {
				// don't allow next input when mention selected
				if i, ok := j.tabKindList.SelectedItem().(listItem); ok && i.title == mentionTabKind.String() {
					j.selectedInput = tabSelect
					return j, nil
				}

				if j.selectedInput == tabSelect {
					j.selectedInput = channelInput
					cmd = j.input.InputModel.Cursor.BlinkCmd()
				} else if j.selectedInput == channelInput {
					j.selectedInput = accountSelect
				} else if j.selectedInput == accountSelect {
					j.selectedInput = tabSelect
				}

				return j, cmd
			}

			if key.Matches(msg, j.keymap.Confirm) && (j.input.Value() != "" || j.tabKindList.SelectedItem().(listItem).kind != broadcastTabKind) {
				return j, func() tea.Msg {
					return joinChannelMessage{
						tabKind: j.tabKindList.SelectedItem().(listItem).kind,
						channel: j.input.Value(),
						account: j.accounts[j.accountList.Cursor()],
					}
				}
			}
		}
	}

	if j.selectedInput == channelInput {
		j.input, cmd = j.input.Update(msg)
		cmds = append(cmds, cmd)
	} else if j.selectedInput == tabSelect {
		j.tabKindList, cmd = j.tabKindList.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		j.accountList, cmd = j.accountList.Update(msg)
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

	labelStyle := lipgloss.NewStyle().MarginBottom(1).MarginTop(2).Foreground(lipgloss.Color("135")).Render

	var (
		labelTab      string
		labelChannel  string
		labelIdentity string
	)

	// If mention tab is selected, only display kind select input, because other values are not needed
	if i, ok := j.tabKindList.SelectedItem().(listItem); ok && i.title == mentionTabKind.String() {
		return style.Render(fmt.Sprintf("%s\n%s\n", labelStyle("> Tab type"), j.tabKindList.View()))
	}

	if j.selectedInput == channelInput {
		labelTab = labelStyle("Tab type")
		labelChannel = labelStyle("> Channel")
		labelIdentity = labelStyle("Identity")
	} else if j.selectedInput == accountSelect {
		labelTab = labelStyle("Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = labelStyle("> Identity")
	} else {
		labelTab = labelStyle("> Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = labelStyle("Identity")
	}

	b := strings.Builder{}
	_, _ = b.WriteString(labelTab + "\n" + j.tabKindList.View() + "\n")
	_, _ = b.WriteString(labelChannel + "\n" + j.input.View() + "\n")
	_, _ = b.WriteString(labelIdentity + "\n" + j.accountList.View() + "\n")

	return style.Render(b.String())
}

func (c *join) focus() {
	c.focused = true
	c.input.Focus()
}

func (c *join) blur() {
	c.focused = false
	c.input.Blur()
}
