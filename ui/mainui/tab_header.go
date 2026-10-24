package mainui

import (
	"fmt"

	"github.com/julez-dev/chatuino/ui/nav"
)

// rowItem adapts a nav.Row to the bubbles list.Item interface used by the
// vertical tab header. It carries no state of its own: every field comes from the
// registry projection it was built from.
type rowItem struct {
	nav.Row
	Badge string // Wn.pane membership or standalone marker
}

func (r rowItem) FilterValue() string { return r.Title }

// rowLabel is the text shown for a tab in either header.
func rowLabel(row nav.Row) string {
	base := fmt.Sprintf("%s (%s)", row.Title, row.Identity)

	if row.HasNotification {
		return base + "[!]"
	}

	return base
}

// Workspace badges appear as soon as any tab belongs to a tiled screen. They
// make ownership visible across workspaces while standalone tabs stay marked.
func showWorkspaceBadges(rows []nav.Row) bool {
	for _, row := range rows {
		if row.Workspace != "" {
			return true
		}
	}
	return false
}

func workspaceBadge(row nav.Row, show bool) string {
	if !show {
		return ""
	}
	if row.Workspace != "" && row.WorkspacePane > 0 {
		return fmt.Sprintf("%s.%d ", row.Workspace, row.WorkspacePane)
	}
	return "· "
}

func workspaceRowLabel(row nav.Row, show bool) string {
	return workspaceBadge(row, show) + rowLabel(row)
}

func workspaceSummary(rows []nav.Row) string {
	activeWorkspace, activeStandalone, anyAssigned := "", false, false
	for _, row := range rows {
		if row.Workspace != "" {
			anyAssigned = true
		}
		if row.Active {
			activeWorkspace = row.Workspace
			activeStandalone = row.Workspace == ""
		}
	}
	if activeStandalone {
		if anyAssigned {
			return "standalone"
		}
		return ""
	}
	if activeWorkspace == "" {
		return ""
	}
	panes := 0
	for _, row := range rows {
		if row.Workspace == activeWorkspace {
			panes++
		}
	}
	if panes > 1 {
		return fmt.Sprintf("%d tiled", panes)
	}
	return ""
}

// tabHeaderLabel keeps pagination and tiling context in the existing border
// label without changing the tab row's order.
func tabHeaderLabel(name string, rows []nav.Row, page, pages int) string {
	label := name
	if pages > 1 {
		label += fmt.Sprintf(" %d/%d", page, pages)
	}
	if summary := workspaceSummary(rows); summary != "" {
		label += " " + summary
	}
	return fmt.Sprintf("[ %s ]", label)
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
