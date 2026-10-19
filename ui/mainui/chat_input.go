package mainui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/ui/component"
)

// chatInputMaxLines is the number of rows the input text may grow to before it
// starts scrolling.
const chatInputMaxLines = 3

// chatInput is the message composer at the bottom of a broadcast tab: a
// suggestion-aware text input wrapped in a labelled border with a character
// counter, plus the optional reply indicator rendered above the frame.
//
// Its height depends on the typed text (soft wrapping) and on whether a reply is
// armed. Update reports when the height changed, so the owner can relayout.
type chatInput struct {
	input *component.SuggestionTextInput
	reply replyState

	width int // outer width including the border columns

	borderStyle lipgloss.Style
	accentStyle lipgloss.Style
	dimmedStyle lipgloss.Style
}

// newChatInput builds the composer. emoteReplacer may be nil to disable inline
// emote image replacement.
func newChatInput(
	userColors map[string]func(...string) string,
	customSuggestions map[string]string,
	theme save.Theme,
	emoteReplacer component.Replacer,
) *chatInput {
	input := component.NewSuggestionTextInput(userColors, customSuggestions)
	input.EmoteReplacer = emoteReplacer
	input.SetMaxVisibleLines(chatInputMaxLines)

	accent := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.InputPromptColor))

	styles := input.InputModel.Styles()
	styles.Focused.Prompt = accent
	input.InputModel.SetStyles(styles)

	return &chatInput{
		input:       input,
		borderStyle: accent,
		accentStyle: accent,
		dimmedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor)),
	}
}

// Update forwards msg to the text input. heightChanged reports whether the
// composer now occupies a different number of rows than before.
func (c *chatInput) Update(msg tea.Msg) (cmd tea.Cmd, heightChanged bool) {
	before := c.input.LineCount()
	c.input, cmd = c.input.Update(msg)

	return cmd, c.input.LineCount() != before
}

// View renders the reply indicator (if armed) and the framed input.
func (c *chatInput) View() string {
	inputView := c.input.View()

	// Labels
	topLabel := "[ Chat ]"
	charCount := fmt.Sprintf("[ %d / %d ]", utf8.RuneCountInString(c.input.InputModel.Value()), c.input.InputModel.CharLimit)

	innerWidth := c.width - 2 // -2 for left/right border chars

	// Top border: ┌─[ Chat ]─────...─┐
	topFill := max(0, innerWidth-len(topLabel)-2)
	topBorder := "┌─" + topLabel + strings.Repeat("─", topFill) + "─┐"

	// Bottom border: └─────...─[ 7 / 500 ]─┘ (counter on RIGHT)
	bottomFill := max(0, innerWidth-len(charCount)-2)
	bottomBorder := "└─" + strings.Repeat("─", bottomFill) + charCount + "─┘"

	// Wrap input lines with │ borders
	inputLines := strings.Split(inputView, "\n")
	borderedLines := make([]string, 0, len(inputLines))
	for _, line := range inputLines {
		padNeeded := max(0, innerWidth-lipgloss.Width(line))
		borderedLines = append(borderedLines, "│"+line+strings.Repeat(" ", padNeeded)+"│")
	}

	var b strings.Builder

	if indicator := c.reply.View(c.width, c.accentStyle, c.dimmedStyle); indicator != "" {
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
func (c *chatInput) Height() int {
	return lipgloss.Height(c.View())
}

// SetWidth sets the outer width of the composer, including the border columns.
func (c *chatInput) SetWidth(width int) {
	c.width = width
	c.input.SetWidth(width - 2)
}

// Focus focuses the text input and returns the cursor blink command.
func (c *chatInput) Focus() tea.Cmd {
	return c.input.InputModel.Focus()
}

func (c *chatInput) Blur() {
	c.input.Blur()
}

// Value returns the trimmed input text.
func (c *chatInput) Value() string {
	return c.input.Value()
}

// RawValue returns the input text without trimming.
func (c *chatInput) RawValue() string {
	return c.input.InputModel.Value()
}

func (c *chatInput) SetValue(value string) {
	c.input.SetValue(value)
}

// Clear empties the input text.
func (c *chatInput) Clear() {
	c.input.SetValue("")
}

// IsAcceptSuggestionKey reports whether msg is the key that accepts the current
// suggestion (and triggers template expansion).
func (c *chatInput) IsAcceptSuggestionKey(msg tea.KeyPressMsg) bool {
	return key.Matches(msg, c.input.KeyMap.AcceptSuggestion)
}

// SetSuggestionKeyMap applies the user-configured suggestion bindings to the
// composer.
func (c *chatInput) SetSuggestionKeyMap(keymap save.KeyMap) {
	c.input.KeyMap = component.KeyMap{
		AcceptSuggestion: keymap.AcceptSuggestion,
		NextSuggestion:   keymap.NextSuggestion,
		PrevSuggestion:   keymap.PrevSuggestion,
	}
}

func (c *chatInput) SetSuggestions(suggestions []string) {
	c.input.SetSuggestions(suggestions)
}

func (c *chatInput) SetChannelSuggestions(channels []string) {
	c.input.SetChannelSuggestions(channels)
}

// SetIncludeModeratorCommands toggles moderator slash commands in the suggestions.
func (c *chatInput) SetIncludeModeratorCommands(include bool) {
	c.input.IncludeModeratorCommands = include
}

// ArmReply targets the given message for the next send attempt.
func (c *chatInput) ArmReply(messageID, displayName, message string) {
	c.reply.Arm(messageID, displayName, message)
}

// ReplyArmed reports whether a reply is armed.
func (c *chatInput) ReplyArmed() bool {
	return c.reply.Armed()
}

// TakeReply returns the armed reply's message ID and disarms it. It returns ""
// when no reply is armed.
func (c *chatInput) TakeReply() string {
	return c.reply.Take()
}

// ClearReply disarms the armed reply and reports whether one was armed.
func (c *chatInput) ClearReply() bool {
	return c.reply.Clear()
}
