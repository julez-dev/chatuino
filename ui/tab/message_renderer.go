package tab

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/julez-dev/chatuino/kittyimg"
	"github.com/julez-dev/chatuino/save"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/reflow/wordwrap"
	"github.com/julez-dev/reflow/wrap"
)

// messageRenderer turns chat events into styled, wrapped terminal rows.
//
// It owns the formatting configuration and the caches derived from it (user
// colors, alert styles, timestamp widths) and has no access to the chat window's
// entries, selection, search state or viewport. Rendering is a pure function of
// the event and the current width, except for the user color cache, which is
// shared through the palette.
type messageRenderer struct {
	width          int // full chat window width, including the selection indicator
	indicatorWidth int // columns the selection indicator occupies
	settings       save.Settings

	// timestamp format and the widths derived from it; recomputed together so
	// wrapped-line padding cannot drift from the rendered prefix.
	timeFormatFunc   func(time.Time) string
	timeFormatWidth  int
	timePaddingWidth int

	palette *userPalette

	subAlertStyle       lipgloss.Style
	noticeAlertStyle    lipgloss.Style
	clearChatAlertStyle lipgloss.Style
	errorAlertStyle     lipgloss.Style
	dimmedStyle         lipgloss.Style
	// replyStyle marks a reply reference and names a parent whose color is unknown.
	replyStyle lipgloss.Style

	// pre-created styles for message modifiers (strikethrough, italic, both)
	strikethroughStyle       lipgloss.Style
	italicStyle              lipgloss.Style
	strikethroughItalicStyle lipgloss.Style
}

func newMessageRenderer(
	width, indicatorWidth int,
	settings save.Settings,
	theme save.Theme,
	palette *userPalette,
) *messageRenderer {
	r := &messageRenderer{
		width:          width,
		indicatorWidth: indicatorWidth,
		settings:       settings,
		palette:        palette,

		subAlertStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatSubAlertColor)).Bold(true),
		noticeAlertStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatNoticeAlertColor)).Bold(true),
		clearChatAlertStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatClearChatColor)).Bold(true),
		errorAlertStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatErrorColor)).Bold(true),
		dimmedStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color(theme.DimmedTextColor)),
		replyStyle:          lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ChatIndicatorColor)),

		strikethroughStyle:       lipgloss.NewStyle().Strikethrough(true).StrikethroughSpaces(false),
		italicStyle:              lipgloss.NewStyle().Italic(true),
		strikethroughItalicStyle: lipgloss.NewStyle().Strikethrough(true).StrikethroughSpaces(false).Italic(true),
	}

	r.SetTimeFormat(settings.Chat.TimeFormat)

	return r
}

// SetWidth sets the width messages are wrapped at.
func (r *messageRenderer) SetWidth(width int) {
	r.width = width
}

// SetTimeFormat replaces the timestamp format and recomputes the widths derived
// from it. The user inspect window uses a separate format for its history.
func (r *messageRenderer) SetTimeFormat(format string) {
	r.timeFormatFunc = func(t time.Time) string {
		return t.Local().Format(format)
	}

	// Using zero-time gives the maximum-length output for variable-width formats
	// (e.g. "3:04 PM" → "12:00 AM"), so padding may be one column wider than the
	// actual time for some hours — acceptable cosmetic tradeoff.
	r.timeFormatWidth = len(r.timeFormatFunc(time.Time{}))
	r.timePaddingWidth = r.timeFormatWidth + 3 // +3 matches the "  " prefix + " " separator
}

// Render formats one chat event into rendered rows ready for the viewport.
func (r *messageRenderer) Render(event ChatEventMessage) []string {
	switch msg := event.Message.(type) {
	case error:
		prefix := "  " + strings.Repeat(" ", r.timeFormatWidth) + " [" + r.errorAlertStyle.Render("Error") + "]: "
		text := strings.ReplaceAll(msg.Error(), "\n", "")
		return r.wrapMessage(prefix, text, event.DisplayModifier)
	case *twitchirc.PrivateMessage:
		userRenderFunc := r.palette.Renderer(msg.LoginName, msg.Color)

		// Build prefix components: time, [guest channel], [badges], username
		parts := []string{"  " + r.dimmedStyle.Render(r.timeFormatFunc(msg.TMISentTS))}

		if event.ChannelGuestDisplayName != "" {
			parts = append(parts, "|"+event.ChannelGuestDisplayName+"|")
		}

		if len(event.DisplayModifier.BadgeReplacement) > 0 && !r.settings.Chat.DisableBadges {
			badges := formatBadgeReplacement(r.settings, event.DisplayModifier.BadgeReplacement)
			if r.settings.Chat.GraphicBadges {
				// Hair space (U+200A) - narrower gap since badges have pixel padding
				parts = append(parts, badges+" "+userRenderFunc(msg.DisplayName)+": ")
			} else {
				parts = append(parts, badges)
				parts = append(parts, userRenderFunc(msg.DisplayName)+": ")
			}
		} else {
			parts = append(parts, userRenderFunc(msg.DisplayName)+": ")
		}
		prefix := strings.Join(parts, " ")

		ctx, isReply := r.replyContextFor(msg)

		body := msg.Message
		if isReply {
			body = stripReplyMention(body, ctx.ParentName, ctx.ParentLogin)
		}

		r.setUserColorModifier(body, &event.DisplayModifier)

		lines := r.wrapMessage(prefix, body, event.DisplayModifier)
		if !isReply {
			return lines
		}

		return append([]string{r.replyContextRow(ctx, event.DisplayModifier)}, lines...)
	case *twitchirc.Notice:
		title := "Notice"
		if event.IsFakeEvent {
			title = "Fake-Notice"
		}

		prefix := r.buildAlertPrefix(msg.FakeTimestamp, title, r.noticeAlertStyle)

		event.DisplayModifier.Italic = true
		r.setUserColorModifier(msg.Message, &event.DisplayModifier)

		return r.wrapMessage(prefix, msg.Message, event.DisplayModifier)
	case *twitchirc.ClearMessage:
		prefix := r.buildAlertPrefix(msg.TMISentTS, "Clear Message", r.clearChatAlertStyle)
		prefix += "A message from "
		text := msg.Login + " was removed."

		r.setUserColorModifier(text, &event.DisplayModifier)

		return r.wrapMessage(prefix, text, event.DisplayModifier)
	case *twitchirc.ClearChat:
		prefix := r.buildAlertPrefix(msg.TMISentTS, "Clear Chat", r.clearChatAlertStyle)

		if msg.TargetUserID == nil {
			return r.wrapMessage(prefix, "Clear chat prevented by Chatuino. Chat restored.", event.DisplayModifier)
		}

		text := *msg.UserName

		if msg.BanDuration != nil && *msg.BanDuration > 0 {
			text += " was timed out for " + humanizeDuration(time.Duration(*msg.BanDuration)*time.Second)
		} else {
			text += " was permanently banned."
		}

		r.setUserColorModifier(text, &event.DisplayModifier)

		return r.wrapMessage(prefix, text, event.DisplayModifier)
	case *twitchirc.SubMessage:
		prefix := r.buildAlertPrefix(msg.TMISentTS, "Sub Alert", r.subAlertStyle)

		subResubText := "subscribed"
		if msg.MsgID == "resub" {
			subResubText = "resubscribed"
		}

		_ = r.palette.Renderer(msg.Login, msg.Color)
		text := fmt.Sprintf("%s just %s with a %s subscription. (%d Months, %d Month Streak)",
			msg.DisplayName,
			subResubText,
			msg.SubPlan.String(),
			msg.CumulativeMonths,
			msg.StreakMonths,
		)

		// Append user message if present
		if msg.Message != "" {
			text += ": " + msg.Message
		}

		r.setUserColorModifier(text, &event.DisplayModifier)

		return r.wrapMessage(prefix, text, event.DisplayModifier)
	case *twitchirc.SubGiftMessage:
		prefix := r.buildAlertPrefix(msg.TMISentTS, "Sub Gift Alert", r.subAlertStyle)

		_ = r.palette.Renderer(msg.Login, msg.Color)

		text := fmt.Sprintf("%s gifted a %s sub to %s. (%d Months)",
			msg.DisplayName,
			msg.SubPlan.String(),
			msg.ReceiptDisplayName,
			msg.Months,
		)

		r.setUserColorModifier(text, &event.DisplayModifier)

		return r.wrapMessage(prefix, text, event.DisplayModifier)
	case *twitchirc.AnnouncementMessage:
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(msg.ParamColor.RGBHex())).Bold(true)
		prefix := "  " + r.timeFormatFunc(msg.TMISentTS) + " [" + style.Render("Announcement") + "] "

		_ = r.palette.Renderer(msg.Login, msg.Color)
		// Unlike other messages the announcer name is part of the wrapped body
		// (not the prefix). Build the raw text and let formatMessageText apply
		// emote/color replacements exactly once, matching the regular message path.
		text := fmt.Sprintf("%s: %s", msg.DisplayName, msg.Message)

		r.setUserColorModifier(text, &event.DisplayModifier)

		return r.wrapMessage(prefix, text, event.DisplayModifier)
	}

	return []string{}
}

// buildAlertPrefix creates a standardized prefix with timestamp and styled alert label.
// Example output: "  15:04:05 [Notice]: "
func (r *messageRenderer) buildAlertPrefix(timestamp time.Time, label string, style lipgloss.Style) string {
	return "  " + r.dimmedStyle.Render(r.timeFormatFunc(timestamp)) + " [" + style.Render(label) + "]: "
}

// wrapMessage formats one message body and wraps it into terminal rows.
//
// Replacements are applied before wrapping because they change the text length.
// The uniform (strikethrough/italic) style is applied after wrapping, per line:
// wrapping a styled string can put a line break between a style's start sequence
// and the character it belongs to, which leaks the attribute into the next line and
// drew a strike-through across the padding of wrapped links.
func (r *messageRenderer) wrapMessage(prefix, content string, modifier MessageContentModifier) []string {
	body, style := r.formatMessageText(content, modifier)

	return r.wordwrapMessage(prefix, body, style)
}

// formatMessageText applies word replacements to the message body and returns the
// style the renderer covers the whole body with, if the event is displayed with
// one. A styled body is returned unstyled: the caller applies the style per wrapped
// line.
func (r *messageRenderer) formatMessageText(content string, modifier MessageContentModifier) (string, *lipgloss.Style) {
	if modifier.Strikethrough && modifier.Italic {
		return content, &r.strikethroughItalicStyle
	}
	if modifier.Strikethrough {
		return content, &r.strikethroughStyle
	}
	if modifier.Italic {
		return content, &r.italicStyle
	}

	content = r.applyWordReplacements(content, modifier.WordReplacements)

	if modifier.MessageSuffix != "" {
		content += modifier.MessageSuffix
	}

	return content, nil
}

// applyWordReplacements applies word replacements from the display modifier to the given content.
// Replacements are matched per space-delimited token, mirroring how Replace builds the map.
// This prevents a single-character emote token from being substituted inside unrelated words
// (e.g. an "o" emote bleeding into "moon"/"pool").
func (r *messageRenderer) applyWordReplacements(content string, replacements WordReplacement) string {
	if len(replacements) == 0 {
		return content
	}

	words := strings.Split(content, " ")
	for i, word := range words {
		if replacement, ok := replacements[word]; ok {
			words[i] = replacement
		}
	}
	return strings.Join(words, " ")
}

// setUserColorModifier colors @-mentions of users whose color is already known.
// It writes to a copy of the replacement map so rendering never mutates the
// event data the entry retains.
func (r *messageRenderer) setUserColorModifier(content string, modifier *MessageContentModifier) {
	var replacements WordReplacement

	for _, word := range strings.Split(content, " ") {
		cleaned := strings.ToLower(stripDisplayNameEdges(word))
		renderFn, ok := r.palette.Lookup(cleaned)

		if !ok {
			// fallback try if empty
			if f, ok := r.palette.Lookup(word); ok {
				renderFn = f
			} else {
				continue
			}
		}

		if replacements == nil {
			replacements = maps.Clone(modifier.WordReplacements)
			if replacements == nil {
				replacements = make(WordReplacement)
			}
		}

		stripped := stripDisplayNameEdges(word)
		replacements[stripped] = renderFn(stripped)
		replacements[word] = renderFn(word)
	}

	if replacements != nil {
		modifier.WordReplacements = replacements
	}
}

func (r *messageRenderer) wordwrapMessage(prefix, content string, style *lipgloss.Style) []string {
	// Strip duplicate-bypass rune (U+E0000) used to bypass Twitch spam detection.
	// Guard with ContainsRune to avoid allocating when the rune isn't present (vast majority of messages).
	if strings.ContainsRune(content, duplicateBypass) {
		content = strings.Map(func(r rune) rune {
			if r == duplicateBypass {
				return -1
			}
			return r
		}, content)
	}

	prefixWidth := lipgloss.Width(prefix)

	// Assure that the prefix is at least prefixPadding wide
	if prefixWidth < prefixPadding {
		prefix = prefix + strings.Repeat(" ", prefixPadding-prefixWidth)
		prefixWidth = lipgloss.Width(prefix)
	}

	contentWidthLimit := r.width - r.indicatorWidth - prefixWidth

	// softwrap text to contentWidthLimit, if soft wrapping fails (for example in links) force break
	wrappedText := wrap.String(wordwrap.String(content, contentWidthLimit), contentWidthLimit)
	splits := strings.Split(wrappedText, "\n")

	lines := make([]string, 0, len(splits))
	lines = append(lines, prefix+styledBody(style, splits[0])) // first line is prefix + content at index 0

	// if there are more lines, add prefixPadding spaces to the beginning of the line
	for _, line := range splits[1:] {
		indent := strings.Repeat(" ", prefixWidth)
		if r.settings.Chat.DisablePaddingWrappedLines {
			indent = strings.Repeat(" ", r.timePaddingWidth)
		}

		lines = append(lines, indent+styledBody(style, line))
	}

	return expandGraphicRows(lines, graphicRows(r.settings))
}

// styledBody applies a uniform event style to one wrapped line of the body. The
// prefix (timestamp, badges, name) and the wrapped-line indentation stay unstyled.
func styledBody(style *lipgloss.Style, body string) string {
	if style == nil || body == "" {
		return body
	}

	return style.Render(body)
}

// formatBadgeReplacement renders the badge set for a message prefix. Text mode
// joins the labels; graphic mode concatenates the image placeholders, where the
// badges already carry their own padding.
func formatBadgeReplacement(settings save.Settings, replacements map[string]string) string {
	// Sort keys for deterministic badge order
	keys := slices.Sorted(maps.Keys(replacements))
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, replacements[k])
	}

	if !settings.Chat.GraphicBadges {
		return fmt.Sprintf("[%s]", strings.Join(values, ","))
	}

	return strings.Join(values, "")
}

// graphicRows returns the number of terminal rows graphic emotes/badges span, 1 if graphics are disabled.
func graphicRows(settings save.Settings) int {
	if !settings.Chat.GraphicEmotes && !settings.Chat.GraphicBadges {
		return 1
	}

	return max(settings.Chat.GraphicScale, 1)
}

// expandGraphicRows makes lines containing images rows high by inserting rows-1 lines above them.
// Images are bottom aligned with the text, their upper rows are rendered in the inserted lines.
// Lines without images are unaffected.
func expandGraphicRows(lines []string, rows int) []string {
	if rows <= 1 {
		return lines
	}

	expanded := make([]string, 0, len(lines)+rows-1)
	for _, line := range lines {
		expanded = append(expanded, kittyimg.PlaceholderRowsAbove(line, rows)...)
		expanded = append(expanded, line)
	}

	return expanded
}
