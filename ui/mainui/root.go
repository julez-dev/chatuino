package mainui

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/badge"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/rs/zerolog/log"
)

// updateNotifier is implemented by tabs that advertise a newer version.
type updateNotifier interface {
	SetUpdateAvailable(latest string)
}

// localFilters is implemented by tabs that keep per-tab display filters, which
// the shell persists and restores with the rest of the app state.
type localFilters interface {
	LocalUniqueOnly() bool
	SetLocalUniqueOnly(bool)
	LocalSubOnly() bool
	SetLocalSubOnly(bool)
}

// latestVersion is the version to advertise, empty when there is no newer one.
func (r *Root) latestVersion() string {
	if r.updateInfo != nil && r.updateInfo.HasUpdate {
		return r.updateInfo.LatestVersion
	}

	return ""
}

// tabRow projects a tab into the registry's rendering row, so headers never look
// at tabs themselves.
func tabRow(t tab.Tab) nav.Row {
	return nav.Row{
		Title:    t.Title(),
		Identity: t.Identity(),
	}
}

// updateTab adapts the self-typed tab.Update to the registry's updater shape.
func updateTab(t tab.Tab, msg tea.Msg) (tab.Tab, tea.Cmd) {
	return t.Update(msg)
}

// Modal routes. Root compares against these to toggle a modal instead of
// type-asserting on concrete screen types.
const (
	joinRoute      = "join"
	quickJoinRoute = "quick-join"
	helpRoute      = "help"
)

type ircConnectionError struct {
	err error
}

func (e ircConnectionError) Error() string {
	return fmt.Sprintf("Disconnected from chat server. (%s)", e.err.Error())
}

func (e ircConnectionError) Unwrap() error {
	return e.err
}

func (e ircConnectionError) IRC() string {
	return e.Error()
}

type Root struct {
	width, height int

	hasLoadedSession bool
	isDetached       bool           // when true, state is not loaded from or saved to disk
	initialState     *save.AppState // when non-nil, overrides persisted state

	userIDDisplayName *sync.Map

	// collaborators, each passed in by main.go. There is no dependency struct:
	// the signature of NewUI is the complete list of what the shell needs.
	settings save.Settings
	theme    save.Theme
	keymap   save.KeyMap
	accounts []save.Account
	version  string

	serverAPI ChatuinoServer
	clients   map[string]tab.ClientCapabilities

	accountProvider     tab.AccountProvider
	emoteCache          tab.EmoteCache
	badgeCache          *badge.Cache
	emoteReplacer       tab.EmoteReplacer
	badgeReplacer       tab.BadgeReplacer
	imageDisplayManager *kittyimg.DisplayManager
	recentMessages      tab.RecentMessageService
	messageLogger       tab.MessageLogger
	pool                tab.ConnectionPool
	appStateManager     AppStateManager
	channelHistory      ChannelHistory

	// message logger
	messageLoggerChan chan<- *twitchirc.PrivateMessage

	// components
	splash splash
	header header
	// stack owns the modal screens layered over the tab view; the tab view
	// itself is the base and is rendered by screenContent.
	stack *nav.Stack

	tabs               *nav.Registry[tab.Tab]
	channelSuggestions []string // cached for broadcast to new tabs
	updateInfo         *UpdateInfo

	// Chat events are enriched off the update loop, one at a time, in arrival
	// order. The preparer owns the enrichment rules; the queue owns ordering.
	preparer *chatEventPreparer
	events   eventQueue
}

// NewUI creates the root Bubble Tea model. When initialState is non-nil the
// session runs in detached mode: the provided state is used instead of loading
// from disk and no state is persisted, allowing multiple independent instances.
func NewUI(
	ctx context.Context,
	messageLoggerChan chan<- *twitchirc.PrivateMessage,
	settings save.Settings,
	theme save.Theme,
	keymap save.KeyMap,
	accounts []save.Account,
	version string,
	serverAPI ChatuinoServer,
	clients map[string]tab.ClientCapabilities,
	accountProvider tab.AccountProvider,
	emoteCache tab.EmoteCache,
	badgeCache *badge.Cache,
	emoteReplacer tab.EmoteReplacer,
	badgeReplacer tab.BadgeReplacer,
	imageDisplayManager *kittyimg.DisplayManager,
	recentMessages tab.RecentMessageService,
	messageLogger tab.MessageLogger,
	pool tab.ConnectionPool,
	appStateManager AppStateManager,
	channelHistory ChannelHistory,
	initialState *save.AppState,
) *Root {
	var header header
	if settings.VerticalTabList {
		header = newVerticalTabHeader(10, 10, theme)
	} else {
		header = newHorizontalTabHeader(10, theme)
	}

	r := &Root{
		initialState:        initialState,
		settings:            settings,
		theme:               theme,
		keymap:              keymap,
		accounts:            accounts,
		version:             version,
		serverAPI:           serverAPI,
		clients:             clients,
		accountProvider:     accountProvider,
		emoteCache:          emoteCache,
		badgeCache:          badgeCache,
		emoteReplacer:       emoteReplacer,
		badgeReplacer:       badgeReplacer,
		imageDisplayManager: imageDisplayManager,
		recentMessages:      recentMessages,
		messageLogger:       messageLogger,
		pool:                pool,
		appStateManager:     appStateManager,
		channelHistory:      channelHistory,
		width:               10,
		height:              10,
		userIDDisplayName:   &sync.Map{},
		tabs:                nav.NewRegistry(tabRow),

		// components
		splash: splash{
			theme:   theme,
			keymap:  keymap,
			spinner: spinner.New(spinner.WithSpinner(tab.LoadingSpinner)),
		},
		header: header,

		messageLoggerChan: messageLoggerChan,
		events:            newEventQueue(512),
	}

	r.preparer = newChatEventPreparer(ctx, r)

	// Modals composite over the tab view, which stays visible but dimmed behind
	// them. The base view is a function so it always renders current state.
	r.stack = nav.NewStack(r.screenContent, func(fg, bg string) string {
		return centerOverlay(fg, lipgloss.NewStyle().Faint(true).Render(bg))
	})

	return r
}

// Close cancels and drains chat-event preparation. Call it after Program.Run,
// including error/cancellation exits. Quit keys also close preparation promptly.
func (r *Root) Close() {
	r.preparer.close()
}

func (r *Root) Init() tea.Cmd {
	// Detached sessions use the state passed on the command line instead of
	// loading it from disk and never persist it back.
	r.isDetached = r.initialState != nil

	return tea.Batch(
		r.splash.Init(),
		newStartupLoader(r).loadCmd(),
		r.tickPollStreamInfos(),
		r.imageCleanUpCommand(),
		r.checkVersionCommand(),
	)
}

func (r *Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	// Addressed messages belong to exactly one tab; the registry decides which,
	// so no child has to re-check a target id.
	if tabCmd, ok := r.tabs.Dispatch(msg, updateTab); ok {
		return r, tabCmd
	}

	switch msg := msg.(type) {
	case persistedDataLoadedMessage:
		return r, r.handlePersistedDataLoaded(msg)
	case imageCleanupTickMessage:
		var cmds []tea.Cmd
		if msg.deletionCommand != "" {
			cmds = append(cmds, tea.Raw(msg.deletionCommand))
		}
		cmds = append(cmds, r.imageCleanUpCommand())
		return r, tea.Batch(cmds...)
	case versionCheckMessage:
		return r, r.handleVersionCheck(msg)
	case tab.JoinChannelMessage:
		// Close whichever modal asked for the join. Pop() blurs it through its
		// Disappeared hook and drops the screen, so its state is collected.
		r.stack.Pop()

		nTab, cmd := r.createTab(msg.Account, msg.Channel, msg.TabKind)

		r.tabs.Add(nTab)
		r.tabs.SetActive(nTab.ID())

		r.handleResize()

		initCmds := []tea.Cmd{nTab.Init(), cmd}

		// Record channel in history for broadcast tabs
		if msg.TabKind == tab.BroadcastKind && msg.Channel != "" {
			channel := msg.Channel

			// update cached suggestions with new channel
			if !slices.Contains(r.channelSuggestions, channel) {
				r.channelSuggestions = append(r.channelSuggestions, channel)
			}

			if r.channelHistory != nil {
				initCmds = append(initCmds, func() tea.Msg {
					if err := r.channelHistory.RecordChannel(channel); err != nil {
						log.Logger.Err(err).Str("channel", channel).Msg("failed to record channel history")
					}
					return nil
				})
			}
		}

		// send cached suggestions to new tab
		if len(r.channelSuggestions) > 0 {
			initCmds = append(initCmds, nav.To(nTab.ID(), tab.ChannelSuggestionsLoadedMessage{
				Channels: r.channelSuggestions,
			}))
		}

		return r, tea.Batch(initCmds...)
	case wspool.IRCEvent:
		// Handle IRC events from the connection pool. Enrichment (guest-channel
		// lookup, emote/badge replacement, link checks) happens off the update
		// loop; the event is routed once its turn in the queue comes up.
		if msg.Error != nil {
			// Connection error - display as notice in all tabs for this account
			return r, r.enqueueChatEvent(msg.AccountID, "", ircConnectionError{err: msg.Error}, false)
		}

		// Log private messages before enrichment so logging never depends on it.
		if privateMsg, ok := msg.Message.(*twitchirc.PrivateMessage); ok {
			r.messageLoggerChan <- privateMsg.Clone()
		}

		return r, r.enqueueChatEvent(msg.AccountID, "", msg.Message, false)
	case tab.ChatEventMessage:
		// Already prepared (for example by a tab emitting a local notice).
		return r, r.dispatchPreparedEvent(msg)
	case tab.RequestLocalMessageHandleMessage:
		return r, r.enqueueChatEvent(msg.AccountID, msg.TabID, msg.Message, true)
	case tab.RequestLocalMessageHandleBatchMessage:
		for ircer := range slices.Values(msg.Messages) {
			r.addChatEvent(msg.AccountID, msg.TabID, ircer, true)
		}

		return r, r.pumpChatEventPreparation()
	case preparedChatEvent:
		return r, r.releasePreparedEvent(msg)
	case routedPreparedEvent:
		return r, r.finishAndRoute(msg.seq, msg.event)
	case polledStreamInfoMessage:
		// The quick join modal shows live status for its channels. Screens that do
		// not care ignore the message.
		cmds = append(cmds, r.stack.Update(msg))
		cmds = append(cmds, r.handlePolledStreamInfo(msg))
		return r, tea.Batch(cmds...)
	case appStateSaveMessage:
		return r, r.tickSaveAppState()
	case tea.WindowSizeMsg:
		r.width = msg.Width
		r.height = msg.Height
		r.handleResize()
		return r, nil
	case tea.MouseClickMsg:
		return r, r.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return r, r.handleMouseWheel(msg)
	case tea.KeyPressMsg:
		// q-to-quit must not steal printable keys while an input is focused.
		// Modifier-driven quit keys (ctrl+c) still work everywhere.
		if key.Matches(msg, r.keymap.Quit) && !(r.isTyping() && msg.Text != "") {
			r.Close()
			return r, tea.Quit
		}

		if !r.hasLoadedSession {
			return r, tea.Batch(cmds...)
		}

		// Help must not steal '?' while any text input (including chat search)
		// is capturing printable characters.
		if key.Matches(msg, r.keymap.Help) && !r.isTyping() && r.stack.Len() == 0 {
			r.tabs.BlurActive()
			return r, r.stack.Push(newHelp(r.height, r.width, r.keymap))
		}

		if key.Matches(msg, r.keymap.Escape) {
			if r.stack.Len() > 0 {
				r.tabs.FocusActive()

				return r, r.stack.Pop()
			}
		}

		if key.Matches(msg, r.keymap.DumpScreen) {
			f, err := os.Create(fmt.Sprintf("%s_dump.txt", time.Now().Format("2006-01-02_15_04_05")))
			if err != nil {
				return r, nil
			}

			defer func() {
				_ = f.Close()
			}()

			_, _ = io.Copy(f, strings.NewReader(tab.StripANSI(r.renderView())))

			return r, nil
		}

		if key.Matches(msg, r.keymap.Create) {
			// The key toggles the join modal.
			if r.stack.Route() == joinRoute {
				r.tabs.FocusActive()
				return r, r.stack.Pop()
			}

			if r.stack.Len() > 0 {
				return r, nil
			}

			r.tabs.BlurActive()

			joinInput := newJoin(r.width,
				r.settings, r.theme, r.keymap,
				r.clients, r.accountProvider)
			joinInput.Resize(r.width, r.height)
			joinInput.setTabOptions(r.availableTabKinds()...)

			return r, r.stack.Push(joinInput)
		}

		if key.Matches(msg, r.keymap.QuickJoin) {
			// The key toggles the quick join modal.
			if r.stack.Route() == quickJoinRoute {
				r.tabs.FocusActive()
				return r, r.stack.Pop()
			}

			if r.stack.Len() > 0 {
				return r, nil
			}

			r.tabs.BlurActive()

			quickJoinInput := newQuickJoin(r.width,
				r.settings, r.theme, r.keymap,
				r.clients, r.accountProvider, r.channelHistory)
			quickJoinInput.Resize(r.width, r.height)

			return r, r.stack.Push(quickJoinInput)
		}

		if r.stack.Len() == 0 {

			if key.Matches(msg, r.keymap.Next) {
				if active, ok := r.tabs.Active(); ok && active.IsComposing() {
					cmd, _ := r.tabs.UpdateActive(msg, updateTab)
					return r, cmd
				}

				r.nextTab()
			}

			if key.Matches(msg, r.keymap.Previous) {
				if active, ok := r.tabs.Active(); ok && active.IsComposing() {
					cmd, _ := r.tabs.UpdateActive(msg, updateTab)
					return r, cmd
				}

				r.prevTab()
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				if active, ok := r.tabs.Active(); ok && !(active.IsComposing() || active.IsSearching()) {
					currentTab := active
					r.closeTab()

					// if tab was connected to IRC, disconnect it
					if currentTab.IsDataLoaded() && currentTab.Kind() == tab.BroadcastKind {
						cmds := make([]tea.Cmd, 0, 2)

						// if there is another tab for the same channel and the same account
						hasTabsSameAccountAndChannel := r.tabs.Any(func(t tab.Tab) bool {
							return t.ID() != currentTab.ID() &&
								t.AccountID() == currentTab.AccountID() &&
								t.ChannelID() == currentTab.ChannelID()
						})

						hasTabsSameChannel := r.tabs.Any(func(t tab.Tab) bool {
							return t.ID() != currentTab.ID() &&
								t.ChannelID() == currentTab.ChannelID()
						})

						if !hasTabsSameAccountAndChannel {
							// send part message
							log.Logger.Info().Str("channel", currentTab.Channel()).Str("id", currentTab.AccountID()).Msg("sending part message")
							accountID := currentTab.AccountID()
							channel := currentTab.Channel()
							cmds = append(cmds, func() tea.Msg {
								r.pool.SendIRC(accountID, twitchirc.PartMessage{Channel: channel})
								return nil
							})
						}

						if !hasTabsSameChannel {
							log.Logger.Info().Str("channel", currentTab.Channel()).Str("channel-id", currentTab.ChannelID()).Msg("removing emote cache entry for channel")
							r.emoteCache.RemoveEmoteSetForChannel(currentTab.ChannelID())
						}

						// Disconnect IRC for this account
						accountID := currentTab.AccountID()
						cmds = append(cmds, func() tea.Msg {
							r.pool.DisconnectIRC(accountID)
							return nil
						})

						return r, tea.Sequence(cmds...)
					}

					return r, nil
				}
			}
		}
	}

	if !r.hasLoadedSession {
		r.splash, cmd = r.splash.Update(msg)
		cmds = append(cmds, cmd)
	}

	if req, ok := msg.(tab.RequestNotificationIconMessage); ok {
		r.tabs.Notify(req.TabID)
	}

	cmds = append(cmds, r.tabs.UpdateAll(msg, updateTab))

	// Only the topmost modal sees input; the stack guarantees that.
	cmds = append(cmds, r.stack.Update(msg))

	return r, tea.Batch(cmds...)
}

func (r *Root) View() tea.View {
	v := tea.NewView(r.renderView())
	v.AltScreen = true
	v.WindowTitle = "Chatuino"

	if r.settings.Mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}

	return v
}

func (r *Root) renderView() string {
	if !r.hasLoadedSession {
		return r.splash.ViewLoading()
	}

	return r.stack.View()
}

// screenContent renders the current screen behind any modal: the header plus the
// active tab, or the splash when no tab is open.
func (r *Root) screenContent() string {
	tab, ok := r.tabs.Active()
	if !ok {
		return r.splash.View()
	}

	if r.settings.VerticalTabList {
		// In vertical mode, render status bar separately at full width
		mainContent := lipgloss.JoinHorizontal(lipgloss.Left, r.header.View(r.tabs.Rows()), tab.ViewWithoutStatusBar())
		if statusBar := tab.StatusBarView(); statusBar != "" {
			return mainContent + "\n" + statusBar
		}
		return mainContent
	}

	return r.header.View(r.tabs.Rows()) + "\n" + tab.View()
}

// dimmedScreenBackground renders screenContent faint, for use behind a modal.
func (r *Root) dimmedScreenBackground() string {
	return lipgloss.NewStyle().Faint(true).Render(r.screenContent())
}

func (r *Root) HasSessionLoaded() bool {
	return r.hasLoadedSession
}

// IsDetached reports whether the session was started with explicit --tab flags
// and is therefore not loading or persisting state to disk.
func (r *Root) IsDetached() bool {
	return r.isDetached
}

func (r *Root) TakeStateSnapshot() save.AppState {
	appState := save.AppState{}

	for _, t := range r.tabs.All() {
		tabState := save.TabState{
			IsFocused:  t.Focused(),
			Channel:    t.Channel(),
			IdentityID: t.AccountID(),
			Kind:       int(t.Kind()),
		}

		if lf, ok := t.(localFilters); ok {
			tabState.IsLocalUnique = lf.LocalUniqueOnly()
			tabState.IsLocalSub = lf.LocalSubOnly()
		}

		appState.Tabs = append(appState.Tabs, tabState)
	}

	return appState
}

func (r *Root) tickSaveAppState() tea.Cmd {
	state := r.TakeStateSnapshot()

	return tea.Tick(time.Second*15, func(_ time.Time) tea.Msg {
		log.Logger.Info().Msg("saving app state inside ticker")

		if err := r.appStateManager.SaveAppState(state); err != nil {
			log.Logger.Err(err).Msg("failed to save app state")
		}

		return appStateSaveMessage{}
	})
}

func (r *Root) tickPollStreamInfos() tea.Cmd {
	clients := maps.Clone(r.clients)

	// collect all open broadcasters
	openBroadcasts := map[string]struct{}{}
	channelIDNames := map[string]string{}

	for _, t := range r.tabs.All() {
		if t.Kind() != tab.BroadcastKind {
			continue
		}
		openBroadcasts[t.ChannelID()] = struct{}{}
		channelIDNames[t.ChannelID()] = t.Channel()
	}

	if len(openBroadcasts) == 0 {
		return tea.Tick(time.Second*90, func(_ time.Time) tea.Msg {
			return polledStreamInfoMessage{}
		})
	}

	broadcastIDs := []string{}
	for broadcast := range openBroadcasts {
		broadcastIDs = append(broadcastIDs, broadcast)
	}

	return tea.Tick(time.Second*90, func(_ time.Time) tea.Msg {
		accounts, err := r.accountProvider.GetAllAccounts()
		if err != nil {
			return polledStreamInfoMessage{}
		}

		var mainAccountID string

		for _, account := range accounts {
			if account.IsMain {
				mainAccountID = account.ID
			}
		}

		var fetcher tab.APIClient
		if client, has := clients[mainAccountID]; has && client.API != nil {
			fetcher = client.API
		} else {
			fetcher = r.serverAPI
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		resp, err := fetcher.GetStreamInfo(ctx, broadcastIDs)
		if err != nil {
			log.Logger.Err(err).Msg("failed polling streamer info")
			return polledStreamInfoMessage{}
		}

		polled := polledStreamInfoMessage{
			streamInfos: make([]tab.SetStreamInfoMessage, 0, len(broadcastIDs)),
		}

		// Update status for all streams
		// If a stream is offline the twitch API does not return any item for this broadcaster_id
		// To still update the title etc. to empty value, send an empty info to component.
		for ib := range broadcastIDs {
			info := tab.SetStreamInfoMessage{
				Target:   broadcastIDs[ib],
				Username: channelIDNames[broadcastIDs[ib]], // fall back channel name, so it can still be displayed when offline
			}

			for id := range resp.Data {
				// data's user does not match broadcaster
				if resp.Data[id].UserID != broadcastIDs[ib] {
					continue
				}

				info.Viewer = resp.Data[id].ViewerCount
				info.Username = resp.Data[id].UserName
				info.Title = resp.Data[id].Title
				info.Game = resp.Data[id].GameName
				info.IsLive = !resp.Data[id].StartedAt.IsZero()
			}

			polled.streamInfos = append(polled.streamInfos, info)
		}

		return polled
	})
}

func (r *Root) handlePolledStreamInfo(polled polledStreamInfoMessage) tea.Cmd {
	var cmds []tea.Cmd

	for _, info := range polled.streamInfos {
		cmds = append(cmds, r.tabs.UpdateAll(info, func(t tab.Tab, msg tea.Msg) (tab.Tab, tea.Cmd) {
			if !t.IsDataLoaded() {
				return t, nil
			}

			return t.Update(msg)
		}))
	}

	cmds = append(cmds, r.tickPollStreamInfos())
	return tea.Batch(cmds...)
}

// createTab builds a tab for the given kind. It does not add the tab to the
// registry: the caller owns that, so tab creation and tab activation stay
// separate decisions.
func (r *Root) createTab(account save.Account, channel string, kind tab.TabKind) (tab.Tab, tea.Cmd) {
	headerHeight := r.getHeaderHeight()

	switch kind {
	case tab.BroadcastKind:
		nTab := tab.NewBroadcast(uuid.NewString(), r.width, r.height-headerHeight, account, channel,
			r.settings, r.theme, r.keymap,
			r.clients[account.ID], r.pool,
			r.recentMessages, r.messageLogger,
			r.emoteCache, r.emoteReplacer, r.badgeCache, r.badgeReplacer)
		nTab.SetUpdateAvailable(r.latestVersion())
		return nTab, nil
	case tab.MentionKind:
		nTab := tab.NewMention(uuid.NewString(), r.width, r.height-headerHeight,
			r.settings, r.theme, r.keymap, r.accountProvider)
		return nTab, nil
	case tab.LiveNotificationKind:
		nTab := tab.NewLiveNotification(uuid.NewString(), r.width, r.height-headerHeight,
			r.settings, r.theme, r.keymap)
		return nTab, nil
	}

	return nil, nil
}

// availableTabKinds returns the tab kinds that are not open yet, in menu order.
func (r *Root) availableTabKinds() []tab.TabKind {
	hasMention := r.tabs.Any(func(t tab.Tab) bool { return t.Kind() == tab.MentionKind })
	hasNotification := r.tabs.Any(func(t tab.Tab) bool { return t.Kind() == tab.LiveNotificationKind })

	kinds := []tab.TabKind{tab.BroadcastKind}

	if !hasMention {
		kinds = append(kinds, tab.MentionKind)
	}

	if !hasNotification {
		kinds = append(kinds, tab.LiveNotificationKind)
	}

	return kinds
}

func (r *Root) getHeaderHeight() int {
	return lipgloss.Height(r.header.View(r.tabs.Rows()))
}

func (r *Root) handleResize() {
	// splash screen
	r.splash.width = r.width
	r.splash.height = r.height

	// open modals follow the terminal size
	r.stack.Resize(r.width, r.height)

	if r.settings.VerticalTabList {
		rows := r.tabs.Rows()
		minWidth := r.header.MinWidth(rows)
		r.header.Resize(minWidth, r.height)

		headerView := r.header.View(rows)
		headerWidth := lipgloss.Width(headerView)
		headerHeight := lipgloss.Height(headerView)

		for _, t := range r.tabs.All() {
			// Tab height matches header height (status bar is rendered separately below both)
			t.SetSize(r.width-headerWidth, headerHeight)
			t.SetFullWidth(r.width) // for status bar to span full width
			t.HandleResize()
		}

		return
	}

	r.header.Resize(r.width-3, 0) // one placeholder space foreach side

	// tab
	headerHeight := r.getHeaderHeight()

	for _, t := range r.tabs.All() {
		t.SetSize(r.width, r.height-headerHeight)
		t.HandleResize()
	}
}

func (r *Root) nextTab() {
	r.tabs.Next()
}

func (r *Root) prevTab() {
	r.tabs.Prev()
}

func (r *Root) checkVersionCommand() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		info, err := checkForUpdate(ctx, r.serverAPI, r.version)
		return versionCheckMessage{info: info, err: err}
	}
}

func (r *Root) handleVersionCheck(msg versionCheckMessage) tea.Cmd {
	if msg.err != nil {
		log.Logger.Err(msg.err).Msg("version check failed")
		return nil
	}

	if !msg.info.HasUpdate {
		return nil
	}

	r.updateInfo = &msg.info
	r.splash.updateInfo = &msg.info

	for _, t := range r.tabs.All() {
		if n, ok := t.(updateNotifier); ok {
			n.SetUpdateAvailable(r.latestVersion())
		}
	}

	return nil
}

func (r *Root) handlePersistedDataLoaded(msg persistedDataLoadedMessage) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(msg.state.Tabs))
	r.hasLoadedSession = true

	if msg.err != nil {
		log.Logger.Err(msg.err).Msg("failed to load persisted data")
		return nil
	}

	// restore tabs
	var hasActiveTab bool
	for _, t := range msg.state.Tabs {
		var (
			newTab tab.Tab
			cmd    tea.Cmd
		)
		switch tab.TabKind(t.Kind) {
		case tab.BroadcastKind:
			var account save.Account

			for _, a := range r.accounts {
				if a.ID == t.IdentityID {
					account = a
				}
			}

			if account.ID == "" || t.Channel == "" {
				continue
			}

			newTab, cmd = r.createTab(account, t.Channel, tab.BroadcastKind)
			if lf, ok := newTab.(localFilters); ok {
				lf.SetLocalUniqueOnly(t.IsLocalUnique)
				lf.SetLocalSubOnly(t.IsLocalSub)
			}
		case tab.MentionKind:
			// don't load mention tab, when there are no longer any non-anonymous accounts
			hasNormalAccount := slices.ContainsFunc(r.accounts, func(e save.Account) bool {
				return !e.IsAnonymous
			})

			if !hasNormalAccount {
				continue
			}

			newTab, cmd = r.createTab(save.Account{}, "", tab.MentionKind)
		case tab.LiveNotificationKind:
			newTab, cmd = r.createTab(save.Account{}, "", tab.LiveNotificationKind)
		}

		if newTab == nil {
			continue
		}

		cmds = append(cmds, cmd)
		r.tabs.Add(newTab)

		if t.IsFocused {
			hasActiveTab = true
			r.tabs.SetActive(newTab.ID())
		}
		log.Logger.Info().Any("data", msg.ttvUsers).Msg("Initialized tab with user data")
		cmds = append(cmds, newTab.InitWithUserData(msg.ttvUsers[t.Channel]))
	}

	// if for some reason there were tabs persisted but none has the focus flag set
	// focus the first tab
	if tabs := r.tabs.All(); len(tabs) > 0 && !hasActiveTab {
		r.tabs.SetActive(tabs[0].ID())
	}

	r.handleResize()

	// cache and broadcast channel suggestions to all tabs
	r.channelSuggestions = msg.channelSuggestions
	if len(r.channelSuggestions) > 0 {
		for _, t := range r.tabs.All() {
			cmds = append(cmds, nav.To(t.ID(), tab.ChannelSuggestionsLoadedMessage{
				Channels: r.channelSuggestions,
			}))
		}
	}

	// initial app state tick (skip in detached mode)
	if !r.isDetached {
		cmds = append(cmds, r.tickSaveAppState())
	}

	return tea.Batch(cmds...)
}

// addChatEvent appends an event to the ordered preparation queue. It reports the
// request so callers can test the ordering rules.
func (r *Root) addChatEvent(accountID, tabID string, ircer twitchirc.IRCer, isFake bool) (prepareRequest, bool) {
	if r.preparer.ctx.Err() != nil {
		return prepareRequest{}, false
	}
	req, ok := r.events.add(accountID, tabID, ircer, isFake)
	if !ok {
		log.Logger.Warn().
			Int("pending", r.events.len()).
			Str("account_id", accountID).
			Msg("chat event dropped: preparation queue is full")

		return prepareRequest{}, false
	}

	return req, true
}

// enqueueChatEvent appends an event and starts the next preparation when the
// queue is idle.
func (r *Root) enqueueChatEvent(accountID, tabID string, ircer twitchirc.IRCer, isFake bool) tea.Cmd {
	if _, ok := r.addChatEvent(accountID, tabID, ircer, isFake); !ok {
		return nil
	}

	return r.pumpChatEventPreparation()
}

// pumpChatEventPreparation starts exactly one preparation command, capturing the
// preparer's collaborators instead of the Root model. Results re-enter the update
// loop and pump the next event, so enrichment runs one event at a time in arrival
// order without one goroutine per incoming message.
func (r *Root) pumpChatEventPreparation() tea.Cmd {
	if r.preparer.ctx.Err() != nil {
		return nil
	}
	req, ok := r.events.start()
	if !ok {
		return nil
	}

	preparer := r.preparer

	return func() tea.Msg {
		return preparedChatEvent{seq: req.seq, event: preparer.prepare(req)}
	}
}

// releasePreparedEvent hands a finished preparation to the terminal or directly
// to the tabs.
//
// When the event carries a kitty graphics command, the raw write is run first and
// routing happens only after its message has been handled, so the image exists
// before the row that references it is rendered. The queue stays in flight across
// that hand-off, which keeps routing in arrival order.
func (r *Root) releasePreparedEvent(msg preparedChatEvent) tea.Cmd {
	if r.preparer.ctx.Err() != nil || !r.events.release(msg.seq) {
		return nil
	}
	if msg.event.PrepareCommand == "" {
		return r.finishAndRoute(msg.seq, msg.event)
	}

	prepareCommand := msg.event.PrepareCommand
	msg.event.PrepareCommand = ""

	return tea.Sequence(
		tea.Raw(prepareCommand),
		func() tea.Msg {
			return routedPreparedEvent(msg)
		},
	)
}

// finishAndRoute releases the queue slot, routes the event and starts the next
// preparation. A superseded result is dropped.
func (r *Root) finishAndRoute(seq uint64, event tab.ChatEventMessage) tea.Cmd {
	if r.preparer.ctx.Err() != nil {
		return nil
	}
	var cmds []tea.Cmd

	if r.events.finish(seq) {
		cmds = append(cmds, r.dispatchPreparedEvent(event))
	} else {
		log.Logger.Warn().Uint64("seq", seq).Msg("dropped a superseded chat event result")
	}

	cmds = append(cmds, r.pumpChatEventPreparation())

	return tea.Batch(cmds...)
}

// dispatchPreparedEvent routes an event to its tab, or to every tab when it is
// not tab-addressed. A tab-addressed event whose tab has closed in the meantime is
// dropped by the registry instead of reaching a different tab.
func (r *Root) dispatchPreparedEvent(event tab.ChatEventMessage) tea.Cmd {
	var cmds []tea.Cmd

	if event.PrepareCommand != "" {
		cmds = append(cmds, tea.Raw(event.PrepareCommand))
		event.PrepareCommand = ""
	}

	if event.TabID != "" {
		if cmd, ok := r.tabs.UpdateOne(event.TabID, event, updateTab); ok {
			cmds = append(cmds, cmd)
		}

		return tea.Batch(cmds...)
	}

	cmds = append(cmds, r.tabs.UpdateAll(event, updateTab))

	return tea.Batch(cmds...)
}

// imageCleanUpCommand returns a command that ticks after 1 minute and
// clean all images that were not used in the last 10 minutes
func (r *Root) imageCleanUpCommand() tea.Cmd {
	if !r.settings.Chat.GraphicEmotes && !r.settings.Chat.GraphicBadges {
		return nil
	}

	return tea.Tick(time.Minute*1, func(_ time.Time) tea.Msg {
		log.Logger.Info().Msg("image clean up tick start")
		defer log.Logger.Info().Msg("image clean up tick end")

		data := r.imageDisplayManager.CleanupOldImagesCommand(time.Minute * 10)

		return imageCleanupTickMessage{
			deletionCommand: data,
		}
	})
}

func (r *Root) closeTab() {
	active, ok := r.tabs.Active()
	if !ok {
		return
	}

	active.Close()

	// Remove activates the neighbour, so no cursor bookkeeping is needed here.
	r.tabs.Remove(active.ID())
	r.handleResize()
}
