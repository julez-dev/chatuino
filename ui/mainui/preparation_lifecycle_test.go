package mainui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

type emotePreparationFunc func(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error)

func (f emotePreparationFunc) Replace(ctx context.Context, channel, content string, emotes []twitchirc.Emote) (string, map[string]string, error) {
	return f(ctx, channel, content, emotes)
}

type badgePreparationFuncs struct {
	replace func(context.Context, string, []twitchirc.Badge) (string, map[string]string, error)
	inject  func(context.Context, string, map[string]string) (string, error)
}

func (f badgePreparationFuncs) Replace(ctx context.Context, channel string, badges []twitchirc.Badge) (string, map[string]string, error) {
	if f.replace != nil {
		return f.replace(ctx, channel, badges)
	}
	return "", nil, nil
}

func (f badgePreparationFuncs) InjectContributorBadge(ctx context.Context, login string, badges map[string]string) (string, error) {
	if f.inject != nil {
		return f.inject(ctx, login, badges)
	}
	return "", nil
}

func runPreparation(cmd tea.Cmd) <-chan tea.Msg {
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	return result
}

func awaitPreparation(t *testing.T, result <-chan tea.Msg) tea.Msg {
	t.Helper()
	select {
	case msg := <-result:
		return msg
	case <-time.After(time.Second):
		t.Fatal("preparation did not finish")
		return nil
	}
}

func TestChatEventPreparer_DeadlineCoversEveryReplacement(t *testing.T) {
	t.Parallel()

	for _, stage := range []string{"emote", "badge", "contributor"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()

			prep, _, _, _ := newTestPreparer(t)
			prep.timeout = 20 * time.Millisecond
			seen := make(chan error, 1)
			wait := func(ctx context.Context) error {
				<-ctx.Done()
				seen <- ctx.Err()
				return ctx.Err()
			}
			switch stage {
			case "emote":
				prep.emoteReplacer = emotePreparationFunc(func(ctx context.Context, _, _ string, _ []twitchirc.Emote) (string, map[string]string, error) {
					return "", nil, wait(ctx)
				})
			case "badge":
				prep.badgeReplacer = badgePreparationFuncs{replace: func(ctx context.Context, _ string, _ []twitchirc.Badge) (string, map[string]string, error) {
					return "", nil, wait(ctx)
				}}
			case "contributor":
				prep.badgeReplacer = badgePreparationFuncs{inject: func(ctx context.Context, _ string, _ map[string]string) (string, error) {
					return "", wait(ctx)
				}}
			}

			message := privateMessage("still displayed")
			message.Badges = []twitchirc.Badge{{Name: "subscriber", Version: "1"}}
			result := runPreparation(func() tea.Msg {
				return prep.prepare(prepareRequest{ircer: message})
			})
			event := awaitPreparation(t, result).(tab.ChatEventMessage)
			require.Same(t, message, event.Message, "enrichment failure must not discard the original message")
			require.NotNil(t, event.DisplayModifier.BadgeReplacement, "failed badge replacement must retain a writable contributor map")
			require.Empty(t, event.PrepareCommand)
			select {
			case err := <-seen:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			default:
				t.Fatal("replacement did not observe the preparation deadline")
			}
		})
	}
}

func TestRoot_ShutdownCancelsInFlightPreparation(t *testing.T) {
	t.Parallel()

	for _, trigger := range []string{"close", "quit", "parent cancellation"} {
		t.Run(trigger, func(t *testing.T) {
			t.Parallel()

			root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			root.preparer.close()
			root.preparer = newChatEventPreparer(ctx, root)
			started := make(chan struct{})
			finished := make(chan struct{})
			root.preparer.emoteReplacer = emotePreparationFunc(func(ctx context.Context, _, _ string, _ []twitchirc.Emote) (string, map[string]string, error) {
				close(started)
				defer close(finished)
				<-ctx.Done()
				return "", nil, ctx.Err()
			})
			ft := newFakeTab("tab-1")
			root.tabs.Add(ft)
			result := runPreparation(root.enqueueChatEvent("acc", "tab-1", privateMessage("blocked"), true))
			require.Nil(t, root.enqueueChatEvent("acc", "tab-1", privateMessage("pending"), true))
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("enrichment did not start")
			}

			switch trigger {
			case "close":
				root.Close()
			case "quit":
				_, cmd := root.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
				require.IsType(t, tea.QuitMsg{}, cmd())
			case "parent cancellation":
				cancel()
				select {
				case <-finished:
				case <-time.After(time.Second):
					t.Fatal("application cancellation did not reach enrichment")
				}
				root.Close()
			}
			select {
			case <-finished:
			default:
				t.Fatal("Close returned before in-flight work unwound")
			}
			_, cmd := root.Update(awaitPreparation(t, result))
			require.Nil(t, cmd, "shutdown must neither route late content nor start pending work")
			require.Empty(t, ft.messages())
			require.Nil(t, root.enqueueChatEvent("acc", "tab-1", privateMessage("after shutdown"), true))
			root.Close() // idempotent, including the deferred application cleanup
		})
	}
}

func TestRoot_CloseBeforeScheduledPreparationStarts(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	var calls atomic.Int32
	root.preparer.emoteReplacer = emotePreparationFunc(func(context.Context, string, string, []twitchirc.Emote) (string, map[string]string, error) {
		calls.Add(1)
		return "", nil, nil
	})
	cmd := root.enqueueChatEvent("acc", "", privateMessage("scheduled"), false)
	root.Close()
	_, next := root.Update(cmd())
	require.Nil(t, next)
	require.Zero(t, calls.Load())
}
