package mainui

import (
	"context"
	"fmt"
	"maps"
	"slices"
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

type joinState int

const (
	joinViewMode joinState = iota
	joinInsertMode
)

func (j joinState) String() string {
	if j == joinViewMode {
		return "View/Select Input"
	} else {
		return "Insert"
	}
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

	state joinState
}

func createDefaultList(height int) list.Model {
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

	tabKindList := createDefaultList(height)
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
		listItem{
			title: liveNotificationTabKind.String(),
			kind:  liveNotificationTabKind,
		},
	})
	tabKindList.Select(0)
	tabKindList.SetHeight(4)

	channelList := createDefaultList(height)
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
		state:            joinInsertMode,
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

			return setJoinSuggestionMessage{suggestions: slices.Collect(maps.Keys(uniqueChannels))}
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

		hasNormalAccount := slices.ContainsFunc(j.accounts, func(e save.Account) bool {
			return !e.IsAnonymous
		})

		// remove mention tab, when no non-anonymous accounts were found
		if !hasNormalAccount {
			j.tabKindList.RemoveItem(1)
		}

		j.accountList.SetItems(listItems)
		j.accountList.Select(index)
		j.accountList.SetHeight(len(j.accounts) + 1)

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

			if j.state == joinViewMode && key.Matches(msg, j.keymap.InsertMode) {
				j.state = joinInsertMode
				return j, nil
			}

			if j.state == joinInsertMode && key.Matches(msg, j.keymap.Escape) {
				j.state = joinViewMode
				return j, nil
			}

			if j.state == joinViewMode && key.Matches(msg, j.keymap.Up) {
				// don't allow next input when mention or live notification tab selected
				if i, ok := j.tabKindList.SelectedItem().(listItem); ok && (i.title == mentionTabKind.String() || i.title == liveNotificationTabKind.String()) {
					j.selectedInput = tabSelect
					return j, nil
				}

				switch j.selectedInput {
				case tabSelect:
					j.selectedInput = channelInput
					cmd = j.input.InputModel.Cursor.BlinkCmd()
				case channelInput:
					j.selectedInput = accountSelect
				case accountSelect:
					j.selectedInput = tabSelect
				}

				return j, cmd
			}

			if key.Matches(msg, j.keymap.Confirm) && (j.input.Value() != "" || j.tabKindList.SelectedItem().(listItem).kind != broadcastTabKind || j.tabKindList.SelectedItem().(listItem).kind != liveNotificationTabKind) {
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

	if j.state == joinInsertMode {
		switch j.selectedInput {
		case channelInput:
			j.input, cmd = j.input.Update(msg)
			cmds = append(cmds, cmd)
		case tabSelect:
			j.tabKindList, cmd = j.tabKindList.Update(msg)
			cmds = append(cmds, cmd)
		default:
			j.accountList, cmd = j.accountList.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return j, tea.Batch(cmds...)
}

func (j *join) View() string {
	style := lipgloss.NewStyle().
		Width(j.width).
		MaxWidth(j.width).
		Height(j.height).
		MaxHeight(j.height)

	styleCenter := lipgloss.NewStyle().Width(j.width - 2).AlignHorizontal(lipgloss.Center)

	labelStyle := lipgloss.NewStyle().MarginBottom(1).MarginTop(2).Foreground(lipgloss.Color("135")).Render

	var (
		labelTab      string
		labelChannel  string
		labelIdentity string
	)

	switch j.selectedInput {
	case channelInput:
		labelTab = labelStyle("Tab type")
		labelChannel = labelStyle("> Channel")
		labelIdentity = labelStyle("Identity")
	case accountSelect:
		labelTab = labelStyle("Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = labelStyle("> Identity")
	default:
		labelTab = labelStyle("> Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = labelStyle("Identity")
	}

	b := strings.Builder{}

	// If mention tab is selected, only display kind select input, because other values are not needed
	if i, ok := j.tabKindList.SelectedItem().(listItem); ok && (i.title == mentionTabKind.String() || i.title == liveNotificationTabKind.String()) {
		_, _ = b.WriteString(styleCenter.Render(labelTab + "\n" + j.tabKindList.View() + "\n"))
	} else {
		_, _ = b.WriteString(styleCenter.Render(labelTab + "\n" + j.tabKindList.View() + "\n"))
		_, _ = b.WriteString(styleCenter.Render(labelChannel + "\n" + j.input.View() + "\n"))
		_, _ = b.WriteString(styleCenter.Render(labelIdentity + "\n" + j.accountList.View() + "\n"))
	}

	// show status at bottom
	heightUntilNow := lipgloss.Height(b.String())
	spacerHeight := j.height - heightUntilNow
	if spacerHeight > 0 {
		_, _ = b.WriteString(strings.Repeat("\n", spacerHeight))
	}

	stateStr := fmt.Sprintf(" -- %s --", lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Render(j.state.String()))
	_, _ = b.WriteString(stateStr)

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

func (c *join) setTabOptions(kinds ...tabKind) {
	var items []list.Item

	for _, kind := range kinds {
		items = append(items, listItem{
			title: kind.String(),
			kind:  kind,
		})
	}

	c.tabKindList.SetItems(
		items,
	)
}
