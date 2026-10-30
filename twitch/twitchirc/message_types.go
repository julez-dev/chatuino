package twitchirc

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// IRCer are types that can be turned into an IRC command
type IRCer interface {
	IRC() string
}

type Badge struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (b Badge) String() string {
	splits := strings.Split(b.Name, "-")

	for i, split := range splits {
		r := []rune(split)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
		}
		splits[i] = string(r)
	}

	return strings.Join(splits, "-")
}

type Emote struct {
	ID        string          `json:"id"`
	Positions []EmotePosition `json:"positions"`
}

type EmotePosition struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type UserType string

const (
	Empty     UserType = "" // normal user
	Admin     UserType = "admin"
	GlobalMod UserType = "global_mod"
	Staff     UserType = "staff"
)

// PrivateMessage is persisted as JSON in the message log, so the snake_case
// member names below are a storage format: keep them stable.
type PrivateMessage struct {
	BadgeInfo   []Badge `json:"badge_info"`
	Badges      []Badge `json:"badges"`
	Bits        int     `json:"bits"`
	Color       string  `json:"color"`
	DisplayName string  `json:"display_name"`
	Emotes      []Emote `json:"emotes"`
	ID          string  `json:"id"`
	Mod         bool    `json:"mod"`
	FirstMsg    bool    `json:"first_msg"`

	// Hype chat
	PaidAmount          int    `json:"paid_amount"`
	PaidCurrency        string `json:"paid_currency"`
	PaidExponent        int    `json:"paid_exponent"`
	PaidLevel           string `json:"paid_level"`
	PaidIsSystemMessage bool   `json:"paid_is_system_message"`

	// Reply
	ParentMsgID           string `json:"parent_msg_id"`
	ParentUserID          string `json:"parent_user_id"`
	ParentUserLogin       string `json:"parent_user_login"`
	ParentDisplayName     string `json:"parent_display_name"`
	ParentMsgBody         string `json:"parent_msg_body"`
	ThreadParentMsgID     string `json:"thread_parent_msg_id"`
	ThreadParentUserLogin string `json:"thread_parent_user_login"`

	RoomID          string    `json:"room_id"`
	ChannelUserName string    `json:"channel_user_name"`
	LoginName       string    `json:"login_name"`
	Subscriber      bool      `json:"subscriber"`
	TMISentTS       time.Time `json:"tmi_sent_ts"`
	Turbo           bool      `json:"turbo"`
	UserID          string    `json:"user_id"`
	UserType        UserType  `json:"user_type"`
	VIP             bool      `json:"vip"`

	// Joined chat rooms
	SourceID     string  `json:"source_id"`
	SourceRoomID string  `json:"source_room_id"`
	SourceBadges []Badge `json:"source_badges"`

	Message string `json:"message"`
}

func (p *PrivateMessage) IRC() string {
	if p.ParentMsgID != "" {
		return fmt.Sprintf("@reply-parent-msg-id=%s PRIVMSG #%s :%s", p.ParentMsgID, p.ChannelUserName, p.Message)
	}

	return fmt.Sprintf("PRIVMSG #%s :%s", p.ChannelUserName, p.Message)
}

func (p *PrivateMessage) Clone() *PrivateMessage {
	p2 := new(PrivateMessage)
	*p2 = *p

	p2.BadgeInfo = slices.Clone(p.BadgeInfo)
	p2.Badges = slices.Clone(p.Badges)
	p2.Emotes = slices.Clone(p.Emotes)
	return p2
}

type PongMessage struct{}

func (p PongMessage) IRC() string {
	return "PONG :tmi.twitch.tv"
}

type PingMessage struct{}

func (p PingMessage) IRC() string {
	return "PING :tmi.twitch.tv"
}

type JoinMessage struct {
	Channel string
}

func (j JoinMessage) IRC() string {
	return "JOIN #" + j.Channel
}

type PartMessage struct {
	Channel string
}

func (p PartMessage) IRC() string {
	return "PART #" + p.Channel
}

type MsgID string

const (
	// UserNotice
	Sub                 MsgID = "sub"
	ReSub               MsgID = "resub"
	SubGift             MsgID = "subgift"
	SubMysteryGift      MsgID = "submysterygift"
	GiftPaidUpgrade     MsgID = "giftpaidupgrade"
	RewardGift          MsgID = "rewardgift"
	AnonGiftPaidUpgrade MsgID = "anongiftpaidupgrade"
	Raid                MsgID = "raid"
	UnRaid              MsgID = "unraid"
	Ritual              MsgID = "ritual"
	BitsBadgeTier       MsgID = "bitsbadgetier"
	Announcement        MsgID = "announcement"

	// Notice
	// SubsOn       MsgID = "subs_on"
	// SubsOff      MsgID = "subs_off"
	// EmoteOnlyOn  MsgID = "emote_only_on"
	// EmoteOnlyOff MsgID = "emote_only_off"
	// FollowersOn  MsgID = "followers_on"
	// FollowersOff MsgID = "followers_off"
	// SlowOn       MsgID = "slow_on"
	// SlowOff      MsgID = "slow_off"
	// R9kOn        MsgID = "r9k_on" // also known as unique chat
	// R9kOff       MsgID = "r9k_off"
)

type UserNotice struct {
	BadgeInfo       []Badge
	Badges          []Badge
	Color           string
	DisplayName     string
	Emotes          []Emote
	ID              string
	Login           string
	Mod             bool
	MsgID           MsgID
	RoomID          string
	ChannelUserName string
	Subscriber      bool
	SystemMsg       string
	TMISentTS       time.Time
	Turbo           bool
	UserID          string
	UserType        UserType
}

func (u *UserNotice) IRC() string {
	return ""
}

type SubPlan string

func (s SubPlan) String() string {
	switch s {
	case Prime:
		return "Prime"
	case Tier1:
		return "Tier 1"
	case Tier2:
		return "Tier 2"
	case Tier3:
		return "Tier 3"
	}

	return ""
}

const (
	Prime SubPlan = "Prime"
	Tier1 SubPlan = "1000"
	Tier2 SubPlan = "2000"
	Tier3 SubPlan = "3000"
)

type AnnouncementColor string

func (a AnnouncementColor) String() string {
	return string(a)
}

func (a AnnouncementColor) RGBHex() string {
	switch a {
	case Primary:
		return "#EFEFF1"
	case Blue:
		return "#4E88EC"
	case Green:
		return "#1ED1A6"
	case Orange:
		return "#EAD008"
	case Purple:
		return "#C65DF3"
	}

	return ""
}

const (
	Primary AnnouncementColor = "PRIMARY"
	Blue    AnnouncementColor = "BLUE"
	Green   AnnouncementColor = "GREEN"
	Orange  AnnouncementColor = "ORANGE"
	Purple  AnnouncementColor = "PURPLE"
)

type SubMessage struct {
	UserNotice
	Message           string
	CumulativeMonths  int
	ShouldShareStreak bool
	StreakMonths      int
	SubPlan           SubPlan
	SubPlanName       string
}

type SubGiftMessage struct {
	UserNotice
	Months             int
	ReceiptDisplayName string
	RecipientID        string
	RecipientUserName  string
	SubPlan            SubPlan
	SubPlanName        string
	GiftMonths         int
}

type AnnouncementMessage struct {
	UserNotice
	ParamColor AnnouncementColor
	Message    string
}

type RaidMessage struct {
	UserNotice
	DisplayName string
	Login       string
	ViewerCount int
}

type AnonGiftPaidUpgradeMessage struct {
	UserNotice
	PromoGiftTotal int
	PromoName      string
}

type GiftPaidUpgradeMessage struct {
	UserNotice
	PromoGiftTotal int
	PromoName      string
	SenderLogin    string
	SenderName     string
}

type RitualMessage struct {
	UserNotice
	RitualName string
	Message    string
}

type UserState struct {
	BadgeInfo       []Badge
	Badges          []Badge
	Color           string
	DisplayName     string
	ChannelUserName string
	EmoteSets       []string
	ID              string
	Subscriber      bool
	Turbo           bool
	UserType        UserType
}

func (u *UserState) IRC() string {
	return ""
}

type Whisper struct {
	Badges      []Badge
	Color       string
	DisplayName string
	Emotes      []Emote
	ID          string
	ThreadID    string
	Turbo       bool
	UserID      string
	UserType    UserType
	Message     string
}

func (w *Whisper) IRC() string {
	return ""
}

type Notice struct {
	ChannelUserName string
	Message         string
	MsgID           MsgID

	FakeTimestamp time.Time
}

func (n *Notice) IRC() string {
	return ""
}

// Only the changed values are set
// RoomState does not represent the final state
type RoomState struct {
	EmoteOnly       *bool
	FollowersOnly   *int
	R9K             *bool
	RoomID          string
	ChannelUserName string
	Slow            *int
	SubsOnly        *bool
}

func (r *RoomState) IRC() string {
	return ""
}

type ClearChat struct {
	BanDuration     *int // in seconds
	RoomID          string
	ChannelUserName string
	TargetUserID    *string
	TMISentTS       time.Time
	UserName        *string
}

func (c *ClearChat) IRC() string {
	return ""
}

type ClearMessage struct {
	Login           string
	RoomID          string
	ChannelUserName string
	TargetMsgID     string
	TMISentTS       time.Time
}

func (c *ClearMessage) IRC() string {
	return ""
}
