package command

import (
	"fmt"
	"time"
)

type Badge struct {
	Name    string
	Version int
}

type Emote struct {
	ID    string
	Start int
	End   int
}

type PrivateMessage struct {
	ID             string
	ParentThreadID string
	ParentMsgID    string

	From      string
	In        string
	Message   string
	UserColor string
	SentAt    time.Time

	Badges []Badge
}

// socket.send(`PRIVMSG ${room} :${message}`);
func (p *PrivateMessage) IRC() string {
	if p.ParentMsgID != "" {
		return fmt.Sprintf("@reply-parent-msg-id=%s PRIVMSG #%s :%s", p.ParentMsgID, p.In, p.Message)
	}

	return fmt.Sprintf("PRIVMSG #%s :%s", p.In, p.Message)
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

type MsgID string

const (
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
)

type UserType string

const (
	Empty     UserType = "" // normal user
	Admin     UserType = "admin"
	GlobalMod UserType = "global_mod"
	Staff     UserType = "staff"
)

type UserNotice struct {
	BadgeInfo   []Badge
	Badges      []Badge
	Color       string
	DisplayName string
	Emotes      []Emote
	ID          string
	Login       string
	Mod         bool
	MsgID       MsgID
	RoomID      string
	Subscriber  bool
	SystemMsg   string
	TMISentTS   time.Time
	Turbo       bool
	UserID      string
	UserType    UserType
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
	Message string
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
