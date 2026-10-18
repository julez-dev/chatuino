package mainui

import (
	"context"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/stretchr/testify/require"
)

// fakeUserClient records the send requests a tab makes and satisfies both the
// APIClient surface used by the dependency container and the user authenticated
// surface used for sending chat messages.
type fakeUserClient struct {
	sentRequests []twitchapi.SendChatMessageRequest
}

func (f *fakeUserClient) GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error) {
	return twitchapi.UserResponse{}, nil
}

func (f *fakeUserClient) GetStreamInfo(ctx context.Context, broadcastID []string) (twitchapi.GetStreamsResponse, error) {
	return twitchapi.GetStreamsResponse{}, nil
}

func (f *fakeUserClient) GetChatSettings(ctx context.Context, broadcasterID string, moderatorID string) (twitchapi.GetChatSettingsResponse, error) {
	return twitchapi.GetChatSettingsResponse{}, nil
}

func (f *fakeUserClient) CreateClip(ctx context.Context, broadcastID string, hasDelay bool) (twitchapi.CreatedClip, error) {
	return twitchapi.CreatedClip{}, nil
}

func (f *fakeUserClient) GetUserChatColor(ctx context.Context, userIDs []string) ([]twitchapi.UserChatColor, error) {
	return nil, nil
}

func (f *fakeUserClient) SendChatMessage(ctx context.Context, data twitchapi.SendChatMessageRequest) (twitchapi.SendChatMessageResponse, error) {
	f.sentRequests = append(f.sentRequests, data)

	return twitchapi.SendChatMessageResponse{}, nil
}

// newReplyTab builds the smallest tab that can arm and send a reply.
func newReplyTab(t *testing.T) (*broadcastTab, *fakeUserClient) {
	t.Helper()

	client := &fakeUserClient{}

	deps := &DependencyContainer{
		UserConfig: UserConfiguration{
			Settings: save.BuildDefaultSettings(),
			Theme:    save.BuildDefaultTheme(),
		},
		Keymap:         save.BuildDefaultKeyMap(),
		APIUserClients: map[string]APIClient{"account": client},
	}

	tab := &broadcastTab{
		id:      "tab",
		account: save.Account{ID: "account", DisplayName: "tester"},
		state:   insertMode,
		width:   80,
		height:  24,
		deps:    deps,
	}

	tab.chatWindow = newChatWindow(tab.width, tab.height, deps)
	tab.input = newChatInput(tab.chatWindow.userColorCache, deps.UserConfig.Settings.BuildCustomSuggestionMap(), deps.UserConfig.Theme, nil)
	tab.input.SetWidth(tab.width)

	return tab, client
}

// newLoadedReplyTab additionally marks the tab as having channel data, so that
// HandleResize performs the real layout calculation instead of bailing out.
func newLoadedReplyTab(t *testing.T) (*broadcastTab, *fakeUserClient) {
	t.Helper()

	tab, client := newReplyTab(t)
	tab.channel = "channel"
	tab.channelID = "channel-id"
	tab.channelLogin = "channel"
	tab.channelDataLoaded = true
	tab.statusInfo = newStreamStatus(tab.width, tab.height, tab, tab.account.ID, tab.channelID, tab.deps)
	tab.streamInfo = newStreamInfo(tab.channelID, client, tab.width)
	tab.poll = newPoll(tab.width)
	tab.HandleResize()

	return tab, client
}

// layoutRows is the number of terminal rows the message input and the chat window
// take together. It has to stay constant while the reply indicator appears and
// disappears, otherwise the status bar is pushed away from the bottom edge.
func layoutRows(tab *broadcastTab) int {
	return tab.chatWindow.height + lipgloss.Height(tab.renderMessageInput())
}

func Test_replySnippet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "plain message unchanged", message: "nice play", want: "nice play"},
		{name: "empty message", message: "", want: ""},
		{name: "newlines and tabs collapse", message: "line one\n\tline two", want: "line one line two"},
		{name: "repeated whitespace collapses", message: "too    many     spaces", want: "too many spaces"},
		{name: "duplicate bypass rune stripped", message: "spam" + string(duplicateBypass), want: "spam"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, replySnippet(tt.message))
		})
	}
}

func Test_truncateToWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{name: "fits exactly", input: "abcde", width: 5, want: "abcde"},
		{name: "shorter than width", input: "abc", width: 5, want: "abc"},
		{name: "cut appends ellipsis", input: "abcdefgh", width: 5, want: "abcd…"},
		{name: "zero width", input: "abc", width: 0, want: ""},
		{name: "single cell keeps ellipsis only", input: "abc", width: 1, want: "…"},
		{name: "negative width", input: "abc", width: -3, want: ""},
		{name: "wide runes counted as two cells", input: "日本語です", width: 5, want: "日本…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncateToWidth(tt.input, tt.width)
			require.Equal(t, tt.want, got)
			require.LessOrEqual(t, lipgloss.Width(got), max(tt.width, 1))
		})
	}
}

func Test_broadcastTab_takeReplyTarget(t *testing.T) {
	t.Parallel()

	tab, _ := newReplyTab(t)

	require.Empty(t, tab.takeReplyTarget(), "no armed reply yields no message ID")

	tab.input.ArmReply("msg-1", "other", "hi")
	require.Equal(t, "msg-1", tab.takeReplyTarget())
	require.False(t, tab.input.ReplyArmed(), "taking disarms the reply")
	require.Empty(t, tab.takeReplyTarget(), "a reply target is only used once")
}

func Test_broadcastTab_handleMessageSent_sendsArmedReply(t *testing.T) {
	t.Parallel()

	tab, client := newReplyTab(t)
	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.input.SetValue("thanks")

	cmd := tab.handleMessageSent(false)

	require.False(t, tab.input.ReplyArmed(), "sending consumes the armed reply")
	require.NotNil(t, cmd)
	require.Nil(t, cmd(), "a successful send produces no notice")

	require.Len(t, client.sentRequests, 1)
	require.Equal(t, "msg-1", client.sentRequests[0].ReplyMessageID)
	require.Equal(t, "thanks", client.sentRequests[0].Message)
}

func Test_broadcastTab_handleMessageSent_commandDoesNotReuseReply(t *testing.T) {
	t.Parallel()

	tab, client := newReplyTab(t)
	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.input.SetValue("/emotes")

	require.NotNil(t, tab.handleMessageSent(false), "the emote overview command is dispatched")

	require.False(t, tab.input.ReplyArmed(), "using a command disarms the reply")

	tab.input.SetValue("not a reply anymore")

	secondSend := tab.handleMessageSent(false)

	require.NotNil(t, secondSend)
	require.Nil(t, secondSend(), "a successful send produces no notice")

	require.Len(t, client.sentRequests, 1)
	require.Empty(t, client.sentRequests[0].ReplyMessageID, "the next message must not be a reply")
}

func Test_broadcastTab_handleMessageSent_quickSendConsumesReply(t *testing.T) {
	t.Parallel()

	tab, client := newReplyTab(t)
	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.input.SetValue("first")

	firstSend := tab.handleMessageSent(true)

	require.NotNil(t, firstSend)
	require.Nil(t, firstSend(), "a successful send produces no notice")

	tab.input.SetValue("second")

	secondSend := tab.handleMessageSent(true)

	require.NotNil(t, secondSend)
	require.Nil(t, secondSend(), "a successful send produces no notice")

	require.Len(t, client.sentRequests, 2)
	require.Equal(t, "msg-1", client.sentRequests[0].ReplyMessageID)
	require.Empty(t, client.sentRequests[1].ReplyMessageID)
}

func Test_broadcastTab_clearReplyTarget(t *testing.T) {
	t.Parallel()

	tab, _ := newReplyTab(t)
	tab.input.ArmReply("msg-1", "other", "hi")

	tab.clearReplyTarget()

	require.False(t, tab.input.ReplyArmed())
}

// Regression: cancelling an armed reply used to leave the chat window at the height
// it had while the indicator was visible. The view then ended up a row short and the
// status bar was no longer rendered at the bottom of the terminal.
func Test_broadcastTab_cancellingReplyRestoresLayout(t *testing.T) {
	t.Parallel()

	tab, _ := newLoadedReplyTab(t)
	rowsWithoutReply := layoutRows(tab)
	chatHeightWithoutReply := tab.chatWindow.height

	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.HandleResize()

	require.Less(t, tab.chatWindow.height, chatHeightWithoutReply, "the indicator takes a row from the chat window")
	require.Equal(t, rowsWithoutReply, layoutRows(tab), "the indicator only moves rows around")

	tab.state = insertMode
	tab.handleEscapePressed()

	require.False(t, tab.input.ReplyArmed(), "escape disarms the reply")
	require.Equal(t, chatHeightWithoutReply, tab.chatWindow.height, "cancelling gives the row back to the chat window")
	require.Equal(t, rowsWithoutReply, layoutRows(tab))
}

// Regression: alt+enter keeps insert mode, so it skipped the layout recalculation that
// the plain send path performs in its state reset.
func Test_broadcastTab_quickSendRestoresLayout(t *testing.T) {
	t.Parallel()

	tab, _ := newLoadedReplyTab(t)
	rowsWithoutReply := layoutRows(tab)
	chatHeightWithoutReply := tab.chatWindow.height

	tab.input.ArmReply("msg-1", "otheruser", "nice play")
	tab.HandleResize()

	tab.input.SetValue("thanks")

	cmd := tab.handleMessageSent(true)

	require.False(t, tab.input.ReplyArmed(), "quick send consumes the armed reply")
	require.Equal(t, chatHeightWithoutReply, tab.chatWindow.height, "quick send gives the row back to the chat window")
	require.Equal(t, rowsWithoutReply, layoutRows(tab))
	require.NotNil(t, cmd)
}
