package mainui

import (
	"fmt"

	"github.com/julez-dev/chatuino/ui/nav"
)

// rowItem adapts a nav.Row to the bubbles list.Item interface used by the
// vertical tab header. It carries no state of its own: every field comes from the
// registry projection it was built from.
type rowItem struct{ nav.Row }

func (r rowItem) FilterValue() string { return r.Title }

// rowLabel is the text shown for a tab in either header.
func rowLabel(row nav.Row) string {
	base := fmt.Sprintf("%s (%s)", row.Title, row.Identity)

	if row.HasNotification {
		return base + "[!]"
	}

	return base
}

// textCapturer marks a screen that swallows printable keys because one of its
// text inputs has focus.
type textCapturer interface {
	CapturesText() bool
}

// wheelScroller marks a screen that consumes mouse wheel messages.
type wheelScroller interface {
	scrollsOnWheel()
}

// header renders the tab list. Headers are pure renderers over the registry
// projection: they store no tab set, no selection and no notification flags, and
// they have no Update of their own.
type header interface {
	View(rows []nav.Row) string
	Resize(width, height int)
	MinWidth(rows []nav.Row) int
	// TabAt returns the id of the tab rendered at the given cell, if any.
	TabAt(rows []nav.Row, x, y int) (string, bool)
}
