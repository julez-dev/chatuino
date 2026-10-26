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

var unbanRequestFilters = []string{"pending", "approved", "denied", "acknowledged", "canceled", "all"}

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

	requests []twitchapi.UnbanRequest // every fetched request, across all statuses
	visible  []twitchapi.UnbanRequest // requests matching filter
	filter   string
	filterAt int

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
		filter:        "pending",
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
		request, ok := q.requestByID(msg.requestID)
		if !ok {
			request = msg.request
		}
		request.ID = msg.requestID
		request.Status = msg.status
		if msg.request.ResolutionText != "" {
			request.ResolutionText = msg.request.ResolutionText
		}
		if !msg.request.ResolvedAt.IsZero() {
			request.ResolvedAt = msg.request.ResolvedAt
		}
		q.notice = fmt.Sprintf("Request from %s was %s", preferredDisplayName(request.UserName, request.UserLogin), msg.status)
		q.upsertRequest(request)
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
	case msg.Text == "h" || msg.Code == tea.KeyLeft:
		q.cycleFilter(-1)
	case msg.Text == "l" || msg.Code == tea.KeyRight:
		q.cycleFilter(1)
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
	if len(q.visible) == 0 {
		q.selected = 0
		return
	}

	q.selected = clamp(q.selected+delta, 0, len(q.visible)-1)
}

func (q *unbanRequestQueue) cycleFilter(delta int) {
	q.setFilter(unbanRequestFilters[(q.filterAt+delta+len(unbanRequestFilters))%len(unbanRequestFilters)])
}

func (q *unbanRequestQueue) setFilter(filter string) {
	q.filter = filter
	q.filterAt = max(0, slicesIndex(unbanRequestFilters, filter))
	q.selected = 0
	q.offset = 0
	q.rebuildVisible()
}

func (q *unbanRequestQueue) startResolution(status string) {
	request, ok := q.selectedRequest()
	if !ok {
		return
	}
	if request.Status != "pending" {
		q.notice = "Only pending requests can be approved or denied"
		return
	}

	q.resolveStatus = status
	q.resolveID = request.ID
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

	q.rebuildVisible()
}

func (q *unbanRequestQueue) rebuildVisible() {
	q.visible = q.visible[:0]
	for _, request := range q.requests {
		if q.filter == "all" || request.Status == q.filter {
			q.visible = append(q.visible, request)
		}
	}

	if len(q.visible) == 0 {
		q.selected = 0
		q.offset = 0
		return
	}

	q.selected = clamp(q.selected, 0, len(q.visible)-1)
	q.restoreResolutionSelection()
}

func (q *unbanRequestQueue) restoreResolutionSelection() {
	if !q.Resolving() {
		return
	}

	for i, request := range q.visible {
		if request.ID == q.resolveID {
			q.selected = i
			return
		}
	}

	// The selected request disappeared from this filter while its resolution
	// input was open, so there is nothing left to submit against.
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

func (q *unbanRequestQueue) selectedRequest() (twitchapi.UnbanRequest, bool) {
	if q.selected < 0 || q.selected >= len(q.visible) {
		return twitchapi.UnbanRequest{}, false
	}

	return q.visible[q.selected], true
}

func (q *unbanRequestQueue) upsertRequest(updated twitchapi.UnbanRequest) {
	for i, request := range q.requests {
		if request.ID != updated.ID {
			continue
		}

		q.requests[i] = updated
		q.rebuildVisible()
		return
	}

	q.requests = append(q.requests, updated)
	q.rebuildVisible()
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

		request, ok := q.requestByID(event.ID)
		if !ok {
			request = twitchapi.UnbanRequest{
				ID:              event.ID,
				BroadcasterID:   event.BroadcasterUserID,
				BroadcasterName: event.BroadcasterUserName,
				UserID:          event.UserID,
				UserLogin:       event.UserLogin,
				UserName:        event.UserName,
			}
		}
		request.Status = event.Status
		request.ResolutionText = event.ResolutionText
		request.ModeratorID = event.ModeratorID
		request.ModeratorLogin = event.ModeratorLogin
		request.ModeratorName = event.ModeratorName
		q.upsertRequest(request)
		if ok {
			q.notice = fmt.Sprintf("Unban request from %s was %s", preferredDisplayName(request.UserName, request.UserLogin), event.Status)
		} else {
			q.notice = fmt.Sprintf("Unban request %s was %s", event.ID, event.Status)
		}
	}
}

func (q *unbanRequestQueue) View() string {
	if q.height <= 0 {
		return ""
	}

	if q.Resolving() {
		if _, ok := q.selectedRequest(); !ok {
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
	activePill := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(q.theme.ListBackgroundColor)).
		Background(lipgloss.Color(q.theme.ListSelectedColor)).
		Padding(0, 1)
	inactivePill := lipgloss.NewStyle().
		Foreground(lipgloss.Color(q.theme.DimmedTextColor)).
		Padding(0, 1)

	header := []string{
		titleStyle.Render("Unban Requests") + dimStyle.Render(fmt.Sprintf("  %d total", len(q.requests))),
		q.filterLine(activePill, inactivePill),
		dimStyle.Render(strings.Repeat("─", max(1, q.width))),
	}
	switch {
	case q.err != nil:
		header = append(header, errorStyle.Render(q.err.Error()))
	case q.notice != "":
		header = append(header, q.notice)
	case q.loading:
		header = append(header, dimStyle.Render("Refreshing..."))
	case len(q.visible) == 0:
		if q.filter == "all" {
			header = append(header, dimStyle.Render("No unban requests."))
		} else {
			header = append(header, dimStyle.Render(fmt.Sprintf("No %s unban requests.", strings.ToLower(unbanStatusLabel(q.filter)))))
		}
	default:
		label := strings.ToLower(unbanStatusLabel(q.filter))
		noun := "requests"
		if len(q.visible) == 1 {
			noun = "request"
		}
		if q.filter == "all" {
			header = append(header, dimStyle.Render(fmt.Sprintf("Showing all %d %s", len(q.visible), noun)))
		} else {
			header = append(header, dimStyle.Render(fmt.Sprintf("Showing %d %s %s", len(q.visible), label, noun)))
		}
	}

	footer := []string{dimStyle.Render("h/l filter  ·  j/k select  ·  a approve  ·  d deny  ·  r refresh  ·  esc close")}

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
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.DimmedTextColor))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.ChatErrorColor))
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ListSelectedColor))

	action := "Approve"
	actionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ChatSubAlertColor))
	if q.resolveStatus == "denied" {
		action = "Deny"
		actionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ChatErrorColor))
	}

	name := preferredDisplayName(request.UserName, request.UserLogin)
	out := []string{
		actionStyle.Render(action) + " unban request from " + nameStyle.Render(name),
		dimStyle.Render(strings.Repeat("─", max(1, q.width))),
		dimStyle.Render(fmt.Sprintf("Requested %s", unbanRequestAge(request.CreatedAt))),
	}

	text := request.Text
	if strings.TrimSpace(text) == "" {
		text = "(no request text)"
	}

	bodyWidth := max(1, q.width-4)
	for _, line := range strings.Split(wordwrap.String(text, bodyWidth), "\n") {
		out = append(out, "  "+dimStyle.Render("│ ")+line)
	}

	out = append(out, "", dimStyle.Render("Resolution text (optional, 500 characters):"), q.input.View())
	if q.err != nil {
		out = append(out, errorStyle.Render(q.err.Error()))
	}

	help := "enter confirm  ·  esc cancel"
	if q.submitting {
		help = "Resolving..."
	}
	out = append(out, "", dimStyle.Render(help))

	return fitLines(out, q.width, q.height)
}

func (q *unbanRequestQueue) filterLine(activePill, inactivePill lipgloss.Style) string {
	counts := make(map[string]int, len(unbanRequestFilters))
	for _, request := range q.requests {
		counts[request.Status]++
	}

	parts := make([]string, 0, len(unbanRequestFilters))
	for i, filter := range unbanRequestFilters {
		label := unbanStatusLabel(filter)
		if filter == "all" {
			label = fmt.Sprintf("All %d", len(q.requests))
		} else {
			label = fmt.Sprintf("%s %d", label, counts[filter])
		}

		if i == q.filterAt {
			parts = append(parts, activePill.Render(label))
		} else {
			parts = append(parts, inactivePill.Render(label))
		}
	}

	return strings.Join(parts, " ")
}

func (q *unbanRequestQueue) requestStatusStyle(status string) lipgloss.Style {
	switch status {
	case "pending":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ChatNoticeAlertColor))
	case "approved":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ChatSubAlertColor))
	case "denied":
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(q.theme.ChatErrorColor))
	case "acknowledged":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.ActiveLabelColor))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(q.theme.DimmedTextColor))
	}
}

func unbanStatusLabel(status string) string {
	if status == "" || status == "all" {
		return "all"
	}

	return strings.ToUpper(status[:1]) + status[1:]
}

func slicesIndex(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}

	return 0
}

func (q *unbanRequestQueue) listLines(selectedStyle, dimStyle lipgloss.Style) ([]string, []int) {
	lines := make([]string, 0, len(q.visible)*3)
	starts := make([]int, 0, len(q.visible))
	bodyWidth := max(1, q.width-4)

	for i, request := range q.visible {
		starts = append(starts, len(lines))

		marker := "  "
		name := preferredDisplayName(request.UserName, request.UserLogin)
		if i == q.selected {
			marker = "▸ "
			name = selectedStyle.Render(name)
		}
		status := q.requestStatusStyle(request.Status).Render(unbanStatusLabel(request.Status))
		lines = append(lines, marker+name+"  "+status+"  "+dimStyle.Render(unbanRequestAge(request.CreatedAt)))

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

		if request.Status != "pending" {
			meta := "Resolved"
			if request.ModeratorName != "" || request.ModeratorLogin != "" {
				meta += " by " + preferredDisplayName(request.ModeratorName, request.ModeratorLogin)
			}
			if !request.ResolvedAt.IsZero() {
				meta += " " + unbanRequestAge(request.ResolvedAt)
			}
			if strings.TrimSpace(request.ResolutionText) != "" {
				meta += ": " + request.ResolutionText
			}

			metaLines := strings.Split(wordwrap.String(meta, bodyWidth), "\n")
			if len(metaLines) > 2 {
				metaLines = append(metaLines[:1], "…")
			}
			for _, metaLine := range metaLines {
				lines = append(lines, "    "+dimStyle.Render("↳ "+metaLine))
			}
		}

		if i != len(q.visible)-1 {
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
