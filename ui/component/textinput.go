package component

import (
	"fmt"
	"reflect"
	"strings"

	trie "github.com/Vivino/go-autocomplete-trie"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	ti   textinput.Model

	KeyMap          KeyMap
	suggestionIndex int
	suggestions     []string
}

// NewSuggestionTextInput creates a new model with default settings.
func NewSuggestionTextInput() *SuggestionTextInput {
	input := textinput.New()
	input.Width = 20

	input.Validate = func(s string) error {
		if strings.ContainsRune(s, '\n') {
			return fmt.Errorf("disallowed input")
		}

		return nil
	}

	input.PromptStyle = input.PromptStyle.Foreground(lipgloss.Color("135"))
	t := trie.New()

	return &SuggestionTextInput{
		trie:   t,
		KeyMap: DefaultKeyMap,
		ti:     input,
	}
}

func (s *SuggestionTextInput) Update(msg tea.Msg) (*SuggestionTextInput, tea.Cmd) {
	if !s.ti.Focused() {
		return s, nil
	}

	var cmd tea.Cmd

	s.ti, cmd = s.ti.Update(msg)
	s.updateSuggestions()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, s.KeyMap.AcceptSuggestion):
			if s.canAcceptSuggestion() {
				tiVal := s.ti.Value()

				_, start, end := selectWordAtIndex(tiVal, s.ti.Position())
				before := tiVal[:start]
				after := tiVal[end:]

				suggestion := s.suggestions[s.suggestionIndex]
				s.ti.SetValue(before + suggestion + after)

				_, _, end = selectWordAtIndex(s.ti.Value(), s.ti.Position())
				s.ti.SetCursor(end)
			}

			return s, nil

		case key.Matches(msg, s.KeyMap.NextSuggestion):
			s.nextSuggestion()
		case key.Matches(msg, s.KeyMap.PrevSuggestion):
			s.previousSuggestion()
		}

		s.updateSuggestions()
	}

	return s, cmd
}

func (s *SuggestionTextInput) View() string {
	if s.canAcceptSuggestion() {
		word, _, end := selectWordAtIndex(s.ti.Value(), s.ti.Position())
		suggestion := s.suggestions[s.suggestionIndex]
		oldVal := s.ti.Value()

		var val string

		if len(s.suggestions) == 1 {
			val = s.ti.Value()[:end] + " |" + suggestion + "|" + s.ti.Value()[end:]
		} else {
			val = s.ti.Value()[:end] + " |" + suggestion + fmt.Sprintf("/%dx|", len(s.suggestions)) + s.ti.Value()[end:]
		}

		if suggestion != word {
			s.ti.SetValue(val)
		}

		view := s.ti.View()
		s.ti.SetValue(oldVal)
		return view
	}

	return s.ti.View()
}

func (s *SuggestionTextInput) Blur() {
	s.ti.Blur()
}

func (s *SuggestionTextInput) Focus() {
	s.ti.Focus()
}

func (s *SuggestionTextInput) SetWidth(width int) {
	s.ti.Width = width - 3 // -3 for prompt
}

func (s *SuggestionTextInput) Value() string {
	return s.ti.Value()
}

func (s *SuggestionTextInput) SetSuggestions(suggestions []string) {
	sugg := make([]string, len(suggestions))
	copy(sugg, suggestions)

	trie := trie.New()
	trie.Insert(sugg...)

	s.trie = trie

	s.suggestionIndex = 0
	s.updateSuggestions()
}

func (s *SuggestionTextInput) SetValue(val string) {
	s.ti.SetValue(val)
	s.suggestionIndex = 0
	s.updateSuggestions()
}

func (s *SuggestionTextInput) canAcceptSuggestion() bool {
	return len(s.suggestions) > 0
}

func (s *SuggestionTextInput) updateSuggestions() {
	if len(s.ti.Value()) <= 0 {
		s.suggestions = nil
		return
	}

	currWord, _, _ := selectWordAtIndex(s.ti.Value(), s.ti.Position())
	matches := s.trie.SearchAll(currWord)

	if !reflect.DeepEqual(matches, s.suggestions) {
		s.suggestionIndex = 0
	}

	s.suggestions = matches
}

func (s *SuggestionTextInput) nextSuggestion() {
	s.suggestionIndex = (s.suggestionIndex + 1)
	if s.suggestionIndex >= len(s.suggestions) {
		s.suggestionIndex = 0
	}
}

func (s *SuggestionTextInput) previousSuggestion() {
	s.suggestionIndex = (s.suggestionIndex - 1)
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
