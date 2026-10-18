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

	if t.isUniqueOnlyChat {
		messagesInStore := t.lastMessages.Keys()
		wordsSrc := strings.Fields(cast.Message)
		lenWords := len(wordsSrc)

		// ignore if message is only one word and the message is in the last messages
		if lenWords == 1 && slices.ContainsFunc(messagesInStore, func(e string) bool { return strings.EqualFold(e, cast.Message) }) {
			return true
		} else if lenWords == 1 {
			return false
		}

		uniqueWordsSrc := map[string]struct{}{}
		for word := range slices.Values(wordsSrc) {
			uniqueWordsSrc[word] = struct{}{}
		}

		// uniqueWordsTarget := map[string]struct{}{}
		for stored := range slices.Values(messagesInStore) {
			distance := fuzzy.LevenshteinDistance(cast.Message, stored)
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

	}

	return false
}

func (t *broadcastTab) handleMessageSent(quickSend bool) tea.Cmd {
	input := t.messageInput.Value()

	// Consume the armed reply before the command dispatch below: commands, quick-sends
	// and unsupported entries must never send a stale reply target.
	replyTo := t.takeReplyTarget()

	if !quickSend {
		// reset state
		if t.state == userInspectInsertMode {
			t.state = userInspectMode
			t.userInspect.chatWindow.Focus()
		} else {
			t.state = inChatWindow
			t.chatWindow.Focus()
		}

		t.messageInput.Blur()
		t.messageInput.SetValue("")
		t.HandleResize() // Recalculate layout after clearing input
	}

	t.chatWindow.moveToBottom()

	// Check if input is a command
	if strings.HasPrefix(input, "/") {
		// Get command name
		end := strings.Index(input, " ")
		if end == -1 {
			end = len(input)
		}

		commandName := input[1:end]

		argStr := strings.TrimSpace(input[end:])
		args := strings.Split(argStr, " ")
		channelID := t.channelID
		channel := t.channelLogin
		accountID := t.account.ID

		switch commandName {
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

		// Message input is only allowed for authenticated users
		// so ttvAPI is guaranteed to be a moderationAPIClient
		client := t.deps.APIUserClients[t.account.ID].(moderationAPIClient)

		return handleCommand(commandName, args, channelID, channel, accountID, client)
	}

	// Check if message is the same as the last message sent
	// If so, append special character to bypass twitch duplicate message filter
	if strings.EqualFold(input, t.lastMessageSent) {
		input = input + " " + string(duplicateBypass)
	}

	lastSent := t.lastMessageSentAt
	client := t.deps.APIUserClients[t.account.ID].(userAuthenticatedAPIClient)
	broadcasterID := t.channelID
	userID := t.account.ID

	cmd := func() tea.Msg {
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

	t.lastMessageSent = input
	t.lastMessageSentAt = time.Now()

	return cmd
}

func (t *broadcastTab) replaceInputTemplate() tea.Cmd {
	input := t.messageInput.InputModel.Value()

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

	t.messageInput.SetValue(inputText)
	t.HandleResize() // Recalculate layout after template replacement
	return nil
}
