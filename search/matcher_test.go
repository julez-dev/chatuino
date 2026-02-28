package search

import (
	"testing"

	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func msg(displayName, message string) *twitchirc.PrivateMessage {
	return &twitchirc.PrivateMessage{
		DisplayName: displayName,
		Message:     message,
	}
}

func TestContentMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		term string
		msg  *twitchirc.PrivateMessage
		want bool
	}{
		{name: "exact match", term: "hello", msg: msg("user", "hello"), want: true},
		{name: "substring", term: "ell", msg: msg("user", "hello"), want: true},
		{name: "case insensitive", term: "HELLO", msg: msg("user", "hello world"), want: true},
		{name: "no match", term: "xyz", msg: msg("user", "hello"), want: false},
		{name: "empty message", term: "test", msg: msg("user", ""), want: false},
		{name: "empty term matches all", term: "", msg: msg("user", "anything"), want: true},
		{name: "does not match username", term: "julez", msg: msg("julez", "hello"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewContentMatcher(tt.term)
			require.Equal(t, tt.want, m.Match(tt.msg))
		})
	}
}

func TestUserMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		term string
		msg  *twitchirc.PrivateMessage
		want bool
	}{
		{name: "exact match", term: "nightbot", msg: msg("Nightbot", ""), want: true},
		{name: "substring", term: "night", msg: msg("Nightbot", ""), want: true},
		{name: "case insensitive", term: "NIGHTBOT", msg: msg("Nightbot", ""), want: true},
		{name: "no match", term: "streamelements", msg: msg("Nightbot", ""), want: false},
		{name: "does not match content", term: "hello", msg: msg("user", "hello"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewUserMatcher(tt.term)
			require.Equal(t, tt.want, m.Match(tt.msg))
		})
	}
}

func TestDefaultMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		term string
		msg  *twitchirc.PrivateMessage
		want bool
	}{
		{name: "matches content", term: "hello", msg: msg("user", "hello world"), want: true},
		{name: "matches username", term: "julez", msg: msg("julez", "something"), want: true},
		{name: "matches either", term: "test", msg: msg("tester", "testing"), want: true},
		{name: "no match", term: "xyz", msg: msg("user", "hello"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewDefaultMatcher(tt.term)
			require.Equal(t, tt.want, m.Match(tt.msg))
		})
	}
}

func TestRegexMatcher(t *testing.T) {
	t.Parallel()

	t.Run("matches content", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcher("hel+o")
		require.NoError(t, err)
		require.True(t, m.Match(msg("user", "helllllo world")))
	})

	t.Run("matches username", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcher("^Night")
		require.NoError(t, err)
		require.True(t, m.Match(msg("Nightbot", "")))
	})

	t.Run("case insensitive", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcher("hello")
		require.NoError(t, err)
		require.True(t, m.Match(msg("user", "HELLO WORLD")))
	})

	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcher("^exact$")
		require.NoError(t, err)
		require.False(t, m.Match(msg("user", "not exact match")))
	})

	t.Run("invalid pattern", func(t *testing.T) {
		t.Parallel()
		_, err := NewRegexMatcher("[invalid")
		require.Error(t, err)
	})

	t.Run("field content only", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcherField("^hello", FieldContent)
		require.NoError(t, err)
		require.True(t, m.Match(msg("user", "hello world")))
		require.False(t, m.Match(msg("hello_user", "goodbye")))
	})

	t.Run("field user only", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcherField("^Night", FieldUser)
		require.NoError(t, err)
		require.True(t, m.Match(msg("Nightbot", "random")))
		require.False(t, m.Match(msg("user", "Nightbot said hi")))
	})

	t.Run("field any matches both", func(t *testing.T) {
		t.Parallel()
		m, err := NewRegexMatcherField("test", FieldAny)
		require.NoError(t, err)
		require.True(t, m.Match(msg("tester", "nope")))
		require.True(t, m.Match(msg("nope", "testing")))
		require.False(t, m.Match(msg("nope", "nope")))
	})
}

func TestBadgeMatcher(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		badge  string
		badges []twitchirc.Badge
		want   bool
	}{
		{
			name:   "has badge",
			badge:  "moderator",
			badges: []twitchirc.Badge{{Name: "moderator", Version: "1"}, {Name: "subscriber", Version: "12"}},
			want:   true,
		},
		{
			name:   "case insensitive",
			badge:  "MOD",
			badges: []twitchirc.Badge{{Name: "moderator", Version: "1"}},
			want:   true,
		},
		{
			name:   "substring match",
			badge:  "sub",
			badges: []twitchirc.Badge{{Name: "subscriber", Version: "1"}},
			want:   true,
		},
		{
			name:   "no match",
			badge:  "vip",
			badges: []twitchirc.Badge{{Name: "moderator", Version: "1"}},
			want:   false,
		},
		{
			name:   "empty badges",
			badge:  "mod",
			badges: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := NewBadgeMatcher(tt.badge)
			require.Equal(t, tt.want, m.Match(&twitchirc.PrivateMessage{Badges: tt.badges}))
		})
	}
}

func TestPropertyMatcher(t *testing.T) {
	t.Parallel()

	t.Run("mod true", func(t *testing.T) {
		t.Parallel()
		m := NewPropertyMatcher(PropertyMod)
		require.True(t, m.Match(&twitchirc.PrivateMessage{Mod: true}))
	})

	t.Run("mod false", func(t *testing.T) {
		t.Parallel()
		m := NewPropertyMatcher(PropertyMod)
		require.False(t, m.Match(&twitchirc.PrivateMessage{Mod: false}))
	})

	t.Run("sub", func(t *testing.T) {
		t.Parallel()
		m := NewPropertyMatcher(PropertySub)
		require.True(t, m.Match(&twitchirc.PrivateMessage{Subscriber: true}))
		require.False(t, m.Match(&twitchirc.PrivateMessage{Subscriber: false}))
	})

	t.Run("vip", func(t *testing.T) {
		t.Parallel()
		m := NewPropertyMatcher(PropertyVIP)
		require.True(t, m.Match(&twitchirc.PrivateMessage{VIP: true}))
	})

	t.Run("first", func(t *testing.T) {
		t.Parallel()
		m := NewPropertyMatcher(PropertyFirst)
		require.True(t, m.Match(&twitchirc.PrivateMessage{FirstMsg: true}))
	})

	t.Run("unknown property", func(t *testing.T) {
		t.Parallel()
		m := NewPropertyMatcher(Property(99))
		require.False(t, m.Match(&twitchirc.PrivateMessage{}))
	})
}

func TestAndMatcher(t *testing.T) {
	t.Parallel()

	t.Run("all match", func(t *testing.T) {
		t.Parallel()
		m := NewAndMatcher(
			NewContentMatcher("hello"),
			NewUserMatcher("julez"),
		)
		require.True(t, m.Match(msg("julez", "hello world")))
	})

	t.Run("short circuits on first failure", func(t *testing.T) {
		t.Parallel()
		m := NewAndMatcher(
			NewContentMatcher("xyz"),
			NewUserMatcher("julez"),
		)
		require.False(t, m.Match(msg("julez", "hello")))
	})

	t.Run("empty matchers matches all", func(t *testing.T) {
		t.Parallel()
		m := NewAndMatcher()
		require.True(t, m.Match(msg("user", "hello")))
	})
}

func TestNotMatcher(t *testing.T) {
	t.Parallel()

	t.Run("inverts true to false", func(t *testing.T) {
		t.Parallel()
		m := NewNotMatcher(NewContentMatcher("hello"))
		require.False(t, m.Match(msg("user", "hello")))
	})

	t.Run("inverts false to true", func(t *testing.T) {
		t.Parallel()
		m := NewNotMatcher(NewContentMatcher("hello"))
		require.True(t, m.Match(msg("user", "goodbye")))
	})
}
