package mainui

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
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

const (
	// linkCheckTimeout bounds one link check. A target that does not answer in time
	// loses its annotation for this message; the next one retries.
	linkCheckTimeout = 2 * time.Second

	// linkCheckBudget bounds all link checks of one message together, so a message
	// full of slow links cannot hold up the pipeline either.
	linkCheckBudget = 4 * time.Second

	// linkCheckTTL is how long a successful annotation is reused. Chat repeats the
	// same links (a streamer's socials, the clip of the moment), and every repeat
	// would otherwise cost a round-trip through the chatuino server.
	linkCheckTTL = 10 * time.Minute

	// linkAnnotationMaxEntries bounds the cache. Annotations are a latency
	// optimisation, not state, so the least recently used entries are dropped when
	// it grows past this.
	linkAnnotationMaxEntries = 1024
)

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
	links         *linkAnnotations
	linkTimeout   time.Duration
	linkBudget    time.Duration
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
		links:         newLinkAnnotations(linkCheckTTL),
		linkTimeout:   linkCheckTimeout,
		linkBudget:    linkCheckBudget,
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
	p.links.stop()
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

	// Restored history is prepared in bulk at startup and repeats the same URLs, so
	// checking it costs a round-trip per link without telling the user anything new
	// about live chat. Live messages are still annotated, from the cache when the
	// URL was seen before.
	if !req.isFake && p.settings.Security.CheckLinks && len(message) > 0 && p.server != nil {
		p.annotateLinks(ctx, &event, message)
	}

	event.PrepareCommand = replaceCommand
	return event
}

// annotateLinks renders the "[status, content type, redirects]" annotation after
// every URL of a live message.
func (p *chatEventPreparer) annotateLinks(ctx context.Context, event *tab.ChatEventMessage, message string) {
	ctx, cancel := context.WithTimeout(ctx, p.linkBudget)
	defer cancel()

	// Key link annotations on the whole space-delimited token so they match
	// exactly in applyWordReplacements (same contract as emotes). The URL is
	// annotated in place, preserving any surrounding punctuation in the token.
	for _, token := range strings.Split(message, " ") {
		annotated := token
		for _, u := range tab.ExtractValidURLs(token) {
			annotation, ok := p.links.get(u)
			if !ok {
				annotation, ok = p.checkLink(ctx, u)
				if !ok {
					continue
				}
			}

			annotated = strings.Replace(annotated, u, fmt.Sprintf("%s [%s]", u, annotation), 1)
		}

		if annotated != token {
			event.DisplayModifier.WordReplacements[token] = annotated
		}
	}
}

// checkLink asks the server about one URL and caches the annotation it produces. A
// check that fails or runs out of time stays uncached, so a later message retries
// it instead of showing a stale verdict.
func (p *chatEventPreparer) checkLink(ctx context.Context, target string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, p.linkTimeout)
	defer cancel()

	resp, err := p.server.CheckLink(ctx, target)
	if err != nil {
		log.Logger.Info().Err(err).Str("url", target).Msg("failed to check link")
		return "", false
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

	annotation := strings.Join(parts, ", ")
	p.links.put(target, annotation)

	return annotation, true
}

// linkAnnotations caches the annotation chat renders after a URL, keyed by the URL
// a message contains. Chat repeats the same links (a streamer's socials, the clip of
// the moment), and every repeat would otherwise cost a round-trip through the
// chatuino server.
type linkAnnotations struct {
	entries *ttlcache.Cache[string, string]
}

func newLinkAnnotations(ttl time.Duration) *linkAnnotations {
	entries := ttlcache.New(
		ttlcache.WithTTL[string, string](ttl),
		ttlcache.WithCapacity[string, string](linkAnnotationMaxEntries),
		// The TTL counts from when the annotation was stored: a cache hit must not
		// keep a stale verdict alive just because the link stays popular.
		ttlcache.WithDisableTouchOnHit[string, string](),
	)

	go entries.Start()

	return &linkAnnotations{entries: entries}
}

// stop ends the cache's expiry janitor. It is safe to call more than once.
func (c *linkAnnotations) stop() { c.entries.Stop() }

// get returns the cached annotation of a URL when one was stored and is fresh.
func (c *linkAnnotations) get(target string) (string, bool) {
	item := c.entries.Get(target)
	if item == nil {
		return "", false
	}

	return item.Value(), true
}

func (c *linkAnnotations) put(target, text string) {
	c.entries.Set(target, text, ttlcache.DefaultTTL)
}

// eventQueueCapacity bounds the enrichment backlog. Restoring a session queues a
// history batch per tab in one burst, so the bound has to cover a workspace full
// of tabs on top of live traffic. When the backlog still grows past it, history is
// evicted before live chat is dropped.
const eventQueueCapacity = 4096

// eventQueue serializes chat-event preparation for Root.
//
// Root adds live and tab-local history events in Update arrival order; a history
// batch is inserted contiguously. At most one preparation command runs at a time.
// A result must claim its handoff before terminal side effects, then finish that
// same request to route content. Duplicate/stale handoffs are ignored.
//
// The queue is bounded. When it is full (chat is arriving faster than enrichment
// completes), the oldest queued history event is evicted to make room, because
// restored history is cosmetic while live chat is not. An event is only dropped
// once no history is left to evict; previously queued and in-flight events keep
// their order either way.
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

// add appends a request in arrival order. It reports false only when the queue is
// full and holds nothing evictable.
func (q *eventQueue) add(accountID, tabID string, ircer twitchirc.IRCer, isFake bool) (prepareRequest, bool) {
	if len(q.pending) >= q.capacity && !q.evictOldestHistory() {
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

// evictOldestHistory removes the oldest queued history event and reports whether
// it found one. Dropping the oldest keeps what remains in arrival order.
func (q *eventQueue) evictOldestHistory() bool {
	for i := range q.pending {
		if !q.pending[i].isFake {
			continue
		}

		q.pending = slices.Delete(q.pending, i, i+1)

		return true
	}

	return false
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
