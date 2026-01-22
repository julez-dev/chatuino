package mainui

import (
	"fmt"
)

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
	base := fmt.Sprintf("%s (%s)", t.name, t.identity)

	if t.hasNotification {
		return base + "[!]"
	}

	return base
}
