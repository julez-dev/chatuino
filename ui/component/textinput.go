package component

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	trie "github.com/Vivino/go-autocomplete-trie"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var commandSuggestions = [...]string{
	"/ban <user> [reason]",
	"/unban <user>",
	"/timeout <username> [duration] [reason]",
	"/inspect <username>",
	"/popupchat",
	"/channel",
	"/banrequests",
	"/pyramid <word> <count>",
	"/localsubscribers",
	"/localsubscribersoff",
	"/uniqueonly",
	"/uniqueonlysoff",
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

	history                   []string
	historyIndex              int
	IncludeCommandSuggestions bool

	userCache map[string]func(...string) string // [username]render func
}

func defaultTrie() *trie.Trie {
	t := trie.New()
	t = t.WithoutFuzzy()
	t = t.WithoutLevenshtein()
	// t = t.WithoutNormalisation()
	return t
}

// NewSuggestionTextInput creates a new model with default settings.
func NewSuggestionTextInput(userCache map[string]func(...string) string) *SuggestionTextInput {
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
	}
}

func (s *SuggestionTextInput) Update(msg tea.Msg) (*SuggestionTextInput, tea.Cmd) {
	if !s.InputModel.Focused() {
		return s, nil
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "enter":
			s.history = append(s.history, s.InputModel.Value())
			s.historyIndex = len(s.history)
			return s, nil
		case key.Matches(msg, s.KeyMap.PrevSuggestion) && (slices.Contains(s.history, s.InputModel.Value()) || s.InputModel.Value() == ""):
			s.historyIndex--

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
		case key.Matches(msg, s.KeyMap.NextSuggestion) && (slices.Contains(s.history, s.InputModel.Value()) || s.InputModel.Value() == ""):
			s.historyIndex++

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

			s.InputModel.SetValue(before + suggestion + after)
			s.InputModel.SetCursor(len(before) + len(suggestion) + 1) // set cursor to end of suggestion + 1 for space

			return s, nil
		case key.Matches(msg, s.KeyMap.NextSuggestion):
			s.nextSuggestion()
		case key.Matches(msg, s.KeyMap.PrevSuggestion):
			s.previousSuggestion()
		default:
			s.updateSuggestions()
		}
	}

	s.InputModel, cmd = s.InputModel.Update(msg)

	return s, cmd
}

func (s *SuggestionTextInput) View() string {
	if s.canAcceptSuggestion() {
		suggestion := s.suggestions[s.suggestionIndex]

		// If the suggestion is a username, render it with the users color function
		if renderFunc, ok := s.userCache[strings.TrimPrefix(suggestion, "@")]; ok {
			suggestion = renderFunc(suggestion)
		}

		return fmt.Sprintf(" %s (%dx)\n%s", suggestion, len(s.suggestions), s.InputModel.View())
	}

	return "\n" + s.InputModel.View()
}

func (s *SuggestionTextInput) Blur() {
	s.InputModel.Blur()
}

func (s *SuggestionTextInput) Focus() {
	s.InputModel.Focus()
}

func (s *SuggestionTextInput) SetWidth(width int) {
	s.InputModel.Width = width - 3 // -3 for prompt
}

func (s *SuggestionTextInput) Value() string {
	return s.InputModel.Value()
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
	return len(word) > 2 && len(s.suggestions) > 0 && s.suggestions[s.suggestionIndex] != word
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
	if s.IncludeCommandSuggestions && strings.HasPrefix(currWord, "/") && startIndex == 0 {
		for _, suggestion := range commandSuggestions {
			if strings.Contains(suggestion, currWord) {
				s.suggestions = append(s.suggestions, suggestion)
			}
		}
	}

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
		s.suggestionIndex = len(s.suggestions) - 1
	}
}

func selectWordAtIndex(sentence string, index int) (string, int, int) {
	if index > len(sentence) || sentence == "" {
		return "", 0, 0
	}

	before, after := sentence[:index], sentence[index:]

	spaceIndexBefore := strings.LastIndex(before, " ")

	if spaceIndexBefore == -1 {
		spaceIndexBefore = 0
	} else {
		spaceIndexBefore++
	}

	spaceIndexAfter := strings.Index(after, " ")

	if spaceIndexAfter == -1 {
		spaceIndexAfter = index + len(after)
	} else {
		spaceIndexAfter = index + spaceIndexAfter
	}

	return sentence[spaceIndexBefore:spaceIndexAfter], spaceIndexBefore, spaceIndexAfter
}
