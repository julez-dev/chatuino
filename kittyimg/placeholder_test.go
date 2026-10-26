package kittyimg

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
)

func TestPlaceholderRowsAbove(t *testing.T) {
	t.Parallel()

	twoRowImage := DecodedImage{ID: 1, Cols: 2, Rows: 2}
	threeRowImage := DecodedImage{ID: 1, Cols: 2, Rows: 3}
	otherThreeRowImage := DecodedImage{ID: 256, Cols: 1, Rows: 3}
	otherImage := DecodedImage{ID: 256, Cols: 1, Rows: 2}

	tests := []struct {
		name     string
		line     string
		rows     int
		expected []string
	}{
		{
			name:     "single row is noop",
			line:     "hi " + twoRowImage.DisplayUnicodePlaceholder(),
			rows:     1,
			expected: nil,
		},
		{
			name:     "no placeholder is noop",
			line:     "  12:00 \x1b[38;2;1;2;3muser\x1b[39m: hello",
			rows:     3,
			expected: nil,
		},
		{
			name: "text replaced with spaces, placeholder moved to row 0, trailing text dropped",
			line: "\x1b[1mab\x1b[0m " + twoRowImage.DisplayUnicodePlaceholder() + " cd",
			rows: 2,
			expected: []string{
				"   \x1b[38;2;0;0;1m\U0010EEEE\u0305\u0305\U0010EEEE\x1b[39m",
			},
		},
		{
			name: "adjacent images keep their own color",
			line: threeRowImage.DisplayUnicodePlaceholder() + otherThreeRowImage.DisplayUnicodePlaceholder(),
			rows: 3,
			expected: []string{
				"\x1b[38;2;0;0;1m\U0010EEEE\u0305\u0305\U0010EEEE\x1b[38;2;0;1;0m\U0010EEEE\u0305\u0305\x1b[39m",
				"\x1b[38;2;0;0;1m\U0010EEEE\u030D\u0305\U0010EEEE\x1b[38;2;0;1;0m\U0010EEEE\u030D\u0305\x1b[39m",
			},
		},
		{
			name: "wide characters keep alignment",
			line: "日本 " + otherImage.DisplayUnicodePlaceholder(),
			rows: 2,
			expected: []string{
				"     \x1b[38;2;0;1;0m\U0010EEEE\u0305\u0305\x1b[39m",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := PlaceholderRowsAbove(tt.line, tt.rows)
			require.Equal(t, tt.expected, got)

			for _, above := range got {
				require.LessOrEqual(t, lipgloss.Width(above), lipgloss.Width(tt.line))
			}
		})
	}
}

func TestDecodedImage_MultiRow(t *testing.T) {
	t.Parallel()

	decoded := DecodedImage{
		ID:     1,
		Cols:   3,
		Rows:   2,
		Images: []DecodedImageFrame{{Width: 20, Height: 20, EncodedPath: "dGVzdHBhdGg="}},
	}

	require.Contains(t, decoded.PrepareCommand(), "\x1b_Ga=p,i=1,p=1,q=2,U=1,r=2,c=3\x1b\\")

	// placeholder text displays the bottom row (row 1), column 0
	placeholder := decoded.DisplayUnicodePlaceholder()
	require.Equal(t, "\x1b[38;2;0;0;1m\U0010EEEE\u030D\u0305\U0010EEEE\U0010EEEE\x1b[39m", placeholder)
	require.Equal(t, 3, lipgloss.Width(placeholder))
}

func TestDisplayManager_ApplyLayout(t *testing.T) {
	t.Parallel()

	frames := []DecodedImageFrame{{Width: 28, Height: 28}}

	single := DecodedImage{Cols: 99, Images: frames}
	NewDisplayManager(nil, 10, 20, DisplayOptions{Rows: 1}).applyLayout(&single)
	require.Equal(t, 1, single.Rows)
	require.Equal(t, 2, single.Cols) // 28px -> 20px high -> 20px wide -> 2 cols

	double := DecodedImage{Cols: 99, Images: frames}
	NewDisplayManager(nil, 10, 20, DisplayOptions{Rows: 2}).applyLayout(&double)
	require.Equal(t, 2, double.Rows)
	require.Equal(t, 4, double.Cols) // 28px -> 40px high -> 40px wide -> 4 cols

	clamped := DecodedImage{Images: frames}
	NewDisplayManager(nil, 10, 20, DisplayOptions{Rows: 10}).applyLayout(&clamped)
	require.Equal(t, MaxRows, clamped.Rows)
}

func TestPlaceholderSpans(t *testing.T) {
	line := "  hi \x1b[38;2;0;0;5m" + "\U0010EEEE\U0010EEEE" + "\x1b[39m x \x1b[38;2;0;1;2m\U0010EEEE\x1b[39m"

	spans := PlaceholderSpans(line)

	require.Equal(t, []PlaceholderSpan{
		{Col: 5, Width: 2, ID: 5},
		{Col: 10, Width: 1, ID: 0x102},
	}, spans)

	id, ok := PlaceholderID(line)
	require.True(t, ok)
	require.Equal(t, int32(5), id)

	require.Nil(t, PlaceholderSpans("plain"))
}
