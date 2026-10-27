package mainui

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/julez-dev/chatuino/twitch/twitchirc"
	"github.com/julez-dev/chatuino/ui/nav"
	"github.com/julez-dev/chatuino/ui/tab"
	"github.com/julez-dev/chatuino/ui/workspace"
	"github.com/rs/zerolog/log"
)

const panePickerRoute = "pane-picker"

type paneSplitRequest struct {
	ticket, revision uint64
	target           string
	axis             workspace.Axis
}

type paneChoiceMessage struct {
	ticket uint64
	tabID  string // empty means open the existing new-tab flow
}

type paneJoinMessage struct {
	ticket uint64
	join   tab.JoinChannelMessage
}

type panePlacement struct {
	tab            string
	frame, content workspace.Rect // relative to workspace bounds
}

type panePlan struct {
	bounds   workspace.Rect
	panes    []panePlacement
	dividers []workspace.Rect
	fallback bool
}

func (r *Root) activeID() string {
	if active, ok := r.tabs.Active(); ok {
		return active.ID()
	}
	return ""
}

func (r *Root) snapshotWorkspace(refs map[string]string) json.RawMessage {
	if r.tabs.Len() == 0 {
		return nil
	}
	// A standalone tab is saved by TabState.IsFocused. Layout focus must stay on
	// an actual pane or restore would silently promote the standalone tab.
	focus := r.activeID()
	if !r.workspace.Contains(focus) {
		focus = r.workspace.First()
	}
	state := r.workspace.Snapshot(focus, refs)
	if state.Root == nil {
		var single workspace.Layout
		single.Select("", r.activeID())
		state = single.Snapshot(r.activeID(), refs)
	}
	data, err := json.Marshal(state)
	if err != nil {
		log.Logger.Error().Err(err).Msg("could not snapshot workspace layout")
		return nil
	}
	return data
}

func (r *Root) stepTab(step int) {
	rows := r.visibleRows()
	for i, row := range rows {
		if row.Active {
			r.navigateToTab(rows[((i+step)%len(rows)+len(rows))%len(rows)].ID)
			return
		}
	}
}

// navigateToTab is non-destructive navigation: it never adds, removes, or
// reassigns a pane. Unassigned tabs render in the ordinary single-tab view while
// the saved tiled layout stays intact behind them.
func (r *Root) navigateToTab(id string) {
	if r.tabByID(id) == nil {
		return
	}
	r.tabs.SetActive(id)
	if !r.workspace.Contains(id) {
		r.paneMode = false
	}
	r.handleResize()
}

func (r *Root) tabByID(id string) tab.Tab {
	for _, t := range r.tabs.All() {
		if t.ID() == id {
			return t
		}
	}
	return nil
}

func (r *Root) workspaceVisible() bool {
	return r.workspace.Count() > 0 && r.workspace.Contains(r.activeID())
}

func (r *Root) usesWorkspace() bool {
	return r.workspaceVisible() && (r.workspace.Count() > 1 || r.paneMode || r.paneZoom || r.paneHint != "")
}

func (r *Root) workspaceBounds() workspace.Rect {
	if r.settings.VerticalTabList {
		x := min(max(0, r.width), lipgloss.Width(r.header.View(r.visibleRows())))
		return workspace.Rect{X: x, Width: max(0, r.width-x), Height: max(0, r.height-1)}
	}
	y := min(max(0, r.height-1), r.getHeaderHeight())
	return workspace.Rect{Y: y, Width: max(0, r.width), Height: max(0, r.height-y-1)}
}

// Reserve the widest page-number label as well as tab labels. Otherwise a
// focus change from page/item 9 to 10 can move every pane under the pointer.
// This sizing is specific to the workspace; ordinary single-pane chrome stays
// unchanged. The real header renderer remains the authority for label widths.
func (r *Root) paneHeaderWidth(rows []nav.Row) int {
	header := newVerticalTabHeader(0, 0, r.theme)
	width := min(max(0, r.width), header.MinWidth(rows))

	// Measure with an assigned row active, not whichever tab currently has
	// focus. Otherwise moving to a standalone tab changes the summary label and
	// shifts pane origins when focus returns to the workspace.
	stable := slices.Clone(rows)
	activeSet := false
	for i := range stable {
		stable[i].Active = false
		if !activeSet && stable[i].WorkspacePane > 0 {
			stable[i].Active, activeSet = true, true
		}
	}
	if !activeSet && len(stable) > 0 {
		stable[0].Active = true
	}

	header.Resize(width, max(0, r.height-1))
	return min(max(0, r.width), max(width, lipgloss.Width(header.View(stable))))
}

// A newly joined tab can widen a vertical tab list. Validate against that
// prospective projection before committing the tab or split, not yesterday's
// available width. Reuse the real header renderer rather than duplicate its math.
func (r *Root) boundsWithCaption(row nav.Row) workspace.Rect {
	bounds := r.workspaceBounds()
	if !r.settings.VerticalTabList {
		return bounds
	}
	rows := slices.Clone(r.visibleRows())
	for i := range rows {
		rows[i].Active = false
	}
	row.Active = true
	rows = append(rows, row)
	header := newVerticalTabHeader(0, 0, r.theme)
	header.Resize(r.paneHeaderWidth(rows), max(0, r.height-1))
	bounds.X = min(max(0, r.width), lipgloss.Width(header.View(rows)))
	bounds.Width = max(0, r.width-bounds.X)
	return bounds
}

// Drawing, child sizing, and mouse hit-testing all consume this same plan.
func (r *Root) planPanes() panePlan {
	bounds := r.workspaceBounds()
	arrangement := r.workspace.Arrange(bounds.Width, bounds.Height)
	p := panePlan{bounds: bounds, fallback: !arrangement.Fits, dividers: arrangement.Dividers}
	panes := arrangement.Panes
	if p.fallback || r.paneZoom {
		panes = []workspace.Pane{{Tab: r.activeID(), Rect: workspace.Rect{Width: bounds.Width, Height: bounds.Height}}}
		p.dividers = nil
	}
	for _, pane := range panes {
		content := pane.Rect
		if r.workspace.Count() > 1 && content.Height > 0 {
			content.Y++
			content.Height--
		}
		p.panes = append(p.panes, panePlacement{tab: pane.Tab, frame: pane.Rect, content: content})
	}
	return p
}

func (r *Root) paneMouseTarget(x, y int) (id string, localX, localY int, content bool) {
	plan := r.planPanes()
	if !plan.bounds.Contains(x, y) {
		return "", 0, 0, false
	}
	x, y = x-plan.bounds.X, y-plan.bounds.Y
	for _, pane := range plan.panes {
		if pane.frame.Contains(x, y) {
			return pane.tab, x - pane.content.X, y - pane.content.Y, pane.content.Contains(x, y)
		}
	}
	return "", 0, 0, false
}

func (r *Root) resizeWorkspace() {
	if r.settings.VerticalTabList {
		r.header.Resize(r.paneHeaderWidth(r.visibleRows()), max(0, r.height-1))
	} else {
		r.header.Resize(max(0, r.width-3), 0)
	}
	for _, pane := range r.planPanes().panes {
		if t := r.tabByID(pane.tab); t != nil {
			t.SetSize(pane.content.Width, pane.content.Height)
			t.SetFullWidth(max(0, r.width)) // exclude each tab's local status allocation
			t.HandleResize()
		}
	}
}

// clampScreen leaves the ordinary single-pane view byte-for-byte unchanged
// unless it actually exceeds the terminal. It also bounds modal backgrounds.
func (r *Root) clampScreen(text string) string {
	if r.width <= 0 || r.height <= 0 {
		return ""
	}
	if lipgloss.Width(text) > r.width || lipgloss.Height(text) > r.height {
		return strings.Join(blockLines(text, r.width, r.height), "\n")
	}
	return text
}

// blockLines renders text into exactly height lines of exactly width terminal cells,
// clipping and padding by cell width so ANSI sequences and Kitty placeholders keep
// their attributes. Sizing text this way replaced a lipgloss canvas: a canvas
// allocates a full cell buffer per call, and the tiled layout needs one per pane,
// divider and heading on every frame.
func blockLines(text string, width, height int) []string {
	if width <= 0 || height <= 0 {
		return nil
	}

	lines := make([]string, height)
	blank := strings.Repeat(" ", width)
	source := strings.Split(text, "\n")

	for i := range lines {
		if i >= len(source) {
			// Every row of a block is bounded: a tab cannot paint into its
			// neighbour's rectangle by returning fewer lines than it was given.
			lines[i] = blank

			continue
		}

		lines[i] = fitLine(source[i], width)
	}

	return lines
}

// fitLine clips or pads one line to exactly width cells. A wide grapheme that
// straddles the edge cannot be clipped in half, so the leftover cell is padded: the
// line has to be exactly as wide as its block or the columns of the screen drift.
func fitLine(line string, width int) string {
	measured := lipgloss.Width(line)
	if measured > width {
		line = ansi.Truncate(line, width, "")
		measured = lipgloss.Width(line)
	}

	if measured < width {
		line += strings.Repeat(" ", width-measured)
	}

	return line
}

// placement is one already sized block of lines at a screen position. Placements of
// the tiled layout never overlap: panes, headings, dividers, header and footer tile
// the screen.
type placement struct {
	x, y  int
	lines []string
}

// composeScreen assembles the placements row by row, filling the gaps between them
// with spaces. The result is height rows of exactly width cells, built as text: no
// cell buffer is allocated for the screen.
func composeScreen(width, height int, placements []placement) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	type segment struct {
		x    int
		text string
	}

	rows := make([][]segment, height)
	for _, p := range placements {
		for i, line := range p.lines {
			y := p.y + i
			if y < 0 || y >= height || p.x >= width || line == "" {
				continue
			}

			rows[y] = append(rows[y], segment{p.x, line})
		}
	}

	blank := strings.Repeat(" ", width)
	var out strings.Builder
	out.Grow(height * (width + 1))

	for y, segments := range rows {
		if y > 0 {
			out.WriteByte('\n')
		}

		if len(segments) == 0 {
			out.WriteString(blank)

			continue
		}

		slices.SortFunc(segments, func(a, b segment) int { return a.x - b.x })

		filled := 0
		for _, s := range segments {
			if s.x > filled {
				out.WriteString(strings.Repeat(" ", s.x-filled))
			}

			text := s.text
			if s.x+lipgloss.Width(text) > width {
				text = ansi.Truncate(text, width-s.x, "")
			}

			out.WriteString(text)
			filled = s.x + lipgloss.Width(text)
		}

		if filled < width {
			out.WriteString(strings.Repeat(" ", width-filled))
		}
	}

	return out.String()
}

func (r *Root) paneFooter(plan panePlan) string {
	if r.paneMode {
		return r.paneModeFooter(plan)
	}

	var parts []string
	if plan.fallback {
		parts = append(parts, "SMALL TERMINAL")
	} else if r.paneZoom {
		parts = append(parts, "ZOOM")
	}
	if r.paneHint != "" {
		parts = append(parts, r.paneHint)
	}
	if plan.fallback {
		parts = append(parts, "layout preserved; enlarge terminal or close panes")
	}
	if active := r.activeTab(); active != nil {
		if status := active.StatusBarView(); status != "" {
			if len(parts) == 0 {
				return status
			}
			parts = append(parts, status)
		}
	}
	return strings.Join(parts, " | ")
}

type paneHintGroup struct {
	keys string
	desc string
}

func (r *Root) paneModeFooter(plan panePlan) string {
	if r.width <= 0 {
		return ""
	}

	stateStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(r.theme.StatusColor))
	tagStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(r.theme.ChatNoticeAlertColor))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(r.theme.ChatErrorColor))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(r.theme.DimmedTextColor)).Faint(true)

	left := stateStyle.Render("-- Pane --")

	var tags []string
	if plan.fallback {
		tags = append(tags, tagStyle.Render("SMALL TERMINAL"))
	} else if r.paneZoom {
		tags = append(tags, tagStyle.Render("ZOOM"))
	}
	if len(tags) > 0 {
		left += " " + strings.Join(tags, " ")
	}

	if r.paneHint != "" {
		return fitPaneFooter(left+" "+errorStyle.Render(r.paneHint), r.width)
	}

	// Without an editable layout, key hints for splitting/resizing would be
	// misleading. Keep the fallback explanation compact instead.
	if plan.fallback {
		return fitPaneFooter(lineWithRight(left, descStyle.Render("enlarge terminal or close panes"), r.width), r.width)
	}

	for _, groups := range r.paneHintCandidates() {
		hint := r.renderPaneHints(groups, descStyle)
		if lipgloss.Width(left)+1+lipgloss.Width(hint) <= r.width {
			return left + strings.Repeat(" ", r.width-lipgloss.Width(left)-lipgloss.Width(hint)) + hint
		}
	}

	return fitPaneFooter(left, r.width)
}

func lineWithRight(left, right string, width int) string {
	if right == "" {
		return left
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap <= 0 {
		return fitPaneFooter(left+" "+right, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func fitPaneFooter(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(text)
}

func (r *Root) renderPaneHints(groups []paneHintGroup, descStyle lipgloss.Style) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(r.theme.StatusColor))
	parts := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.keys == "" || group.desc == "" {
			continue
		}
		parts = append(parts, keyStyle.Render(group.keys)+" "+descStyle.Render(group.desc))
	}
	return strings.Join(parts, descStyle.Render(" · "))
}

func (r *Root) paneHintCandidates() [][]paneHintGroup {
	focus := paneDirectionKeys(r.keymap.PaneFocusLeft, r.keymap.PaneFocusDown, r.keymap.PaneFocusUp, r.keymap.PaneFocusRight)
	move := paneDirectionKeys(r.keymap.PaneSwapLeft, r.keymap.PaneSwapDown, r.keymap.PaneSwapUp, r.keymap.PaneSwapRight)
	split := slashKeys(r.keymap.PaneSplitColumns, r.keymap.PaneSplitRows)
	resize := concatKeys(r.keymap.PaneNarrow, r.keymap.PaneWiden, r.keymap.PaneShorten, r.keymap.PaneTaller)
	cycle := firstKey(r.keymap.PaneNext)
	create := firstKey(r.keymap.Create)
	closeKey := firstKey(r.keymap.PaneClose)
	zoom := firstKey(r.keymap.PaneZoom)
	exit := slashKeys(r.keymap.Escape, r.keymap.PaneMode)

	full := []paneHintGroup{
		{focus, "focus"},
		{move, "move"},
		{split, "split"},
		{create, "new"},
		{closeKey, "close"},
		{zoom, "zoom"},
		{resize, "size"},
		{cycle, "cycle"},
		{exit, "exit"},
	}
	medium := []paneHintGroup{
		{focus, "focus"},
		{move, "move"},
		{split, "split"},
		{closeKey, "close"},
		{zoom, "zoom"},
		{resize, "size"},
		{exit, "exit"},
	}
	compact := []paneHintGroup{
		{focus, "focus"},
		{move, "move"},
		{split, "split"},
		{closeKey, "close"},
		{exit, "exit"},
	}
	minimal := []paneHintGroup{
		{focus, "focus"},
		{move, "move"},
		{split, "split"},
		{exit, "exit"},
	}
	return [][]paneHintGroup{full, medium, compact, minimal}
}

func firstKey(binding key.Binding) string {
	keys := binding.Keys()
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func slashKeys(bindings ...key.Binding) string {
	keys := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if keyName := firstKey(binding); keyName != "" && !slices.Contains(keys, keyName) {
			keys = append(keys, keyName)
		}
	}
	return strings.Join(keys, "/")
}

func concatKeys(bindings ...key.Binding) string {
	keys := make([]string, 0, len(bindings))
	allSingleRune := true
	for _, binding := range bindings {
		if keyName := firstKey(binding); keyName != "" {
			keys = append(keys, keyName)
			if len([]rune(keyName)) != 1 {
				allSingleRune = false
			}
		}
	}
	if allSingleRune {
		return strings.Join(keys, "")
	}
	return strings.Join(keys, "/")
}

// paneDirectionKeys prefers the compact letter alias over arrow keys, while
// still respecting remapped and disabled bindings.
func paneDirectionKeys(bindings ...key.Binding) string {
	keys := make([]string, 0, len(bindings))
	allSingleRune := true
	for _, binding := range bindings {
		available := binding.Keys()
		if len(available) == 0 {
			continue
		}
		chosen := available[0]
		for _, candidate := range available {
			if candidate != "left" && candidate != "right" && candidate != "up" && candidate != "down" {
				chosen = candidate
				break
			}
		}
		keys = append(keys, chosen)
		if len([]rune(chosen)) != 1 {
			allSingleRune = false
		}
	}
	if allSingleRune {
		return strings.Join(keys, "")
	}
	return strings.Join(keys, "/")
}

func (r *Root) renderWorkspace() string {
	if r.width <= 0 || r.height <= 0 {
		return ""
	}
	plan := r.planPanes()
	placements := make([]placement, 0, len(plan.panes)*2+len(plan.dividers)+2)
	if r.settings.VerticalTabList {
		placements = append(placements, placement{0, 0, blockLines(r.header.View(r.visibleRows()), plan.bounds.X, max(0, r.height-1))})
	} else {
		placements = append(placements, placement{0, 0, blockLines(r.header.View(r.visibleRows()), r.width, plan.bounds.Y)})
	}
	for _, divider := range plan.dividers {
		text := strings.Repeat("─", divider.Width)
		if divider.Width == 1 {
			text = strings.Repeat("│\n", divider.Height)
		}
		text = lipgloss.NewStyle().Foreground(lipgloss.Color(r.theme.DimmedTextColor)).Render(text)
		placements = append(placements, placement{plan.bounds.X + divider.X, plan.bounds.Y + divider.Y, blockLines(text, divider.Width, divider.Height)})
	}
	for _, pane := range plan.panes {
		t := r.tabByID(pane.tab)
		if t == nil {
			continue
		}
		if pane.content.Y > pane.frame.Y {
			prefix, color := "  ", r.theme.DimmedTextColor
			if pane.tab == r.activeID() {
				prefix, color = "> ", r.theme.ActiveLabelColor
			}
			label := prefix
			if ws := r.activeWorkspace(); ws != nil {
				label += fmt.Sprintf("%s.%d ", r.workspaceLabelFor(ws), r.workspace.PaneNumber(pane.tab))
			}
			label += t.Title()
			if t.Identity() != "" {
				label += " (" + t.Identity() + ")"
			}
			heading := lipgloss.NewStyle().Bold(pane.tab == r.activeID()).Foreground(lipgloss.Color(color)).Render(label)
			placements = append(placements, placement{plan.bounds.X + pane.frame.X, plan.bounds.Y + pane.frame.Y, blockLines(heading, pane.frame.Width, 1)})
		}
		placements = append(placements, placement{plan.bounds.X + pane.content.X, plan.bounds.Y + pane.content.Y, blockLines(t.ViewWithoutStatusBar(), pane.content.Width, pane.content.Height)})
	}
	placements = append(placements, placement{0, r.height - 1, blockLines(r.paneFooter(plan), r.width, 1)})

	return composeScreen(r.width, r.height, placements)
}

func paneDirection(msg tea.KeyPressMsg, bindings [4]key.Binding) (workspace.Direction, bool) {
	for i, binding := range bindings {
		if key.Matches(msg, binding) {
			return workspace.Direction(i), true
		}
	}
	return 0, false
}

func (r *Root) handlePaneKey(msg tea.KeyPressMsg) tea.Cmd {
	if key.Matches(msg, r.keymap.PaneMode) || key.Matches(msg, r.keymap.Escape) {
		r.paneMode, r.paneHint = false, ""
		r.handleResize()
		return nil
	}
	if key.Matches(msg, r.keymap.Create) {
		// In pane mode Create adds a tab to the current workspace as a new pane.
		// Outside pane mode the same binding opens a standalone tab. The pane
		// inherits its container's orientation, like i3's layout inheritance, so
		// a vertical stack grows downwards instead of always to the right.
		return r.beginPaneJoin(r.workspace.SplitAxis(r.activeID()))
	}
	r.paneHint = ""
	active, bounds := r.activeID(), r.workspaceBounds()
	focus, focuses := paneDirection(msg, [4]key.Binding{r.keymap.PaneFocusLeft, r.keymap.PaneFocusDown, r.keymap.PaneFocusUp, r.keymap.PaneFocusRight})
	swap, swaps := paneDirection(msg, [4]key.Binding{r.keymap.PaneSwapLeft, r.keymap.PaneSwapDown, r.keymap.PaneSwapUp, r.keymap.PaneSwapRight})
	switch {
	case focuses:
		if target := r.workspace.Neighbour(active, focus, bounds.Width, bounds.Height); target != "" {
			r.tabs.SetActive(target)
		}
	case key.Matches(msg, r.keymap.PaneNext):
		r.tabs.SetActive(r.workspace.Cycle(active, 1, bounds.Width, bounds.Height))
	case key.Matches(msg, r.keymap.PanePrevious):
		r.tabs.SetActive(r.workspace.Cycle(active, -1, bounds.Width, bounds.Height))
	case key.Matches(msg, r.keymap.PaneClose):
		if r.workspace.Count() > 1 {
			target := r.workspace.Remove(active)
			r.tabs.SetActive(target)
			r.paneZoom = false
		}
	case key.Matches(msg, r.keymap.PaneZoom):
		if r.workspace.Count() > 1 {
			r.paneZoom = !r.paneZoom
		}
	case !r.workspace.Arrange(bounds.Width, bounds.Height).Fits:
		r.paneHint = "Enlarge the terminal or close panes before editing the layout"
	case key.Matches(msg, r.keymap.PaneSplitColumns):
		return r.beginPaneSplit(workspace.Columns)
	case key.Matches(msg, r.keymap.PaneSplitRows):
		return r.beginPaneSplit(workspace.Rows)
	case swaps:
		if r.workspace.Move(active, swap, bounds.Width, bounds.Height) {
			r.paneZoom = false
		} else {
			r.paneHint = "Move refused: no room in that direction"
		}
	default:
		for _, resize := range []struct {
			binding key.Binding
			axis    workspace.Axis
			step    int
		}{
			{r.keymap.PaneNarrow, workspace.Columns, -1},
			{r.keymap.PaneWiden, workspace.Columns, 1},
			{r.keymap.PaneShorten, workspace.Rows, -1},
			{r.keymap.PaneTaller, workspace.Rows, 1},
		} {
			if key.Matches(msg, resize.binding) && r.workspace.Resize(active, resize.axis, resize.step, bounds.Width, bounds.Height) {
				r.paneZoom = false
				break
			}
		}
	}
	r.handleResize()
	return nil
}

func (r *Root) beginPaneJoin(axis workspace.Axis) tea.Cmd {
	bounds := r.workspaceBounds()
	if err := r.workspace.CanSplit(r.activeID(), axis, bounds.Width, bounds.Height); err != nil {
		r.paneHint = err.Error()
		r.handleResize()
		return nil
	}
	r.splitSequence++
	r.pendingSplit = &paneSplitRequest{ticket: r.splitSequence, revision: r.workspace.Revision(), target: r.activeID(), axis: axis}
	r.tabs.BlurActive()
	j := newJoin(r.width, r.settings, r.theme, r.keymap, r.clients, r.accountProvider)
	j.setTabOptions(r.availableTabKinds()...)
	j.Resize(r.width, r.height)
	ticket := r.splitSequence
	j.onJoin = func(join tab.JoinChannelMessage) tea.Msg { return paneJoinMessage{ticket: ticket, join: join} }
	return r.stack.Push(j)
}

func (r *Root) beginPaneSplit(axis workspace.Axis) tea.Cmd {
	bounds := r.workspaceBounds()
	if err := r.workspace.CanSplit(r.activeID(), axis, bounds.Width, bounds.Height); err != nil {
		r.paneHint = err.Error()
		r.handleResize()
		return nil
	}
	r.splitSequence++
	r.pendingSplit = &paneSplitRequest{ticket: r.splitSequence, revision: r.workspace.Revision(), target: r.activeID(), axis: axis}
	var choices []nav.Row
	for _, row := range r.visibleRows() {
		// Only standalone tabs can join a pane. A tab owned by another workspace
		// must not be duplicated across layouts.
		if r.workspaceForTab(row.ID) == nil {
			choices = append(choices, row)
		}
	}
	r.tabs.BlurActive()
	return r.stack.Push(newPanePicker(r.width, r.height, r.theme, r.keymap, r.splitSequence, choices))
}

func (r *Root) splitValid(ticket uint64) bool {
	return r.pendingSplit != nil && r.pendingSplit.ticket == ticket &&
		r.pendingSplit.revision == r.workspace.Revision() && r.workspace.Contains(r.pendingSplit.target)
}

func (r *Root) choosePane(msg paneChoiceMessage) tea.Cmd {
	if r.pendingSplit == nil || r.pendingSplit.ticket != msg.ticket {
		return nil // a cancelled/older picker's result must not affect a new dialog
	}
	if !r.splitValid(msg.ticket) {
		return r.failPaneSplit("Split cancelled: its target has changed")
	}
	if msg.tabID == "" {
		j := newJoin(r.width, r.settings, r.theme, r.keymap, r.clients, r.accountProvider)
		j.setTabOptions(r.availableTabKinds()...)
		j.Resize(r.width, r.height)
		ticket := msg.ticket
		j.onJoin = func(join tab.JoinChannelMessage) tea.Msg { return paneJoinMessage{ticket: ticket, join: join} }
		return r.stack.Replace(j)
	}
	if r.tabByID(msg.tabID) == nil {
		return r.failPaneSplit("Split cancelled: the selected tab has closed")
	}
	bounds := r.workspaceBounds()
	if err := r.workspace.Split(r.pendingSplit.target, msg.tabID, r.pendingSplit.axis, bounds.Width, bounds.Height); err != nil {
		return r.failPaneSplit(err.Error())
	}
	r.pendingSplit = nil
	r.paneZoom = false
	r.stack.Pop()
	r.tabs.SetActive(msg.tabID)
	r.tabs.FocusActive()
	r.handleResize()
	return nil
}

func (r *Root) failPaneSplit(message string) tea.Cmd {
	cmd := r.dismissModal()
	r.paneHint = message
	r.handleResize()
	return cmd
}

func (r *Root) dismissModal() tea.Cmd {
	r.pendingSplit = nil
	cmd := r.stack.Pop()
	if r.stack.Len() == 0 {
		r.tabs.FocusActive()
	}
	r.handleResize()
	return cmd
}

func (r *Root) closeTab(id string) tea.Cmd {
	closed := r.tabByID(id)
	if closed == nil {
		return nil
	}
	wasActive, hadSplits, assigned := r.activeID() == id, r.workspace.Count() > 1, r.workspace.Contains(id)
	closed.Close()
	if target := r.removeTabFromWorkspaces(id); wasActive && target != "" {
		// Avoid a transient tab-order activation and its notification clearing.
		r.tabs.SetActive(target)
	}
	r.tabs.Remove(id)
	if assigned && hadSplits {
		r.paneZoom = false
	}
	if !r.workspace.Contains(r.activeID()) {
		// Closing the last pane while its workspace is live leaves the tab view
		// standalone; pane commands must not continue controlling an empty layout.
		r.paneMode, r.paneZoom = false, false
	}
	if r.tabs.Len() == 0 {
		r.paneMode, r.paneZoom, r.paneHint = false, false, ""
	}
	if r.stack.Len() > 0 {
		r.tabs.BlurActive()
	}
	r.handleResize()
	if !closed.IsDataLoaded() || closed.Kind() != tab.BroadcastKind {
		return nil
	}
	sameAccountChannel := r.tabs.Any(func(t tab.Tab) bool {
		return t.AccountID() == closed.AccountID() && t.ChannelID() == closed.ChannelID()
	})
	sameChannel := r.tabs.Any(func(t tab.Tab) bool { return t.ChannelID() == closed.ChannelID() })
	if !sameChannel && r.emoteCache != nil {
		log.Logger.Info().Str("channel", closed.Channel()).Str("channel-id", closed.ChannelID()).Msg("removing emote cache entry for channel")
		r.emoteCache.RemoveEmoteSetForChannel(closed.ChannelID())
	}
	if r.pool == nil {
		return nil
	}
	pool, accountID, channel := r.pool, closed.AccountID(), closed.Channel()
	var cmds []tea.Cmd
	if !sameAccountChannel {
		log.Logger.Info().Str("channel", channel).Str("id", accountID).Msg("sending part message")
		cmds = append(cmds, func() tea.Msg {
			pool.SendIRC(accountID, twitchirc.PartMessage{Channel: channel})
			return nil
		})
	}
	cmds = append(cmds, func() tea.Msg {
		pool.DisconnectIRC(accountID)
		return nil
	})
	return tea.Sequence(cmds...)
}

// finishJoin shares the normal creation flow with the split picker, but commits
// split selection only after validating its ticket, target generation, and fit.
func (r *Root) finishJoin(msg tab.JoinChannelMessage, ticket uint64) tea.Cmd {
	if ticket == 0 && r.pendingSplit != nil {
		return nil
	}
	var bounds workspace.Rect
	if ticket != 0 {
		if r.pendingSplit == nil || r.pendingSplit.ticket != ticket {
			return nil
		}
		if !r.splitValid(ticket) {
			return r.failPaneSplit("Split cancelled: its target has changed")
		}
		// Validate the final sidebar width before constructing a tab: constructors
		// may start owned resources, which a rejected split must never allocate.
		title, identity := tab.Caption(msg.TabKind, msg.Account, msg.Channel)
		bounds = r.boundsWithCaption(nav.Row{Title: title, Identity: identity})
		if err := r.workspace.CanSplit(r.pendingSplit.target, r.pendingSplit.axis, bounds.Width, bounds.Height); err != nil {
			return r.failPaneSplit(err.Error())
		}
	}
	n, cmd := r.createTab(msg.Account, msg.Channel, msg.TabKind)
	if n == nil {
		return nil
	}
	if ticket != 0 {
		if err := r.workspace.Split(r.pendingSplit.target, n.ID(), r.pendingSplit.axis, bounds.Width, bounds.Height); err != nil {
			n.Close()
			return r.failPaneSplit(err.Error())
		}
		r.paneZoom = false
	}
	r.pendingSplit = nil
	r.stack.Pop()
	r.tabs.Add(n)
	if ticket != 0 {
		// The split already assigned this tab; focus it without rewriting panes.
		r.tabs.SetActive(n.ID())
		r.handleResize()
	} else {
		// A newly opened standalone tab must not evict a visible pane.
		r.navigateToTab(n.ID())
	}
	r.tabs.FocusActive()
	cmds := []tea.Cmd{n.Init(), cmd}
	if msg.TabKind == tab.BroadcastKind && msg.Channel != "" {
		channel := msg.Channel
		if !slices.Contains(r.channelSuggestions, channel) {
			r.channelSuggestions = append(r.channelSuggestions, channel)
		}
		if history := r.channelHistory; history != nil {
			cmds = append(cmds, func() tea.Msg {
				if err := history.RecordChannel(channel); err != nil {
					log.Logger.Err(err).Str("channel", channel).Msg("failed to record channel history")
				}
				return nil
			})
		}
	}
	if len(r.channelSuggestions) > 0 {
		cmds = append(cmds, nav.To(n.ID(), tab.ChannelSuggestionsLoadedMessage{Channels: r.channelSuggestions}))
	}
	return tea.Batch(cmds...)
}
