package mainui

import (
	"fmt"
)

const bellEmojiPrefix = string(rune(128276)) + " "

type tabHeaderEntry struct {
	id              string
	name            string
	identity        string
	selected        bool
	hasNotification bool
}

func (t tabHeaderEntry) FilterValue() string {
	return ""
}

func (t tabHeaderEntry) render() string {
	if t.hasNotification {
		return fmt.Sprintf("%s [%s]", bellEmojiPrefix+t.name, t.identity)
	}

	return fmt.Sprintf("%s [%s]", t.name, t.identity)
}
