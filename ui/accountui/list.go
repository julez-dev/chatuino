package accountui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/server"
	"github.com/julez-dev/chatuino/twitch/twitchapi"
)

type AccountProvider interface {
	GetAllAccounts() ([]save.Account, error)
	GetAccountBy(id string) (save.Account, error)
	UpdateTokensFor(id, accessToken, refreshToken string) error
	MarkAccountAsMain(id string) error
	Remove(id string) error
	Add(account save.Account) error
}

type state int

const (
	inTable state = iota
	inCreate
	inConfirmDelete
)

// accountRow holds display data for one account
type accountRow struct {
	id          string
	displayName string
	isMain      bool
	createdAt   time.Time
	tokenValid  bool
}

type setAccountsMessage struct {
	err      error
	accounts []accountRow
}

type tokenValidationMessage struct {
	accountID string
	valid     bool
}

type List struct {
	keymap          save.KeyMap
	accountProvider AccountProvider
	create          createModel
	state           state
	width, height   int
	err             error
	theme           save.Theme

	// Account list state
	accounts []accountRow
	cursor   int

	// Confirmation dialog
	confirmDeleteID   string
	confirmDeleteName string

	// Styles
	borderStyle       lipgloss.Style
	headerStyle       lipgloss.Style
	selectedStyle     lipgloss.Style
	dimmedStyle       lipgloss.Style
	mainBadgeStyle    lipgloss.Style
	validTokenStyle   lipgloss.Style
	invalidTokenStyle lipgloss.Style
	errorStyle        lipgloss.Style
	footerStyle       lipgloss.Style

	clientID, apiHost string
}

func NewList(clientID, apiHost string, accountProvider AccountProvider, keymap save.KeyMap, theme save.Theme) List {
	borderColor := lipgloss.Color(theme.InputPromptColor)

	return List{
		apiHost:         apiHost,
		clientID:        clientID,
		accountProvider: accountProvider,
		keymap:          keymap,
		theme:           theme,

		borderStyle:       lipgloss.NewStyle().Foreground(borderColor),
		headerStyle:       lipgloss.NewStyle().Foreground(borderColor).Bold(true),
		selectedStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ListSelectedColor)).Bold(true),
		dimmedStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor)),
		mainBadgeStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ActiveLabelColor)),
		validTokenStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("#a3be8c")), // Nord green
		invalidTokenStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#bf616a")), // Nord red
		errorStyle:        lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatErrorColor)),
		footerStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor)),
	}
}

func (l List) Init() tea.Cmd {
	return l.fetchAccounts
}

func (l List) fetchAccounts() tea.Msg {
	accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
	if err != nil {
		return setAccountsMessage{err: err}
	}

	return setAccountsMessage{accounts: accountsToRows(accounts, nil)}
}

func (l List) validateTokens() tea.Cmd {
	// Capture current state to avoid stale closures
	accounts := l.accounts
	provider := l.accountProvider

	cmds := make([]tea.Cmd, 0, len(accounts))
	for _, acc := range accounts {
		acc := acc
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			account, err := provider.GetAccountBy(acc.id)
			if err != nil {
				return tokenValidationMessage{accountID: acc.id, valid: false}
			}

			valid, err := twitchapi.ValidateToken(ctx, nil, account.AccessToken)
			if err != nil {
				// Network error - assume valid to avoid false negatives
				return tokenValidationMessage{accountID: acc.id, valid: true}
			}
			return tokenValidationMessage{accountID: acc.id, valid: valid}
		})
	}
	return tea.Batch(cmds...)
}

func (l List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	if l.state == inCreate {
		l.create, cmd = l.create.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		l.create.width = msg.Width
		l.create.height = msg.Height

	case cancelCreateMessage:
		l.state = inTable
		return l, nil

	case setAccountMessage:
		l.err = msg.err
		l.state = inTable

		if msg.err == nil {
			cmds = append(cmds, l.addNewAccountRefresh(msg.account))
		}

		return l, tea.Batch(cmds...)

	case setAccountsMessage:
		l.err = msg.err
		if l.err == nil {
			l.accounts = msg.accounts
			if l.cursor >= len(l.accounts) {
				l.cursor = max(0, len(l.accounts)-1)
			}
			// Start token validation
			cmds = append(cmds, l.validateTokens())
		}

	case tokenValidationMessage:
		for i, acc := range l.accounts {
			if acc.id == msg.accountID {
				l.accounts[i].tokenValid = msg.valid
				break
			}
		}

	case tea.KeyMsg:
		l.err = nil

		if l.state == inConfirmDelete {
			switch {
			case key.Matches(msg, l.keymap.Confirm):
				// Confirmed delete
				cmds = append(cmds, l.removeAccountRefresh(l.confirmDeleteID))
				l.state = inTable
				return l, tea.Batch(cmds...)
			case key.Matches(msg, l.keymap.Quit), key.Matches(msg, l.keymap.Escape):
				// Cancel delete
				l.state = inTable
				return l, nil
			}
			return l, nil
		}

		if l.state == inTable {
			switch {
			case key.Matches(msg, l.keymap.Up):
				if l.cursor > 0 {
					l.cursor--
				}
			case key.Matches(msg, l.keymap.Down):
				if l.cursor < len(l.accounts)-1 {
					l.cursor++
				}
			case key.Matches(msg, l.keymap.Create):
				l.state = inCreate
				l.create = newCreateModel(l.width, l.height, l.clientID, l.apiHost, l.keymap, l.theme)
				return l, l.create.Init()
			case key.Matches(msg, l.keymap.Remove):
				if len(l.accounts) > 0 && l.cursor < len(l.accounts) {
					acc := l.accounts[l.cursor]
					l.confirmDeleteID = acc.id
					l.confirmDeleteName = acc.displayName
					l.state = inConfirmDelete
				}
			case key.Matches(msg, l.keymap.MarkLeader):
				if len(l.accounts) > 0 && l.cursor < len(l.accounts) {
					cmds = append(cmds, l.markAccountMain(l.accounts[l.cursor].id))
					return l, tea.Batch(cmds...)
				}
			case key.Matches(msg, l.keymap.Quit):
				return l, tea.Quit
			}
		} else if l.state == inCreate {
			if key.Matches(msg, l.keymap.Create) {
				l.state = inTable
			}
		}
	}

	return l, tea.Batch(cmds...)
}

func (l List) View() string {
	if l.state == inCreate {
		return l.create.View()
	}

	if l.state == inConfirmDelete {
		return l.renderConfirmDialog()
	}

	return l.renderAccountList()
}

func (l List) renderAccountList() string {
	// Build content
	var content strings.Builder

	if l.err != nil {
		content.WriteString(l.errorStyle.Render(fmt.Sprintf("Error: %s", l.err)))
		content.WriteString("\n\n")
	}

	if len(l.accounts) == 0 {
		// Empty state
		emptyMsg := l.dimmedStyle.Render(fmt.Sprintf("No accounts yet. Press '%s' to add one.", firstKey(l.keymap.Create)))
		content.WriteString("\n")
		content.WriteString(emptyMsg)
		content.WriteString("\n")
	} else {
		// Render account rows
		for i, acc := range l.accounts {
			row := l.renderAccountRow(acc, i == l.cursor)
			content.WriteString(row)
			content.WriteString("\n")
		}
	}

	// Calculate box dimensions
	boxWidth := 60
	if l.width > 0 && l.width < boxWidth+4 {
		boxWidth = max(20, l.width-4) // minimum viable width
	}

	// Build bordered box
	contentStr := strings.TrimSuffix(content.String(), "\n")
	contentLines := strings.Split(contentStr, "\n")

	// Pad lines to box width
	innerWidth := boxWidth - 4 // account for "│ " and " │"
	for i, line := range contentLines {
		lineLen := lipgloss.Width(line)
		if lineLen < innerWidth {
			contentLines[i] = line + strings.Repeat(" ", innerWidth-lineLen)
		}
	}

	// Build box
	var box strings.Builder

	// Top border with header
	header := "[ Accounts ]"
	topBorder := "─" + l.headerStyle.Render(header) + strings.Repeat("─", boxWidth-lipgloss.Width(header)-3) + "┐"
	box.WriteString(l.borderStyle.Render("┌" + topBorder))
	box.WriteString("\n")

	// Empty line after header
	box.WriteString(l.borderStyle.Render("│") + strings.Repeat(" ", boxWidth-2) + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Content lines
	for _, line := range contentLines {
		box.WriteString(l.borderStyle.Render("│") + " " + line + " " + l.borderStyle.Render("│"))
		box.WriteString("\n")
	}

	// Empty line before footer
	box.WriteString(l.borderStyle.Render("│") + strings.Repeat(" ", boxWidth-2) + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Bottom border with footer
	footer := l.renderFooter()
	footerLen := lipgloss.Width(footer)
	bottomBorder := "─" + footer + strings.Repeat("─", boxWidth-footerLen-3) + "┘"
	box.WriteString(l.borderStyle.Render("└" + bottomBorder))

	// Center in viewport
	return lipgloss.NewStyle().
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Width(l.width - 2).
		Height(l.height - 2).
		Render(box.String())
}

func (l List) renderAccountRow(acc accountRow, selected bool) string {
	var parts []string

	// Selection arrow
	if selected {
		parts = append(parts, l.selectedStyle.Render("▸"))
	} else {
		parts = append(parts, " ")
	}

	// Token status indicator
	if acc.tokenValid {
		parts = append(parts, l.validTokenStyle.Render("●"))
	} else {
		parts = append(parts, l.invalidTokenStyle.Render("●"))
	}

	// ID (shortened)
	idStr := acc.id
	if len(idStr) > 10 {
		idStr = idStr[:10]
	}
	idPadded := fmt.Sprintf("%-10s", idStr)
	if selected {
		parts = append(parts, l.selectedStyle.Render(idPadded))
	} else {
		parts = append(parts, l.dimmedStyle.Render(idPadded))
	}

	// Main badge
	if acc.isMain {
		parts = append(parts, l.mainBadgeStyle.Render("Main"))
	} else {
		parts = append(parts, "    ")
	}

	// Display name
	namePadded := fmt.Sprintf("%-15s", acc.displayName)
	if selected {
		parts = append(parts, l.selectedStyle.Render(namePadded))
	} else {
		parts = append(parts, namePadded)
	}

	// Date
	dateStr := acc.createdAt.Local().Format("02.01.2006 15:04")
	if selected {
		parts = append(parts, l.selectedStyle.Render(dateStr))
	} else {
		parts = append(parts, l.dimmedStyle.Render(dateStr))
	}

	return strings.Join(parts, " ")
}

func (l List) renderFooter() string {
	// Extract first key from each binding for display
	addKey := firstKey(l.keymap.Create)
	delKey := firstKey(l.keymap.Remove)
	mainKey := firstKey(l.keymap.MarkLeader)
	quitKey := firstKey(l.keymap.Quit)

	hints := []string{
		addKey + ":Add",
		delKey + ":Delete",
		mainKey + ":Main",
		quitKey + ":Quit",
	}
	return l.footerStyle.Render("[ " + strings.Join(hints, " ") + " ]")
}

func firstKey(b key.Binding) string {
	keys := b.Keys()
	if len(keys) > 0 {
		return keys[0]
	}
	return "?"
}

func (l List) renderConfirmDialog() string {
	boxWidth := 50
	if l.width > 0 && l.width < boxWidth+4 {
		boxWidth = max(30, l.width-4)
	}

	var box strings.Builder

	// Top border with header
	header := "[ Confirm Delete ]"
	topBorder := "─" + l.errorStyle.Render(header) + strings.Repeat("─", boxWidth-lipgloss.Width(header)-3) + "┐"
	box.WriteString(l.borderStyle.Render("┌" + topBorder))
	box.WriteString("\n")

	// Empty line
	box.WriteString(l.borderStyle.Render("│") + strings.Repeat(" ", boxWidth-2) + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Message
	msg := fmt.Sprintf("Delete account '%s'?", l.confirmDeleteName)
	msgPadded := fmt.Sprintf("%-*s", boxWidth-4, msg)
	box.WriteString(l.borderStyle.Render("│") + " " + msgPadded + " " + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Empty line
	box.WriteString(l.borderStyle.Render("│") + strings.Repeat(" ", boxWidth-2) + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Instructions
	instr := "Enter: Confirm  Esc: Cancel"
	instrPadded := fmt.Sprintf("%-*s", boxWidth-4, instr)
	box.WriteString(l.borderStyle.Render("│") + " " + l.dimmedStyle.Render(instrPadded) + " " + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Empty line
	box.WriteString(l.borderStyle.Render("│") + strings.Repeat(" ", boxWidth-2) + l.borderStyle.Render("│"))
	box.WriteString("\n")

	// Bottom border
	box.WriteString(l.borderStyle.Render("└" + strings.Repeat("─", boxWidth-2) + "┘"))

	return lipgloss.NewStyle().
		AlignHorizontal(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Width(l.width - 2).
		Height(l.height - 2).
		Render(box.String())
}

func (l List) markAccountMain(id string) tea.Cmd {
	existingRows := l.accounts // capture to preserve token validation status
	return func() tea.Msg {
		if err := l.accountProvider.MarkAccountAsMain(id); err != nil {
			return setAccountsMessage{err: err}
		}

		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		if err != nil {
			return setAccountsMessage{err: err}
		}

		return setAccountsMessage{accounts: accountsToRows(accounts, existingRows)}
	}
}

func (l List) removeAccountRefresh(id string) tea.Cmd {
	return func() tea.Msg {
		acc, err := l.accountProvider.GetAccountBy(id)
		if err != nil {
			return setAccountsMessage{err: err}
		}

		if acc.IsAnonymous {
			return nil
		}

		srv := server.NewClient(l.apiHost, nil)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()

		if err := srv.RevokeToken(ctx, acc.AccessToken); err != nil {
			return setAccountsMessage{err: err}
		}

		if err := l.accountProvider.Remove(acc.ID); err != nil {
			return setAccountsMessage{err: err}
		}

		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		if err != nil {
			return setAccountsMessage{err: err}
		}

		// After delete, don't preserve old validation status (account is gone)
		return setAccountsMessage{accounts: accountsToRows(accounts, nil)}
	}
}

func (l List) addNewAccountRefresh(account save.Account) tea.Cmd {
	return func() tea.Msg {
		var shouldSetMain bool

		if accounts, err := l.accountProvider.GetAllAccounts(); err == nil {
			accounts = slices.DeleteFunc(accounts, func(a save.Account) bool {
				return a.IsAnonymous
			})
			shouldSetMain = len(accounts) == 0
		}

		account.IsMain = shouldSetMain

		if err := l.accountProvider.Add(account); err != nil {
			return setAccountsMessage{err: err}
		}

		accounts, err := fetchAccountsNonAnonymous(l.accountProvider)
		if err != nil {
			return setAccountsMessage{err: err}
		}

		// New account, no existing validation status to preserve
		return setAccountsMessage{accounts: accountsToRows(accounts, nil)}
	}
}

// accountsToRows converts save.Account slice to accountRow slice, preserving token validation status from existing rows.
func accountsToRows(accounts []save.Account, existingRows []accountRow) []accountRow {
	rows := make([]accountRow, 0, len(accounts))
	for _, acc := range accounts {
		valid := true
		for _, existing := range existingRows {
			if existing.id == acc.ID {
				valid = existing.tokenValid
				break
			}
		}
		rows = append(rows, accountRow{
			id:          acc.ID,
			displayName: acc.DisplayName,
			isMain:      acc.IsMain,
			createdAt:   acc.CreatedAt,
			tokenValid:  valid,
		})
	}
	return rows
}

func fetchAccountsNonAnonymous(provider AccountProvider) ([]save.Account, error) {
	accounts, err := provider.GetAllAccounts()
	if err != nil {
		return nil, err
	}

	accounts = slices.DeleteFunc(accounts, func(a save.Account) bool {
		return a.IsAnonymous
	})

	return accounts, nil
}
