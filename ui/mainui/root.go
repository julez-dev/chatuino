package mainui

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
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
	"github.com/julez-dev/chatuino/emote"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type TabKind int

const (
	BroadcastTabKind TabKind = iota
	MentionTabKind
	LiveNotificationTabKind
)

func (t TabKind) String() string {
	switch t {
	case BroadcastTabKind:
		return "Channel (Default)"
	case MentionTabKind:
		return "Mention"
	case LiveNotificationTabKind:
		return "Live Notifications"
	}

	return "<not implemented>"
}

type tab interface {
	Init() tea.Cmd
	InitWithUserData(twitchapi.UserData) tea.Cmd
	Update(tea.Msg) (tab, tea.Cmd)
	View() string
	ViewWithoutStatusBar() string // for vertical tab mode
	StatusBarView() string        // for vertical tab mode
	Focus()
	Blur()
	AccountID() string
	Channel() string
	State() broadcastTabState
	IsSearching() bool
	IsDataLoaded() bool
	ID() string
	Focused() bool
	ChannelID() string
	HandleResize()
	SetSize(width, height int)
	SetFullWidth(width int) // for status bar in vertical tab mode
	Kind() TabKind
	Title() string    // header label, e.g. the channel name
	Identity() string // account label shown next to the title
}

// tabRow projects a tab into the registry's rendering row, so headers never look
// at tabs themselves.
func tabRow(t tab) nav.Row {
	return nav.Row{
		Title:    t.Title(),
		Identity: t.Identity(),
	}
}

// updateTab adapts the self-typed tab.Update to the registry's updater shape.
func updateTab(t tab, msg tea.Msg) (tab, tea.Cmd) {
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

	dependencies *DependencyContainer

	// message logger
	messageLoggerChan chan<- *twitchirc.PrivateMessage

	// components
	splash splash
	header header
	// stack owns the modal screens layered over the tab view; the tab view
	// itself is the base and is rendered by screenContent.
	stack *nav.Stack

	tabs               *nav.Registry[tab]
	channelSuggestions []string // cached for broadcast to new tabs
	updateInfo         *UpdateInfo
}

// NewUI creates the root Bubble Tea model. When initialState is non-nil the
// session runs in detached mode: the provided state is used instead of loading
// from disk and no state is persisted, allowing multiple independent instances.
func NewUI(
	messageLoggerChan chan<- *twitchirc.PrivateMessage,
	dependencies *DependencyContainer,
	initialState *save.AppState,
) *Root {
	var header header
	if dependencies.UserConfig.Settings.VerticalTabList {
		header = newVerticalTabHeader(10, 10, dependencies)
	} else {
		header = newHorizontalTabHeader(10, dependencies)
	}

	r := &Root{
		initialState:      initialState,
		dependencies:      dependencies,
		width:             10,
		height:            10,
		userIDDisplayName: &sync.Map{},
		tabs:              nav.NewRegistry(tabRow),

		// components
		splash: splash{
			keymap:            dependencies.Keymap,
			userConfiguration: dependencies.UserConfig,
			spinner:           spinner.New(spinner.WithSpinner(loadingSpinner)),
		},
		header: header,

		messageLoggerChan: messageLoggerChan,
	}

	// Modals composite over the tab view, which stays visible but dimmed behind
	// them. The base view is a function so it always renders current state.
	r.stack = nav.NewStack(r.screenContent, func(fg, bg string) string {
		return centerOverlay(fg, lipgloss.NewStyle().Faint(true).Render(bg))
	})

	return r
}

func (r *Root) Init() tea.Cmd {
	r.isDetached = r.initialState != nil

	return tea.Batch(
		r.splash.Init(),
		func() tea.Msg {
			var (
				state save.AppState
				err   error
			)

			if r.initialState != nil {
				state = *r.initialState
			} else {
				state, err = r.dependencies.AppStateManager.LoadAppState()
				if err != nil {
					return persistedDataLoadedMessage{
						err: fmt.Errorf("failed to load save state: %w", err),
					}
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
			defer cancel()
			wg, ctx := errgroup.WithContext(ctx)

			wg.Go(func() error {
				ctx, cancel := context.WithTimeout(ctx, time.Second*5)
				defer cancel()

				if err := r.dependencies.BadgeCache.RefreshGlobal(ctx); err != nil {
					log.Logger.Error().Err(err).Msg("could not fetch global badges")
				}

				return nil
			})

			wg.Go(func() error {
				ctx, cancel := context.WithTimeout(ctx, time.Second*5)
				defer cancel()

				if err := r.dependencies.EmoteCache.RefreshGlobal(ctx); err != nil {
					log.Logger.Error().Err(err).Msg("could not fetch global emotes")
				}

				return nil
			})

			// fetch usable emotes for all users
			for _, acc := range r.dependencies.Accounts {
				if acc.IsAnonymous {
					continue
				}

				client, has := r.dependencies.Clients[acc.ID]
				if !has {
					continue
				}

				fetcher := client.Emotes
				if fetcher == nil {
					log.Logger.Error().Msg("failed to parse user emote client")
					continue
				}

				wg.Go(func() error {
					set, template, err := fetcher.FetchAllUserEmotes(ctx, acc.ID, "")
					if err != nil {
						log.Logger.Error().Str("user_id", acc.ID).Err(err).Msg("could not fetch user emotes")
						return nil
					}

					emotes := make(emote.EmoteSet, 0, len(set))
					for _, e := range set {
						animated := slices.Contains(e.Format, "animated")

						format := "static"
						if animated {
							format = "animated"
						}

						url := strings.ReplaceAll(template, "{{id}}", e.ID)
						url = strings.ReplaceAll(url, "{{format}}", format)
						url = strings.ReplaceAll(url, "{{theme_mode}}", "light")
						url = strings.ReplaceAll(url, "{{scale}}", "1.0")

						emotes = append(emotes, emote.Emote{
							ID:         e.ID,
							Text:       e.Name,
							Platform:   emote.Twitch,
							IsAnimated: animated,
							URL:        url,
						})
					}

					r.dependencies.EmoteCache.AddUserEmotes(acc.ID, emotes)
					return nil
				})
			}

			// pre fetch all of tabs twitch users in one single call, this saves a lot of calls if the app was previously closed with a lot of tabs
			ttvUsers := make(map[string]twitchapi.UserData, len(state.Tabs))
			usersLock := &sync.Mutex{}

			loginsUnique := make(map[string]struct{}, len(state.Tabs))
			logins := make([]string, 0, len(state.Tabs))

			for _, tab := range state.Tabs {
				if tab.Kind != int(BroadcastTabKind) {
					continue
				}
				loginsUnique[tab.Channel] = struct{}{}
			}

			logins = slices.AppendSeq(logins, maps.Keys(loginsUnique))

			if len(logins) > 0 {
				wg.Go(func() error {
					resp, err := r.dependencies.ServerAPI.GetUsers(ctx, logins, nil)
					if err != nil {
						log.Logger.Error().Err(err).Msg("could not fetch users for tabs")
						return fmt.Errorf("could not fetch users for tabs: %w", err)
					}

					for _, data := range resp.Data {
						usersLock.Lock()
						ttvUsers[data.Login] = data
						usersLock.Unlock()
					}

					return nil
				})
			}

			// pre-fetch channel suggestions (history + followed) once for all tabs
			var channelSuggestions []string
			wg.Go(func() error {
				seen := make(map[string]struct{})

				if r.dependencies.ChannelHistory != nil {
					entries, err := r.dependencies.ChannelHistory.LoadHistory()
					if err != nil {
						log.Logger.Err(err).Msg("could not load channel history for suggestions")
					} else {
						for _, e := range entries {
							if _, ok := seen[e.ChannelLogin]; !ok {
								seen[e.ChannelLogin] = struct{}{}
								channelSuggestions = append(channelSuggestions, e.ChannelLogin)
							}
						}
					}
				}

				for accountID, client := range r.dependencies.Clients {
					fc := client.Follows
					if fc == nil {
						continue
					}

					followed, err := fc.FetchUserFollowedChannels(ctx, accountID, "")
					if err != nil {
						log.Logger.Err(err).Str("account-id", accountID).Msg("could not fetch followed channels for suggestions")
						continue
					}

					for _, f := range followed {
						login := strings.ToLower(f.BroadcasterLogin)
						if _, ok := seen[login]; !ok {
							seen[login] = struct{}{}
							channelSuggestions = append(channelSuggestions, login)
						}
					}
				}

				return nil
			})

			err = wg.Wait()

			return persistedDataLoadedMessage{
				state:              state,
				ttvUsers:           ttvUsers,
				channelSuggestions: channelSuggestions,
				err:                err,
			}
		},
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
	case joinChannelMessage:
		// Close whichever modal asked for the join. Pop() blurs it through its
		// Disappeared hook and drops the screen, so its state is collected.
		r.stack.Pop()

		nTab, cmd := r.createTab(msg.account, msg.channel, msg.tabKind)

		r.tabs.Add(nTab)
		r.tabs.SetActive(nTab.ID())

		r.handleResize()

		initCmds := []tea.Cmd{nTab.Init(), cmd}

		// Record channel in history for broadcast tabs
		if msg.tabKind == BroadcastTabKind && msg.channel != "" {
			channel := msg.channel

			// update cached suggestions with new channel
			if !slices.Contains(r.channelSuggestions, channel) {
				r.channelSuggestions = append(r.channelSuggestions, channel)
			}

			if r.dependencies.ChannelHistory != nil {
				initCmds = append(initCmds, func() tea.Msg {
					if err := r.dependencies.ChannelHistory.RecordChannel(channel); err != nil {
						log.Logger.Err(err).Str("channel", channel).Msg("failed to record channel history")
					}
					return nil
				})
			}
		}

		// send cached suggestions to new tab
		if len(r.channelSuggestions) > 0 {
			initCmds = append(initCmds, nav.To(nTab.ID(), channelSuggestionsLoadedMessage{
				channels: r.channelSuggestions,
			}))
		}

		return r, tea.Batch(initCmds...)
	case wspool.IRCEvent:
		// Handle IRC events from the connection pool
		if msg.Error != nil {
			// Connection error - display as notice in all tabs for this account
			errEvt := r.buildChatEventMessage(msg.AccountID, "", ircConnectionError{err: msg.Error}, false)
			if errEvt.prepareCommand != "" {
				cmds = append(cmds, tea.Raw(errEvt.prepareCommand))
				errEvt.prepareCommand = ""
			}
			cmds = append(cmds, r.tabs.UpdateAll(errEvt, updateTab))
			return r, tea.Batch(cmds...)
		}

		// Log private messages
		if privateMsg, ok := msg.Message.(*twitchirc.PrivateMessage); ok {
			r.messageLoggerChan <- privateMsg.Clone()
		}

		// Build and forward event to tabs
		evt := r.buildChatEventMessage(msg.AccountID, "", msg.Message, false)
		if evt.prepareCommand != "" {
			cmds = append(cmds, tea.Raw(evt.prepareCommand))
			evt.prepareCommand = ""
		}
		cmds = append(cmds, r.tabs.UpdateAll(evt, updateTab))
		return r, tea.Batch(cmds...)
	case chatEventMessage:
		// Handle locally-generated chat events (e.g., from recent messages)
		if msg.prepareCommand != "" {
			cmds = append(cmds, tea.Raw(msg.prepareCommand))
			msg.prepareCommand = ""
		}
		if msg.tabID != "" {
			if cmd, ok := r.tabs.UpdateOne(msg.tabID, msg, updateTab); ok {
				cmds = append(cmds, cmd)
			}

			return r, tea.Batch(cmds...)
		}

		cmds = append(cmds, r.tabs.UpdateAll(msg, updateTab))
		return r, tea.Batch(cmds...)
	case requestLocalMessageHandleMessage:
		return r, func() tea.Msg {
			return r.buildChatEventMessage(msg.accountID, msg.tabID, msg.message, true)
		}
	case requestLocalMessageHandleBatchMessage:
		batched := make([]tea.Cmd, 0, len(msg.messages))

		for ircer := range slices.Values(msg.messages) {
			batched = append(batched, func() tea.Msg {
				return r.buildChatEventMessage(msg.accountID, msg.tabID, ircer, true)
			})
		}

		cmds = append(cmds, tea.Sequence(batched...))
		return r, tea.Batch(cmds...)
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
		if key.Matches(msg, r.dependencies.Keymap.Quit) && !(r.isTyping() && msg.Text != "") {
			return r, tea.Quit
		}

		if !r.hasLoadedSession {
			return r, tea.Batch(cmds...)
		}

		// Help must not steal '?' while any text input (including chat search)
		// is capturing printable characters.
		if key.Matches(msg, r.dependencies.Keymap.Help) && !r.isTyping() && r.stack.Len() == 0 {
			r.tabs.BlurActive()
			return r, r.stack.Push(newHelp(r.height, r.width, r.dependencies))
		}

		if key.Matches(msg, r.dependencies.Keymap.Escape) {
			if r.stack.Len() > 0 {
				r.tabs.FocusActive()

				return r, r.stack.Pop()
			}
		}

		if key.Matches(msg, r.dependencies.Keymap.DumpScreen) {
			f, err := os.Create(fmt.Sprintf("%s_dump.txt", time.Now().Format("2006-01-02_15_04_05")))
			if err != nil {
				return r, nil
			}

			defer func() {
				_ = f.Close()
			}()

			_, _ = io.Copy(f, strings.NewReader(stripAnsi(r.renderView())))

			return r, nil
		}

		if key.Matches(msg, r.dependencies.Keymap.Create) {
			// The key toggles the join modal.
			if r.stack.Route() == joinRoute {
				r.tabs.FocusActive()
				return r, r.stack.Pop()
			}

			if r.stack.Len() > 0 {
				return r, nil
			}

			r.tabs.BlurActive()

			joinInput := newJoin(r.width, r.dependencies)
			joinInput.Resize(r.width, r.height)
			joinInput.setTabOptions(r.availableTabKinds()...)

			return r, r.stack.Push(joinInput)
		}

		if key.Matches(msg, r.dependencies.Keymap.QuickJoin) {
			// The key toggles the quick join modal.
			if r.stack.Route() == quickJoinRoute {
				r.tabs.FocusActive()
				return r, r.stack.Pop()
			}

			if r.stack.Len() > 0 {
				return r, nil
			}

			r.tabs.BlurActive()

			quickJoinInput := newQuickJoin(r.width, r.dependencies)
			quickJoinInput.Resize(r.width, r.height)

			return r, r.stack.Push(quickJoinInput)
		}

		if r.stack.Len() == 0 {

			if key.Matches(msg, r.dependencies.Keymap.Next) {
				if active, ok := r.tabs.Active(); ok && active.State().isComposing() {
					cmd, _ := r.tabs.UpdateActive(msg, updateTab)
					return r, cmd
				}

				r.nextTab()
			}

			if key.Matches(msg, r.dependencies.Keymap.Previous) {
				if active, ok := r.tabs.Active(); ok && active.State().isComposing() {
					cmd, _ := r.tabs.UpdateActive(msg, updateTab)
					return r, cmd
				}

				r.prevTab()
			}

			if key.Matches(msg, r.dependencies.Keymap.CloseTab) {
				if active, ok := r.tabs.Active(); ok && !(active.State().isComposing() || active.IsSearching()) {
					currentTab := active
					r.closeTab()

					// if tab was connected to IRC, disconnect it
					if currentTab.IsDataLoaded() && currentTab.Kind() == BroadcastTabKind {
						cmds := make([]tea.Cmd, 0, 2)

						// if there is another tab for the same channel and the same account
						hasTabsSameAccountAndChannel := r.tabs.Any(func(t tab) bool {
							return t.ID() != currentTab.ID() &&
								t.AccountID() == currentTab.AccountID() &&
								t.ChannelID() == currentTab.ChannelID()
						})

						hasTabsSameChannel := r.tabs.Any(func(t tab) bool {
							return t.ID() != currentTab.ID() &&
								t.ChannelID() == currentTab.ChannelID()
						})

						if !hasTabsSameAccountAndChannel {
							// send part message
							log.Logger.Info().Str("channel", currentTab.Channel()).Str("id", currentTab.AccountID()).Msg("sending part message")
							accountID := currentTab.AccountID()
							channel := currentTab.Channel()
							cmds = append(cmds, func() tea.Msg {
								r.dependencies.Pool.SendIRC(accountID, twitchirc.PartMessage{Channel: channel})
								return nil
							})
						}

						if !hasTabsSameChannel {
							log.Logger.Info().Str("channel", currentTab.Channel()).Str("channel-id", currentTab.ChannelID()).Msg("removing emote cache entry for channel")
							r.dependencies.EmoteCache.RemoveEmoteSetForChannel(currentTab.ChannelID())
						}

						// Disconnect IRC for this account
						accountID := currentTab.AccountID()
						cmds = append(cmds, func() tea.Msg {
							r.dependencies.Pool.DisconnectIRC(accountID)
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

	if req, ok := msg.(requestNotificationIconMessage); ok {
		r.tabs.Notify(req.tabID)
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

	if r.dependencies.UserConfig.Settings.Mouse {
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

	if r.dependencies.UserConfig.Settings.VerticalTabList {
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

		if t.Kind() == BroadcastTabKind {
			tabState.IsLocalUnique = t.(*broadcastTab).isUniqueOnlyChat
			tabState.IsLocalSub = t.(*broadcastTab).isLocalSub
		}

		appState.Tabs = append(appState.Tabs, tabState)
	}

	return appState
}

func (r *Root) tickSaveAppState() tea.Cmd {
	state := r.TakeStateSnapshot()

	return tea.Tick(time.Second*15, func(_ time.Time) tea.Msg {
		log.Logger.Info().Msg("saving app state inside ticker")

		if err := r.dependencies.AppStateManager.SaveAppState(state); err != nil {
			log.Logger.Err(err).Msg("failed to save app state")
		}

		return appStateSaveMessage{}
	})
}

func (r *Root) tickPollStreamInfos() tea.Cmd {
	clients := maps.Clone(r.dependencies.Clients)

	// collect all open broadcasters
	openBroadcasts := map[string]struct{}{}
	channelIDNames := map[string]string{}

	for _, tab := range r.tabs.All() {
		if tab.Kind() != BroadcastTabKind {
			continue
		}
		openBroadcasts[tab.ChannelID()] = struct{}{}
		channelIDNames[tab.ChannelID()] = tab.Channel()
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
		accounts, err := r.dependencies.AccountProvider.GetAllAccounts()
		if err != nil {
			return polledStreamInfoMessage{}
		}

		var mainAccountID string

		for _, account := range accounts {
			if account.IsMain {
				mainAccountID = account.ID
			}
		}

		var fetcher APIClient
		if client, has := clients[mainAccountID]; has && client.API != nil {
			fetcher = client.API
		} else {
			fetcher = r.dependencies.ServerAPI
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		resp, err := fetcher.GetStreamInfo(ctx, broadcastIDs)
		if err != nil {
			log.Logger.Err(err).Msg("failed polling streamer info")
			return polledStreamInfoMessage{}
		}

		polled := polledStreamInfoMessage{
			streamInfos: make([]setStreamInfoMessage, 0, len(broadcastIDs)),
		}

		// Update status for all streams
		// If a stream is offline the twitch API does not return any item for this broadcaster_id
		// To still update the title etc. to empty value, send an empty info to component.
		for ib := range broadcastIDs {
			info := setStreamInfoMessage{
				target:   broadcastIDs[ib],
				username: channelIDNames[broadcastIDs[ib]], // fall back channel name, so it can still be displayed when offline
			}

			for id := range resp.Data {
				// data's user does not match broadcaster
				if resp.Data[id].UserID != broadcastIDs[ib] {
					continue
				}

				info.viewer = resp.Data[id].ViewerCount
				info.username = resp.Data[id].UserName
				info.title = resp.Data[id].Title
				info.game = resp.Data[id].GameName
				info.isLive = !resp.Data[id].StartedAt.IsZero()
			}

			polled.streamInfos = append(polled.streamInfos, info)
		}

		return polled
	})
}

func (r *Root) handlePolledStreamInfo(polled polledStreamInfoMessage) tea.Cmd {
	var cmds []tea.Cmd

	for _, info := range polled.streamInfos {
		cmds = append(cmds, r.tabs.UpdateAll(info, func(t tab, msg tea.Msg) (tab, tea.Cmd) {
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
func (r *Root) createTab(account save.Account, channel string, kind TabKind) (tab, tea.Cmd) {
	headerHeight := r.getHeaderHeight()

	switch kind {
	case BroadcastTabKind:
		nTab := newBroadcastTab(uuid.NewString(), r.width, r.height-headerHeight, account, channel, r.dependencies)
		nTab.updateInfo = r.updateInfo
		return nTab, nil
	case MentionTabKind:
		nTab := newMentionTab(uuid.NewString(), r.width, r.height-headerHeight, r.dependencies)
		return nTab, nil
	case LiveNotificationTabKind:
		nTab := newLiveNotificationTab(uuid.NewString(), r.width, r.height-headerHeight, r.dependencies)
		return nTab, nil
	}

	return nil, nil
}

// availableTabKinds returns the tab kinds that are not open yet, in menu order.
func (r *Root) availableTabKinds() []TabKind {
	hasMention := r.tabs.Any(func(t tab) bool { return t.Kind() == MentionTabKind })
	hasNotification := r.tabs.Any(func(t tab) bool { return t.Kind() == LiveNotificationTabKind })

	kinds := []TabKind{BroadcastTabKind}

	if !hasMention {
		kinds = append(kinds, MentionTabKind)
	}

	if !hasNotification {
		kinds = append(kinds, LiveNotificationTabKind)
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

	if r.dependencies.UserConfig.Settings.VerticalTabList {
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

		info, err := checkForUpdate(ctx, r.dependencies.ServerAPI, r.dependencies.Version)
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
		if bt, ok := t.(*broadcastTab); ok {
			bt.updateInfo = r.updateInfo
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
			newTab tab
			cmd    tea.Cmd
		)
		switch TabKind(t.Kind) {
		case BroadcastTabKind:
			var account save.Account

			for _, a := range r.dependencies.Accounts {
				if a.ID == t.IdentityID {
					account = a
				}
			}

			if account.ID == "" || t.Channel == "" {
				continue
			}

			newTab, cmd = r.createTab(account, t.Channel, BroadcastTabKind)
			newTab.(*broadcastTab).isUniqueOnlyChat = t.IsLocalUnique
			newTab.(*broadcastTab).isLocalSub = t.IsLocalSub
		case MentionTabKind:
			// don't load mention tab, when there are no longer any non-anonymous accounts
			hasNormalAccount := slices.ContainsFunc(r.dependencies.Accounts, func(e save.Account) bool {
				return !e.IsAnonymous
			})

			if !hasNormalAccount {
				continue
			}

			newTab, cmd = r.createTab(save.Account{}, "", MentionTabKind)
		case LiveNotificationTabKind:
			newTab, cmd = r.createTab(save.Account{}, "", LiveNotificationTabKind)
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
			cmds = append(cmds, nav.To(t.ID(), channelSuggestionsLoadedMessage{
				channels: r.channelSuggestions,
			}))
		}
	}

	// initial app state tick (skip in detached mode)
	if !r.isDetached {
		cmds = append(cmds, r.tickSaveAppState())
	}

	return tea.Batch(cmds...)
}

func (r *Root) buildChatEventMessage(accountID string, tabID string, ircer twitchirc.IRCer, isFakeEvent bool) chatEventMessage {
	var (
		channel                 string
		message                 string
		badges                  []twitchirc.Badge
		emotes                  []twitchirc.Emote
		emoteSourceRoom         string
		channelID               string
		channelGuestID          string
		channelGuestDisplayName string
		loginName               string
	)

	// Check when currently in shared session.
	// If so then load emotes and badges for guest so the message content can be replaced
	if msg, ok := ircer.(*twitchirc.PrivateMessage); ok {
		channelID = msg.RoomID
		channelGuestID = msg.SourceRoomID

		// shared chat, get display name of guest stream chat, channelGuestID will be empty when not shared chat
		if channelID != "" && channelGuestID != "" {
			if v, ok := r.userIDDisplayName.Load(channelGuestID); ok {
				channelGuestDisplayName = v.(string)
			} else {
				// try refresh emotes for guest
				_ = r.dependencies.EmoteCache.RefreshLocal(context.Background(), channelGuestID)
				_ = r.dependencies.BadgeCache.RefreshChannel(context.Background(), channelGuestID)

				resp, err := r.dependencies.ServerAPI.GetUsers(context.Background(), nil, []string{channelGuestID})
				if err == nil && len(resp.Data) > 0 {
					channelGuestDisplayName = resp.Data[0].DisplayName
					r.userIDDisplayName.Store(channelGuestID, channelGuestDisplayName)
				}
			}
		}
	}

	switch ircMessage := ircer.(type) {
	case *twitchirc.PrivateMessage:
		channelID = ircMessage.RoomID
		channelGuestID = ircMessage.SourceRoomID
		channel = ircMessage.ChannelUserName
		message = ircMessage.Message
		loginName = ircMessage.LoginName

		// if is shared display emotes from guest channel, when message is from guest
		emoteSourceRoom = channelID
		badges = ircMessage.Badges
		emotes = ircMessage.Emotes

		if channelGuestID != "" && channelID != channelGuestID {
			emoteSourceRoom = channelGuestID
			badges = ircMessage.SourceBadges
		}

	case *twitchirc.RoomState:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
	case *twitchirc.UserNotice:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.UserState:
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.ClearChat:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
	case *twitchirc.ClearMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
	case *twitchirc.SubMessage:
		channelID = ircMessage.RoomID
		emoteSourceRoom = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		message = ircMessage.Message
		emotes = ircMessage.Emotes
		badges = ircMessage.Badges
	case *twitchirc.RaidMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.SubGiftMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.RitualMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		emoteSourceRoom = ircMessage.RoomID
		message = ircMessage.Message
		emotes = ircMessage.Emotes
		badges = ircMessage.Badges
	case *twitchirc.AnnouncementMessage:
		channelID = ircMessage.RoomID
		emoteSourceRoom = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		message = ircMessage.Message
		emotes = ircMessage.Emotes
		badges = ircMessage.Badges
	case *twitchirc.Notice:
		channel = ircMessage.ChannelUserName
	}

	event := chatEventMessage{
		isFakeEvent:             isFakeEvent,
		accountID:               accountID,
		channel:                 channel,
		channelID:               channelID,
		channelGuestID:          channelGuestID,
		channelGuestDisplayName: channelGuestDisplayName,
		tabID:                   tabID,

		message: ircer,
		displayModifier: messageContentModifier{
			wordReplacements: make(wordReplacement),
			badgeReplacement: make(wordReplacement),
		},
	}

	var replaceCommand string

	if len(message) > 0 {
		p, replacement, err := r.dependencies.EmoteReplacer.Replace(emoteSourceRoom, message, emotes)
		if err != nil {
			log.Logger.Info().Err(err).Str("message", message).Msg("failed to replace emotes")
		}

		for k, v := range replacement {
			event.displayModifier.wordReplacements[k] = v
		}

		replaceCommand += p
	}

	if len(badges) > 0 {
		p, replace, err := r.dependencies.BadgeReplacer.Replace(emoteSourceRoom, badges)
		if err != nil {
			log.Logger.Info().Err(err).Str("message", message).Msg("failed to replace badges")
		}

		event.displayModifier.badgeReplacement = replace
		replaceCommand += p
	}

	if loginName != "" {
		p, err := r.dependencies.BadgeReplacer.InjectContributorBadge(loginName, event.displayModifier.badgeReplacement)
		if err != nil {
			log.Logger.Info().Err(err).Str("login", loginName).Msg("failed to inject contributor badge")
		}
		replaceCommand += p
	}

	if r.dependencies.UserConfig.Settings.Security.CheckLinks && len(message) > 0 {
		// Key link annotations on the whole space-delimited token so they match
		// exactly in applyWordReplacements (same contract as emotes). The URL is
		// annotated in place, preserving any surrounding punctuation in the token.
		for _, token := range strings.Split(message, " ") {
			annotated := token
			for _, u := range extractValidURLs(token) {
				resp, err := r.dependencies.ServerAPI.CheckLink(context.Background(), u)
				if err != nil {
					log.Logger.Info().Err(err).Str("url", u).Msg("failed to check link")
					continue
				}

				parts := []string{http.StatusText(resp.RemoteStatusCode)}

				if resp.RemoteContentType != "" {
					before, _, _ := strings.Cut(resp.RemoteContentType, ";")
					parts = append(parts, before)
				}

				for _, visited := range resp.VisitedURLs {
					d, err := url.QueryUnescape(visited)
					if err != nil {
						log.Logger.Info().Err(err).Str("url", visited).Msg("failed to unescape url")
						continue
					}
					parts = append(parts, d)
				}

				annotated = strings.Replace(annotated, u, fmt.Sprintf("%s [%s]", u, strings.Join(parts, ", ")), 1)
			}

			if annotated != token {
				event.displayModifier.wordReplacements[token] = annotated
			}
		}
	}

	event.prepareCommand = replaceCommand
	return event
}

// imageCleanUpCommand returns a command that ticks after 1 minute and
// clean all images that were not used in the last 10 minutes
func (r *Root) imageCleanUpCommand() tea.Cmd {
	if !r.dependencies.UserConfig.Settings.Chat.GraphicEmotes && !r.dependencies.UserConfig.Settings.Chat.GraphicBadges {
		return nil
	}

	return tea.Tick(time.Minute*1, func(_ time.Time) tea.Msg {
		log.Logger.Info().Msg("image clean up tick start")
		defer log.Logger.Info().Msg("image clean up tick end")

		data := r.dependencies.ImageDisplayManager.CleanupOldImagesCommand(time.Minute * 10)

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

	if active.Kind() == BroadcastTabKind {
		active.(*broadcastTab).close()
	}

	// Remove activates the neighbour, so no cursor bookkeeping is needed here.
	r.tabs.Remove(active.ID())
	r.handleResize()
}
