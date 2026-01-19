package component

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	trie "github.com/Vivino/go-autocomplete-trie"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/julez-dev/chatuino/command"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/rs/zerolog/log"
)

type emoteReplacementMessage struct {
	word        string
	prepare     string
	replaceCode string
}

type Replacer interface {
	Replace(channelID, content string, emoteList []twitchirc.Emote) (string, map[string]string, error)
}

// KeyMap is the key bindings for different actions within the textinput.
type KeyMap struct {
	AcceptSuggestion key.Binding
	NextSuggestion   key.Binding
	PrevSuggestion   key.Binding
}

// DefaultKeyMap is the default set of key bindings for navigating and acting
// upon the textinput.
var DefaultKeyMap = KeyMap{
	AcceptSuggestion: key.NewBinding(key.WithKeys("tab")),
	NextSuggestion:   key.NewBinding(key.WithKeys("down", "ctrl+n")),
	PrevSuggestion:   key.NewBinding(key.WithKeys("up", "ctrl+p")),
}

type SuggestionTextInput struct {
	trie *trie.Trie

	InputModel textinput.Model

	KeyMap          KeyMap
	suggestionIndex int
	suggestions     []string

	history                    []string
	historyIndex               int
	browsingHistory            bool // true when navigating history with up/down
	IncludeCommandSuggestions  bool
	IncludeModeratorCommands   bool
	DisableAutoSpaceSuggestion bool
	DisableHistory             bool
	EmoteReplacer              Replacer

	customSuggestions map[string]string
	emoteReplacements map[string]string // emoteText:unicode

	userCache map[string]func(...string) string // [username]render func

	// Multi-line display support
	maxVisibleLines int // 1 = single line (default), >1 = wrapped multi-line display
	width           int // stored width for wrapping calculations
	viewOffset      int // scroll offset when wrapped lines > maxVisibleLines
}

func defaultTrie() *trie.Trie {
	t := trie.New()
	t = t.WithoutFuzzy()
	t = t.WithoutLevenshtein()
	// t = t.WithoutNormalisation()
	return t
}

// NewSuggestionTextInput creates a new model with default settings.
func NewSuggestionTextInput(userCache map[string]func(...string) string, customSuggestions map[string]string) *SuggestionTextInput {
	input := textinput.New()
	input.Width = 20

	input.Validate = func(s string) error {
		if strings.ContainsRune(s, '\n') {
			return fmt.Errorf("disallowed input")
		}

		return nil
	}

	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))
	t := defaultTrie()

	return &SuggestionTextInput{
		trie:                      t,
		KeyMap:                    DefaultKeyMap,
		InputModel:                input,
		history:                   []string{},
		userCache:                 userCache,
		IncludeCommandSuggestions: true,
		IncludeModeratorCommands:  false,
		customSuggestions:         customSuggestions,
		emoteReplacements:         map[string]string{},
		maxVisibleLines:           1, // default single-line for backward compat
	}
}

func (s *SuggestionTextInput) Update(msg tea.Msg) (*SuggestionTextInput, tea.Cmd) {
	if !s.InputModel.Focused() {
		return s, nil
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case emoteReplacementMessage:
		_, _ = io.WriteString(os.Stdout, msg.prepare)
		s.emoteReplacements[msg.word] = msg.replaceCode
	case tea.KeyMsg:
		switch {
		case msg.String() == "enter" && !s.DisableHistory:
			s.history = append(s.history, s.InputModel.Value())
			s.historyIndex = len(s.history)
			s.browsingHistory = false
			return s, nil
		case key.Matches(msg, s.KeyMap.PrevSuggestion) && (s.InputModel.Value() == "" || s.browsingHistory):
			s.historyIndex--
			s.browsingHistory = true

			if s.historyIndex < 0 {
				if len(s.history) != 0 {
					s.historyIndex = len(s.history) - 1
				} else {
					s.historyIndex = 0
				}
			}

			if len(s.history) > s.historyIndex {
				s.SetValue(s.history[s.historyIndex])
				s.InputModel.CursorEnd()
			}

			return s, nil
		case key.Matches(msg, s.KeyMap.NextSuggestion) && (s.InputModel.Value() == "" || s.browsingHistory):
			s.historyIndex++
			s.browsingHistory = true

			if s.historyIndex >= len(s.history) {
				s.historyIndex = 0
			}

			if len(s.history) > s.historyIndex {
				s.SetValue(s.history[s.historyIndex])
				s.InputModel.CursorEnd()
			}

			return s, nil
		case key.Matches(msg, s.KeyMap.AcceptSuggestion) && s.canAcceptSuggestion():
			_, startIndex, endIndex := selectWordAtIndex(s.InputModel.Value(), s.InputModel.Position())
			before := s.InputModel.Value()[:startIndex]
			after := s.InputModel.Value()[endIndex:]
			suggestion := s.suggestions[s.suggestionIndex]

			// if the suggestion is in custom suggestions, replace with custom suggestion text
			if s.customSuggestions != nil {
				if customSuggestion, ok := s.customSuggestions[suggestion]; ok {
					suggestion = customSuggestion
				}
			}

			// add space on non command suggestions
			if !strings.HasPrefix(suggestion, "/") && !s.DisableAutoSpaceSuggestion {
				suggestion = suggestion + " "
			}

			s.InputModel.SetValue(before + suggestion + after)
			s.InputModel.SetCursor(len(before) + len(suggestion)) // set cursor to end of suggestion + 1 for space

			return s, nil
		case key.Matches(msg, s.KeyMap.NextSuggestion):
			s.nextSuggestion()

			// if emote replacer is enabled we try to display the actual emote, before that we need to fetch the emote
			if s.EmoteReplacer != nil && s.canAcceptSuggestion() {
				return s, s.loadEmoteImageCommand()
			}
		case key.Matches(msg, s.KeyMap.PrevSuggestion):
			s.previousSuggestion()

			// if emote replacer is enabled we try to display the actual emote, before that we need to fetch the emote
			if s.EmoteReplacer != nil && s.canAcceptSuggestion() {
				return s, s.loadEmoteImageCommand()
			}
		default:
			s.InputModel, cmd = s.InputModel.Update(msg)
			s.updateSuggestions()
			s.browsingHistory = false // exit history mode when typing

			// if emote replacer is enabled we try to display the actual emote, before that we need to fetch the emote
			if s.EmoteReplacer != nil && s.canAcceptSuggestion() {
				return s, tea.Batch(cmd, s.loadEmoteImageCommand())
			}

			return s, cmd
		}
	}

	s.InputModel, cmd = s.InputModel.Update(msg)

	return s, cmd
}

func (s *SuggestionTextInput) loadEmoteImageCommand() tea.Cmd {
	suggestion := s.suggestions[s.suggestionIndex]

	// command should never be emotes, same as users
	if strings.HasPrefix(suggestion, "/") || strings.HasPrefix(suggestion, "@") {
		return nil
	}

	if _, ok := s.userCache[strings.TrimPrefix(suggestion, "@")]; ok {
		return nil
	}

	return func() tea.Msg {
		prepare, replace, err := s.EmoteReplacer.Replace("", suggestion, nil)
		if err != nil {
			return nil
		}

		log.Logger.Info().Str("sugg", suggestion).Any("replace", replace).Msg("suggestion emote replaced")

		// skip when empty
		if replace[suggestion] == "" {
			return nil
		}

		return emoteReplacementMessage{
			prepare:     prepare,
			replaceCode: replace[suggestion],
			word:        suggestion,
		}
	}
}

func (s *SuggestionTextInput) View() string {
	// Determine which input view to use
	var inputView string
	if s.maxVisibleLines > 1 {
		inputView = s.renderMultiLineView()
	} else {
		inputView = s.InputModel.View()
	}

	if s.canAcceptSuggestion() {
		suggestion := s.suggestions[s.suggestionIndex]

		// If the suggestion is a username, render it with the users color function
		if renderFunc, ok := s.userCache[strings.TrimPrefix(suggestion, "@")]; ok {
			suggestion = renderFunc(suggestion)
		}

		// current suggestion is emote and has a relacement
		if replace, ok := s.emoteReplacements[suggestion]; ok && replace != suggestion {
			return fmt.Sprintf(" %s %s (%dx)\n%s", suggestion, replace, len(s.suggestions), inputView)
		}

		return fmt.Sprintf(" %s (%dx)\n%s", suggestion, len(s.suggestions), inputView)
	}

	return "\n" + inputView
}

func (s *SuggestionTextInput) Blur() {
	s.InputModel.Blur()
}

func (s *SuggestionTextInput) Focus() {
	s.InputModel.Focus()
}

func (s *SuggestionTextInput) SetWidth(width int) {
	s.width = width
	s.InputModel.Width = width - 3 // -3 for prompt
}

// SetMaxVisibleLines sets the maximum number of visible lines for wrapped display.
// When n > 1, input text will be soft-wrapped and displayed across multiple lines.
// When n == 1 (default), the original single-line behavior is used.
func (s *SuggestionTextInput) SetMaxVisibleLines(n int) {
	if n < 1 {
		n = 1
	}
	s.maxVisibleLines = n
}

func (s *SuggestionTextInput) Value() string {
	return strings.TrimSpace(s.InputModel.Value())
}

func (s *SuggestionTextInput) SetSuggestions(suggestions []string) {
	sugg := make([]string, len(suggestions))
	copy(sugg, suggestions)

	trie := defaultTrie()
	trie.Insert(sugg...)

	s.trie = trie

	s.suggestionIndex = 0
	s.updateSuggestions()
}

func (s *SuggestionTextInput) SetValue(val string) {
	s.InputModel.SetValue(val)
	s.InputModel.CursorEnd()
	s.suggestionIndex = 0
	s.updateSuggestions()
}

func (s *SuggestionTextInput) canAcceptSuggestion() bool {
	tiVal := s.InputModel.Value()
	word, _, _ := selectWordAtIndex(tiVal, s.InputModel.Position())

	// only show if the current word is longer than 2 characters and the suggestion is different from the current word
	// or if the current word is a command
	return (len(word) > 2 || strings.HasPrefix(tiVal, "/")) && len(s.suggestions) > 0 && s.suggestions[s.suggestionIndex] != word
}

func (s *SuggestionTextInput) updateSuggestions() {
	if len(s.InputModel.Value()) <= 0 {
		s.suggestions = nil
		return
	}

	currWord, startIndex, _ := selectWordAtIndex(s.InputModel.Value(), s.InputModel.Position())
	if currWord == "" {
		s.suggestions = nil
		return
	}

	matches := s.trie.SearchAll(currWord)

	if !reflect.DeepEqual(matches, s.suggestions) {
		s.suggestionIndex = 0
	}

	s.suggestions = matches

	// If the current word is a command and is at the start of the message, add command help to suggestions
	if strings.HasPrefix(currWord, "/") && startIndex == 0 {
		if s.IncludeCommandSuggestions {
			for _, suggestion := range command.CommandSuggestions {
				if strings.Contains(suggestion, currWord) {
					s.suggestions = append(s.suggestions, suggestion)
				}
			}
		}

		if s.IncludeModeratorCommands {
			for _, suggestion := range command.ModeratorSuggestions {
				if strings.Contains(suggestion, currWord) {
					s.suggestions = append(s.suggestions, suggestion)
				}
			}
		}

		if s.customSuggestions != nil {
			for command := range s.customSuggestions {
				if strings.Contains(command, currWord) {
					s.suggestions = append(s.suggestions, command)
				}
			}
		}
	}

	// sort suggestions by word length
	slices.SortFunc(s.suggestions, func(a, b string) int {
		if len(a) == len(b) {
			return strings.Compare(a, b)
		}

		return len(a) - len(b)
	})

	// If the current word is a user, add user suggestions to suggestions (with @ prefix)
	if strings.HasPrefix(currWord, "@") {
		var matchedUsers []string

		for user := range s.userCache {
			if strings.Contains(user, strings.ToLower(currWord[1:])) {
				// if the current word is a command, don't add the @ prefix, since commands don't support it
				// else add mention (@) prefix, so the target user gets a notification
				if strings.HasPrefix(s.InputModel.Value(), "/") {
					matchedUsers = append(matchedUsers, user)
				} else {
					matchedUsers = append(matchedUsers, "@"+user)
				}
			}
		}

		slices.SortFunc(matchedUsers, func(a, b string) int {
			// sorty by length
			// if same length, sort alphabetically
			if len(a) == len(b) {
				return strings.Compare(a, b)
			}

			return len(a) - len(b)
		})

		s.suggestions = append(s.suggestions, matchedUsers...)
	}
}

func (s *SuggestionTextInput) nextSuggestion() {
	s.suggestionIndex = s.suggestionIndex + 1
	if s.suggestionIndex >= len(s.suggestions) {
		s.suggestionIndex = 0
	}
}

func (s *SuggestionTextInput) previousSuggestion() {
	s.suggestionIndex = s.suggestionIndex - 1
	if s.suggestionIndex < 0 {
		s.suggestionIndex = max(0, len(s.suggestions)-1)
	}
}

// selectWordAtIndex returns the word at the given rune index, along with byte start/end indices.
// The index parameter is a rune position (as returned by textinput.Model.Position()).
// Returns the word, byte start index, and byte end index for use with string slicing.
func selectWordAtIndex(sentence string, runeIndex int) (string, int, int) {
	runes := []rune(sentence)
	if runeIndex > len(runes) || sentence == "" {
		return "", 0, 0
	}

	// Find word boundaries in rune space
	startRune := runeIndex
	for startRune > 0 && runes[startRune-1] != ' ' {
		startRune--
	}

	endRune := runeIndex
	for endRune < len(runes) && runes[endRune] != ' ' {
		endRune++
	}

	// Convert rune indices to byte indices for string slicing
	startByte := len(string(runes[:startRune]))
	endByte := len(string(runes[:endRune]))

	return sentence[startByte:endByte], startByte, endByte
}

// wrapTextPreservingSpaces wraps text while preserving all whitespace.
// Returns lines and break positions (rune indices where each new line starts).
// Unlike reflow libraries, this preserves trailing spaces for accurate cursor positioning.
func (s *SuggestionTextInput) wrapTextPreservingSpaces(text string, wrapWidth int) (lines []string, breaks []int) {
	if text == "" {
		return []string{""}, nil
	}
	if wrapWidth <= 0 {
		return []string{text}, nil
	}

	runes := []rune(text)
	lineStart := 0
	lastSpace := -1
	col := 0

	for i, r := range runes {
		col++

		if col > wrapWidth {
			// Need to wrap
			var breakAt int
			if lastSpace >= lineStart && lastSpace >= 0 {
				// Wrap after the last space (word boundary) - space stays on current line
				breakAt = lastSpace + 1
			} else {
				// No space found, hard wrap at current position
				breakAt = i
			}

			lines = append(lines, string(runes[lineStart:breakAt]))
			breaks = append(breaks, breakAt)
			lineStart = breakAt
			col = i - breakAt + 1
			lastSpace = -1
		}

		// Update lastSpace AFTER wrap check, so it doesn't include the overflowing char
		if r == ' ' {
			lastSpace = i
		}
	}

	// Add remaining text as last line
	lines = append(lines, string(runes[lineStart:]))
	return lines, breaks
}

// cursorLineCol maps the cursor position to (line, col) in wrapped text.
// Note: InputModel.Position() returns rune position, not byte position.
// Returns the line index and rune column within that line.
func (s *SuggestionTextInput) cursorLineCol(text string, runePos int, wrapWidth int) (line, col int) {
	textRuneCount := utf8.RuneCountInString(text)
	if runePos > textRuneCount {
		runePos = textRuneCount
	}
	if text == "" {
		return 0, 0
	}

	_, breaks := s.wrapTextPreservingSpaces(text, wrapWidth)

	// Find which line the cursor is on
	lineStart := 0
	for i, breakPos := range breaks {
		if runePos < breakPos {
			return i, runePos - lineStart
		}
		lineStart = breakPos
	}

	return len(breaks), runePos - lineStart
}

// updateViewOffset adjusts viewOffset to keep the cursor line visible.
func (s *SuggestionTextInput) updateViewOffset(cursorLine, totalLines int) {
	if totalLines <= s.maxVisibleLines {
		s.viewOffset = 0
		return
	}

	// Cursor above visible area - scroll up
	if cursorLine < s.viewOffset {
		s.viewOffset = cursorLine
	}

	// Cursor below visible area - scroll down
	if cursorLine >= s.viewOffset+s.maxVisibleLines {
		s.viewOffset = cursorLine - s.maxVisibleLines + 1
	}

	// Clamp viewOffset
	maxOffset := totalLines - s.maxVisibleLines
	if s.viewOffset > maxOffset {
		s.viewOffset = maxOffset
	}
	if s.viewOffset < 0 {
		s.viewOffset = 0
	}
}

// getWrappedLines returns the wrapped lines for display, preserving all whitespace.
func (s *SuggestionTextInput) getWrappedLines(value string, wrapWidth int) []string {
	lines, _ := s.wrapTextPreservingSpaces(value, wrapWidth)
	return lines
}

// renderMultiLineView renders the input as a multi-line wrapped view with cursor.
func (s *SuggestionTextInput) renderMultiLineView() string {
	value := s.InputModel.Value()
	prompt := s.InputModel.Prompt
	promptWidth := lipgloss.Width(prompt)

	// Calculate wrap width (total width minus prompt, minus 1 for cursor at end of line)
	wrapWidth := s.width - promptWidth - 1
	if wrapWidth <= 0 {
		wrapWidth = 1
	}

	// Handle empty input with placeholder
	if value == "" {
		placeholder := s.InputModel.PlaceholderStyle.Render(s.InputModel.Placeholder)
		cursorChar := ""
		if s.InputModel.Focused() && !s.InputModel.Cursor.Blink {
			cursorChar = s.cursorStyle().Render(" ")
		}
		return s.InputModel.PromptStyle.Render(prompt) + cursorChar + placeholder
	}

	// Get wrapped lines (preserving all whitespace) for display
	lines := s.getWrappedLines(value, wrapWidth)
	totalLines := len(lines)

	// Find cursor position in wrapped text
	cursorLine, cursorCol := s.cursorLineCol(value, s.InputModel.Position(), wrapWidth)

	// Update scroll offset to keep cursor visible
	s.updateViewOffset(cursorLine, totalLines)

	// Determine visible line range
	endLine := s.viewOffset + s.maxVisibleLines
	if endLine > totalLines {
		endLine = totalLines
	}
	visibleLines := lines[s.viewOffset:endLine]

	// Build scroll indicators
	showUpArrow := s.viewOffset > 0
	showDownArrow := endLine < totalLines

	// Render each visible line
	var result strings.Builder
	promptPadding := strings.Repeat(" ", promptWidth)

	for i, line := range visibleLines {
		actualLineIdx := s.viewOffset + i

		// Add newline between lines (not before first)
		if i > 0 {
			result.WriteString("\n")
		}

		// Prompt only on first line (when viewOffset is 0), padding otherwise
		if actualLineIdx == 0 {
			result.WriteString(s.InputModel.PromptStyle.Render(prompt))
		} else {
			result.WriteString(promptPadding)
		}

		// Render line content with cursor if this is the cursor line
		if actualLineIdx == cursorLine {
			result.WriteString(s.renderLineWithCursor(line, cursorCol))
		} else {
			result.WriteString(s.InputModel.TextStyle.Render(line))
		}
	}

	// Add scroll indicators
	if showUpArrow || showDownArrow {
		indicators := ""
		if showUpArrow {
			indicators += "↑"
		}
		if showDownArrow {
			if showUpArrow {
				indicators += "/"
			}
			indicators += "↓"
		}
		result.WriteString(" " + lipgloss.NewStyle().Faint(true).Render(indicators))
	}

	return result.String()
}

// cursorStyle returns the style to use for the cursor character.
// Falls back to reverse video if cursor style is empty.
func (s *SuggestionTextInput) cursorStyle() lipgloss.Style {
	// If cursor style has no settings, use reverse video as default
	if s.InputModel.Cursor.Style.Value() == "" {
		return lipgloss.NewStyle().Reverse(true)
	}
	return s.InputModel.Cursor.Style
}

// renderLineWithCursor renders a single line with the cursor at the specified column.
func (s *SuggestionTextInput) renderLineWithCursor(line string, cursorCol int) string {
	runes := []rune(line)
	lineLen := len(runes)
	curStyle := s.cursorStyle()

	// Cursor at end of line
	if cursorCol >= lineLen {
		rendered := s.InputModel.TextStyle.Render(line)
		if s.InputModel.Focused() && !s.InputModel.Cursor.Blink {
			rendered += curStyle.Render(" ")
		}
		return rendered
	}

	// Cursor within line
	before := string(runes[:cursorCol])
	cursorRune := string(runes[cursorCol])
	after := string(runes[cursorCol+1:])

	var result strings.Builder
	result.WriteString(s.InputModel.TextStyle.Render(before))

	if s.InputModel.Focused() && !s.InputModel.Cursor.Blink {
		result.WriteString(curStyle.Render(cursorRune))
	} else {
		result.WriteString(s.InputModel.TextStyle.Render(cursorRune))
	}

	result.WriteString(s.InputModel.TextStyle.Render(after))

	return result.String()
}
