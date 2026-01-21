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
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/ui/component"
	"github.com/rs/zerolog/log"
)

type followedFetcher interface {
	FetchUserFollowedChannels(ctx context.Context, userID string, broadcasterID string) ([]twitchapi.FollowedChannel, error)
}

type currentJoinInput int

const (
	channelInput currentJoinInput = iota
	accountSelect
	tabSelect
)

func (c currentJoinInput) String() string {
	switch c {
	case channelInput:
		return "Channel Input"
	case accountSelect:
		return "Account Input"
	case tabSelect:
		return "Tab Input"
	default:
		return "Unknown"
	}
}

type listItem struct {
	title string
	kind  tabKind
}

func (i listItem) Title() string       { return i.title }
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return i.title }

type setJoinAccountsMessage struct {
	accounts []save.Account
}

type setJoinSuggestionMessage struct {
	suggestions []string
}

type join struct {
	focused       bool
	width, height int
	input         *component.SuggestionTextInput
	tabKindList   list.Model
	accountList   list.Model
	selectedInput currentJoinInput

	accounts         []save.Account
	deps             *DependencyContainer
	followedFetchers map[string]followedFetcher
	hasLoaded        bool
}

func createDefaultList(height int, selectedColor string) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = lipgloss.NewStyle().AlignHorizontal(lipgloss.Center)
	delegate.Styles.SelectedTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color(selectedColor))
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

func newJoin(parentWidth int, deps *DependencyContainer) *join {
	emptyUserMap := map[string]func(...string) string{}

	// Calculate modal width as 60% of parent
	modalWidth := int(float64(parentWidth) * 0.6)
	if modalWidth < 40 {
		modalWidth = 40 // Minimum width
	}
	if modalWidth > 80 {
		modalWidth = 80 // Maximum width to prevent being too wide on large terminals
	}

	input := component.NewSuggestionTextInput(emptyUserMap, nil)
	input.DisableAutoSpaceSuggestion = true
	input.InputModel.CharLimit = 25
	input.InputModel.Prompt = " "
	input.InputModel.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(deps.UserConfig.Theme.InputPromptColor))
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
	input.DisableHistory = true
	input.InputModel.Cursor.BlinkSpeed = time.Millisecond * 750
	// Set input width to reasonable size (will be centered in modal)
	input.SetWidth(7)
	input.InputModel.Width = 7
	// Set Space key to accept autocomplete suggestions
	// Note: Must use " " (literal space) not "space" for key.Matches to work
	input.KeyMap.AcceptSuggestion.SetKeys(" ")

	tabKindList := createDefaultList(0, deps.UserConfig.Theme.ListSelectedColor)
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

	channelList := createDefaultList(0, deps.UserConfig.Theme.ListSelectedColor)
	channelList.SetStatusBarItemName("account", "accounts")

	followedFetchers := map[string]followedFetcher{}
	for id, client := range deps.APIUserClients {
		if c, ok := client.(followedFetcher); ok {
			followedFetchers[id] = c
		}
	}

	return &join{
		width:            modalWidth,
		height:           0, // Height will be dynamic based on content
		input:            input,
		deps:             deps,
		accountList:      channelList,
		tabKindList:      tabKindList,
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
	return tea.Batch(
		func() tea.Msg {
			accounts, err := j.deps.AccountProvider.GetAllAccounts()
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
			accounts, err := j.deps.AccountProvider.GetAllAccounts()
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

			if key.Matches(msg, j.deps.Keymap.Next) {
				// don't allow next input when mention or live notification tab selected
				if i, ok := j.tabKindList.SelectedItem().(listItem); ok && (i.title == mentionTabKind.String() || i.title == liveNotificationTabKind.String()) {
					// For mention/live notification tabs, Tab does nothing (only one field)
					return j, nil
				}

				switch j.selectedInput {
				case tabSelect:
					j.selectedInput = accountSelect
				case accountSelect:
					j.selectedInput = channelInput
					cmd = j.input.InputModel.Cursor.BlinkCmd()
				case channelInput:
					j.selectedInput = tabSelect
				}

				return j, cmd
			}

			if key.Matches(msg, j.deps.Keymap.Previous) {
				// don't allow previous input when mention or live notification tab selected
				if i, ok := j.tabKindList.SelectedItem().(listItem); ok && (i.title == mentionTabKind.String() || i.title == liveNotificationTabKind.String()) {
					// For mention/live notification tabs, Shift+Tab does nothing (only one field)
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

			// Enter key confirms join from any field when inputs are valid
			kind := j.tabKindList.SelectedItem().(listItem).kind

			// Check if inputs are valid for confirmation
			isValid := (j.input.Value() != "" && kind == broadcastTabKind) ||
				kind == mentionTabKind ||
				kind == liveNotificationTabKind

			if key.Matches(msg, j.deps.Keymap.Confirm) && isValid {
				channel := j.input.Value()
				account := j.accounts[j.accountList.Cursor()]

				return j, func() tea.Msg {
					// Normalize channel name via Twitch API for broadcast tabs
					if kind == broadcastTabKind {
						for accountID, client := range j.deps.APIUserClients {
							if accountID != account.ID {
								continue
							}

							resp, err := client.GetUsers(context.Background(), []string{channel}, nil)
							if err != nil {
								break
							}

							if len(resp.Data) < 1 {
								break
							}

							channel = resp.Data[0].Login
							break
						}
					}

					return joinChannelMessage{
						tabKind: kind,
						channel: channel,
						account: account,
					}
				}
			}
		}
	}

	switch j.selectedInput {
	case channelInput:
		// For channel input, always pass to input component for handling
		// This includes Space key for autocomplete
		j.input, cmd = j.input.Update(msg)
		cmds = append(cmds, cmd)

		// on update change the width to the width of content
		iw := lipgloss.Width(j.input.Value())

		if iw < 7 {
			iw = 7
		}

		j.input.SetWidth(iw)
		j.input.InputModel.Width = iw
	case tabSelect:
		j.tabKindList, cmd = j.tabKindList.Update(msg)
		cmds = append(cmds, cmd)
	default:
		j.accountList, cmd = j.accountList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return j, tea.Batch(cmds...)
}

func (j *join) View() string {
	// Modal content style with rounded borders and theme colors
	// Use calculated width (60% of parent, updated on resize)
	style := lipgloss.NewStyle().
		Width(j.width).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(j.deps.UserConfig.Theme.ListLabelColor))

	// Center content within the calculated width
	styleCenter := lipgloss.NewStyle().Width(j.width).AlignHorizontal(lipgloss.Center)

	// Left-aligned style for status
	styleLeft := lipgloss.NewStyle().Width(j.width).AlignHorizontal(lipgloss.Left)

	labelStyle := lipgloss.NewStyle().MarginBottom(1).MarginTop(1).Foreground(lipgloss.Color(j.deps.UserConfig.Theme.ListLabelColor)).Render

	var (
		labelTab      string
		labelChannel  string
		labelIdentity string
	)

	selectedLabelStyle := lipgloss.NewStyle().MarginBottom(1).MarginTop(1).Foreground(lipgloss.Color(j.deps.UserConfig.Theme.ActiveLabelColor)).Bold(true).Render

	switch j.selectedInput {
	case channelInput:
		labelTab = labelStyle("Tab type")
		labelChannel = selectedLabelStyle("Channel")
		labelIdentity = labelStyle("Identity")
	case accountSelect:
		labelTab = labelStyle("Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = selectedLabelStyle("Identity")
	case tabSelect:
		labelTab = selectedLabelStyle("Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = labelStyle("Identity")
	default:
		labelTab = labelStyle("Tab type")
		labelChannel = labelStyle("Channel")
		labelIdentity = labelStyle("Identity")
	}

	b := strings.Builder{}

	// Headline
	headlineStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(j.deps.UserConfig.Theme.ActiveLabelColor)).MarginBottom(1)
	_, _ = b.WriteString(styleCenter.Render(headlineStyle.Render("Create new Tab")) + "\n")

	// If mention tab is selected, only display kind select input, because other values are not needed
	if i, ok := j.tabKindList.SelectedItem().(listItem); ok && (i.title == mentionTabKind.String() || i.title == liveNotificationTabKind.String()) {
		_, _ = b.WriteString(styleCenter.Render(labelTab + "\n" + j.tabKindList.View() + "\n"))
	} else {
		_, _ = labelIdentity, labelChannel

		_, _ = b.WriteString(styleCenter.Render(labelTab))
		_, _ = b.WriteString(styleCenter.Render(j.tabKindList.View()))
		_, _ = b.WriteString("\n")

		_, _ = b.WriteString(styleCenter.Render(labelIdentity))
		_, _ = b.WriteString(styleCenter.Render(j.accountList.View()))
		_, _ = b.WriteString("\n")

		_, _ = b.WriteString(styleCenter.Render(labelChannel))
		_, _ = b.WriteString("\n")

		// Center each line separately so suggestion text aligns with input
		for _, line := range strings.Split(j.input.View(), "\n") {
			_, _ = b.WriteString(styleCenter.Render(line) + "\n")
		}
	}

	// Show keybind hints (centered, styled with theme)
	_, _ = b.WriteString("\n\n")
	var hints string
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(j.deps.UserConfig.Theme.StatusColor)).Faint(true)

	switch j.selectedInput {
	case channelInput:
		hints = hintStyle.Render("Space: autocomplete | Enter: confirm | Tab: next field")
	case tabSelect, accountSelect:
		hints = hintStyle.Render("↑/↓: select | Enter: confirm | Tab: next field")
	default:
		hints = hintStyle.Render("Enter: confirm | Tab: next field")
	}
	_, _ = b.WriteString(styleCenter.PaddingBottom(1).Render(hints))

	// Show status at bottom (left-aligned)
	_, _ = b.WriteString("\n")
	stateStr := fmt.Sprintf(" -- %s --", lipgloss.NewStyle().Foreground(lipgloss.Color(j.deps.UserConfig.Theme.StatusColor)).Render(j.selectedInput.String()))
	_, _ = b.WriteString(styleLeft.Render(stateStr))

	return style.Padding(0).Render(b.String())
}

func (c *join) focus() {
	c.focused = true
	c.input.Focus()
}

func (c *join) blur() {
	c.focused = false
	c.input.Blur()
}

func (c *join) handleResize(parentWidth, parentHeight int) {
	// Recalculate modal width as 60% of parent
	modalWidth := int(float64(parentWidth) * 0.6)
	if modalWidth < 40 {
		modalWidth = 40 // Minimum width
	}
	if modalWidth > 80 {
		modalWidth = 80 // Maximum width to prevent being too wide on large terminals
	}

	c.width = modalWidth
	c.height = 0 // Dynamic height based on content
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
