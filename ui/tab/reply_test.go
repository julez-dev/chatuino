package tab

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func Test_replySnippet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "plain message unchanged", message: "nice play", want: "nice play"},
		{name: "empty message", message: "", want: ""},
		{name: "newlines and tabs collapse", message: "line one\n\tline two", want: "line one line two"},
		{name: "repeated whitespace collapses", message: "too    many     spaces", want: "too many spaces"},
		{name: "duplicate bypass rune stripped", message: "spam" + string(duplicateBypass), want: "spam"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, replySnippet(tt.message))
		})
	}
}

func Test_truncateToWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{name: "fits exactly", input: "abcde", width: 5, want: "abcde"},
		{name: "shorter than width", input: "abc", width: 5, want: "abc"},
		{name: "cut appends ellipsis", input: "abcdefgh", width: 5, want: "abcd…"},
		{name: "zero width", input: "abc", width: 0, want: ""},
		{name: "single cell keeps ellipsis only", input: "abc", width: 1, want: "…"},
		{name: "negative width", input: "abc", width: -3, want: ""},
		{name: "wide runes counted as two cells", input: "日本語です", width: 5, want: "日本…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := truncateToWidth(tt.input, tt.width)
			require.Equal(t, tt.want, got)
			require.LessOrEqual(t, lipgloss.Width(got), max(tt.width, 1))
		})
	}
}
