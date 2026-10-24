// Package tab implements the tabs a chatuino session is made of.
//
// A tab owns everything behind one header entry: its own model, its own children
// (chat window, composer, modals), its own state machine and the messages it
// exchanges with the shell. The shell only knows this interface plus the concrete
// constructors (NewBroadcast, NewMention, NewLiveNotification) it calls when a tab
// is opened.
package tab

import (
	tea "charm.land/bubbletea/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
)

// Tab is what the shell needs from a tab: lifecycle, layout, identity, and the few
// questions Root asks while deciding what an input event means.
type Tab interface {
	Init() tea.Cmd
	InitWithUserData(twitchapi.UserData) tea.Cmd
	Update(tea.Msg) (Tab, tea.Cmd)
	View() string
	ViewWithoutStatusBar() string // body-only rendering, with Root's shared status line
	StatusBarView() string        // status at full terminal width
	Focus()
	Blur()
	Close() // release whatever the tab started; safe on tabs that own nothing
	AccountID() string
	Channel() string
	IsComposing() bool // a text input inside the tab is capturing printable keys
	IsSearching() bool
	IsDataLoaded() bool
	ID() string
	Focused() bool
	ChannelID() string
	HandleResize()
	SetSize(width, height int)
	SetFullWidth(width int) // nonzero when Root renders the status outside this tab
	Kind() TabKind
	Title() string    // header label, e.g. the channel name
	Identity() string // account label shown next to the title
}

// TabKind is the set of tab implementations a session can open.
type TabKind int

const (
	BroadcastKind TabKind = iota
	MentionKind
	LiveNotificationKind
)

// Caption is the common source of tab/header labels. The shell can measure a
// prospective tab before constructing it and starting any owned resources.
func Caption(kind TabKind, account save.Account, channel string) (title, identity string) {
	switch kind {
	case BroadcastKind:
		if account.IsAnonymous {
			return channel, "Anonymous"
		}
		return channel, account.DisplayName
	case MentionKind:
		return "mentioned", "all"
	case LiveNotificationKind:
		return "live notifications", "all"
	default:
		return "", ""
	}
}

func (t TabKind) String() string {
	switch t {
	case BroadcastKind:
		return "Channel (Default)"
	case MentionKind:
		return "Mention"
	case LiveNotificationKind:
		return "Live Notifications"
	}

	return "<not implemented>"
}
