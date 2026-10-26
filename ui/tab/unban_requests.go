package tab

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/eventsub"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
	"github.com/julez-dev/reflow/wordwrap"
)

const (
	unbanResolutionLimit = 500
	unbanRequestTextRows = 4
)

type openUnbanRequestsMessage struct{}

type unbanRequestsLoadedMessage struct {
	owner    string
	requests []twitchapi.UnbanRequest
	err      error
}

type unbanRequestResolvedMessage struct {
	owner     string
	requestID string
	status    string
	request   twitchapi.UnbanRequest
	err       error
}

// unbanRequestQueue is the moderator-facing queue of pending unban requests. It
// owns loading and resolving because both are scoped to one tab; the Broadcast
// only decides when the queue is visible and which keys reach it.
type unbanRequestQueue struct {
	id string

	width, height int
	theme         save.Theme
	keymap        save.KeyMap

	api           ModerationAPI
	broadcasterID string
	moderatorID   string

	requests []twitchapi.UnbanRequest
	selected int
	offset   int

	loading    bool
	submitting bool
	err        error
	notice     string

	// resolveStatus is empty while browsing. "approved" or "denied" turns the
	// view into the optional-resolution input for the selected request.
	resolveStatus string
	resolveID     string
	input         textinput.Model
}

func newUnbanRequestQueue(
	id string,
	broadcasterID string,
	moderatorID string,
	api ModerationAPI,
	width, height int,
	theme save.Theme,
	keymap save.KeyMap,
) *unbanRequestQueue {
	ti := textinput.New()
	ti.CharLimit = unbanResolutionLimit
	ti.Placeholder = "optional resolution text"
	ti.SetWidth(max(0, width-4))

	return &unbanRequestQueue{
		id:            id,
		width:         width,
		height:        height,
		theme:         theme,
		keymap:        keymap,
		api:           api,
		broadcasterID: broadcasterID,
		moderatorID:   moderatorID,
		loading:       true,
		input:         ti,
	}
}

func (q *unbanRequestQueue) Init() tea.Cmd {
	return q.refreshCmd()
}

func (q *unbanRequestQueue) Resize(width, height int) {
	q.width = width
	q.height = height
	q.input.SetWidth(max(0, width-4))
}

func (q *unbanRequestQueue) Focus() tea.Cmd {
	if q.resolveStatus == "" {
		return nil
	}

	return q.input.Focus()
}

func (q *unbanRequestQueue) Blur() {
	q.input.Blur()
}

// Resolving reports whether the queue is asking for resolution text. Escape
// cancels that step before it closes the queue.
func (q *unbanRequestQueue) Resolving() bool {
	return q.resolveStatus != ""
}

func (q *unbanRequestQueue) fetchCmd() tea.Cmd {
	id := q.id
	api := q.api
	broadcasterID, moderatorID := q.broadcasterID, q.moderatorID

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		requests, err := api.FetchUnbanRequests(ctx, broadcasterID, moderatorID)
		if err == nil {
			pending := make([]twitchapi.UnbanRequest, 0, len(requests))
			for _, request := range requests {
				if request.Status == "pending" {
					pending = append(pending, request)
				}
			}
			requests = pending
		}

		return unbanRequestsLoadedMessage{owner: id, requests: requests, err: err}
	}
}

func (q *unbanRequestQueue) refreshCmd() tea.Cmd {
	q.loading = true
	q.err = nil
	q.notice = ""

	return q.fetchCmd()
}

func (q *unbanRequestQueue) Update(msg tea.Msg) (*unbanRequestQueue, tea.Cmd) {
	switch msg := msg.(type) {
	case unbanRequestsLoadedMessage:
		if msg.owner != q.id {
			return q, nil
		}

		q.loading = false
		if msg.err != nil {
			// Keep the last good queue: a failed refresh must not erase what the
			// moderator was working on.
			q.err = fmt.Errorf("could not load unban requests: %w", msg.err)
			return q, nil
		}

		q.err = nil
		q.setRequests(msg.requests)
		return q, nil
	case unbanRequestResolvedMessage:
		if msg.owner != q.id {
			return q, nil
		}

		q.submitting = false
		if msg.err != nil {
			q.err = fmt.Errorf("could not resolve unban request: %w", msg.err)
			return q, nil
		}

		q.err = nil
		q.notice = fmt.Sprintf("Request from %s was %s", preferredDisplayName(msg.request.UserName, msg.request.UserLogin), msg.status)
		q.removeRequest(msg.requestID)
		q.cancelResolution()
		return q, nil
	case tea.KeyPressMsg:
		return q.updateKey(msg)
	case tea.PasteMsg:
		if q.Resolving() {
			var cmd tea.Cmd
			q.input, cmd = q.input.Update(msg)
			return q, cmd
		}
	}

	return q, nil
}

func (q *unbanRequestQueue) updateKey(msg tea.KeyPressMsg) (*unbanRequestQueue, tea.Cmd) {
	if q.Resolving() {
		switch {
		case key.Matches(msg, q.keymap.Escape):
			q.cancelResolution()
			return q, nil
		case key.Matches(msg, q.keymap.Confirm):
			if q.submitting {
				return q, nil
			}
			return q, q.resolveCmd()
		}

		var cmd tea.Cmd
		q.input, cmd = q.input.Update(msg)
		return q, cmd
	}

	switch {
	case key.Matches(msg, q.keymap.Up):
		q.move(-1)
	case key.Matches(msg, q.keymap.Down):
		q.move(1)
	case msg.Text == "a":
		q.startResolution("approved")
	case msg.Text == "d":
		q.startResolution("denied")
	case msg.Text == "r":
		return q, q.refreshCmd()
	}

	return q, nil
}

func (q *unbanRequestQueue) move(delta int) {
	if len(q.requests) == 0 {
		q.selected = 0
		return
	}

	q.selected = clamp(q.selected+delta, 0, len(q.requests)-1)
}

func (q *unbanRequestQueue) startResolution(status string) {
	if len(q.requests) == 0 || q.selected < 0 || q.selected >= len(q.requests) {
		return
	}

	q.resolveStatus = status
	q.resolveID = q.requests[q.selected].ID
	q.err = nil
	q.notice = ""
	q.input.Reset()
	q.input.Focus()
}

func (q *unbanRequestQueue) cancelResolution() {
	q.resolveStatus = ""
	q.resolveID = ""
	q.submitting = false
	q.input.Reset()
	q.input.Blur()
}

func (q *unbanRequestQueue) resolveCmd() tea.Cmd {
	request, ok := q.requestByID(q.resolveID)
	if !ok {
		q.cancelResolution()
		return nil
	}

	id := q.id
	api := q.api
	broadcasterID, moderatorID := q.broadcasterID, q.moderatorID
	status, resolutionText := q.resolveStatus, q.input.Value()

	q.submitting = true

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resolved, err := api.ResolveBanRequest(ctx, broadcasterID, moderatorID, request.ID, status, resolutionText)
		return unbanRequestResolvedMessage{
			owner:     id,
			requestID: request.ID,
			status:    status,
			request:   resolved,
			err:       err,
		}
	}
}

func (q *unbanRequestQueue) setRequests(requests []twitchapi.UnbanRequest) {
	q.requests = append(q.requests[:0], requests...)
	sort.SliceStable(q.requests, func(i, j int) bool {
		return q.requests[i].CreatedAt.After(q.requests[j].CreatedAt)
	})

	if len(q.requests) == 0 {
		q.selected = 0
		q.offset = 0
		return
	}

	q.selected = clamp(q.selected, 0, len(q.requests)-1)
	q.restoreResolutionSelection()
}

func (q *unbanRequestQueue) restoreResolutionSelection() {
	if !q.Resolving() {
		return
	}

	for i, request := range q.requests {
		if request.ID == q.resolveID {
			q.selected = i
			return
		}
	}

	// The selected request disappeared while its resolution input was open.
	q.cancelResolution()
}

func (q *unbanRequestQueue) requestByID(id string) (twitchapi.UnbanRequest, bool) {
	for _, request := range q.requests {
		if request.ID == id {
			return request, true
		}
	}

	return twitchapi.UnbanRequest{}, false
}

func (q *unbanRequestQueue) removeRequest(id string) {
	for i, request := range q.requests {
		if request.ID != id {
			continue
		}

		q.requests = append(q.requests[:i], q.requests[i+1:]...)
		if len(q.requests) == 0 {
			q.selected = 0
			q.offset = 0
		} else {
			q.selected = clamp(q.selected, 0, len(q.requests)-1)
		}
		return
	}
}

// applyEvent folds a live EventSub notification into the queue. Creation events
// add the request, resolve events remove it from the pending list.
func (q *unbanRequestQueue) applyEvent(msg eventsub.Message[eventsub.NotificationPayload]) {
	event := msg.Payload.Event
	switch msg.Payload.Subscription.Type {
	case "channel.unban_request.create":
		for _, request := range q.requests {
			if request.ID == event.ID {
				return
			}
		}

		q.requests = append(q.requests, twitchapi.UnbanRequest{
			ID:              event.ID,
			BroadcasterID:   event.BroadcasterUserID,
			BroadcasterName: event.BroadcasterUserName,
			UserID:          event.UserID,
			UserLogin:       event.UserLogin,
			UserName:        event.UserName,
			Text:            event.Text,
			Status:          "pending",
			CreatedAt:       event.CreatedAt,
		})
		q.setRequests(q.requests)
		q.notice = fmt.Sprintf("New unban request from %s", preferredDisplayName(event.UserName, event.UserLogin))
	case "channel.unban_request.resolve":
		if q.resolveID == event.ID {
			q.cancelResolution()
		}
		q.removeRequest(event.ID)
		q.notice = fmt.Sprintf("Unban request %s was %s", event.ID, event.Status)
	}
}

func (q *unbanRequestQueue) View() string {
	if q.height <= 0 {
		return ""
	}

	if q.Resolving() {
		if len(q.requests) == 0 || q.selected >= len(q.requests) {
			q.cancelResolution()
			return q.listView()
		}
		return q.resolutionView()
	}

	return q.listView()
}

func (q *unbanRequestQueue) listView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ListLabelColor))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ListSelectedColor))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.DimmedTextColor))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.ChatErrorColor))

	header := []string{titleStyle.Render(fmt.Sprintf("Unban requests (%d pending)", len(q.requests)))}
	switch {
	case q.err != nil:
		header = append(header, errorStyle.Render(q.err.Error()))
	case q.notice != "":
		header = append(header, q.notice)
	case q.loading:
		header = append(header, dimStyle.Render("Refreshing..."))
	default:
		header = append(header, dimStyle.Render("Pending requests for this channel"))
	}

	footer := []string{dimStyle.Render("j/k select · a approve · d deny · r refresh · esc close")}
	if len(q.requests) == 0 && !q.loading && q.err == nil {
		header = append(header, "", dimStyle.Render("No pending unban requests."))
	}

	bodyHeight := max(0, q.height-len(header)-len(footer))
	lines, starts := q.listLines(selectedStyle, dimStyle)
	q.scrollToSelection(starts, lines, bodyHeight)

	start := min(q.offset, len(lines))
	end := min(start+bodyHeight, len(lines))

	out := append([]string{}, header...)
	out = append(out, lines[start:end]...)
	out = append(out, footer...)

	return fitLines(out, q.width, q.height)
}

func (q *unbanRequestQueue) resolutionView() string {
	request, ok := q.requestByID(q.resolveID)
	if !ok {
		q.cancelResolution()
		return q.listView()
	}
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ListLabelColor))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.DimmedTextColor))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.ChatErrorColor))

	action := "Approve"
	if q.resolveStatus == "denied" {
		action = "Deny"
	}

	out := []string{
		titleStyle.Render(fmt.Sprintf("%s unban request from %s", action, preferredDisplayName(request.UserName, request.UserLogin))),
	}

	text := request.Text
	if strings.TrimSpace(text) == "" {
		text = "(no request text)"
	}

	bodyWidth := max(1, q.width-4)
	for _, line := range strings.Split(wordwrap.String(text, bodyWidth), "\n") {
		out = append(out, "  "+line)
	}

	out = append(out, "", "Resolution text (optional):", q.input.View())
	if q.err != nil {
		out = append(out, errorStyle.Render(q.err.Error()))
	}

	help := "enter confirm · esc cancel"
	if q.submitting {
		help = "Resolving..."
	}
	out = append(out, dimStyle.Render(help))

	return fitLines(out, q.width, q.height)
}

func (q *unbanRequestQueue) listLines(selectedStyle, dimStyle lipgloss.Style) ([]string, []int) {
	lines := make([]string, 0, len(q.requests)*3)
	starts := make([]int, 0, len(q.requests))
	bodyWidth := max(1, q.width-4)

	for i, request := range q.requests {
		starts = append(starts, len(lines))

		prefix := "  "
		name := preferredDisplayName(request.UserName, request.UserLogin)
		line := fmt.Sprintf("%s - %s - %s", name, request.Status, unbanRequestAge(request.CreatedAt))
		if i == q.selected {
			prefix = "> "
			line = selectedStyle.Render(line)
		}
		lines = append(lines, prefix+line)

		text := request.Text
		if strings.TrimSpace(text) == "" {
			text = "(no request text)"
		}

		wrapped := strings.Split(wordwrap.String(text, bodyWidth), "\n")
		if len(wrapped) > unbanRequestTextRows {
			wrapped = append(wrapped[:unbanRequestTextRows-1], "…")
		}
		for _, textLine := range wrapped {
			lines = append(lines, "    "+dimStyle.Render(textLine))
		}

		if i != len(q.requests)-1 {
			lines = append(lines, "")
		}
	}

	return lines, starts
}

func (q *unbanRequestQueue) scrollToSelection(starts []int, lines []string, bodyHeight int) {
	if len(starts) == 0 || bodyHeight <= 0 {
		q.offset = 0
		return
	}

	start := starts[q.selected]
	end := len(lines)
	if q.selected+1 < len(starts) {
		end = starts[q.selected+1]
	}

	if start < q.offset {
		q.offset = start
	}
	if end > q.offset+bodyHeight {
		q.offset = end - bodyHeight
	}

	q.offset = clamp(q.offset, 0, max(0, len(lines)-bodyHeight))
}

func unbanRequestAge(created time.Time) string {
	if created.IsZero() {
		return "unknown time"
	}

	age := time.Since(created)
	if age < 0 {
		age = 0
	}

	return humanizeDuration(age) + " ago"
}

func fitLines(lines []string, width, height int) string {
	if height <= 0 {
		return ""
	}

	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	if width > 0 {
		for i := range lines {
			lines[i] = lipgloss.NewStyle().MaxWidth(width).Render(lines[i])
		}
	}

	return strings.Join(lines, "\n")
}
