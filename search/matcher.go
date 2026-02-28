// Package search provides composable chat message matchers
// that can be combined to build structured search queries.
package search

import (
	"regexp"
	"strings"

	"github.com/julez-dev/chatuino/twitch/twitchirc"
)

// Matcher tests whether a PrivateMessage satisfies a search criterion.
type Matcher interface {
	Match(msg *twitchirc.PrivateMessage) bool
}

// Property identifies a boolean field on PrivateMessage for property matching.
type Property int

const (
	PropertyMod   Property = iota // Mod field
	PropertySub                   // Subscriber field
	PropertyVIP                   // VIP field
	PropertyFirst                 // FirstMsg field
)

// ContentMatcher matches case-insensitive substrings in message content.
type ContentMatcher struct {
	search string // pre-lowered at construction
}

func NewContentMatcher(term string) *ContentMatcher {
	return &ContentMatcher{search: strings.ToLower(term)}
}

func (m *ContentMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	return strings.Contains(strings.ToLower(msg.Message), m.search)
}

// UserMatcher matches case-insensitive substrings in the display name.
type UserMatcher struct {
	search string // pre-lowered at construction
}

func NewUserMatcher(term string) *UserMatcher {
	return &UserMatcher{search: strings.ToLower(term)}
}

func (m *UserMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	return strings.Contains(strings.ToLower(msg.DisplayName), m.search)
}

// DefaultMatcher matches when either content or username contains the term.
// Preserves backward-compatible behavior with the old search.
type DefaultMatcher struct {
	content *ContentMatcher
	user    *UserMatcher
}

func NewDefaultMatcher(term string) *DefaultMatcher {
	return &DefaultMatcher{
		content: NewContentMatcher(term),
		user:    NewUserMatcher(term),
	}
}

func (m *DefaultMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	return m.content.Match(msg) || m.user.Match(msg)
}

// Field selects which message fields a matcher operates on.
type Field int

const (
	FieldAny     Field = iota // match content OR username
	FieldContent              // match message content only
	FieldUser                 // match username only
)

// RegexMatcher matches a compiled regular expression against selected fields.
type RegexMatcher struct {
	re    *regexp.Regexp
	field Field
}

// NewRegexMatcher compiles the pattern into a case-insensitive regex matching both content and username.
// Returns an error if the pattern is invalid.
func NewRegexMatcher(pattern string) (*RegexMatcher, error) {
	return NewRegexMatcherField(pattern, FieldAny)
}

// NewRegexMatcherField compiles the pattern into a case-insensitive regex scoped to the given field.
func NewRegexMatcherField(pattern string, field Field) (*RegexMatcher, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, err
	}

	return &RegexMatcher{re: re, field: field}, nil
}

func (m *RegexMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	switch m.field {
	case FieldContent:
		return m.re.MatchString(msg.Message)
	case FieldUser:
		return m.re.MatchString(msg.DisplayName)
	default:
		return m.re.MatchString(msg.Message) || m.re.MatchString(msg.DisplayName)
	}
}

// BadgeMatcher matches when the user has a badge whose name contains the term.
type BadgeMatcher struct {
	search string // pre-lowered at construction
}

func NewBadgeMatcher(name string) *BadgeMatcher {
	return &BadgeMatcher{search: strings.ToLower(name)}
}

func (m *BadgeMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	for _, b := range msg.Badges {
		if strings.Contains(strings.ToLower(b.Name), m.search) {
			return true
		}
	}

	return false
}

// PropertyMatcher matches a boolean property on the message.
type PropertyMatcher struct {
	prop Property
}

func NewPropertyMatcher(prop Property) *PropertyMatcher {
	return &PropertyMatcher{prop: prop}
}

func (m *PropertyMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	switch m.prop {
	case PropertyMod:
		return msg.Mod
	case PropertySub:
		return msg.Subscriber
	case PropertyVIP:
		return msg.VIP
	case PropertyFirst:
		return msg.FirstMsg
	default:
		return false
	}
}

// AndMatcher requires all child matchers to match. Short-circuits on first failure.
type AndMatcher struct {
	Matchers []Matcher
}

func NewAndMatcher(matchers ...Matcher) *AndMatcher {
	return &AndMatcher{Matchers: matchers}
}

func (m *AndMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	for _, child := range m.Matchers {
		if !child.Match(msg) {
			return false
		}
	}

	return true
}

// NotMatcher inverts the result of the inner matcher.
type NotMatcher struct {
	Inner Matcher
}

func NewNotMatcher(inner Matcher) *NotMatcher {
	return &NotMatcher{Inner: inner}
}

func (m *NotMatcher) Match(msg *twitchirc.PrivateMessage) bool {
	return !m.Inner.Match(msg)
}
