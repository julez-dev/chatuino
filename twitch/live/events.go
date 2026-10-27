package live

import (
	"time"

	"github.com/julez-dev/chatuino/twitch/twitchapi"
)

// Event is a typed channel-live event. Translate returns exactly one event per
// accepted EventSub notification. The unexported marker keeps the set closed.
type Event interface {
	isEvent()
}

// Poll is the state of one poll at the moment an event was sent.
type Poll struct {
	Title   string
	Choices []PollChoice
}

// PollChoice is one option in a poll.
type PollChoice struct {
	ID    string
	Title string
	Votes int
}

// Prediction is the state of one prediction at the moment an event was sent.
type Prediction struct {
	Title    string
	LocksAt  time.Time
	Outcomes []PredictionOutcome
}

// PredictionOutcome is one outcome in a prediction.
type PredictionOutcome struct {
	ID     string
	Title  string
	Points int
}

// Redemption is a channel point redemption, shared by add and update events.
type Redemption struct {
	ID          string
	User        string
	Reward      string
	Cost        int
	UserInput   string
	MessageText string
	Status      string
}

// UnbanRequest is an unban request as carried by a create or resolve event.
type UnbanRequest struct {
	ID              string
	BroadcasterID   string
	BroadcasterName string
	UserID          string
	UserLogin       string
	UserName        string
	Text            string
	Status          string
	ResolutionText  string
	ModeratorID     string
	ModeratorLogin  string
	ModeratorName   string
	CreatedAt       time.Time
}

// AutoMod is a held or updated AutoMod message.
type AutoMod struct {
	MessageID string
	User      string
	Reason    string
	Category  string
	Status    string
	Message   string
	Level     int
}

type PollStarted struct {
	Poll Poll
}

func (PollStarted) isEvent() {}

type PollProgress struct {
	Poll Poll
}

func (PollProgress) isEvent() {}

type PollEnded struct {
	Poll   Poll
	Winner *PollChoice
}

func (PollEnded) isEvent() {}

type PredictionStarted struct {
	Prediction Prediction
}

func (PredictionStarted) isEvent() {}

type PredictionProgress struct {
	Prediction Prediction
}

func (PredictionProgress) isEvent() {}

type PredictionLocked struct {
	Prediction Prediction
}

func (PredictionLocked) isEvent() {}

type PredictionEnded struct {
	Prediction       Prediction
	Status           string
	WinningOutcomeID string
}

func (PredictionEnded) isEvent() {}

type RedemptionAdded struct {
	Redemption Redemption
	Custom     bool
}

func (RedemptionAdded) isEvent() {}

type RedemptionUpdated struct {
	Redemption Redemption
}

func (RedemptionUpdated) isEvent() {}

type UnbanRequestCreated struct {
	Request UnbanRequest
}

func (UnbanRequestCreated) isEvent() {}

type UnbanRequestResolved struct {
	Request UnbanRequest
}

func (UnbanRequestResolved) isEvent() {}

// Raid carries both ends of a raid. The tab decides whether it is incoming or
// outgoing by comparing FromID with its own channel.
type Raid struct {
	FromID   string
	FromName string
	ToName   string
	Viewers  int
}

func (Raid) isEvent() {}

type ChatSettingsChanged struct {
	Settings twitchapi.ChatSettingData
}

func (ChatSettingsChanged) isEvent() {}

type WarningSent struct {
	Moderator string
	User      string
	Reason    string
}

func (WarningSent) isEvent() {}

type WarningAcknowledged struct {
	User string
}

func (WarningAcknowledged) isEvent() {}

type ShoutoutSent struct {
	Target  string
	Viewers int
}

func (ShoutoutSent) isEvent() {}

type ShoutoutReceived struct {
	From    string
	Viewers int
}

func (ShoutoutReceived) isEvent() {}

type AutoModHeld struct {
	AutoMod AutoMod
}

func (AutoModHeld) isEvent() {}

type AutoModUpdated struct {
	AutoMod AutoMod
}

func (AutoModUpdated) isEvent() {}

type AdBreakStarted struct {
	Duration  time.Duration
	Automatic bool
	Requester string
}

func (AdBreakStarted) isEvent() {}
