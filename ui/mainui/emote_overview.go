package mainui

import (
	"context"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/julez-dev/chatuino/emote"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/semaphore"
)

type emoteWithOverwrite struct {
	emote     emote.Emote
	overwrite string
}

type emoteOverviewSetDataMessage struct {
	id  string
	set map[string][]emoteWithOverwrite
}

type emoteOverview struct {
	id        string
	store     EmoteStore
	channelID string

	vp      viewport.Model
	spinner spinner.Model

	ctx    context.Context
	cancel context.CancelFunc

	emoteReplacer EmoteReplacer
	emotes        map[string][]emoteWithOverwrite
	isLoaded      bool
}

var customEllipsisSpinner = spinner.Spinner{
	Frames: []string{"   ", "  .", " ..", "..."},
	FPS:    time.Second / 3, //nolint:mnd
}

func NewEmoteOverview(channelID string, store EmoteStore, replacer EmoteReplacer, width, height int) *emoteOverview {
	vp := viewport.New(width, height)

	ctx, cancel := context.WithCancel(context.Background())

	return &emoteOverview{
		id:            uuid.New().String(),
		store:         store,
		channelID:     channelID,
		emoteReplacer: replacer,
		vp:            vp,
		spinner:       spinner.New(spinner.WithSpinner(customEllipsisSpinner)),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (e *emoteOverview) Init() tea.Cmd {
	initCmd := func() tea.Msg {
		set := e.store.GetAllForUser(e.channelID)

		type channelData struct {
			emote   emoteWithOverwrite
			prepare string
		}

		ch := make(chan channelData)

		r := make(map[string][]emoteWithOverwrite)

		chunks := slices.Collect(slices.Chunk(set, 200))
		wg := sync.WaitGroup{}
		wg.Add(len(chunks))
		sema := semaphore.NewWeighted(3) // no more than 3 goroutines at once

		for _, chunk := range chunks {
			go func(set emote.EmoteSet) {
				defer wg.Done()

				if err := sema.Acquire(e.ctx, 1); err != nil {
					return
				}

				defer sema.Release(1)

				for _, emote := range set {
					if e.ctx.Err() != nil {
						return
					}

					prepare, overwrite, err := e.emoteReplacer.Replace(e.channelID, emote.Text, nil)
					if err != nil {
						log.Logger.Error().Err(err).Send()
						continue
					}

					ch <- channelData{
						emote: emoteWithOverwrite{
							emote:     emote,
							overwrite: overwrite,
						},
						prepare: prepare,
					}
				}

			}(chunk)
		}

		go func() {
			wg.Wait()
			close(ch)
		}()

		sb := strings.Builder{}
		for d := range ch {
			r[d.emote.emote.Platform.String()] = append(r[d.emote.emote.Platform.String()], d.emote)
			sb.WriteString(d.prepare)
		}

		start := time.Now()
		io.WriteString(os.Stdout, sb.String())
		log.Logger.Info().Str("duration", time.Since(start).String()).Msg("emote overview loaded")

		return emoteOverviewSetDataMessage{
			id:  e.id,
			set: r,
		}
	}

	return tea.Batch(e.spinner.Tick, initCmd)
}

func (e *emoteOverview) Update(msg tea.Msg) (*emoteOverview, tea.Cmd) {
	switch msg := msg.(type) {
	case emoteOverviewSetDataMessage:
		if msg.id != e.id {
			return e, nil
		}

		e.isLoaded = true
		e.emotes = msg.set
		e.updateContent()
		return e, nil
	}

	var cmd tea.Cmd
	if !e.isLoaded {
		e.spinner, cmd = e.spinner.Update(msg)
		return e, cmd
	}

	e.vp, cmd = e.vp.Update(msg)
	return e, cmd
}

func (e *emoteOverview) View() string {
	if !e.isLoaded {
		return lipgloss.NewStyle().Width(e.vp.Width).Height(e.vp.Height).AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Render(e.spinner.View() + " Loading Emote Overview")
	}

	return e.vp.View()
}

func (e *emoteOverview) resize(width, height int) {
	e.vp.Width = width
	e.vp.Height = height
	e.updateContent()
}

func (e *emoteOverview) updateContent() {
	maxWidthRow := e.vp.Width

	var sb strings.Builder
	for provider, emotes := range e.emotes {
		// write provider header
		_, _ = sb.WriteString(lipgloss.NewStyle().Margin(1).MarginBottom(2).Render(provider))

		var totalSpaceTakenInCurrentRow int
		var rowIndex int
		var emoteRows [][]emoteWithOverwrite
		emoteWidths := map[string]int{}

		// calculate emote rows for each provider based on available space
		for _, emoteData := range emotes {
			emoteTextWidth := lipgloss.Width(emoteData.emote.Text)
			emoteOverwriteWidth := lipgloss.Width(emoteData.overwrite)
			emoteWidth := emoteTextWidth

			if emoteOverwriteWidth > emoteTextWidth {
				emoteWidth = emoteOverwriteWidth
			}

			emoteWidths[emoteData.emote.Platform.String()+emoteData.emote.ID] = emoteWidth

			//log.Logger.Info().Int("current-width-row", totalSpaceTakenInCurrentRow).Int("emote-width", emoteWidth).Int("max-width-row", maxWidthRow).Str("emote", emoteData.emote.Text).Msg("")

			// does not fit add to next row
			if totalSpaceTakenInCurrentRow+emoteWidth+2 > maxWidthRow {
				totalSpaceTakenInCurrentRow = 0
				rowIndex++
				emoteRows = append(emoteRows, []emoteWithOverwrite{
					emoteData,
				})
			} else {
				// fits add to current row, create new one if not exists yet
				totalSpaceTakenInCurrentRow += emoteWidth + 4
				// ensure row at index exists
				if len(emoteRows) <= rowIndex {
					emoteRows = append(emoteRows, []emoteWithOverwrite{})
				}
				emoteRows[rowIndex] = append(emoteRows[rowIndex], emoteData)
			}
		}

		_, _ = sb.WriteString("\n")

		for _, row := range emoteRows {
			// write overwritten emote, then start new line and align the text for the emote
			for _, emote := range row {
				key := emote.emote.Platform.String() + emote.emote.ID
				_, _ = sb.WriteString(lipgloss.NewStyle().Width(emoteWidths[key]).MarginRight(2).AlignHorizontal(lipgloss.Center).Render(emote.overwrite))
			}

			_, _ = sb.WriteString("\n")

			for _, emote := range row {
				key := emote.emote.Platform.String() + emote.emote.ID
				_, _ = sb.WriteString(lipgloss.NewStyle().Width(emoteWidths[key]).MarginRight(2).AlignHorizontal(lipgloss.Center).Render(emote.emote.Text))
			}

			sb.WriteString("\n\n")
		}
	}

	e.vp.SetContent(sb.String())
}

func (e *emoteOverview) close() {
	e.cancel()
}
