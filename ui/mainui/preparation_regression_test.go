package mainui

import (
	"testing"

	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/stretchr/testify/require"
)

func TestRoot_DuplicateGraphicsPreparationHasNoSideEffects(t *testing.T) {
	t.Parallel()

	root := newTestRoot(t, nil, map[string]tab.ClientCapabilities{}, nil)
	ft := newFakeTab("tab-1")
	root.tabs.Add(ft)
	root.enqueueChatEvent("acc", "tab-1", privateMessage("image"), false)
	seq := root.events.inFlight
	prepared := preparedChatEvent{seq: seq, event: tab.ChatEventMessage{
		TabID: "tab-1", Message: privateMessage("image"), PrepareCommand: "kitty-image",
	}}

	_, first := root.Update(prepared)
	require.NotNil(t, first)
	require.Empty(t, ft.messages())

	_, duplicate := root.Update(prepared)
	require.Nil(t, duplicate, "a duplicate must not resend the terminal command during the handoff")

	event := prepared.event
	event.PrepareCommand = ""
	root.Update(routedPreparedEvent{seq: seq, event: event})
	require.Len(t, ft.messages(), 1)

	_, duplicate = root.Update(prepared)
	require.Nil(t, duplicate, "a completed request must not resend the terminal command")
	_, duplicate = root.Update(routedPreparedEvent{seq: seq, event: event})
	require.Nil(t, duplicate)
	require.Len(t, ft.messages(), 1)

	next := root.enqueueChatEvent("acc", "tab-1", privateMessage("next"), false)
	require.NotNil(t, next)
	nextSeq := root.events.inFlight
	_, duplicate = root.Update(prepared)
	require.Nil(t, duplicate, "a stale result must not affect a newer request")
	require.Equal(t, nextSeq, root.events.inFlight)
	require.Len(t, ft.messages(), 1)
}
