package mainui

import (
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/wspool"
	"github.com/stretchr/testify/require"
)

func chatEvents(ft *fakeTab) []tab.ChatEventMessage {
	var events []tab.ChatEventMessage
	for _, msg := range ft.messages() {
		if event, ok := msg.(tab.ChatEventMessage); ok {
			events = append(events, event)
		}
	}
	return events
}

func eventContents(ft *fakeTab) []string {
	var contents []string
	for _, event := range chatEvents(ft) {
		contents = append(contents, event.Message.(*twitchirc.PrivateMessage).Message)
	}
	return contents
}

func blockFirstPreparation(t *testing.T, root *Root) (<-chan struct{}, chan<- struct{}) {
	t.Helper()
	started := make(chan struct{})
	release := make(chan struct{})
	root.preparer.emoteReplacer = emotePreparationFunc(func(ctx context.Context, _, content string, _ []twitchirc.Emote) (string, map[string]string, error) {
		if content == "first" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return "", nil, ctx.Err()
			}
		}
		return "", nil, nil
	})
	return started, release
}

func drainPreparation(t *testing.T, root *Root, msg tea.Msg) {
	t.Helper()
	_, cmd := root.Update(msg)
	for cmd != nil {
		messages := execCmd(t, cmd)
		require.Len(t, messages, 1)
		_, cmd = root.Update(messages[0])
	}
}

func TestRoot_DelayedLiveEventKeepsHistoryBatchContiguous(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	logger := make(chan *twitchirc.PrivateMessage, 2)
	root.messageLoggerChan = logger
	one, two := newFakeTab("tab-1"), newFakeTab("tab-2")
	root.tabs.Add(one)
	root.tabs.Add(two)
	started, release := blockFirstPreparation(t, root)
	_, cmd := root.Update(wspool.IRCEvent{AccountID: "acc", Message: privateMessage("first")})
	result := runPreparation(cmd)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first preparation did not start")
	}

	_, cmd = root.Update(tab.RequestLocalMessageHandleBatchMessage{
		AccountID: "acc", TabID: "tab-1",
		Messages: []twitchirc.IRCer{privateMessage("history-1"), privateMessage("history-2")},
	})
	require.Nil(t, cmd)
	_, cmd = root.Update(wspool.IRCEvent{AccountID: "acc", Message: privateMessage("last")})
	require.Nil(t, cmd)
	require.Empty(t, chatEvents(one), "later work cannot overtake the delayed event")
	require.Len(t, logger, 2, "live logging must not wait for enrichment or include tab-local history")

	close(release)
	drainPreparation(t, root, awaitPreparation(t, result))
	require.Equal(t, []string{"first", "history-1", "history-2", "last"}, eventContents(one))
	require.Equal(t, []string{"first", "last"}, eventContents(two), "history remains tab-local")
	require.True(t, chatEvents(one)[1].IsFakeEvent)
	require.False(t, chatEvents(one)[3].IsFakeEvent)
	require.Zero(t, root.events.inFlight)
	require.Zero(t, root.events.len())
}

func TestRoot_TabClosesWhilePreparationIsInFlight(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	closed, other := newFakeTab("closed"), newFakeTab("other")
	root.tabs.Add(closed)
	root.tabs.Add(other)
	started, release := blockFirstPreparation(t, root)
	result := runPreparation(root.enqueueChatEvent("acc", "closed", privateMessage("first"), true))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	root.tabs.Remove("closed")
	replacement := newFakeTab("replacement") // same account/channel, different UI instance
	root.tabs.Add(replacement)
	root.enqueueChatEvent("acc", "", privateMessage("live"), false)

	close(release)
	drainPreparation(t, root, awaitPreparation(t, result))
	require.Empty(t, chatEvents(closed))
	require.Equal(t, []string{"live"}, eventContents(replacement))
	require.Equal(t, []string{"live"}, eventContents(other))
}

// Use Bubble Tea's actual command scheduler rather than simulating Sequence: raw
// preparation must reach the terminal before either fan-out recipient gets data.
type preparationProgramModel struct {
	root        *Root
	initial     tea.Cmd
	recipients  []*fakeTab
	rawCommands []string
	rawAtRoute  []int
}

func (m *preparationProgramModel) Init() tea.Cmd  { return m.initial }
func (m *preparationProgramModel) View() tea.View { return tea.NewView("") }
func (m *preparationProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The headless program has no terminal size; keep shell geometry fixed so
	// this test exercises scheduling rather than zero-sized header rendering.
	if _, ok := msg.(tea.WindowSizeMsg); ok {
		msg = tea.WindowSizeMsg{Width: 80, Height: 24}
	}
	if raw, ok := msg.(tea.RawMsg); ok {
		m.rawCommands = append(m.rawCommands, raw.Msg.(string))
	}
	before := len(chatEvents(m.recipients[0]))
	_, cmd := m.root.Update(msg)
	if len(chatEvents(m.recipients[0])) > before {
		for _, recipient := range m.recipients {
			m.rawAtRoute = append(m.rawAtRoute, len(m.rawCommands))
			if len(chatEvents(recipient)) != 1 {
				m.rawAtRoute = append(m.rawAtRoute, -1)
			}
		}
		return m, tea.Quit
	}
	return m, cmd
}

func TestRoot_KittyPreparationPrecedesFanOutWithRealScheduler(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	root.preparer.emoteReplacer = emotePreparationFunc(func(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error) {
		return "kitty-image", map[string]string{"image": "placeholder"}, nil
	})
	one, two := newFakeTab("one"), newFakeTab("two")
	root.tabs.Add(one)
	root.tabs.Add(two)
	model := &preparationProgramModel{
		root: root, recipients: []*fakeTab{one, two},
		initial: root.enqueueChatEvent("acc", "", privateMessage("image"), false),
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	_, err := program.Run()
	require.NoError(t, err)
	require.Equal(t, []string{"kitty-image"}, model.rawCommands)
	require.Equal(t, []int{1, 1}, model.rawAtRoute)
	for _, recipient := range model.recipients {
		require.Len(t, chatEvents(recipient), 1)
		require.Empty(t, chatEvents(recipient)[0].PrepareCommand)
	}
}
