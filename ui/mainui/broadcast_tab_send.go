package mainui

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/lithammer/fuzzysearch/fuzzy"

	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

func (t *broadcastTab) shouldIgnoreMessage(msg twitchirc.IRCer) bool {
	if messageMatchesBlocked(msg, t.deps.UserConfig.Settings.BlockSettings) {
		return true
	}

	cast, ok := msg.(*twitchirc.PrivateMessage)

	// all non private messages are okay
	if !ok {
		return false
	}

	// never ignore messages from the user,broadcaster,subs,mods,vips,paid messages,staff,bits or message mentions user
	if cast.UserID == t.account.ID || cast.UserID == t.channelID || cast.Mod || cast.PaidAmount != 0 || cast.VIP ||
		messageContainsCaseInsensitive(cast, t.account.DisplayName) || cast.Bits != 0 ||
		cast.UserType == twitchirc.Admin || cast.UserType == twitchirc.GlobalMod || cast.UserType == twitchirc.Staff {
		return false
	}

	// is sub and sender is not sub
	if t.isLocalSub && !cast.Subscriber {
		return true
	}

	if t.isUniqueOnlyChat && isNearDuplicate(cast.Message, t.lastMessages.Keys()) {
		return true
	}

	return false
}

// isNearDuplicate reports whether message is (nearly) identical to one of the
// recently seen messages. A single-word message only matches a case-insensitively
// equal entry; longer messages match when the Levenshtein distance to any recent
// message is small.
func isNearDuplicate(message string, recent []string) bool {
	wordsSrc := strings.Fields(message)
	lenWords := len(wordsSrc)

	// a one-word message is a duplicate when it was recently seen verbatim
	if lenWords == 1 {
		return slices.ContainsFunc(recent, func(e string) bool { return strings.EqualFold(e, message) })
	}

	uniqueWordsSrc := map[string]struct{}{}
	for word := range slices.Values(wordsSrc) {
		uniqueWordsSrc[word] = struct{}{}
	}

	// uniqueWordsTarget := map[string]struct{}{}
	for stored := range slices.Values(recent) {
		distance := fuzzy.LevenshteinDistance(message, stored)
		if distance < 3 {
			return true
		}

		// for word := range slices.Values(strings.Fields(stored)) {
		// 	uniqueWordsTarget[strings.ToLower(word)] = struct{}{}
		// }

		// wordListSrc := slices.Collect(maps.Keys(uniqueWordsSrc))
		// var matches int
		// for word := range slices.Values(wordListSrc) {
		// 	word = strings.ToLower(word)
		// 	if _, ok := uniqueWordsTarget[word]; ok {
		// 		matches++
		// 	}
		// }

		// // if more than 70% of the words are the same, ignore the message
		// if float64(matches)/float64(lenWords) > 0.7 {
		// 	return true
		// }

		// clear(uniqueWordsTarget)
	}

	return false
}

// handleMessageSent processes the content of the message input after a send key
// press: slash commands are dispatched to their handlers, everything else is sent
// to the channel as a chat message.
func (t *broadcastTab) handleMessageSent(quickSend bool) tea.Cmd {
	input := t.input.Value()

	// Consume the armed reply before the command dispatch below: commands, quick-sends
	// and unsupported entries must never send a stale reply target.
	replyTo := t.takeReplyTarget()

	if !quickSend {
		t.resetAfterSend()
	}

	t.chatWindow.moveToBottom()

	if name, args, ok := parseSlashCommand(input); ok {
		return t.dispatchSlashCommand(name, args)
	}

	input = applyDuplicateBypass(input, t.lastMessageSent)

	cmd := t.sendChatMessageCmd(input, replyTo)

	t.lastMessageSent = input
	t.lastMessageSentAt = time.Now()

	return cmd
}

// resetAfterSend leaves insert mode and clears the message input after a regular
// (non quick) send.
func (t *broadcastTab) resetAfterSend() {
	_, _ = t.transit(eventSend)
	t.HandleResize() // Recalculate layout after clearing input
}

// parseSlashCommand splits a chat input of the form "/name arg1 arg2" into its
// command name and arguments. ok is false when the input is not a slash command.
// A command without arguments yields args == [""]; the command handlers rely on
// this historical shape when validating their arguments.
func parseSlashCommand(input string) (name string, args []string, ok bool) {
	if !strings.HasPrefix(input, "/") {
		return "", nil, false
	}

	end := strings.Index(input, " ")
	if end == -1 {
		end = len(input)
	}

	name = input[1:end]
	argStr := strings.TrimSpace(input[end:])

	return name, strings.Split(argStr, " "), true
}

// dispatchSlashCommand routes a parsed slash command to its handler. Commands
// without a local handler fall through to the moderation command handling, which
// is only available to moderators.
func (t *broadcastTab) dispatchSlashCommand(name string, args []string) tea.Cmd {
	switch name {
	case "inspect":
		return t.handleOpenUserInspect(args)
	case "popupchat":
		return t.handleOpenBrowserChatPopUp()
	case "channel":
		return t.handleOpenBrowserChannel()
	case "pyramid":
		return t.handlePyramidMessagesCommand(args)
	case "localsubscribers":
		return t.handleLocalSubCommand(true)
	case "localsubscribersoff":
		return t.handleLocalSubCommand(false)
	case "uniqueonly":
		return t.handleUniqueOnlyChatCommand(true)
	case "uniqueonlyoff":
		return t.handleUniqueOnlyChatCommand(false)
	case "createclip":
		return t.handleCreateClipMessage()
	case "emotes":
		return t.handleOpenEmoteOverview()
	case "refreshemotes":
		return t.handleManualRefreshEmotes()
	case "join":
		return t.handleJoinCommand(args)
	}

	if !t.isUserMod {
		return func() tea.Msg {
			respMsg := chatEventMessage{
				isFakeEvent: true,
				accountID:   t.account.ID,
				tabID:       t.id,
				message: &twitchirc.Notice{
					FakeTimestamp: time.Now(),
					Message:       "Moderator commands are not available since you are not a moderator",
				},
			}
			return respMsg
		}
	}

	// Moderation commands need the account's own token.
	client := t.deps.Clients[t.account.ID].Moderation
	if client == nil {
		return t.localNoticeCmd("This account cannot use moderation commands")
	}

	return handleCommand(name, args, t.channelID, t.channelLogin, t.account.ID, client)
}

// applyDuplicateBypass appends an invisible character when input repeats the
// previously sent message, bypassing Twitch's duplicate message filter.
func applyDuplicateBypass(input, lastSent string) string {
	if strings.EqualFold(input, lastSent) {
		return input + " " + string(duplicateBypass)
	}

	return input
}

// sendChatMessageCmd returns the command that sends input as a chat message via
// the API, delayed so that consecutive sends are at least a second apart.
func (t *broadcastTab) sendChatMessageCmd(input, replyTo string) tea.Cmd {
	lastSent := t.lastMessageSentAt
	client := t.deps.Clients[t.account.ID].Chat
	if client == nil {
		return nil
	}

	broadcasterID := t.channelID
	userID := t.account.ID

	return func() tea.Msg {
		const delay = time.Second
		diff := time.Since(lastSent)
		if diff < delay {
			time.Sleep(delay - diff)
		}

		notice := &twitchirc.Notice{
			FakeTimestamp: time.Now(),
		}

		resp := chatEventMessage{
			isFakeEvent: true,
			accountID:   userID,
			channel:     t.channelLogin,
			channelID:   t.channelID,
			tabID:       t.id,
			message:     notice,
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		r, err := client.SendChatMessage(ctx, twitchapi.SendChatMessageRequest{
			BroadcasterID:  broadcasterID,
			SenderID:       userID,
			Message:        input,
			ReplyMessageID: replyTo,
		})
		if err != nil {
			notice.Message = fmt.Sprintf("Could not send message: %s", err.Error())
			return resp
		}

		if len(r.Data) > 0 && !r.Data[0].IsSent {
			notice.Message = fmt.Sprintf("Could not send message: %s", r.Data[0].DropReason.Message)
			return resp
		}

		return nil
	}
}

func (t *broadcastTab) replaceInputTemplate() tea.Cmd {
	input := t.input.RawValue()

	notice := &twitchirc.Notice{
		FakeTimestamp: time.Now(),
	}

	resp := chatEventMessage{
		isFakeEvent: true,
		accountID:   t.account.ID,
		channel:     t.channelLogin,
		channelID:   t.channelID,
		tabID:       t.id,
		message:     notice,
	}

	tmpl, err := template.New("").Parse(input)
	if err != nil {
		notice.Message = fmt.Sprintf("Error while parsing template: %s", err)

		return func() tea.Msg {
			return resp
		}
	}

	data := map[string]any{
		"CurrentTime":     time.Now().Local().Format("15:04:05"),
		"CurrentDateTime": time.Now().Local().Format("2006-01-02 15:04:05"),
		"BroadcastID":     t.channelID,
		"BroadcastName":   t.channelLogin,
	}

	// if a row is currently selected
	if _, e := t.chatWindow.entryForCurrentCursor(); e != nil {
		switch msg := e.Event.message.(type) {
		case *twitchirc.PrivateMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg.Message
			data["SelectedUserID"] = msg.UserID
			data["MessageID"] = msg.ID

			data["RawMessage"] = msg
			data["MessageType"] = "PrivateMessage"
		case *twitchirc.SubMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg.Message
			data["SelectedUserID"] = msg.UserID
			data["MessageID"] = msg.ID

			data["SubMessageCumulativeMonths"] = msg.CumulativeMonths
			data["SubMessageStreakMonths"] = msg.StreakMonths
			data["SubMessageSubPlan"] = msg.SubPlan.String()

			data["RawMessage"] = msg
			data["MessageType"] = "SubMessage"
		case *twitchirc.SubGiftMessage:
			data["SelectedDisplayName"] = msg.DisplayName
			data["SelectedMessageContent"] = msg
			data["SelectedUserID"] = msg.UserID
			data["MessageID"] = msg.ID

			data["SubGiftReceiptDisplayName"] = msg.ReceiptDisplayName
			data["SubGiftRecipientID"] = msg.RecipientID
			data["SubGiftMonths"] = msg.Months
			data["SubGiftSubPlan"] = msg.SubPlan.String()
			data["SubGiftGiftMonths"] = msg.GiftMonths

			data["RawMessage"] = msg
			data["MessageType"] = "SubGiftMessage"
		}
	}

	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		notice.Message = fmt.Sprintf("Error while executing template: %s", err)
		return func() tea.Msg {
			return resp
		}
	}

	inputText := out.String()

	// replace alternative mod commands
	if t.isUserMod && strings.HasPrefix(inputText, "/") {
		for from, to := range maps.All(modCommandAlternativeMapping) {
			if strings.HasPrefix(inputText, from) {
				inputText = strings.Replace(inputText, from, to, 1)
				break
			}
		}
	}

	t.input.SetValue(inputText)
	t.HandleResize() // Recalculate layout after template replacement
	return nil
}
