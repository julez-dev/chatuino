package mainui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/rs/zerolog/log"
)

// preparationTimeout bounds one chat event's enrichment. The queue processes one
// event at a time, so a stuck link check must not stall live chat forever.
const preparationTimeout = 15 * time.Second

// preparationServer is the part of the chatuino server the preparer uses.
type preparationServer interface {
	GetUsers(ctx context.Context, logins []string, ids []string) (twitchapi.UserResponse, error)
	CheckLink(ctx context.Context, targetURL string) (server.CheckLinkResponse, error)
}

// preparationEmotes is the emote-cache surface used for shared-chat guests.
type preparationEmotes interface {
	RefreshLocal(ctx context.Context, channelID string) error
}

// preparationBadges is the badge-cache surface used for shared-chat guests.
type preparationBadges interface {
	RefreshChannel(ctx context.Context, broadcasterID string) error
}

// prepareRequest is one chat event waiting to be enriched. All inputs are
// captured when the event arrives; preparation never reads Root's model.
type prepareRequest struct {
	seq       uint64
	accountID string
	tabID     string
	ircer     twitchirc.IRCer
	isFake    bool
}

// preparedChatEvent is the result of one preparation, tagged with the request it
// belongs to so a stale result cannot be routed as if it were current.
type preparedChatEvent struct {
	seq   uint64
	event tab.ChatEventMessage
}

// routedPreparedEvent continues a prepared event after its terminal preparation
// command was written.
type routedPreparedEvent struct {
	seq   uint64
	event tab.ChatEventMessage
}

// chatEventPreparer owns the enrichment rules and their collaborators: protocol
// projection, shared-chat guest resolution, emote/badge replacement, contributor
// badges and link checking. It does not know about tabs or the registry.
type chatEventPreparer struct {
	ctx    context.Context
	cancel context.CancelFunc

	// Close and command startup share this gate: no new work can be added
	// after shutdown starts, including commands scheduled but not yet run.
	mu     sync.Mutex
	closed bool
	work   sync.WaitGroup

	settings      save.Settings
	emoteReplacer tab.EmoteReplacer
	badgeReplacer tab.BadgeReplacer
	emoteCache    preparationEmotes
	badgeCache    preparationBadges
	server        preparationServer
	userNames     *sync.Map
	timeout       time.Duration
}

func newChatEventPreparer(ctx context.Context, root *Root) *chatEventPreparer {
	ctx, cancel := context.WithCancel(ctx)
	return &chatEventPreparer{
		ctx:           ctx,
		cancel:        cancel,
		settings:      root.settings,
		emoteReplacer: root.emoteReplacer,
		badgeReplacer: root.badgeReplacer,
		emoteCache:    root.emoteCache,
		badgeCache:    root.badgeCache,
		server:        root.serverAPI,
		userNames:     root.userIDDisplayName,
		timeout:       preparationTimeout,
	}
}

// close cancels in-flight enrichment and waits for it to unwind. It is safe to
// call more than once, or before a previously scheduled command starts running.
func (p *chatEventPreparer) close() {
	p.mu.Lock()
	p.closed = true
	p.cancel()
	p.mu.Unlock()
	p.work.Wait()
}

// prepare enriches one event. Network failures are logged and the event is still
// produced, matching the previous best-effort behaviour. All I/O shares one
// per-event deadline, derived from the UI lifetime.
func (p *chatEventPreparer) prepare(req prepareRequest) tab.ChatEventMessage {
	p.mu.Lock()
	if p.closed || p.ctx.Err() != nil {
		p.mu.Unlock()
		return tab.ChatEventMessage{}
	}
	p.work.Add(1)
	p.mu.Unlock()
	defer p.work.Done()

	ctx, cancel := context.WithTimeout(p.ctx, p.timeout)
	defer cancel()

	var (
		channel                 string
		message                 string
		badges                  []twitchirc.Badge
		emotes                  []twitchirc.Emote
		emoteSourceRoom         string
		channelID               string
		channelGuestID          string
		channelGuestDisplayName string
		loginName               string
	)

	// Check when currently in shared session.
	// If so then load emotes and badges for guest so the message content can be replaced
	if msg, ok := req.ircer.(*twitchirc.PrivateMessage); ok {
		channelID = msg.RoomID
		channelGuestID = msg.SourceRoomID

		// shared chat, get display name of guest stream chat, channelGuestID will be empty when not shared chat
		if channelID != "" && channelGuestID != "" {
			if v, ok := p.userNames.Load(channelGuestID); ok {
				channelGuestDisplayName = v.(string)
			} else {
				// try refresh emotes for guest
				if p.emoteCache != nil {
					_ = p.emoteCache.RefreshLocal(ctx, channelGuestID)
				}

				if p.badgeCache != nil {
					_ = p.badgeCache.RefreshChannel(ctx, channelGuestID)
				}

				if p.server != nil {
					resp, err := p.server.GetUsers(ctx, nil, []string{channelGuestID})
					if err == nil && len(resp.Data) > 0 {
						channelGuestDisplayName = resp.Data[0].DisplayName
						p.userNames.Store(channelGuestID, channelGuestDisplayName)
					}
				}
			}
		}
	}

	switch ircMessage := req.ircer.(type) {
	case *twitchirc.PrivateMessage:
		channelID = ircMessage.RoomID
		channelGuestID = ircMessage.SourceRoomID
		channel = ircMessage.ChannelUserName
		message = ircMessage.Message
		loginName = ircMessage.LoginName

		// if is shared display emotes from guest channel, when message is from guest
		emoteSourceRoom = channelID
		badges = ircMessage.Badges
		emotes = ircMessage.Emotes

		if channelGuestID != "" && channelID != channelGuestID {
			emoteSourceRoom = channelGuestID
			badges = ircMessage.SourceBadges
		}

	case *twitchirc.RoomState:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
	case *twitchirc.UserNotice:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.UserState:
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.ClearChat:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
	case *twitchirc.ClearMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
	case *twitchirc.SubMessage:
		channelID = ircMessage.RoomID
		emoteSourceRoom = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		message = ircMessage.Message
		emotes = ircMessage.Emotes
		badges = ircMessage.Badges
	case *twitchirc.RaidMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.SubGiftMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		badges = ircMessage.Badges
	case *twitchirc.RitualMessage:
		channelID = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		emoteSourceRoom = ircMessage.RoomID
		message = ircMessage.Message
		emotes = ircMessage.Emotes
		badges = ircMessage.Badges
	case *twitchirc.AnnouncementMessage:
		channelID = ircMessage.RoomID
		emoteSourceRoom = ircMessage.RoomID
		channel = ircMessage.ChannelUserName
		message = ircMessage.Message
		emotes = ircMessage.Emotes
		badges = ircMessage.Badges
	case *twitchirc.Notice:
		channel = ircMessage.ChannelUserName
	}

	event := tab.ChatEventMessage{
		IsFakeEvent:             req.isFake,
		AccountID:               req.accountID,
		Channel:                 channel,
		ChannelID:               channelID,
		ChannelGuestID:          channelGuestID,
		ChannelGuestDisplayName: channelGuestDisplayName,
		TabID:                   req.tabID,

		Message: req.ircer,
		DisplayModifier: tab.MessageContentModifier{
			WordReplacements: make(tab.WordReplacement),
			BadgeReplacement: make(tab.WordReplacement),
		},
	}

	var replaceCommand string

	if len(message) > 0 && p.emoteReplacer != nil {
		p, replacement, err := p.emoteReplacer.Replace(ctx, emoteSourceRoom, message, emotes)
		if err != nil {
			log.Logger.Info().Err(err).Str("message", message).Msg("failed to replace emotes")
		}

		for k, v := range replacement {
			event.DisplayModifier.WordReplacements[k] = v
		}

		replaceCommand += p
	}

	if len(badges) > 0 && p.badgeReplacer != nil {
		p, replace, err := p.badgeReplacer.Replace(ctx, emoteSourceRoom, badges)
		if err != nil {
			log.Logger.Info().Err(err).Str("message", message).Msg("failed to replace badges")
		}

		if replace != nil {
			event.DisplayModifier.BadgeReplacement = replace
		}
		replaceCommand += p
	}

	if loginName != "" && p.badgeReplacer != nil {
		p, err := p.badgeReplacer.InjectContributorBadge(ctx, loginName, event.DisplayModifier.BadgeReplacement)
		if err != nil {
			log.Logger.Info().Err(err).Str("login", loginName).Msg("failed to inject contributor badge")
		}
		replaceCommand += p
	}

	if p.settings.Security.CheckLinks && len(message) > 0 && p.server != nil {
		// Key link annotations on the whole space-delimited token so they match
		// exactly in applyWordReplacements (same contract as emotes). The URL is
		// annotated in place, preserving any surrounding punctuation in the token.
		for _, token := range strings.Split(message, " ") {
			annotated := token
			for _, u := range tab.ExtractValidURLs(token) {
				resp, err := p.server.CheckLink(ctx, u)
				if err != nil {
					log.Logger.Info().Err(err).Str("url", u).Msg("failed to check link")
					continue
				}

				parts := []string{http.StatusText(resp.RemoteStatusCode)}

				if resp.RemoteContentType != "" {
					before, _, _ := strings.Cut(resp.RemoteContentType, ";")
					parts = append(parts, before)
				}

				for _, visited := range resp.VisitedURLs {
					d, err := url.QueryUnescape(visited)
					if err != nil {
						log.Logger.Info().Err(err).Str("url", visited).Msg("failed to unescape url")
						continue
					}
					parts = append(parts, d)
				}

				annotated = strings.Replace(annotated, u, fmt.Sprintf("%s [%s]", u, strings.Join(parts, ", ")), 1)
			}

			if annotated != token {
				event.DisplayModifier.WordReplacements[token] = annotated
			}
		}
	}

	event.PrepareCommand = replaceCommand
	return event
}

// eventQueue serializes chat-event preparation for Root.
//
// Root adds live and tab-local history events in Update arrival order; a history
// batch is inserted contiguously. At most one preparation command runs at a time.
// A result must claim its handoff before terminal side effects, then finish that
// same request to route content. Duplicate/stale handoffs are ignored.
//
// The queue is bounded. When it is full (chat is arriving faster than enrichment
// completes), the new event is dropped and logged instead of growing memory
// without bound; previously queued and in-flight events keep their order.
type eventQueue struct {
	capacity int
	inFlight uint64
	released bool // the in-flight result has claimed its terminal/routing handoff
	nextSeq  uint64
	pending  []prepareRequest
}

func newEventQueue(capacity int) eventQueue {
	return eventQueue{capacity: capacity, pending: make([]prepareRequest, 0, capacity)}
}

// add appends a request in arrival order. It reports false when the queue is full.
func (q *eventQueue) add(accountID, tabID string, ircer twitchirc.IRCer, isFake bool) (prepareRequest, bool) {
	if len(q.pending) >= q.capacity {
		return prepareRequest{}, false
	}

	q.nextSeq++
	req := prepareRequest{
		seq:       q.nextSeq,
		accountID: accountID,
		tabID:     tabID,
		ircer:     ircer,
		isFake:    isFake,
	}

	q.pending = append(q.pending, req)

	return req, true
}

// start pops the next request when no preparation is running.
func (q *eventQueue) start() (prepareRequest, bool) {
	if q.inFlight != 0 || len(q.pending) == 0 {
		return prepareRequest{}, false
	}

	req := q.pending[0]
	q.pending[0] = prepareRequest{}
	q.pending = q.pending[1:]
	q.inFlight = req.seq
	q.released = false

	return req, true
}

// release claims the result exactly once, before any terminal side effects.
func (q *eventQueue) release(seq uint64) bool {
	if seq == 0 || seq != q.inFlight || q.released {
		return false
	}
	q.released = true
	return true
}

// finish releases the in-flight slot when seq belongs to the current request. A
// stale result leaves the queue untouched.
func (q *eventQueue) finish(seq uint64) bool {
	if seq == 0 || seq != q.inFlight {
		return false
	}

	q.inFlight = 0
	q.released = false

	return true
}

// len reports how many events are waiting; the in-flight event is not counted.
func (q *eventQueue) len() int { return len(q.pending) }
