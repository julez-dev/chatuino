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
	"github.com/julez-dev/chatuino/chatstate"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/ui/workspace"
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
// at tabs themselves. Pane membership is projected here because it changes
// independently of registry membership.
func (r *Root) tabRow(t tab.Tab) nav.Row {
	row := nav.Row{Title: t.Title(), Identity: t.Identity()}
	if owner := r.workspaceForTab(t.ID()); owner != nil {
		layout := owner.layout
		if owner.id == r.activeWorkspaceID {
			layout = r.workspace
		}
		row.Workspace = r.workspaceLabelFor(owner)
		row.WorkspacePane = layout.PaneNumber(t.ID())
	}
	return row
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

	hoverTabID string // tab the pointer is currently over, for the emote tooltip

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

	// chatSettings is owned by the shell: the startup loader fills it before the
	// tabs restore, and every tab of a channel renders from it.
	chatSettings *chatstate.Cache

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

	workspace         workspace.Layout // live layout of activeWorkspaceID
	workspaces        []*namedWorkspace
	activeWorkspaceID string
	workspaceSeq      int
	paneMode          bool
	paneZoom          bool
	paneHint          string
	pendingSplit      *paneSplitRequest
	splitSequence     uint64

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
		isDetached:          initialState != nil,
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
		chatSettings:        chatstate.NewCache(),
		imageDisplayManager: imageDisplayManager,
		recentMessages:      recentMessages,
		messageLogger:       messageLogger,
		pool:                pool,
		appStateManager:     appStateManager,
		channelHistory:      channelHistory,
		width:               10,
		height:              10,
		userIDDisplayName:   &sync.Map{},

		// components
		splash: splash{
			theme:   theme,
			keymap:  keymap,
			spinner: spinner.New(spinner.WithSpinner(tab.LoadingSpinner)),
		},
		header: header,

		messageLoggerChan: messageLoggerChan,
		events:            newEventQueue(eventQueueCapacity),
	}

	r.tabs = nav.NewRegistry(r.tabRow)
	r.workspaces = []*namedWorkspace{{id: "workspace-1", name: "Workspace 1"}}
	r.activeWorkspaceID, r.workspaceSeq = "workspace-1", 1

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
		return r, r.finishJoin(msg, 0)
	case paneChoiceMessage:
		return r, r.choosePane(msg)
	case paneJoinMessage:
		return r, r.finishJoin(msg.join, msg.ticket)
	case CloseTabMessage:
		return r, r.closeTab(msg.TabID)
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
		r.width = max(0, msg.Width)
		r.height = max(0, msg.Height)
		r.handleResize()
		return r, nil
	case tea.MouseClickMsg:
		return r, r.handleMouseClick(msg)
	case tea.MouseMotionMsg:
		return r, r.handleMouseMotion(msg)
	case tea.MouseWheelMsg:
		return r, r.handleMouseWheel(msg)
	case tea.KeyPressMsg:
		// q-to-quit must not steal printable keys while an input is focused.
		// Modifier-driven quit keys (ctrl+c) still work everywhere.
		if key.Matches(msg, r.keymap.Quit) && !((r.isTyping() || r.paneMode) && msg.Text != "") {
			r.Close()
			return r, tea.Quit
		}

		if !r.hasLoadedSession {
			return r, tea.Batch(cmds...)
		}

		// Help must not steal '?' while any text input (including chat search)
		// is capturing printable characters.
		if key.Matches(msg, r.keymap.Help) && (!r.isTyping() || r.paneMode) && r.stack.Len() == 0 {
			r.tabs.BlurActive()
			return r, r.stack.Push(newHelp(r.height, r.width, r.keymap))
		}

		if key.Matches(msg, r.keymap.Escape) {
			if r.stack.Len() > 0 {
				return r, r.dismissModal()
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

		if r.stack.Len() == 0 {
			if r.paneMode {
				return r, r.handlePaneKey(msg)
			}
			if key.Matches(msg, r.keymap.PaneMode) && r.activeID() != "" && r.enterPaneMode() {
				return r, nil
			}
			if r.paneHint != "" {
				r.paneHint = ""
				r.handleResize()
			}
		}

		if key.Matches(msg, r.keymap.Create) {
			// The key toggles the join modal.
			if r.stack.Route() == joinRoute {
				return r, r.dismissModal()
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
				return r, r.dismissModal()
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
				if active, ok := r.tabs.Active(); ok && (active.IsComposing() || active.IsSearching()) {
					cmd, _ := r.tabs.UpdateActive(msg, updateTab)
					return r, cmd
				}

				r.nextTab()
				return r, nil
			}

			if key.Matches(msg, r.keymap.Previous) {
				if active, ok := r.tabs.Active(); ok && (active.IsComposing() || active.IsSearching()) {
					cmd, _ := r.tabs.UpdateActive(msg, updateTab)
					return r, cmd
				}

				r.prevTab()
				return r, nil
			}

			if key.Matches(msg, r.keymap.CloseTab) {
				if active, ok := r.tabs.Active(); ok && !(active.IsComposing() || active.IsSearching()) {
					return r, r.closeTab(active.ID())
				}
			}
		}
	}

	// Input belongs to one tab or the top dialog, never the event fan-out.
	switch msg.(type) {
	case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.PasteMsg:
		if r.stack.Len() > 0 {
			return r, r.stack.Update(msg)
		}
		if r.paneMode {
			return r, nil
		}
		if r.hasLoadedSession {
			cmd, _ := r.tabs.UpdateActive(msg, updateTab)
			return r, cmd
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
		if r.emoteTooltipEnabled() {
			// hovering needs motion events without a pressed button
			v.MouseMode = tea.MouseModeAllMotion
		}
	}

	return v
}

func (r *Root) renderView() string {
	if !r.hasLoadedSession {
		return r.clampScreen(r.splash.ViewLoading())
	}

	return r.clampScreen(r.stack.View())
}

// screenContent renders the current screen behind any modal: the header plus the
// active tab, or the splash when no tab is open.
func (r *Root) screenContent() string {
	tab, ok := r.tabs.Active()
	if !ok {
		return r.clampScreen(r.splash.View())
	}

	if r.usesWorkspace() {
		return r.renderWorkspace()
	}

	if r.settings.VerticalTabList {
		// In vertical mode, render status bar separately at full width
		mainContent := lipgloss.JoinHorizontal(lipgloss.Left, r.header.View(r.visibleRows()), tab.ViewWithoutStatusBar())
		if statusBar := tab.StatusBarView(); statusBar != "" {
			return r.clampScreen(mainContent + "\n" + statusBar)
		}
		return r.clampScreen(mainContent)
	}

	return r.clampScreen(r.header.View(r.visibleRows()) + "\n" + tab.View())
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
	r.syncWorkspaceToActive()
	appState := save.AppState{}
	refs := make(map[string]string, r.tabs.Len())

	for _, t := range r.tabs.All() {
		refs[t.ID()] = fmt.Sprint(len(appState.Tabs))
		tabState := save.TabState{
			IsFocused:  t.ID() == r.activeID(),
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
	appState.Layout = r.snapshotWorkspace(refs)
	appState.Workspaces = r.snapshotWorkspaces(refs)

	return appState
}

func (r *Root) tickSaveAppState() tea.Cmd {
	if r.isDetached || r.appStateManager == nil {
		return nil
	}
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
			r.emoteCache, r.emoteReplacer, r.badgeCache, r.badgeReplacer,
			r.chatSettings)
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
	view := r.header.View(r.visibleRows())
	if view == "" {
		return 0
	}
	return lipgloss.Height(view)
}

func (r *Root) handleResize() {
	r.syncWorkspaceToActive()
	// splash screen
	r.splash.width = r.width
	r.splash.height = r.height

	// open modals follow the terminal size
	r.stack.Resize(r.width, r.height)

	if r.usesWorkspace() {
		r.resizeWorkspace()
		return
	}

	if r.settings.VerticalTabList {
		rows := r.visibleRows()
		minWidth := r.header.MinWidth(rows)
		r.header.Resize(min(minWidth, max(0, r.width)), max(0, r.height))

		headerView := r.header.View(rows)
		headerWidth := lipgloss.Width(headerView)
		headerHeight := lipgloss.Height(headerView)

		for _, t := range r.tabs.All() {
			// Tab height matches header height (status bar is rendered separately below both)
			t.SetSize(max(0, r.width-headerWidth), max(0, headerHeight))
			t.SetFullWidth(r.width) // for status bar to span full width
			t.HandleResize()
		}

		return
	}

	r.header.Resize(max(0, r.width-3), 0) // one placeholder space foreach side

	// tab
	headerHeight := r.getHeaderHeight()

	for _, t := range r.tabs.All() {
		t.SetSize(max(0, r.width), max(0, r.height-headerHeight))
		t.SetFullWidth(0)
		t.HandleResize()
	}
}

func (r *Root) nextTab() { r.stepTab(1) }

func (r *Root) prevTab() { r.stepTab(-1) }

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

	// restore tabs; layout references use original record positions, even when
	// unavailable accounts/tabs cause some records to be skipped.
	var hasActiveTab bool
	refs := make(map[string]string, len(msg.state.Tabs))
	for index, t := range msg.state.Tabs {
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
		refs[fmt.Sprint(index)] = newTab.ID()

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

	r.restoreWorkspaces(msg.state.Layout, msg.state.Workspaces, refs)
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
			Bool("history", isFake).
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
