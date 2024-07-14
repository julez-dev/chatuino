package unbanrequest

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/browser"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch"
	"github.com/rs/zerolog/log"
)

const twitchPopoutLogFmt = "https://www.twitch.tv/popout/moderator/%s/unban-requests?id=%s"

type filter string

const (
	filterAll          filter = "all"
	filterPending      filter = "pending"
	filterApproved     filter = "approved"
	filterDenied       filter = "denied"
	filterAcknowledged filter = "acknowledged"
	filterCanceled     filter = "canceled"
)

func (f filter) String() string {
	switch f {
	case filterAll:
		return "All"
	case filterPending:
		return "Pending"
	case filterApproved:
		return "Approved"
	case filterDenied:
		return "Denied"
	case filterAcknowledged:
		return "Acknowledged"
	case filterCanceled:
		return "Canceled"
	}

	return "Unknown"
}

type unbanRequestService interface {
	FetchUnbanRequests(ctx context.Context, broadcasterID, moderatorID string) ([]twitch.UnbanRequest, error)
	ResolveBanRequest(ctx context.Context, broadcasterID, moderatorID, requestID, status string) (twitch.UnbanRequest, error)
}

type setDataMsg struct {
	requests []twitch.UnbanRequest
	err      error
}

type setResolveDataMsg struct {
	request twitch.UnbanRequest
	err     error
}

type UnbanWindow struct {
	broadcasterID      string
	moderatorID        string
	broadcasterChannel string

	// state
	dataLoaded            bool
	err                   error
	totalUnbanRequests    []twitch.UnbanRequest
	filteredUnbanRequests []twitch.UnbanRequest
	width, height         int
	filter                filter
	cursor                int

	// Dependencies
	unbanRequestService unbanRequestService

	// Components
	paginator paginator.Model
	keymap    save.KeyMap
}

func New(unbanRequestService unbanRequestService, keymap save.KeyMap, broadcasterChannel, broadcasterID, moderatorID string, height, width int) *UnbanWindow {
	pag := paginator.New()
	pag.Type = paginator.Arabic
	pag.KeyMap = paginator.KeyMap{
		NextPage: keymap.NextPage,
		PrevPage: keymap.PrevPage,
	}

	heightMinusHeader := height - 5
	pag.PerPage = heightMinusHeader / 3 // 3 lines per unban request

	return &UnbanWindow{
		unbanRequestService: unbanRequestService,
		paginator:           pag,
		width:               width,
		height:              height,
		filter:              filterPending,
		keymap:              keymap,
		broadcasterChannel:  broadcasterChannel,
		broadcasterID:       broadcasterID,
		moderatorID:         moderatorID,
	}
}

func (u *UnbanWindow) Init() tea.Cmd {
	return func() tea.Msg {
		requests, err := u.unbanRequestService.FetchUnbanRequests(context.Background(), u.broadcasterID, u.moderatorID)

		return setDataMsg{
			requests: requests,
			err:      err,
		}
	}
}

func (u *UnbanWindow) Update(msg tea.Msg) (*UnbanWindow, tea.Cmd) {
	var cmd tea.Cmd

	if msg, ok := msg.(setDataMsg); ok {
		u.dataLoaded = true
		u.err = msg.err
		u.totalUnbanRequests = msg.requests
		u.filteredUnbanRequests = u.requestsForCurrentFilter()

		u.cursor = 0

		u.paginator.Page = 0
		u.paginator.TotalPages = 0
		u.paginator.SetTotalPages(len(u.filteredUnbanRequests))
	}

	if msg, ok := msg.(setResolveDataMsg); ok {
		u.dataLoaded = true
		u.err = msg.err

		if msg.err != nil {
			return u, nil
		}

		for i, r := range u.totalUnbanRequests {
			if r.ID == msg.request.ID {
				u.totalUnbanRequests[i] = msg.request
				break
			}
		}

		u.filteredUnbanRequests = u.requestsForCurrentFilter()
		u.cursor = 0
		if u.paginator.ItemsOnPage(len(u.filteredUnbanRequests)) == 0 {
			u.paginator.PrevPage()
		}
		u.paginator.SetTotalPages(len(u.filteredUnbanRequests))
	}

	if !u.dataLoaded {
		return u, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {

		if key.Matches(msg, u.keymap.NextFilter) {
			// advance filter
			switch u.filter {
			case filterAll:
				u.filter = filterPending
			case filterPending:
				u.filter = filterApproved
			case filterApproved:
				u.filter = filterDenied
			case filterDenied:
				u.filter = filterAcknowledged
			case filterAcknowledged:
				u.filter = filterCanceled
			case filterCanceled:
				u.filter = filterAll
			}

			u.paginator.Page = 0
			u.cursor = 0
			u.paginator.TotalPages = 0
			u.filteredUnbanRequests = u.requestsForCurrentFilter()
			u.paginator.SetTotalPages(len(u.filteredUnbanRequests))

			return u, nil
		}

		if key.Matches(msg, u.keymap.PrevFilter) {
			// reverse filter
			switch u.filter {
			case filterAll:
				u.filter = filterCanceled
			case filterPending:
				u.filter = filterAll
			case filterApproved:
				u.filter = filterPending
			case filterDenied:
				u.filter = filterApproved
			case filterAcknowledged:
				u.filter = filterDenied
			case filterCanceled:
				u.filter = filterAcknowledged
			}

			u.paginator.Page = 0
			u.cursor = 0
			u.paginator.TotalPages = 0
			u.filteredUnbanRequests = u.requestsForCurrentFilter()
			u.paginator.SetTotalPages(len(u.filteredUnbanRequests))

			return u, nil
		}

		if key.Matches(msg, u.keymap.Down) && u.paginator.TotalPages > 0 {
			// Advance cursor to next unban request, start from the beginning if we reach the end
			u.cursor++

			start, end := u.paginator.GetSliceBounds(len(u.filteredUnbanRequests))
			if u.cursor >= len(u.filteredUnbanRequests[start:end]) {
				u.cursor = 0
			}

			return u, nil
		}

		if key.Matches(msg, u.keymap.Up) && u.paginator.TotalPages > 0 {
			// Move cursor to the previous unban request, start from the end if we reach the beginning
			u.cursor--

			start, end := u.paginator.GetSliceBounds(len(u.filteredUnbanRequests))
			if u.cursor < 0 {
				u.cursor = len(u.filteredUnbanRequests[start:end]) - 1
			}

			return u, nil
		}

		if key.Matches(msg, u.keymap.ChatPopUp) {
			// if current cursor is valid
			start, end := u.paginator.GetSliceBounds(len(u.filteredUnbanRequests))
			if u.cursor >= 0 && u.cursor < len(u.filteredUnbanRequests[start:end]) {
				request := u.filteredUnbanRequests[start:end][u.cursor]

				return u, func() tea.Msg {
					url := fmt.Sprintf(twitchPopoutLogFmt, u.broadcasterChannel, request.ID) // open channel in browser

					if err := browser.OpenURL(url); err != nil {
						log.Logger.Error().Err(err).Msg("error while opening twitch channel in browser")
					}

					return nil
				}
			}
		}

		if key.Matches(msg, u.keymap.Deny) {
			return u, u.handleResolveRequest("denied")
		}

		if key.Matches(msg, u.keymap.Approve) {
			return u, u.handleResolveRequest("approved")
		}
	}

	if u.paginator.TotalPages > 0 {
		u.paginator, cmd = u.paginator.Update(msg)
	}

	return u, cmd
}

func (u *UnbanWindow) View() string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		AlignHorizontal(lipgloss.Center).
		Width(u.width).
		MaxWidth(u.width)

	if !u.dataLoaded {
		header := headerStyle.Height(u.height).AlignVertical(lipgloss.Center).Render("Loading data...")
		return header
	}

	if u.err != nil {
		header := headerStyle.Height(u.height).AlignVertical(lipgloss.Center).Render("Error loading unban requests")
		return header
	}

	return u.renderTableView(headerStyle)
}

func (u *UnbanWindow) SetHeight(height int) {
	u.height = height
	heightMinusHeader := height - 5
	u.paginator.PerPage = heightMinusHeader / 3 // 3 lines per unban request
	u.paginator.SetTotalPages(len(u.filteredUnbanRequests))
}

func (u *UnbanWindow) SetWidth(width int) {
	u.width = width
}

func (u *UnbanWindow) handleResolveRequest(status string) tea.Cmd {
	// if current cursor is valid
	start, end := u.paginator.GetSliceBounds(len(u.filteredUnbanRequests))
	if u.cursor >= 0 && u.cursor < len(u.filteredUnbanRequests[start:end]) {
		request := u.filteredUnbanRequests[start:end][u.cursor]

		if request.Status != "pending" {
			return nil
		}

		u.dataLoaded = false

		return func() tea.Msg {
			request, err := u.unbanRequestService.ResolveBanRequest(context.Background(), u.broadcasterID, u.moderatorID, request.ID, status)
			return setResolveDataMsg{
				request: request,
				err:     err,
			}
		}
	}

	return nil
}

func (u *UnbanWindow) renderTableView(headerStyle lipgloss.Style) string {
	currentFilter := lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Render(u.filter.String())

	header := headerStyle.Render(fmt.Sprintf("Unban Requests (%s)", currentFilter))

	b := strings.Builder{}
	b.WriteString(header)
	b.WriteRune('\n')
	b.WriteRune('\n')

	start, end := u.paginator.GetSliceBounds(len(u.filteredUnbanRequests))

	for i, req := range u.filteredUnbanRequests[start:end] {
		if i == u.cursor {
			indicator := lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Background(lipgloss.Color("135")).Render("@")
			b.WriteString(indicator + " ")
		} else {
			b.WriteString("  ")
		}
		b.WriteString(u.renderUnbanRequest(req))
		b.WriteRune('\n')
	}
	b.WriteRune('\n')

	if u.paginator.TotalPages > 0 {
		b.WriteString("  " + headerStyle.Render(u.paginator.View()) + "\n")
	} else {
		b.WriteString("  " + headerStyle.Render("0/0") + "\n")
	}

	heightUntilNow := lipgloss.Height(b.String())

	for i := 0; i < u.height-heightUntilNow; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

func (u *UnbanWindow) requestsForCurrentFilter() []twitch.UnbanRequest {
	if u.filter == filterAll {
		return u.totalUnbanRequests
	}

	var requests []twitch.UnbanRequest

	for _, r := range u.totalUnbanRequests {
		if r.Status == string(u.filter) {
			requests = append(requests, r)
		}
	}

	return requests
}

func (u *UnbanWindow) renderUnbanRequest(req twitch.UnbanRequest) string {
	start, end := u.paginator.GetSliceBounds(len(u.filteredUnbanRequests))

	// find the longest username in current page
	userNamePadding := 0
	for _, r := range u.filteredUnbanRequests[start:end] {
		if len(r.UserName) > userNamePadding {
			userNamePadding = len(r.UserName)
		}
	}

	userDisplay := lipgloss.NewStyle().Width(userNamePadding).Align(lipgloss.Left).Render(req.UserName)
	statusDisplay := lipgloss.NewStyle().Width(len(filterAcknowledged)).Align(lipgloss.Left).Render(req.Status)

	prefix := req.CreatedAt.Format("02.01.2006 15:04:05") + " " + statusDisplay
	line := prefix + " " + userDisplay + " " + req.Text

	if req.Status == string(filterDenied) || req.Status == string(filterApproved) {
		modMessage := strings.Repeat(" ", len(prefix)+3) + req.ModeratorName + " resolved at: " + req.ResolvedAt.Format("02.01.2006 15:04:05") + " " + req.ResolutionText
		line += "\n" + modMessage
	}

	return line + "\n"
}
