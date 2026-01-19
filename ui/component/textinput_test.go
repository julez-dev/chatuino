package component

import (
	"strings"
	"testing"
)

func TestSuggestionTextInput_wrapTextPreservingSpaces(t *testing.T) {
	t.Parallel()

	s := NewSuggestionTextInput(nil, nil)

	tests := []struct {
		name       string
		text       string
		wrapWidth  int
		wantLines  []string
		wantBreaks []int
	}{
		{
			name:       "short text no wrap",
			text:       "hello",
			wrapWidth:  20,
			wantLines:  []string{"hello"},
			wantBreaks: nil,
		},
		{
			name:       "text wraps at word boundary",
			text:       "hello world",
			wrapWidth:  6,
			wantLines:  []string{"hello ", "world"},
			wantBreaks: []int{6},
		},
		{
			name:       "empty text",
			text:       "",
			wrapWidth:  20,
			wantLines:  []string{""},
			wantBreaks: nil,
		},
		{
			name:       "zero width returns original",
			text:       "hello",
			wrapWidth:  0,
			wantLines:  []string{"hello"},
			wantBreaks: nil,
		},
		{
			name:       "trailing spaces preserved",
			text:       "hello     ",
			wrapWidth:  6,
			wantLines:  []string{"hello ", "    "},
			wantBreaks: []int{6},
		},
		{
			name:       "spaces only",
			text:       "          ",
			wrapWidth:  5,
			wantLines:  []string{"     ", "     "},
			wantBreaks: []int{5},
		},
		{
			name:       "hard wrap no spaces",
			text:       "abcdefghij",
			wrapWidth:  5,
			wantLines:  []string{"abcde", "fghij"},
			wantBreaks: []int{5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotLines, gotBreaks := s.wrapTextPreservingSpaces(tt.text, tt.wrapWidth)

			if len(gotLines) != len(tt.wantLines) {
				t.Errorf("wrapTextPreservingSpaces() got %d lines, want %d lines\ngot: %q\nwant: %q", len(gotLines), len(tt.wantLines), gotLines, tt.wantLines)
				return
			}
			for i := range gotLines {
				if gotLines[i] != tt.wantLines[i] {
					t.Errorf("wrapTextPreservingSpaces() line %d = %q, want %q", i, gotLines[i], tt.wantLines[i])
				}
			}

			// Verify breaks
			if len(gotBreaks) != len(tt.wantBreaks) {
				t.Errorf("wrapTextPreservingSpaces() got %d breaks, want %d breaks\ngot: %v\nwant: %v", len(gotBreaks), len(tt.wantBreaks), gotBreaks, tt.wantBreaks)
				return
			}
			for i := range gotBreaks {
				if gotBreaks[i] != tt.wantBreaks[i] {
					t.Errorf("wrapTextPreservingSpaces() break %d = %d, want %d", i, gotBreaks[i], tt.wantBreaks[i])
				}
			}

			// Verify reconstruction: joining lines should equal original text
			reconstructed := strings.Join(gotLines, "")
			if reconstructed != tt.text {
				t.Errorf("wrapTextPreservingSpaces() reconstruction mismatch:\ngot:  %q\nwant: %q", reconstructed, tt.text)
			}
		})
	}
}

func TestSuggestionTextInput_cursorLineCol(t *testing.T) {
	t.Parallel()

	s := NewSuggestionTextInput(nil, nil)

	tests := []struct {
		name      string
		text      string
		runePos   int // InputModel.Position() returns rune position
		wrapWidth int
		wantLine  int
		wantCol   int
	}{
		{
			name:      "cursor at start",
			text:      "hello world",
			runePos:   0,
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   0,
		},
		{
			name:      "cursor in middle of first line",
			text:      "hello world",
			runePos:   5,
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   5,
		},
		{
			name:      "cursor at end of single line",
			text:      "hello",
			runePos:   5,
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   5,
		},
		{
			name:      "cursor on second line after wrap",
			text:      "hello world",
			runePos:   7, // at "o" in "world" -> wraps to ["hello ", "world"]
			wrapWidth: 6,
			wantLine:  1,
			wantCol:   1, // 7 - 6 (break position)
		},
		{
			name:      "cursor beyond text length clamped",
			text:      "hello",
			runePos:   100,
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   5,
		},
		{
			name:      "unicode text",
			text:      "héllo wörld",
			runePos:   6, // after "héllo " (6 runes)
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   6,
		},
		{
			name:      "trailing space single line",
			text:      "hello ",
			runePos:   6, // cursor after trailing space
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   6, // trailing space should be counted
		},
		{
			name:      "emoji text cursor at end",
			text:      "hi🌊bye",
			runePos:   6, // 6 runes: h, i, 🌊, b, y, e
			wrapWidth: 20,
			wantLine:  0,
			wantCol:   6,
		},
		{
			name:      "cursor at wrap boundary",
			text:      "hello world",
			runePos:   6, // at start of "world"
			wrapWidth: 6,
			wantLine:  1,
			wantCol:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotLine, gotCol := s.cursorLineCol(tt.text, tt.runePos, tt.wrapWidth)
			if gotLine != tt.wantLine || gotCol != tt.wantCol {
				t.Errorf("cursorLineCol() = (%d, %d), want (%d, %d)", gotLine, gotCol, tt.wantLine, tt.wantCol)
			}
		})
	}
}

func TestSuggestionTextInput_updateViewOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxVisible     int
		initialOffset  int
		cursorLine     int
		totalLines     int
		wantViewOffset int
	}{
		{
			name:           "no scroll needed - content fits",
			maxVisible:     3,
			initialOffset:  0,
			cursorLine:     1,
			totalLines:     2,
			wantViewOffset: 0,
		},
		{
			name:           "cursor above view - scroll up",
			maxVisible:     3,
			initialOffset:  5,
			cursorLine:     2,
			totalLines:     10,
			wantViewOffset: 2,
		},
		{
			name:           "cursor below view - scroll down",
			maxVisible:     3,
			initialOffset:  0,
			cursorLine:     5,
			totalLines:     10,
			wantViewOffset: 3,
		},
		{
			name:           "cursor at bottom of view - no change",
			maxVisible:     3,
			initialOffset:  2,
			cursorLine:     4,
			totalLines:     10,
			wantViewOffset: 2,
		},
		{
			name:           "clamp to max offset",
			maxVisible:     3,
			initialOffset:  10,
			cursorLine:     8,
			totalLines:     10,
			wantViewOffset: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewSuggestionTextInput(nil, nil)
			s.maxVisibleLines = tt.maxVisible
			s.viewOffset = tt.initialOffset

			s.updateViewOffset(tt.cursorLine, tt.totalLines)

			if s.viewOffset != tt.wantViewOffset {
				t.Errorf("updateViewOffset() viewOffset = %d, want %d", s.viewOffset, tt.wantViewOffset)
			}
		})
	}
}

func TestSuggestionTextInput_SetMaxVisibleLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want int
	}{
		{"positive value", 3, 3},
		{"zero clamped to 1", 0, 1},
		{"negative clamped to 1", -5, 1},
		{"one stays one", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewSuggestionTextInput(nil, nil)
			s.SetMaxVisibleLines(tt.n)
			if s.maxVisibleLines != tt.want {
				t.Errorf("SetMaxVisibleLines(%d) = %d, want %d", tt.n, s.maxVisibleLines, tt.want)
			}
		})
	}
}

func TestSelectWordAtIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sentence  string
		runeIndex int
		wantWord  string
		wantStart int // byte index
		wantEnd   int // byte index
	}{
		{
			name:      "word in middle",
			sentence:  "hello world foo",
			runeIndex: 8, // in "world"
			wantWord:  "world",
			wantStart: 6,
			wantEnd:   11,
		},
		{
			name:      "leading spaces preserved",
			sentence:  "  pep",
			runeIndex: 4, // at "e" in "pep"
			wantWord:  "pep",
			wantStart: 2,
			wantEnd:   5,
		},
		{
			name:      "leading spaces cursor at end",
			sentence:  "  test",
			runeIndex: 6, // at end
			wantWord:  "test",
			wantStart: 2,
			wantEnd:   6,
		},
		{
			name:      "single word",
			sentence:  "hello",
			runeIndex: 3,
			wantWord:  "hello",
			wantStart: 0,
			wantEnd:   5,
		},
		{
			name:      "empty string",
			sentence:  "",
			runeIndex: 0,
			wantWord:  "",
			wantStart: 0,
			wantEnd:   0,
		},
		{
			name:      "cursor beyond length",
			sentence:  "test",
			runeIndex: 10,
			wantWord:  "",
			wantStart: 0,
			wantEnd:   0,
		},
		{
			name:      "unicode text",
			sentence:  "héllo wörld",
			runeIndex: 8, // in "wörld" (rune index)
			wantWord:  "wörld",
			wantStart: 7,  // byte index after "héllo "
			wantEnd:   13, // byte 7 + len("wörld")=6 = 13
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotWord, gotStart, gotEnd := selectWordAtIndex(tt.sentence, tt.runeIndex)
			if gotWord != tt.wantWord {
				t.Errorf("selectWordAtIndex() word = %q, want %q", gotWord, tt.wantWord)
			}
			if gotStart != tt.wantStart {
				t.Errorf("selectWordAtIndex() start = %d, want %d", gotStart, tt.wantStart)
			}
			if gotEnd != tt.wantEnd {
				t.Errorf("selectWordAtIndex() end = %d, want %d", gotEnd, tt.wantEnd)
			}
			// Verify that slicing with returned indices gives the word
			if tt.sentence != "" && gotStart < len(tt.sentence) && gotEnd <= len(tt.sentence) {
				sliced := tt.sentence[gotStart:gotEnd]
				if sliced != gotWord {
					t.Errorf("slicing sentence[%d:%d] = %q, want %q", gotStart, gotEnd, sliced, gotWord)
				}
			}
		})
	}
}
