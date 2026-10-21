package tab

import "charm.land/lipgloss/v2"

// layoutSpan is the vertical extent a region occupies in the tab view. Rows are
// zero-based and relative to the top of the tab.
type layoutSpan struct {
	top    int
	height int
}

// visible reports whether the region occupies any rows.
func (s layoutSpan) visible() bool { return s.height > 0 }

// contains reports whether y falls inside the region.
func (s layoutSpan) contains(y int) bool { return y >= s.top && y < s.top+s.height }

// broadcastLayout is the row allocation of a broadcast tab. It is the single
// source of truth for three consumers:
//
//   - HandleResize sizes the children from the spans,
//   - the view renders the blocks in span order,
//   - the mouse handlers map a tab-local row back to the region under the cursor.
//
// Blocks are stacked without separator rows: the newline that joins two blocks
// terminates the upper block's last row, so the next block starts immediately
// below it. The stream info slot is the exception: it always occupies at least
// one row, even when the channel is offline, so the chat does not jump when the
// stream info appears.
type broadcastLayout struct {
	height int

	streamInfo    layoutSpan
	poll          layoutSpan
	chat          layoutSpan
	userInspect   layoutSpan
	composer      layoutSpan
	status        layoutSpan
	emoteOverview layoutSpan // replaces the content area in emote overview mode
}

// layoutInputs are the measured row counts of the fixed-size regions around the
// flexible chat area. Passing measurements instead of child models keeps the
// allocation a pure function that can be tested on its own.
type layoutInputs struct {
	height        int
	streamInfo    int // rows the stream info slot occupies; at least 1
	poll          int // rows the poll occupies; 0 when absent
	composer      int // rows the composer occupies; 0 when absent
	status        int // rows the status bar occupies inside the tab; 0 when absent
	inspecting    bool
	emoteOverview bool
}

// allocateBroadcastLayout distributes the available rows. The chat window and
// the user inspect window share the leftover space; in inspect mode the chat
// keeps half of the space above the composer, matching the pre-refactor
// allocation.
func allocateBroadcastLayout(in layoutInputs) broadcastLayout {
	l := broadcastLayout{height: in.height}

	if in.emoteOverview {
		overviewHeight := max(0, in.height-in.status)
		l.emoteOverview = layoutSpan{top: 0, height: overviewHeight}
		l.status = layoutSpan{top: overviewHeight, height: in.status}

		return l
	}

	l.streamInfo = layoutSpan{top: 0, height: max(1, in.streamInfo)}
	top := l.streamInfo.height

	l.poll = layoutSpan{top: top, height: in.poll}
	top += in.poll

	var chatHeight int
	if in.inspecting {
		chatHeight = (in.height - l.streamInfo.height - in.poll - in.status) / 2
	} else {
		chatHeight = in.height - l.streamInfo.height - in.poll - in.composer - in.status
	}

	chatHeight = max(0, chatHeight)

	l.chat = layoutSpan{top: top, height: chatHeight}
	top += chatHeight

	if in.inspecting {
		inspectHeight := in.height - l.streamInfo.height - in.poll - chatHeight - in.status - in.composer

		l.userInspect = layoutSpan{top: top, height: max(0, inspectHeight)}
		top += l.userInspect.height
	}

	l.composer = layoutSpan{top: top, height: max(0, in.composer)}
	top += l.composer.height

	l.status = layoutSpan{top: top, height: max(0, in.status)}

	return l
}

// viewHeight returns the number of rows a rendered block occupies. An empty
// block occupies no rows, unlike lipgloss.Height which would report one.
func viewHeight(view string) int {
	if view == "" {
		return 0
	}

	return lipgloss.Height(view)
}
