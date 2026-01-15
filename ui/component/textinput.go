package component

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"

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
	IncludeCommandSuggestions  bool
	IncludeModeratorCommands   bool
	DisableAutoSpaceSuggestion bool
	DisableHistory             bool
	EmoteReplacer              Replacer

	customSuggestions map[string]string
	emoteReplacements map[string]string // emoteText:unicode

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
			return s, nil
		case key.Matches(msg, s.KeyMap.PrevSuggestion) && s.InputModel.Value() == "":
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
		case key.Matches(msg, s.KeyMap.NextSuggestion) && s.InputModel.Value() == "":
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
	if s.canAcceptSuggestion() {
		suggestion := s.suggestions[s.suggestionIndex]

		// If the suggestion is a username, render it with the users color function
		if renderFunc, ok := s.userCache[strings.TrimPrefix(suggestion, "@")]; ok {
			suggestion = renderFunc(suggestion)
		}

		// current suggestion is emote and has a relacement
		if replace, ok := s.emoteReplacements[suggestion]; ok && replace != suggestion {
			return fmt.Sprintf(" %s %s (%dx)\n%s", suggestion, replace, len(s.suggestions), s.InputModel.View())
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
