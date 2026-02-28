package search

import (
	"errors"
	"fmt"
	"strings"
)

// Parse converts a query string into a Matcher tree.
//
// Syntax:
//
//	content:term        message content substring
//	msg:term            alias for content:
//	user:term           username substring
//	from:term           alias for user:
//	badge:name          badge name substring
//	is:mod|sub|vip|first boolean property
//	regex:pattern       regex on content + username
//	/pattern/           shorthand for regex:
//	-prefix:value       negation
//	bare term           default (content OR username)
//	"quoted value"      treated as single bare term
//
// Multiple tokens are combined with implicit AND.
// Returns nil when the query is empty.
func Parse(query string) (Matcher, error) {
	tokens, err := tokenize(query)
	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, nil
	}

	matchers := make([]Matcher, 0, len(tokens))

	for _, tok := range tokens {
		m, err := tokenToMatcher(tok)
		if err != nil {
			return nil, err
		}

		matchers = append(matchers, m)
	}

	if len(matchers) == 1 {
		return matchers[0], nil
	}

	return NewAndMatcher(matchers...), nil
}

// token represents a parsed query token before it becomes a Matcher.
type token struct {
	prefix  string // "" for bare terms, "content", "user", etc.
	value   string
	negated bool
	isRegex bool // /pattern/ syntax
}

// tokenize splits a query string into structured tokens.
func tokenize(query string) ([]token, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var tokens []token
	i := 0

	for i < len(query) {
		// skip whitespace
		if query[i] == ' ' || query[i] == '\t' {
			i++
			continue
		}

		tok, advance, err := readToken(query[i:])
		if err != nil {
			return nil, err
		}

		if tok.value != "" || tok.prefix != "" {
			tokens = append(tokens, tok)
		}

		i += advance
	}

	return tokens, nil
}

// readToken reads a single token from the start of s and returns how many bytes were consumed.
func readToken(s string) (token, int, error) {
	var tok token
	i := 0

	// check negation prefix
	if len(s) > 1 && s[0] == '-' {
		tok.negated = true
		i++
	}

	remaining := s[i:]

	// check /regex/ syntax
	if len(remaining) > 0 && remaining[0] == '/' {
		return readRegexToken(s, i, tok.negated)
	}

	// check quoted string
	if len(remaining) > 0 && remaining[0] == '"' {
		return readQuotedToken(s, i, tok.negated)
	}

	// read a word (until whitespace)
	word, consumed := readWord(remaining)
	i += consumed

	// check for prefix:value
	if colonIdx := strings.IndexByte(word, ':'); colonIdx > 0 {
		tok.prefix = strings.ToLower(word[:colonIdx])
		tok.value = word[colonIdx+1:]

		return tok, i, nil
	}

	// bare word
	tok.value = word

	return tok, i, nil
}

// readRegexToken parses /pattern/ syntax.
func readRegexToken(s string, start int, negated bool) (token, int, error) {
	// start is at the position after optional '-', pointing to '/'
	i := start + 1 // skip opening '/'

	closingSlash := strings.IndexByte(s[i:], '/')
	if closingSlash == -1 {
		// no closing slash — treat the whole original input (including any '-' prefix) as a bare word
		word, consumed := readWord(s)

		return token{value: word}, consumed, nil
	}

	pattern := s[i : i+closingSlash]
	end := i + closingSlash + 1 // past closing '/'

	return token{
		value:   pattern,
		negated: negated,
		isRegex: true,
	}, end, nil
}

// readQuotedToken parses "quoted value" syntax.
func readQuotedToken(s string, start int, negated bool) (token, int, error) {
	i := start + 1 // skip opening '"'

	closingQuote := strings.IndexByte(s[i:], '"')
	if closingQuote == -1 {
		// no closing quote — take everything to end
		return token{
			value:   s[i:],
			negated: negated,
		}, len(s), nil
	}

	value := s[i : i+closingQuote]
	end := i + closingQuote + 1 // past closing '"'

	return token{
		value:   value,
		negated: negated,
	}, end, nil
}

// readWord reads until the next whitespace and returns the word and bytes consumed.
func readWord(s string) (string, int) {
	for i := range len(s) {
		if s[i] == ' ' || s[i] == '\t' {
			return s[:i], i
		}
	}

	return s, len(s)
}

// extractRegexValue checks if value is wrapped in /slashes/ and returns the inner pattern.
func extractRegexValue(value string) (string, bool) {
	if len(value) >= 2 && value[0] == '/' && value[len(value)-1] == '/' {
		return value[1 : len(value)-1], true
	}

	return "", false
}

var errEmptyProperty = errors.New("empty property value for is: filter")

// tokenToMatcher converts a parsed token into a Matcher.
func tokenToMatcher(tok token) (Matcher, error) {
	var m Matcher

	switch {
	case tok.isRegex:
		rm, err := NewRegexMatcher(tok.value)
		if err != nil {
			return nil, fmt.Errorf("invalid regex /%s/: %w", tok.value, err)
		}

		m = rm
	case tok.prefix == "":
		m = NewDefaultMatcher(tok.value)
	case tok.prefix == "content" || tok.prefix == "msg":
		if pattern, ok := extractRegexValue(tok.value); ok {
			rm, err := NewRegexMatcherField(pattern, FieldContent)
			if err != nil {
				return nil, fmt.Errorf("invalid regex in %s: %w", tok.prefix, err)
			}

			m = rm
		} else {
			m = NewContentMatcher(tok.value)
		}
	case tok.prefix == "user" || tok.prefix == "from":
		if pattern, ok := extractRegexValue(tok.value); ok {
			rm, err := NewRegexMatcherField(pattern, FieldUser)
			if err != nil {
				return nil, fmt.Errorf("invalid regex in %s: %w", tok.prefix, err)
			}

			m = rm
		} else {
			m = NewUserMatcher(tok.value)
		}
	case tok.prefix == "badge":
		m = NewBadgeMatcher(tok.value)
	case tok.prefix == "regex":
		rm, err := NewRegexMatcher(tok.value)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %w", err)
		}

		m = rm
	case tok.prefix == "is":
		pm, err := parsePropertyMatcher(tok.value)
		if err != nil {
			return nil, err
		}

		m = pm
	default:
		// unknown prefix — treat entire prefix:value as bare term
		m = NewDefaultMatcher(tok.prefix + ":" + tok.value)
	}

	if tok.negated {
		m = NewNotMatcher(m)
	}

	return m, nil
}

func parsePropertyMatcher(value string) (Matcher, error) {
	switch strings.ToLower(value) {
	case "mod":
		return NewPropertyMatcher(PropertyMod), nil
	case "sub":
		return NewPropertyMatcher(PropertySub), nil
	case "vip":
		return NewPropertyMatcher(PropertyVIP), nil
	case "first":
		return NewPropertyMatcher(PropertyFirst), nil
	default:
		if value == "" {
			return nil, errEmptyProperty
		}

		return nil, fmt.Errorf("unknown property %q (valid: mod, sub, vip, first)", value)
	}
}
