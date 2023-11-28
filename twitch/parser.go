package twitch

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/julez-dev/chatuino/twitch/command"
)

var (
	// ErrZeroLengthMessage is returned when parsing if the input is
	// zero-length.
	ErrZeroLengthMessage = errors.New("irc: cannot parse zero-length message")

	// ErrMissingDataAfterPrefix is returned when parsing if there is
	// no message data after the prefix.
	ErrMissingDataAfterPrefix = errors.New("irc: no message data after prefix")

	// ErrMissingDataAfterTags is returned when parsing if there is no
	// message data after the tags.
	ErrMissingDataAfterTags = errors.New("irc: no message data after tags")

	// ErrMissingCommand is returned when parsing if there is no
	// command in the parsed message.
	ErrMissingCommand = errors.New("irc: missing message command")

	ErrUnhandledCommand = errors.New("irc: message command not handled by parser")
)

type tagValue string

type tags map[string]tagValue

type rawTMI struct {
	// Each message can have IRCv3 tags
	tags `json:"tags"`

	// Each message can have a Prefix
	*prefix

	// Command is which command is being called.
	Command string `json:"command"`

	// Params are all the arguments for the command.
	Params []string `json:"params"`

	Message string `json:"message"`
}

type prefix struct {
	// Name will contain the nick of who sent the message, the
	// server who sent the message, or a blank string
	Name string `json:"name"`

	// User will either contain the user who sent the message or a blank string
	User string `json:"user"`

	// Host will either contain the host of who sent the message or a blank string
	Host string `json:"host"`
}

var tagDecodeSlashMap = map[rune]rune{
	':':  ';',
	's':  ' ',
	'\\': '\\',
	'r':  '\r',
	'n':  '\n',
}

func parseIRC(message string) (IRCer, error) {
	// Trim the line and make sure we have data
	message = strings.TrimRight(message, "\r\n")
	if len(message) == 0 {
		return nil, ErrZeroLengthMessage
	}

	c := &rawTMI{
		tags:    tags{},
		prefix:  &prefix{},
		Message: message,
	}

	if message[0] == '@' {
		loc := strings.Index(message, " ")
		if loc == -1 {
			return nil, ErrMissingDataAfterTags
		}

		c.tags = parseTags(message[1:loc])
		message = message[loc+1:]
	}

	if message[0] == ':' {
		loc := strings.Index(message, " ")
		if loc == -1 {
			return nil, ErrMissingDataAfterPrefix
		}

		// Parse the identity, if there was one
		c.prefix = parsePrefix(message[1:loc])
		message = message[loc+1:]
	}

	// Split out the trailing then the rest of the args. Because
	// we expect there to be at least one result as an arg (the
	// command) we don't need to special case the trailing arg and
	// can just attempt a split on " :"
	split := strings.SplitN(message, " :", 2)
	c.Params = strings.FieldsFunc(split[0], func(r rune) bool {
		return r == ' '
	})

	// If there are no args, we need to bail because we need at
	// least the command.
	if len(c.Params) == 0 {
		return nil, ErrMissingCommand
	}

	// If we had a trailing arg, append it to the other args
	if len(split) == 2 {
		c.Params = append(c.Params, split[1])
	}

	// Because of how it's parsed, the Command will show up as the
	// first arg.
	c.Command = strings.ToUpper(c.Params[0])
	c.Params = c.Params[1:]

	// If there are no params, set it to nil, to make writing tests and other
	// things simpler.
	if len(c.Params) == 0 {
		c.Params = nil
	}

	switch c.Command {
	case "PRIVMSG":
		bits, err := strconv.Atoi(emptyStringZero(string(c.tags["bits"])))
		if err != nil {
			return nil, err
		}

		paidAmount, err := strconv.Atoi(emptyStringZero(string(c.tags["pinned-chat-paid-amount"])))
		if err != nil {
			return nil, err
		}

		paidExponent, err := strconv.Atoi(emptyStringZero(string(c.tags["pinned-chat-paid-exponent"])))
		if err != nil {
			return nil, err
		}

		p := command.PrivateMessage{
			BadgeInfo:   parseBadges(string(c.tags["badge-info"])),
			Badges:      parseBadges(string(c.tags["badges"])),
			Bits:        bits,
			Color:       string(c.tags["color"]),
			DisplayName: string(c.tags["display-name"]),
			Emotes:      parseEmotes(string(c.tags["emotes"])),
			ID:          string(c.tags["id"]),
			Mod:         c.tags["mod"] == "1",
			FirstMsg:    c.tags["first-msg"] == "1",

			PaidAmount:          paidAmount,
			PaidCurrency:        string(c.tags["pinned-chat-paid-currency"]),
			PaidExponent:        paidExponent,
			PaidLevel:           string(c.tags["pinned-chat-paid-level"]),
			PaidIsSystemMessage: c.tags["pinned-chat-paid-is-system-message"] == "1",

			ParentMsgID:           string(c.tags["reply-parent-msg-id"]),
			ParentUserID:          string(c.tags["reply-parent-user-id"]),
			ParentUserLogin:       string(c.tags["reply-parent-user-login"]),
			ParentDisplayName:     string(c.tags["reply-parent-display-name"]),
			ParentMsgBody:         string(c.tags["reply-parent-msg-body"]),
			ThreadParentMsgID:     string(c.tags["reply-thread-parent-msg-id"]),
			ThreadParentUserLogin: string(c.tags["reply-thread-parent-user-login"]),

			RoomID:          string(c.tags["room-id"]),
			ChannelUserName: strings.TrimPrefix(c.Params[0], "#"),
			Subscriber:      c.tags["subscriber"] == "1",
			TMISentTS:       parseTimestamp(string(c.tags["tmi-sent-ts"])),
			Turbo:           c.tags["turbo"] == "1",
			UserID:          string(c.tags["user-id"]),
			UserType:        command.UserType(c.tags["user-type"]),
			VIP:             c.tags["vip"] == "1",
		}

		if len(c.Params) > 1 {
			p.Message = c.Params[1]
		}

		return &p, nil
	case "PING":
		return command.PingMessage{}, nil
	case "NOTICE":
		n := command.Notice{
			MsgID: command.MsgID(c.tags["msg-id"]),
		}

		if len(c.Params) > 1 {
			n.ChannelUserName = strings.TrimPrefix(c.Params[0], "#")
			n.Message = c.Params[1]
		}

		return &n, nil
	case "USERNOTICE":
		u := command.UserNotice{
			BadgeInfo:   parseBadges(string(c.tags["badge-info"])),
			Badges:      parseBadges(string(c.tags["badges"])),
			Color:       string(c.tags["color"]),
			DisplayName: string(c.tags["display-name"]),
			Emotes:      parseEmotes(string(c.tags["emotes"])),
			ID:          string(c.tags["id"]),
			Login:       string(c.tags["login"]),
			MsgID:       command.MsgID(c.tags["msg-id"]),
			RoomID:      string(c.tags["room-id"]),
			SystemMsg:   string(c.tags["system-msg"]),
			TMISentTS:   parseTimestamp(string(c.tags["tmi-sent-ts"])),
			UserID:      string(c.tags["user-id"]),
			UserType:    command.UserType(c.tags["user-type"]),
			Mod:         c.tags["mod"] == "1",
			Subscriber:  c.tags["subscriber"] == "1",
			Turbo:       c.tags["turbo"] == "1",
		}

		switch u.MsgID {
		case command.Sub, command.ReSub:
			cumMonths, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-cumulative-months"])))
			if err != nil {
				return nil, err
			}

			streakMonths, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-streak-months"])))
			if err != nil {
				return nil, err
			}

			sub := &command.SubMessage{
				UserNotice:        u,
				CumulativeMonths:  cumMonths,
				ShouldShareStreak: c.tags["msg-param-should-share-streak"] == "1",
				StreakMonths:      streakMonths,
				SubPlan:           command.SubPlan(c.tags["msg-param-sub-plan"]),
				SubPlanName:       string(c.tags["msg-param-sub-plan-name"]),
			}

			if len(c.Params) > 1 {
				sub.Message = c.Params[1]
			}

			return sub, nil
		case command.SubGift:
			months, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-months"])))
			if err != nil {
				return nil, err
			}

			giftMonths, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-gift-months"])))
			if err != nil {
				return nil, err
			}

			sub := command.SubGiftMessage{
				UserNotice:         u,
				Months:             months,
				ReceiptDisplayName: string(c.tags["msg-param-recipient-display-name"]),
				RecipientID:        string(c.tags["msg-param-recipient-id"]),
				RecipientUserName:  string(c.tags["msg-param-recipient-user-name"]),
				SubPlan:            command.SubPlan(c.tags["msg-param-sub-plan"]),
				SubPlanName:        string(c.tags["msg-param-sub-plan-name"]),
				GiftMonths:         giftMonths,
			}

			return &sub, nil
		case command.Announcement:
			announcement := command.AnnouncementMessage{
				UserNotice: u,
			}

			if len(c.Params) > 1 {
				announcement.Message = c.Params[1]
			}

			return &announcement, nil
		case command.Raid:
			viewerCount, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-viewerCount"])))
			if err != nil {
				return nil, err
			}

			raid := command.RaidMessage{
				UserNotice:  u,
				DisplayName: string(c.tags["msg-param-displayName"]),
				Login:       string(c.tags["msg-param-login"]),
				ViewerCount: viewerCount,
			}

			return &raid, nil
		case command.AnonGiftPaidUpgrade:
			giftTotal, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-promo-gift-total"])))
			if err != nil {
				return nil, err
			}

			gift := command.AnonGiftPaidUpgradeMessage{
				UserNotice:     u,
				PromoGiftTotal: giftTotal,
				PromoName:      string(c.tags["msg-param-promo-name"]),
			}

			return &gift, nil
		case command.GiftPaidUpgrade:
			giftTotal, err := strconv.Atoi(emptyStringZero(string(c.tags["msg-param-promo-gift-total"])))
			if err != nil {
				return nil, err
			}

			gift := command.GiftPaidUpgradeMessage{
				UserNotice:     u,
				PromoGiftTotal: giftTotal,
				PromoName:      string(c.tags["msg-param-promo-name"]),
				SenderLogin:    string(c.tags["msg-param-sender-login"]),
				SenderName:     string(c.tags["msg-param-sender-name"]),
			}

			return &gift, nil
		case command.Ritual:
			ritual := command.RitualMessage{
				UserNotice: u,
				RitualName: string(c.tags["msg-param-ritual-name"]),
			}

			if len(c.Params) > 1 {
				ritual.Message = c.Params[1]
			}

			return &ritual, nil
		}

		return &u, nil
	case "USERSTATE":
		u := command.UserState{
			BadgeInfo:   parseBadges(string(c.tags["badge-info"])),
			Badges:      parseBadges(string(c.tags["badges"])),
			Color:       string(c.tags["color"]),
			DisplayName: string(c.tags["display-name"]),
			EmoteSets:   strings.Split(string(c.tags["emote-sets"]), ","),
			ID:          string(c.tags["id"]),
			Subscriber:  c.tags["subscriber"] == "1",
			Turbo:       c.tags["turbo"] == "1",
			UserType:    command.UserType(c.tags["user-type"]),
		}

		return &u, nil
	case "WHISPER":
		w := command.Whisper{
			Badges:      parseBadges(string(c.tags["badges"])),
			Color:       string(c.tags["color"]),
			DisplayName: string(c.tags["display-name"]),
			Emotes:      parseEmotes(string(c.tags["emotes"])),
			ID:          string(c.tags["id"]),
			ThreadID:    string(c.tags["thread-id"]),
			Turbo:       c.tags["turbo"] == "1",
			UserID:      string(c.tags["user-id"]),
			UserType:    command.UserType(c.tags["user-type"]),
		}

		if len(c.Params) > 1 {
			w.Message = c.Params[1]
		}

		return &w, nil
	case "ROOMSTATE":
		r := command.RoomState{
			RoomID: string(c.tags["room-id"]),
		}

		if val, ok := c.tags["emote-only"]; ok {
			r.EmoteOnly = pointer(val == "1")
		}

		if val, ok := c.tags["r9k"]; ok {
			r.R9K = pointer(val == "1")
		}

		if val, ok := c.tags["subs-only"]; ok {
			r.SubsOnly = pointer(val == "1")
		}

		if val, ok := c.tags["followers-only"]; ok {
			followerDelay, err := strconv.Atoi(emptyStringZero(string(val)))
			if err != nil {
				return nil, err
			}

			r.FollowersOnly = pointer(followerDelay)
		}

		if val, ok := c.tags["slow"]; ok {
			slow, err := strconv.Atoi(emptyStringZero(string(val)))
			if err != nil {
				return nil, err
			}

			r.Slow = pointer(slow)
		}

		return &r, nil
	case "CLEARCHAT":
		banDuration, err := strconv.Atoi(emptyStringZero(string(c.tags["ban-duration"])))
		if err != nil {
			return nil, err
		}

		cc := command.ClearChat{
			BanDuration:  banDuration,
			RoomID:       string(c.tags["room-id"]),
			TargetUserID: string(c.tags["target-user-id"]),
			TMISentTS:    parseTimestamp(string(c.tags["tmi-sent-ts"])),
		}

		if len(c.Params) > 1 {
			cc.UserName = c.Params[1]
		}

		return &cc, nil
	case "CLEARMSG":
		c := command.ClearMessage{
			Login:       string(c.tags["login"]),
			RoomID:      string(c.tags["room-id"]),
			TargetMsgID: string(c.tags["target-msg-id"]),
			TMISentTS:   parseTimestamp(string(c.tags["tmi-sent-ts"])),
		}

		return &c, nil
	}

	return nil, ErrUnhandledCommand
}

func pointer[T any](i T) *T {
	return &i
}

func emptyStringZero(s string) string {
	if s == "" {
		return "0"
	}

	return s
}

func parseEmotes(emoteStr string) []command.Emote {
	// emote format 79382:20-24
	emoteSplit := strings.Split(string(emoteStr), ",")
	emotes := make([]command.Emote, 0, len(emoteSplit))

	for _, emote := range emoteSplit {
		parts := strings.Split(emote, ":")
		if len(parts) != 2 {
			continue
		}

		positions := strings.Split(parts[1], "-")

		if len(positions) != 2 {
			continue
		}

		start, err := strconv.Atoi(positions[0])
		if err != nil {
			continue
		}

		end, err := strconv.Atoi(positions[1])
		if err != nil {
			continue
		}

		emotes = append(emotes, command.Emote{
			ID:    parts[0],
			Start: start,
			End:   end,
		})

	}

	return emotes
}

func parseBadges(badgeStr string) []command.Badge {
	if badgeStr == "" {
		return nil
	}

	badgeSplit := strings.Split(string(badgeStr), ",")
	badges := make([]command.Badge, 0, len(badgeSplit))

	for _, badge := range badgeSplit {
		parts := strings.SplitN(badge, "/", 2)
		if len(parts) == 1 {
			badges = append(badges, command.Badge{Name: parts[0]})
			continue
		}

		count, err := strconv.Atoi(parts[1])
		if err != nil {
			badges = append(badges, command.Badge{Name: parts[0]})
			continue
		}

		badges = append(badges, command.Badge{Name: parts[0], Version: count})
	}

	return badges
}

func parseTimestamp(timeStr string) time.Time {
	i, err := strconv.ParseInt(timeStr, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(0, i*1e6)
}

func parsePrefix(line string) *prefix {
	// Start by creating a Prefix with nothing but the host
	id := &prefix{
		Name: line,
	}

	uh := strings.SplitN(id.Name, "@", 2)
	if len(uh) == 2 {
		id.Name, id.Host = uh[0], uh[1]
	}

	nu := strings.SplitN(id.Name, "!", 2)
	if len(nu) == 2 {
		id.Name, id.User = nu[0], nu[1]
	}

	return id
}

func parseTagValue(v string) tagValue {
	ret := &bytes.Buffer{}

	input := bytes.NewBufferString(v)

	for {
		c, _, err := input.ReadRune()
		if err != nil {
			break
		}

		if c == '\\' {
			c2, _, err := input.ReadRune()
			// If we got a backslash then the end of the tag value, we should
			// just ignore the backslash.
			if err != nil {
				break
			}

			if replacement, ok := tagDecodeSlashMap[c2]; ok {
				ret.WriteRune(replacement)
			} else {
				ret.WriteRune(c2)
			}
		} else {
			ret.WriteRune(c)
		}
	}

	return tagValue(ret.String())
}

func parseTags(line string) tags {
	ret := tags{}

	tags := strings.Split(line, ";")
	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) < 2 {
			ret[parts[0]] = ""
			continue
		}

		ret[parts[0]] = parseTagValue(parts[1])
	}

	return ret
}
