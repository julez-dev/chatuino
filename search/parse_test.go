package search

import (
	"testing"

	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/stretchr/testify/require"
)

func TestParse_Structure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		query    string
		wantType string // expected top-level matcher type
		wantErr  bool
		wantNil  bool
	}{
		{name: "empty", query: "", wantNil: true},
		{name: "whitespace only", query: "   ", wantNil: true},
		{name: "bare term", query: "hello", wantType: "*search.DefaultMatcher"},
		{name: "content prefix", query: "content:hello", wantType: "*search.ContentMatcher"},
		{name: "msg prefix alias", query: "msg:hello", wantType: "*search.ContentMatcher"},
		{name: "user prefix", query: "user:julez", wantType: "*search.UserMatcher"},
		{name: "from prefix alias", query: "from:julez", wantType: "*search.UserMatcher"},
		{name: "badge prefix", query: "badge:mod", wantType: "*search.BadgeMatcher"},
		{name: "is:mod", query: "is:mod", wantType: "*search.PropertyMatcher"},
		{name: "is:sub", query: "is:sub", wantType: "*search.PropertyMatcher"},
		{name: "is:vip", query: "is:vip", wantType: "*search.PropertyMatcher"},
		{name: "is:first", query: "is:first", wantType: "*search.PropertyMatcher"},
		{name: "regex prefix", query: "regex:hel+o", wantType: "*search.RegexMatcher"},
		{name: "regex slash syntax", query: "/hel+o/", wantType: "*search.RegexMatcher"},
		{name: "negated", query: "-user:bot", wantType: "*search.NotMatcher"},
		{name: "two tokens AND", query: "user:julez content:gg", wantType: "*search.AndMatcher"},
		{name: "quoted value", query: `"hello world"`, wantType: "*search.DefaultMatcher"},
		{name: "unknown prefix as bare", query: "foo:bar", wantType: "*search.DefaultMatcher"},
		{name: "user with regex value", query: "user:/^julez$/", wantType: "*search.RegexMatcher"},
		{name: "content with regex value", query: "content:/^hello$/", wantType: "*search.RegexMatcher"},
		{name: "invalid regex", query: "/[invalid/", wantErr: true},
		{name: "invalid regex prefix", query: "regex:[bad", wantErr: true},
		{name: "invalid field regex", query: "user:/[bad/", wantErr: true},
		{name: "invalid is value", query: "is:unknown", wantErr: true},
		{name: "empty is value", query: "is:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := Parse(tt.query)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantNil {
				require.Nil(t, m)
				return
			}

			require.NotNil(t, m)

			typeName := typeNameOf(m)
			require.Equal(t, tt.wantType, typeName)
		})
	}
}

func TestParse_Behavior(t *testing.T) {
	t.Parallel()

	julez := &twitchirc.PrivateMessage{
		DisplayName: "julez",
		Message:     "hello world GG",
		Mod:         true,
		Subscriber:  true,
		Badges:      []twitchirc.Badge{{Name: "moderator", Version: "1"}},
	}

	nightbot := &twitchirc.PrivateMessage{
		DisplayName: "Nightbot",
		Message:     "!song currently playing: test",
		Subscriber:  false,
		FirstMsg:    false,
	}

	newChatter := &twitchirc.PrivateMessage{
		DisplayName: "newuser123",
		Message:     "hi chat!",
		FirstMsg:    true,
		VIP:         false,
	}

	tests := []struct {
		name  string
		query string
		msg   *twitchirc.PrivateMessage
		want  bool
	}{
		// bare terms — backward compatible
		{name: "bare matches content", query: "hello", msg: julez, want: true},
		{name: "bare matches username", query: "julez", msg: julez, want: true},
		{name: "bare no match", query: "xyz", msg: julez, want: false},

		// content: prefix
		{name: "content match", query: "content:GG", msg: julez, want: true},
		{name: "content no match user", query: "content:julez", msg: julez, want: false},
		{name: "msg alias", query: "msg:hello", msg: julez, want: true},

		// user: prefix
		{name: "user match", query: "user:julez", msg: julez, want: true},
		{name: "user no match content", query: "user:hello", msg: julez, want: false},
		{name: "from alias", query: "from:Night", msg: nightbot, want: true},

		// regex
		{name: "regex content", query: "/hel+o/", msg: julez, want: true},
		{name: "regex username", query: "/^Night/", msg: nightbot, want: true},
		{name: "regex no match", query: "/^exact$/", msg: julez, want: false},
		{name: "regex prefix", query: "regex:world$", msg: julez, want: false},
		{name: "regex prefix match", query: "regex:GG$", msg: julez, want: true},

		// is: properties
		{name: "is:mod match", query: "is:mod", msg: julez, want: true},
		{name: "is:mod no match", query: "is:mod", msg: nightbot, want: false},
		{name: "is:sub match", query: "is:sub", msg: julez, want: true},
		{name: "is:first match", query: "is:first", msg: newChatter, want: true},
		{name: "is:first no match", query: "is:first", msg: julez, want: false},

		// badge:
		{name: "badge match", query: "badge:moderator", msg: julez, want: true},
		{name: "badge no match", query: "badge:subscriber", msg: nightbot, want: false},

		// field-scoped regex
		{name: "user regex match", query: "user:/^julez$/", msg: julez, want: true},
		{name: "user regex no match content", query: "user:/hello/", msg: julez, want: false},
		{name: "content regex match", query: "content:/^hello/", msg: julez, want: true},
		{name: "content regex no match user", query: "content:/^julez$/", msg: julez, want: false},
		{name: "msg regex alias", query: "msg:/world/", msg: julez, want: true},
		{name: "from regex alias", query: "from:/^Night/", msg: nightbot, want: true},
		{name: "field regex combined", query: "user:/^julez$/ content:/GG/", msg: julez, want: true},

		// negation
		{name: "negated user match", query: "-user:nightbot", msg: julez, want: true},
		{name: "negated user no match", query: "-user:julez", msg: julez, want: false},
		{name: "negated is:mod", query: "-is:mod", msg: nightbot, want: true},

		// combined (AND)
		{name: "combined match", query: "user:julez content:GG", msg: julez, want: true},
		{name: "combined partial fail", query: "user:julez content:xyz", msg: julez, want: false},
		{name: "combined with negation", query: "content:hello -user:nightbot", msg: julez, want: true},
		{name: "three filters", query: "is:mod user:julez content:hello", msg: julez, want: true},

		// quoted values
		{name: "quoted bare", query: `"hello world"`, msg: julez, want: true},
		{name: "quoted no match", query: `"world hello"`, msg: julez, want: false},

		// negated regex
		{name: "negated regex excludes", query: "-/^Night/", msg: nightbot, want: false},
		{name: "negated regex passes", query: "-/^Night/", msg: julez, want: true},

		// empty prefix value matches all
		{name: "empty content prefix", query: "content:", msg: julez, want: true},
		{name: "empty user prefix", query: "user:", msg: julez, want: true},

		// multi-colon value
		{name: "multi colon value", query: "content:hello:world", msg: &twitchirc.PrivateMessage{Message: "hello:world"}, want: true},

		// unknown prefix treated as bare
		{name: "unknown prefix", query: "foo:bar", msg: &twitchirc.PrivateMessage{Message: "foo:bar"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := Parse(tt.query)
			require.NoError(t, err)
			require.NotNil(t, m)
			require.Equal(t, tt.want, m.Match(tt.msg))
		})
	}
}

func TestTokenize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		tokens []token
	}{
		{
			name:   "single bare word",
			input:  "hello",
			tokens: []token{{value: "hello"}},
		},
		{
			name:   "prefixed token",
			input:  "content:hello",
			tokens: []token{{prefix: "content", value: "hello"}},
		},
		{
			name:  "multiple tokens",
			input: "user:julez content:gg",
			tokens: []token{
				{prefix: "user", value: "julez"},
				{prefix: "content", value: "gg"},
			},
		},
		{
			name:   "negated",
			input:  "-user:bot",
			tokens: []token{{prefix: "user", value: "bot", negated: true}},
		},
		{
			name:   "regex slash",
			input:  "/pattern/",
			tokens: []token{{value: "pattern", isRegex: true}},
		},
		{
			name:   "negated regex",
			input:  "-/pattern/",
			tokens: []token{{value: "pattern", isRegex: true, negated: true}},
		},
		{
			name:   "quoted string",
			input:  `"hello world"`,
			tokens: []token{{value: "hello world"}},
		},
		{
			name:   "unclosed quote takes rest",
			input:  `"hello world`,
			tokens: []token{{value: "hello world"}},
		},
		{
			name:   "value with multiple colons",
			input:  "content:hello:world",
			tokens: []token{{prefix: "content", value: "hello:world"}},
		},
		{
			name:   "unclosed regex slash treated as bare word",
			input:  "/noclose",
			tokens: []token{{value: "/noclose"}},
		},
		{
			name:   "negated unclosed regex treated as bare word",
			input:  "-/noclose",
			tokens: []token{{value: "-/noclose"}},
		},
		{
			name:   "leading and trailing whitespace",
			input:  "  hello  ",
			tokens: []token{{value: "hello"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tokenize(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.tokens, got)
		})
	}
}

// typeNameOf returns a string representation of a matcher's concrete type for test assertions.
func typeNameOf(m Matcher) string {
	switch m.(type) {
	case *ContentMatcher:
		return "*search.ContentMatcher"
	case *UserMatcher:
		return "*search.UserMatcher"
	case *DefaultMatcher:
		return "*search.DefaultMatcher"
	case *RegexMatcher:
		return "*search.RegexMatcher"
	case *BadgeMatcher:
		return "*search.BadgeMatcher"
	case *PropertyMatcher:
		return "*search.PropertyMatcher"
	case *AndMatcher:
		return "*search.AndMatcher"
	case *NotMatcher:
		return "*search.NotMatcher"
	default:
		return "unknown"
	}
}
