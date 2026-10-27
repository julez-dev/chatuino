package tab

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/chat"
	"github.com/julez-dev/chatuino/ui/component"
)

// composerMaxLines is the number of rows the input text may grow to before it
// starts scrolling.
const composerMaxLines = 3

// ReplyTarget is the message the next send answers. It is plain data so the
// composer never needs an entry handle or the transcript.
type ReplyTarget struct {
	MessageID   string
	DisplayName string
	Snippet     string
}

// Submission is what the composer hands to the tab when the user sends. The tab
// decides whether the text is a slash command or a chat message and whether the
// account may send at all.
type Submission struct {
	Text    string
	ReplyTo string
}

// SuggestionConfig is the complete suggestion state. Configure applies it in one
// call so the composer cannot hold a half-updated mix of channels, emotes,
// moderator commands and key bindings.
type SuggestionConfig struct {
	Channels                 []string
	Emotes                   []string
	IncludeModeratorCommands bool
	KeyMap                   save.KeyMap
}

// composer owns the draft, the armed reply, the suggestions and the frame that
// shows them. It does not know about slash-command routing, moderator mode or
// connection state.
type composer struct {
	input  *component.SuggestionTextInput
	reply  *ReplyTarget
	config SuggestionConfig

	width int // outer width including the border columns

	borderStyle lipgloss.Style
	accentStyle lipgloss.Style
	dimmedStyle lipgloss.Style
}

// newComposer builds the composer. emoteReplacer may be nil to disable inline
// emote image replacement.
func newComposer(
	userColors component.UserColors,
	customSuggestions map[string]string,
	theme save.Theme,
	emoteReplacer component.Replacer,
) *composer {
	input := component.NewSuggestionTextInput(userColors, customSuggestions)
	input.EmoteReplacer = emoteReplacer
	input.SetMaxVisibleLines(composerMaxLines)

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor))

	styles := input.InputModel.Styles()
	styles.Focused.Prompt = accent
	input.InputModel.SetStyles(styles)

	return &composer{
		input:       input,
		borderStyle: accent,
		accentStyle: accent,
		dimmedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor)),
	}
}

// Configure replaces the complete suggestion state. Sources that did not change
// are left alone, so an emote refresh cannot reset the channel suggestions or the
// user's current selection.
func (c *composer) Configure(config SuggestionConfig) {
	if !slices.Equal(c.config.Channels, config.Channels) {
		c.input.SetChannelSuggestions(config.Channels)
	}
	if !slices.Equal(c.config.Emotes, config.Emotes) {
		c.input.SetSuggestions(config.Emotes)
	}

	c.input.IncludeModeratorCommands = config.IncludeModeratorCommands
	c.input.KeyMap = component.KeyMap{
		AcceptSuggestion: config.KeyMap.AcceptSuggestion,
		NextSuggestion:   config.KeyMap.NextSuggestion,
		PrevSuggestion:   config.KeyMap.PrevSuggestion,
	}
	c.config = config
}

// Update forwards msg to the text input. heightChanged reports whether the
// composer now occupies a different number of rows than before.
func (c *composer) Update(msg tea.Msg) (cmd tea.Cmd, heightChanged bool) {
	before := c.input.LineCount()
	c.input, cmd = c.input.Update(msg)

	return cmd, c.input.LineCount() != before
}

// View renders the reply indicator (if armed) and the framed input.
func (c *composer) View() string {
	inputView := c.input.View()

	topLabel := "[ Chat ]"
	charCount := fmt.Sprintf("[ %d / %d ]", utf8.RuneCountInString(c.input.InputModel.Value()), c.input.InputModel.CharLimit)

	innerWidth := c.width - 2

	topFill := max(0, innerWidth-len(topLabel)-2)
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill) + "─┐"

	bottomFill := max(0, innerWidth-len(charCount)-2)
	bottomBorder := "└─" + strings.Repeat("─", bottomFill) + charCount + "─┘"

	inputLines := strings.Split(inputView, "\n")
	borderedLines := make([]string, 0, len(inputLines))
	for _, line := range inputLines {
		padNeeded := max(0, innerWidth-lipgloss.Width(line))
		borderedLines = append(borderedLines, "│"+line+strings.Repeat(" ", padNeeded)+"│")
	}

	var b strings.Builder

	if indicator := c.replyView(); indicator != "" {
		b.WriteString(indicator)
		b.WriteString("\n")
	}

	b.WriteString(c.borderStyle.Render(topBorder))
	b.WriteString("\n")
	b.WriteString(c.borderStyle.Render(strings.Join(borderedLines, "\n")))
	b.WriteString("\n")
	b.WriteString(c.borderStyle.Render(bottomBorder))

	return b.String()
}

// Height returns the number of rows View currently renders.
func (c *composer) Height() int {
	return lipgloss.Height(c.View())
}

// SetWidth sets the outer width of the composer, including the border columns.
func (c *composer) SetWidth(width int) {
	c.width = width
	c.input.SetWidth(width - 2)
}

// Focus focuses the text input and returns the cursor blink command.
func (c *composer) Focus() tea.Cmd {
	return c.input.InputModel.Focus()
}

func (c *composer) Blur() {
	c.input.Blur()
}

// Focused reports whether the composer currently owns the keyboard.
func (c *composer) Focused() bool {
	return c.input.InputModel.Focused()
}

// Value returns the trimmed input text.
func (c *composer) Value() string {
	return c.input.Value()
}

// RawValue returns the input text without trimming.
func (c *composer) RawValue() string {
	return c.input.InputModel.Value()
}

func (c *composer) SetValue(value string) {
	c.input.SetValue(value)
}

// Clear empties the input text.
func (c *composer) Clear() {
	c.input.SetValue("")
}

// IsAcceptSuggestionKey reports whether msg accepts the current suggestion.
func (c *composer) IsAcceptSuggestionKey(msg tea.KeyPressMsg) bool {
	return key.Matches(msg, c.input.KeyMap.AcceptSuggestion)
}

// ArmReply targets a message for the next send attempt.
func (c *composer) ArmReply(messageID, displayName, message string) {
	c.reply = &ReplyTarget{
		MessageID:   messageID,
		DisplayName: displayName,
		Snippet:     chat.Snippet(message),
	}
}

// ReplyArmed reports whether a reply is armed.
func (c *composer) ReplyArmed() bool {
	return c.reply != nil
}

// TakeReply returns the armed reply's message ID and disarms it.
func (c *composer) TakeReply() string {
	if c.reply == nil {
		return ""
	}

	id := c.reply.MessageID
	c.reply = nil

	return id
}

// ClearReply disarms the armed reply and reports whether one was armed.
func (c *composer) ClearReply() bool {
	if c.reply == nil {
		return false
	}

	c.reply = nil

	return true
}

// Submit returns the draft and the reply it answers, consuming the reply so it
// can only ever be used by one send.
func (c *composer) Submit() Submission {
	return Submission{
		Text:    c.Value(),
		ReplyTo: c.TakeReply(),
	}
}

func (c *composer) replyView() string {
	if c.reply == nil {
		return ""
	}

	const hint = "(esc to cancel)"

	label := " ↩ Replying to @" + c.reply.DisplayName + ": "
	snippet := chat.TruncateToWidth(c.reply.Snippet, c.width-lipgloss.Width(label)-lipgloss.Width(hint)-1)
	gap := max(1, c.width-lipgloss.Width(label)-lipgloss.Width(snippet)-lipgloss.Width(hint))

	return c.accentStyle.Render(label) + c.dimmedStyle.Render(snippet) + strings.Repeat(" ", gap) + c.dimmedStyle.Render(hint)
}
