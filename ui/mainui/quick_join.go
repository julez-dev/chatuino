package mainui

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/rs/zerolog/log"
)

// quickJoinSection tracks which pane has focus: the channel picker or account list.
type quickJoinSection int

const (
	quickJoinChannelSection quickJoinSection = iota
	quickJoinAccountSection
)

// channelListTab tracks which channel list is visible.
type channelListTab int

const (
	recentTab channelListTab = iota
	followedTab
)

const maxVisibleChannels = 10

func (s quickJoinSection) String() string {
	switch s {
	case quickJoinChannelSection:
		return "Channel"
	case quickJoinAccountSection:
		return "Identity"
	default:
		return "Unknown"
	}
}

func formatViewers(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// Messages for async data loading.
type setQuickJoinAccountsMessage struct {
	accounts []save.Account
}

type setQuickJoinChannelsMessage struct {
	recent   []save.ChannelHistoryEntry
	followed []string
}

// quickJoinChannelItem is the source data for a channel (before filtering).
type quickJoinChannelItem struct {
	login   string
	isLive  bool
	viewers int
	game    string
}

type quickJoin struct {
	focused bool
	width   int

	// Channel picker: two togglable lists.
	activeTab channelListTab

	// Recent list state.
	allRecent    []quickJoinChannelItem
	recentCursor int
	recentScroll int

	// Followed list state.
	allFollowed    []quickJoinChannelItem
	followedCursor int
	followedScroll int

	maxVisibleRows int

	// Account picker (manual rendering for consistent alignment).
	accountCursor int

	activeSection    quickJoinSection
	accounts         []save.Account
	deps             *DependencyContainer
	followedFetchers map[string]followedFetcher
	hasLoaded        bool
}

func newQuickJoin(parentWidth int, deps *DependencyContainer) *quickJoin {
	modalWidth := calcModalWidth(parentWidth)

	followedFetchers := map[string]followedFetcher{}
	for id, client := range deps.APIUserClients {
		if c, ok := client.(followedFetcher); ok {
			followedFetchers[id] = c
		}
	}

	return &quickJoin{
		width:            modalWidth,
		maxVisibleRows:   maxVisibleChannels,
		activeTab:        recentTab,
		deps:             deps,
		followedFetchers: followedFetchers,
	}
}

func calcModalWidth(parentWidth int) int {
	w := int(float64(parentWidth) * 0.6)
	if w < 40 {
		w = 40
	}
	if w > 80 {
		w = 80
	}
	return w
}

func (q *quickJoin) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			accounts, err := q.deps.AccountProvider.GetAllAccounts()
			if err != nil {
				return nil
			}
			for i, a := range accounts {
				if a.IsAnonymous {
					accounts[i].DisplayName = "Anonymous"
				}
			}
			return setQuickJoinAccountsMessage{accounts: accounts}
		},
		func() tea.Msg {
			var recent []save.ChannelHistoryEntry
			if q.deps.ChannelHistory != nil {
				var err error
				recent, err = q.deps.ChannelHistory.LoadHistory()
				if err != nil {
					log.Logger.Err(err).Msg("could not load channel history")
				}
			}

			uniqueChannels := map[string]struct{}{}
			for id, fetcher := range q.followedFetchers {
				func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()

					followed, err := fetcher.FetchUserFollowedChannels(ctx, id, "")
					if err != nil {
						log.Logger.Err(err).Str("account-id", id).Msg("could not fetch followed channels")
						return
					}

					for _, f := range followed {
						uniqueChannels[f.BroadcasterLogin] = struct{}{}
					}
				}()
			}

			return setQuickJoinChannelsMessage{
				recent:   recent,
				followed: slices.Sorted(maps.Keys(uniqueChannels)),
			}
		},
	)
}

func (q *quickJoin) Update(msg tea.Msg) (*quickJoin, tea.Cmd) {
	switch msg := msg.(type) {
	case setQuickJoinAccountsMessage:
		q.accounts = msg.accounts
		q.accountCursor = 0
		for i, a := range q.accounts {
			if a.IsMain {
				q.accountCursor = i
				break
			}
		}
		q.hasLoaded = true
		return q, nil

	case setQuickJoinChannelsMessage:
		q.allRecent = make([]quickJoinChannelItem, 0, len(msg.recent))
		for _, r := range msg.recent {
			q.allRecent = append(q.allRecent, quickJoinChannelItem{login: r.ChannelLogin})
		}

		q.allFollowed = make([]quickJoinChannelItem, 0, len(msg.followed))
		for _, f := range msg.followed {
			q.allFollowed = append(q.allFollowed, quickJoinChannelItem{login: f})
		}

		q.recentCursor = 0
		q.recentScroll = 0
		q.followedCursor = 0
		q.followedScroll = 0
		return q, nil

	case polledStreamInfoMessage:
		q.updateStreamInfo(msg)
		return q, nil
	}

	if !q.focused || !q.hasLoaded {
		return q, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Tab/Shift+Tab: switch between channel picker and account list.
		if key.Matches(msg, q.deps.Keymap.Next) || key.Matches(msg, q.deps.Keymap.Previous) {
			q.toggleSection()
			return q, nil
		}

		if q.activeSection == quickJoinChannelSection {
			return q, q.handleChannelKey(msg)
		}

		// Account section.
		if key.Matches(msg, q.deps.Keymap.Confirm) {
			return q, q.confirmSelection()
		}

		if key.Matches(msg, q.deps.Keymap.Up) {
			if q.accountCursor > 0 {
				q.accountCursor--
			}
			return q, nil
		}

		if key.Matches(msg, q.deps.Keymap.Down) {
			if q.accountCursor < len(q.accounts)-1 {
				q.accountCursor++
			}
			return q, nil
		}
	}

	return q, nil
}

func (q *quickJoin) handleChannelKey(msg tea.KeyMsg) tea.Cmd {
	// Left/Right toggle between Recent and Followed tabs.
	switch msg.String() {
	case "left", "right":
		q.switchTab()
		return nil
	}

	if key.Matches(msg, q.deps.Keymap.Up) {
		q.moveCursorActive(-1)
		return nil
	}

	if key.Matches(msg, q.deps.Keymap.Down) {
		q.moveCursorActive(1)
		return nil
	}

	if key.Matches(msg, q.deps.Keymap.Confirm) {
		return q.confirmSelection()
	}

	return nil
}

func (q *quickJoin) toggleSection() {
	if q.activeSection == quickJoinChannelSection {
		q.activeSection = quickJoinAccountSection
	} else {
		q.activeSection = quickJoinChannelSection
	}
}

func (q *quickJoin) switchTab() {
	if q.activeTab == recentTab {
		q.activeTab = followedTab
	} else {
		q.activeTab = recentTab
	}
}

// Active list helpers — delegate to the correct cursor/scroll/items based on activeTab.

func (q *quickJoin) activeItems() []quickJoinChannelItem {
	if q.activeTab == recentTab {
		return q.allRecent
	}
	return q.allFollowed
}

func (q *quickJoin) activeCursor() int {
	if q.activeTab == recentTab {
		return q.recentCursor
	}
	return q.followedCursor
}

func (q *quickJoin) setActiveCursor(v int) {
	if q.activeTab == recentTab {
		q.recentCursor = v
	} else {
		q.followedCursor = v
	}
}

func (q *quickJoin) activeScroll() int {
	if q.activeTab == recentTab {
		return q.recentScroll
	}
	return q.followedScroll
}

func (q *quickJoin) setActiveScroll(v int) {
	if q.activeTab == recentTab {
		q.recentScroll = v
	} else {
		q.followedScroll = v
	}
}

func (q *quickJoin) moveCursorActive(dir int) {
	items := q.activeItems()
	if len(items) == 0 {
		return
	}

	next := q.activeCursor() + dir
	if next < 0 || next >= len(items) {
		return
	}

	q.setActiveCursor(next)
	q.ensureActiveCursorVisible()
}

func (q *quickJoin) ensureActiveCursorVisible() {
	cursor := q.activeCursor()
	scroll := q.activeScroll()

	if cursor < scroll {
		scroll = cursor
	}
	if cursor >= scroll+q.maxVisibleRows {
		scroll = cursor - q.maxVisibleRows + 1
	}
	if scroll < 0 {
		scroll = 0
	}

	q.setActiveScroll(scroll)
}

func (q *quickJoin) confirmSelection() tea.Cmd {
	if len(q.accounts) == 0 {
		return nil
	}

	items := q.activeItems()
	cursor := q.activeCursor()
	if cursor < 0 || cursor >= len(items) {
		return nil
	}

	channel := items[cursor].login
	if channel == "" {
		return nil
	}

	account := q.accounts[q.accountCursor]

	return func() tea.Msg {
		for accountID, client := range q.deps.APIUserClients {
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

		return joinChannelMessage{
			tabKind: broadcastTabKind,
			channel: channel,
			account: account,
		}
	}
}

func (q *quickJoin) updateStreamInfo(msg polledStreamInfoMessage) {
	for _, info := range msg.streamInfos {
		updateItemStreamInfo(q.allRecent, info)
		updateItemStreamInfo(q.allFollowed, info)
	}
}

func updateItemStreamInfo(items []quickJoinChannelItem, info setStreamInfoMessage) {
	for i := range items {
		if !strings.EqualFold(items[i].login, info.username) {
			continue
		}
		items[i].isLive = info.isLive
		items[i].viewers = info.viewer
		items[i].game = info.game
	}
}

// View renders the quick-join modal.
func (q *quickJoin) View() string {
	style := lipgloss.NewStyle().
		Width(q.width).
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(q.deps.UserConfig.Theme.ListLabelColor))

	center := func(s string) string {
		return lipgloss.PlaceHorizontal(q.width, lipgloss.Center, s)
	}

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.deps.UserConfig.Theme.ListLabelColor))
	selectedLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.deps.UserConfig.Theme.ActiveLabelColor)).Bold(true)

	sectionLabel := func(name string, section quickJoinSection) string {
		if q.activeSection == section {
			return selectedLabelStyle.Render(name)
		}
		return labelStyle.Render(name)
	}

	b := strings.Builder{}

	// Headline
	headlineStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.deps.UserConfig.Theme.ActiveLabelColor))
	b.WriteString(center(headlineStyle.Render("Quick Join Channel")) + "\n\n")

	// Channel section
	b.WriteString(center(sectionLabel("Channel", quickJoinChannelSection)) + "\n")
	b.WriteString(center(q.renderTabBar()) + "\n\n")
	for _, line := range q.renderChannelLines() {
		b.WriteString(center(line) + "\n")
	}

	// Account section
	b.WriteString("\n" + center(sectionLabel("Identity", quickJoinAccountSection)) + "\n")
	for _, line := range q.renderAccountLines() {
		b.WriteString(center(line) + "\n")
	}

	// Hints
	b.WriteString("\n")
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.deps.UserConfig.Theme.StatusColor)).Faint(true)
	var hints string
	switch q.activeSection {
	case quickJoinChannelSection:
		hints = hintStyle.Render("\u2190/\u2192: switch list | \u2191/\u2193: select | Tab: identity | Enter: join")
	case quickJoinAccountSection:
		hints = hintStyle.Render("\u2191/\u2193: select | Tab: channel | Enter: join")
	}
	b.WriteString(center(hints) + "\n")

	// Status
	stateStr := fmt.Sprintf(" -- %s --", lipgloss.NewStyle().Foreground(lipgloss.Color(q.deps.UserConfig.Theme.StatusColor)).Render(q.activeSection.String()))
	b.WriteString(stateStr)

	return style.Padding(0).Render(b.String())
}

// renderTabBar renders the [Recent | Followed] toggle.
func (q *quickJoin) renderTabBar() string {
	activeColor := lipgloss.Color(q.deps.UserConfig.Theme.ActiveLabelColor)
	inactiveColor := lipgloss.Color(q.deps.UserConfig.Theme.ListLabelColor)

	active := lipgloss.NewStyle().Bold(true).Foreground(activeColor).Underline(true)
	inactive := lipgloss.NewStyle().Foreground(inactiveColor).Faint(true)

	recentLabel := fmt.Sprintf("Recent (%d)", len(q.allRecent))
	followedLabel := fmt.Sprintf("Followed (%d)", len(q.allFollowed))

	var left, right string
	if q.activeTab == recentTab {
		left = active.Render(recentLabel)
		right = inactive.Render(followedLabel)
	} else {
		left = inactive.Render(recentLabel)
		right = active.Render(followedLabel)
	}

	return left + "  " + right
}

// renderChannelLines returns individual styled lines for the visible channel items.
// Lines have NO width/alignment — callers center them via styleCenter.
func (q *quickJoin) renderChannelLines() []string {
	items := q.activeItems()
	if len(items) == 0 {
		return []string{lipgloss.NewStyle().Faint(true).Render("no channels")}
	}

	cursor := q.activeCursor()
	scroll := q.activeScroll()

	selectedColor := lipgloss.Color(q.deps.UserConfig.Theme.ListSelectedColor)
	liveColor := lipgloss.Color(q.deps.UserConfig.Theme.ActiveLabelColor)
	faintStyle := lipgloss.NewStyle().Faint(true)

	end := scroll + q.maxVisibleRows
	if end > len(items) {
		end = len(items)
	}

	var lines []string

	if scroll > 0 {
		lines = append(lines, faintStyle.Render("\u25b2"))
	}

	for i := scroll; i < end; i++ {
		item := items[i]
		isCurrent := i == cursor && q.activeSection == quickJoinChannelSection

		name := item.login
		var suffix string
		if item.isLive {
			dot := lipgloss.NewStyle().Foreground(liveColor).Render("\u25cf")
			suffix = fmt.Sprintf("  %s %s", dot, faintStyle.Render(formatViewers(item.viewers)))
			if item.game != "" {
				suffix += faintStyle.Render(" \u00b7 " + item.game)
			}
		}

		lineContent := name + suffix
		if isCurrent {
			lineContent = lipgloss.NewStyle().Foreground(selectedColor).Render(lineContent)
		}

		lines = append(lines, lineContent)
	}

	if end < len(items) {
		lines = append(lines, faintStyle.Render("\u25bc"))
	}

	return lines
}

// renderAccountLines returns individual styled lines for the account picker.
// Lines have NO width/alignment — callers center them via styleCenter.
func (q *quickJoin) renderAccountLines() []string {
	if len(q.accounts) == 0 {
		return []string{lipgloss.NewStyle().Faint(true).Render("no accounts")}
	}

	selectedColor := lipgloss.Color(q.deps.UserConfig.Theme.ListSelectedColor)
	var lines []string

	for i, acc := range q.accounts {
		name := acc.DisplayName
		if acc.IsAnonymous {
			name = "Anonymous"
		}

		if i == q.accountCursor && q.activeSection == quickJoinAccountSection {
			name = lipgloss.NewStyle().Foreground(selectedColor).Render(name)
		}

		lines = append(lines, name)
	}

	return lines
}

func (q *quickJoin) focus() {
	q.focused = true
}

func (q *quickJoin) blur() {
	q.focused = false
}

func (q *quickJoin) handleResize(parentWidth, parentHeight int) {
	q.width = calcModalWidth(parentWidth)
}
