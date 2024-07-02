package unbanrequest

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"strings"
	"time"
)

type UnbanRequest struct {
	ID               string    `json:"id"`
	BroadcasterName  string    `json:"broadcaster_name"`
	BroadcasterLogin string    `json:"broadcaster_login"`
	BroadcasterID    string    `json:"broadcaster_id"`
	ModeratorID      string    `json:"moderator_id"`
	ModeratorLogin   string    `json:"moderator_login"`
	ModeratorName    string    `json:"moderator_name"`
	UserID           string    `json:"user_id"`
	UserLogin        string    `json:"user_login"`
	UserName         string    `json:"user_name"`
	Text             string    `json:"text"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	ResolvedAt       time.Time `json:"resolved_at"`
	ResolutionText   string    `json:"resolution_text"`
}

type unbanRequestService interface {
	FetchUnbanRequests(ctx context.Context, broadcasterID, moderatorID string) ([]UnbanRequest, error)
}

type setDataMsg struct {
	requests []UnbanRequest
	err      error
}

type UnbanWindow struct {
	broadcasterID string
	moderatorID   string

	// state
	dataLoaded    bool
	err           error
	requests      []UnbanRequest
	width, height int

	// Dependencies
	unbanRequestService unbanRequestService

	// Components
	paginator paginator.Model
}

type FakeUnbanRequestService struct{}

func (f FakeUnbanRequestService) FetchUnbanRequests(ctx context.Context, broadcasterID, moderatorID string) ([]UnbanRequest, error) {
	requests := make([]UnbanRequest, 0, 100)

	for i := 0; i < 100; i++ {
		requests = append(requests, UnbanRequest{
			BroadcasterID: broadcasterID,
			ModeratorID:   moderatorID,
			ID:            uuid.Must(uuid.NewUUID()).String(),
			Text:          fmt.Sprintf("Unban request %d", i),
			UserName:      "Test User",
			CreatedAt:     time.Now(),
			ResolvedAt:    time.Now(),
		})
	}

	return requests, nil
}

func New(unbanRequestService unbanRequestService, height, width int) *UnbanWindow {
	pag := paginator.New()
	pag.Type = paginator.Arabic
	pag.PerPage = 10

	return &UnbanWindow{
		unbanRequestService: unbanRequestService,
		paginator:           pag,
		width:               width,
		height:              height,
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
		u.requests = msg.requests

		u.paginator.PerPage = 10
		u.paginator.SetTotalPages(len(u.requests))
	}

	u.paginator, cmd = u.paginator.Update(msg)

	return u, cmd
}

func (u *UnbanWindow) View() string {
	if !u.dataLoaded {
		return "Loading..."
	}

	if u.err != nil {
		return "Error loading data"
	}

	b := strings.Builder{}
	b.WriteString("Unban Requests\n\n")

	start, end := u.paginator.GetSliceBounds(len(u.requests))

	for _, req := range u.requests[start:end] {
		b.WriteString(req.Text)
		b.WriteString("\n")
	}

	b.WriteString("  " + u.paginator.View() + "\n")

	count := strings.Count(b.String(), "\n")
	log.Logger.Info().Int("count", count).Send()

	for i := 1; i < u.height-count; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

func (u *UnbanWindow) SetHeight(height int) {
	u.height = height
}

func (u *UnbanWindow) SetWidth(width int) {
	u.width = width
}
