package tab

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_parseSlashCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{name: "not a command", input: "hello world", wantOK: false},
		{name: "empty input", input: "", wantOK: false},
		{name: "command without args keeps historical empty arg", input: "/emotes", wantName: "emotes", wantArgs: []string{""}, wantOK: true},
		{name: "command with one arg", input: "/join sodapoppin", wantName: "join", wantArgs: []string{"sodapoppin"}, wantOK: true},
		{name: "command with multiple args", input: "/pyramid word 3", wantName: "pyramid", wantArgs: []string{"word", "3"}, wantOK: true},
		{name: "bare slash", input: "/", wantName: "", wantArgs: []string{""}, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			name, args, ok := parseSlashCommand(tt.input)

			require.Equal(t, tt.wantOK, ok)
			if !tt.wantOK {
				return
			}

			require.Equal(t, tt.wantName, name)
			require.Equal(t, tt.wantArgs, args)
		})
	}
}

func Test_applyDuplicateBypass(t *testing.T) {
	t.Parallel()

	require.Equal(t, "hi", applyDuplicateBypass("hi", "different"), "non-duplicate stays unchanged")
	require.Equal(t, "hi "+string(duplicateBypass), applyDuplicateBypass("hi", "hi"), "duplicate gets the bypass character")
	require.Equal(t, "Hi "+string(duplicateBypass), applyDuplicateBypass("Hi", "hI"), "duplicate check is case-insensitive")
	require.Equal(t, ""+" "+string(duplicateBypass), applyDuplicateBypass("", ""), "empty input equals empty last message")
}

func Test_isNearDuplicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		message string
		recent  []string
		want    bool
	}{
		{name: "no recent messages", message: "hello there", recent: nil, want: false},
		{name: "single word seen verbatim", message: "Kappa", recent: []string{"kappa"}, want: true},
		{name: "single word not seen", message: "Kappa", recent: []string{"PogChamp"}, want: false},
		{name: "single word never fuzzy matches", message: "Kappas", recent: []string{"Kappa"}, want: false},
		{name: "multi word exact repeat", message: "nice play man", recent: []string{"nice play man"}, want: true},
		{name: "multi word near repeat within distance", message: "nice play man", recent: []string{"nice play men"}, want: true},
		{name: "multi word distinct", message: "nice play man", recent: []string{"completely different message"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, isNearDuplicate(tt.message, tt.recent))
		})
	}
}
