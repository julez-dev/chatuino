package mainui

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"

	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/workspace"
	"github.com/rs/zerolog/log"
)

// namedWorkspace is one automatically managed tiling screen. The live layout of
// the active workspace remains in Root.workspace; layout is synchronized before
// switching, saving, or restoring.
type namedWorkspace struct {
	id, name string
	layout   workspace.Layout
	focus    string
}

// activeWorkspace returns the live workspace record, or nil during construction.
func (r *Root) activeWorkspace() *namedWorkspace {
	for _, ws := range r.workspaces {
		if ws.id == r.activeWorkspaceID {
			return ws
		}
	}
	return nil
}

// syncActiveWorkspace copies the live layout and assigned-tab focus into the
// active record before switching, saving, or restoring it.
func (r *Root) syncActiveWorkspace() {
	ws := r.activeWorkspace()
	if ws == nil {
		return
	}
	ws.layout = r.workspace
	if r.workspace.Contains(r.activeID()) {
		ws.focus = r.activeID()
	}
}

// workspaceForTab returns the workspace owning a tab, or nil for a standalone
// tab. A tab can belong to at most one workspace.
func (r *Root) workspaceForTab(id string) *namedWorkspace {
	if id == "" {
		return nil
	}
	if r.workspace.Contains(id) {
		return r.activeWorkspace()
	}
	for _, ws := range r.workspaces {
		if ws.id != r.activeWorkspaceID && ws.layout.Contains(id) {
			return ws
		}
	}
	return nil
}

// visibleRows returns every tab in header order: each workspace's tabs in pane
// reading order, followed by standalone tabs in registry order. Registry order
// remains the storage/creation order; this projection only changes presentation
// and navigation order so moving a pane also moves its tab entry.
func (r *Root) visibleRows() []nav.Row {
	rows := r.tabs.Rows()
	if len(rows) < 2 {
		return rows
	}
	byID := make(map[string]nav.Row, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	ordered := make([]nav.Row, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, ws := range r.workspaces {
		layout := ws.layout
		if ws.id == r.activeWorkspaceID {
			layout = r.workspace
		}
		for _, id := range layout.PaneOrder() {
			row, ok := byID[id]
			if !ok || seen[id] {
				continue
			}
			ordered = append(ordered, row)
			seen[id] = true
		}
	}
	for _, row := range rows {
		if !seen[row.ID] {
			ordered = append(ordered, row)
		}
	}
	return ordered
}

func workspaceLabel(ws *namedWorkspace, index int) string {
	if strings.HasPrefix(ws.name, "Workspace ") {
		if suffix := strings.TrimPrefix(ws.name, "Workspace "); suffix != "" {
			return "W" + suffix
		}
	}
	return fmt.Sprintf("W%d", index+1)
}

func (r *Root) workspaceLabelFor(ws *namedWorkspace) string {
	for i, candidate := range r.workspaces {
		if candidate.id == ws.id {
			return workspaceLabel(candidate, i)
		}
	}
	return ""
}

// syncWorkspaceToActive makes the active tab's owning workspace live. A
// standalone tab leaves the current workspace intact; selecting a tab owned by
// another workspace restores that workspace automatically.
func (r *Root) syncWorkspaceToActive() {
	active := r.activeID()
	if active == "" {
		return
	}
	owner := r.workspaceForTab(active)
	if owner == nil {
		r.tabs.InvalidateRows()
		return
	}
	if owner.id == r.activeWorkspaceID {
		owner.focus = active
		r.tabs.InvalidateRows()
		return
	}
	r.syncActiveWorkspace()
	r.activeWorkspaceID = owner.id
	r.workspace = owner.layout
	owner.focus = active
	r.paneZoom = false
	r.paneHint = ""
	r.pendingSplit = nil
	r.tabs.InvalidateRows()
}

// emptyWorkspace prefers the live workspace, then any empty workspace. Reusing
// empty screens avoids accumulating invisible workspaces as tabs close.
func (r *Root) emptyWorkspace() *namedWorkspace {
	if ws := r.activeWorkspace(); ws != nil && r.workspace.Count() == 0 {
		return ws
	}
	for _, ws := range r.workspaces {
		if ws.id != r.activeWorkspaceID && ws.layout.Count() == 0 {
			return ws
		}
	}
	return nil
}

func (r *Root) newWorkspace() *namedWorkspace {
	usedIDs := make(map[string]bool, len(r.workspaces))
	usedNames := make(map[string]bool, len(r.workspaces))
	for _, ws := range r.workspaces {
		usedIDs[ws.id] = true
		usedNames[ws.name] = true
	}
	var ws *namedWorkspace
	for {
		r.workspaceSeq++
		ws = &namedWorkspace{
			id:   fmt.Sprintf("workspace-%d", r.workspaceSeq),
			name: fmt.Sprintf("Workspace %d", r.workspaceSeq),
		}
		if !usedIDs[ws.id] && !usedNames[ws.name] {
			break
		}
	}
	r.workspaces = append(r.workspaces, ws)
	return ws
}

// claimStandalone gives a standalone tab its own workspace when pane mode is
// entered. An empty workspace is reused; otherwise a new screen is created.
func (r *Root) claimStandalone(id string) bool {
	ws := r.emptyWorkspace()
	if ws == nil {
		ws = r.newWorkspace()
	}
	if ws.id != r.activeWorkspaceID {
		r.syncActiveWorkspace()
		r.activeWorkspaceID = ws.id
		r.workspace = ws.layout
		r.paneZoom = false
		r.paneHint = ""
		r.pendingSplit = nil
	}
	if !r.workspace.Select("", id) {
		return false
	}
	ws.focus = id
	r.tabs.InvalidateRows()
	return true
}

// enterPaneMode explicitly opts into tiling. A standalone tab becomes the first
// pane of its own (or an empty) workspace; a tab already owned by a workspace
// simply enters that workspace's pane mode.
func (r *Root) enterPaneMode() bool {
	active := r.activeID()
	if active == "" {
		return false
	}
	r.syncWorkspaceToActive()
	if !r.workspace.Contains(active) {
		if !r.claimStandalone(active) {
			return false
		}
		r.paneZoom = false
	}
	r.paneMode = true
	r.paneHint = ""
	r.handleResize()
	return true
}

// removeTabFromWorkspaces enforces exclusive ownership when a tab is closed.
// Closing a pane does not close other workspaces.
func (r *Root) removeTabFromWorkspaces(id string) string {
	target := r.workspace.Remove(id)
	for _, ws := range r.workspaces {
		if ws.id == r.activeWorkspaceID {
			continue
		}
		if ws.layout.Contains(id) {
			ws.layout.Remove(id)
			if ws.focus == id {
				ws.focus = ws.layout.First()
			}
		}
	}
	return target
}

type savedWorkspace struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Layout workspace.State `json:"layout"`
}

type savedWorkspaceSet struct {
	Version    int              `json:"version"`
	Workspaces []savedWorkspace `json:"workspaces"`
}

// snapshotWorkspaces persists every automatically managed layout. The legacy
// single Layout field remains the active workspace for older readers.
func (r *Root) snapshotWorkspaces(refs map[string]string) jsontext.Value {
	r.syncActiveWorkspace()
	set := savedWorkspaceSet{Version: 1}
	for _, ws := range r.workspaces {
		focus := ws.focus
		if !ws.layout.Contains(focus) {
			focus = ws.layout.First()
		}
		set.Workspaces = append(set.Workspaces, savedWorkspace{
			ID: ws.id, Name: ws.name, Layout: ws.layout.Snapshot(focus, refs),
		})
	}
	data, err := json.Marshal(set)
	if err != nil {
		log.Logger.Error().Err(err).Msg("could not snapshot workspaces")
		return nil
	}
	return data
}

// restoreWorkspaces migrates legacy single-layout state, validates ownership,
// and restores layouts/focus. Invalid metadata never blocks valid tabs from
// loading. The active workspace is derived from the focused tab, not chosen by
// the user.
func (r *Root) restoreWorkspaces(legacy, multi jsontext.Value, refs map[string]string) {
	r.workspaces = nil
	r.workspace = workspace.Layout{}
	r.activeWorkspaceID, r.workspaceSeq = "", 0

	if r.isDetached {
		r.workspaces = []*namedWorkspace{{id: "workspace-1", name: "Workspace 1"}}
		r.activeWorkspaceID, r.workspaceSeq = "workspace-1", 1
		return
	}

	var saved []savedWorkspace
	if len(multi) > 0 {
		var set savedWorkspaceSet
		if err := json.Unmarshal(multi, &set); err != nil {
			log.Logger.Warn().Err(err).Msg("invalid workspaces metadata; falling back to legacy layout")
		} else if set.Version != 1 {
			log.Logger.Warn().Int("version", set.Version).Msg("unsupported workspaces metadata version; falling back to legacy layout")
		} else {
			saved = set.Workspaces
		}
	}
	if len(saved) == 0 && len(legacy) > 0 {
		var state workspace.State
		if err := json.Unmarshal(legacy, &state); err != nil {
			log.Logger.Warn().Err(err).Msg("invalid legacy workspace layout; using an empty workspace")
		} else {
			saved = []savedWorkspace{{ID: "workspace-1", Name: "Workspace 1", Layout: state}}
		}
	}
	if len(saved) == 0 {
		saved = []savedWorkspace{{ID: "workspace-1", Name: "Workspace 1"}}
	}

	seen := make(map[string]bool)
	usedIDs := make(map[string]bool)
	usedNames := make(map[string]bool)
	for i, sw := range saved {
		id := sw.ID
		if id == "" || usedIDs[id] {
			for suffix := i + 1; ; suffix++ {
				id = fmt.Sprintf("workspace-%d", suffix)
				if !usedIDs[id] {
					break
				}
			}
		}
		usedIDs[id] = true
		name := sw.Name
		if name == "" || usedNames[name] {
			for suffix := i + 1; ; suffix++ {
				name = fmt.Sprintf("Workspace %d", suffix)
				if !usedNames[name] {
					break
				}
			}
		}
		usedNames[name] = true
		layout, focus, repaired := workspace.Restore(sw.Layout, refs)
		for _, tabID := range layout.PaneOrder() {
			if seen[tabID] {
				layout.Remove(tabID)
				repaired = true
				continue
			}
			seen[tabID] = true
		}
		if !layout.Contains(focus) {
			focus = layout.First()
		}
		if repaired {
			log.Logger.Warn().Str("workspace", name).Msg("saved workspace layout was repaired")
		}
		r.workspaces = append(r.workspaces, &namedWorkspace{id: id, name: name, layout: layout, focus: focus})
		r.workspaceSeq = max(r.workspaceSeq, i+1)
	}

	// The focused tab decides which workspace is live. A standalone focused tab
	// keeps the first workspace in the background, preserving its layout.
	activeID := r.activeID()
	activeWorkspace := r.workspaces[0]
	for _, ws := range r.workspaces {
		if ws.layout.Contains(activeID) {
			activeWorkspace = ws
			activeWorkspace.focus = activeID
			break
		}
	}
	r.activeWorkspaceID = activeWorkspace.id
	r.workspace = activeWorkspace.layout
	r.tabs.InvalidateRows()
}
