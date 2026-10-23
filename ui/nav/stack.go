package nav

import tea "charm.land/bubbletea/v2"

// Screen is a full-window view layered on top of a base view.
type Screen interface {
	Init() tea.Cmd
	Update(tea.Msg) (Screen, tea.Cmd)
	View() string
}

// Overlay is a screen that keeps the view below it visible. Non-overlay screens
// replace what is below them instead of compositing over it.
type Overlay interface {
	Screen
	OverlaysBackground() bool
}

// Lifecycle is called exactly once per push and pop, in stack order. It is the
// place for focus and blur: the stack guarantees a screen only sees input while
// it is on top, so it only has to be focused while it is on top.
type Lifecycle interface {
	Appeared() tea.Cmd
	Disappeared()
}

// Routed screens expose a stable route, so the owner can ask "which screen is
// open?" without asserting on concrete screen types.
type Routed interface {
	Route() string
}

// Resizable screens follow the terminal size.
type Resizable interface {
	Resize(width, height int)
}

// Stack layers screens over a base view owned by the caller. Only the top screen
// receives messages, which makes "a modal owns the keyboard" structural rather
// than something every caller has to remember.
type Stack struct {
	base    func() string
	compose func(fg, bg string) string
	screens []Screen
}

// NewStack returns a stack that renders the given base view when empty and
// composites overlay screens with the given compose function.
func NewStack(base func() string, compose func(fg, bg string) string) *Stack {
	return &Stack{base: base, compose: compose}
}

// Len reports how many screens are pushed on top of the base view.
func (s *Stack) Len() int {
	return len(s.screens)
}

// Top returns the topmost screen. ok is false when only the base view is shown.
func (s *Stack) Top() (Screen, bool) {
	return s.top()
}

// Route returns the route of the topmost screen, or "" when the base view is
// shown or the screen does not declare one.
func (s *Stack) Route() string {
	top, ok := s.top()
	if !ok {
		return ""
	}

	if routed, ok := top.(Routed); ok {
		return routed.Route()
	}

	return ""
}

// Push puts scr on top of the stack and returns its Appeared (or Init) command.
func (s *Stack) Push(scr Screen) tea.Cmd {
	if top, ok := s.top(); ok {
		if lifecycle, ok := top.(Lifecycle); ok {
			lifecycle.Disappeared()
		}
	}

	s.screens = append(s.screens, scr)

	if lifecycle, ok := scr.(Lifecycle); ok {
		return lifecycle.Appeared()
	}

	return scr.Init()
}

// Replace swaps the topmost screen. It is a no-op on an empty stack.
func (s *Stack) Replace(scr Screen) tea.Cmd {
	if len(s.screens) == 0 {
		return s.Push(scr)
	}

	if lifecycle, ok := s.screens[len(s.screens)-1].(Lifecycle); ok {
		lifecycle.Disappeared()
	}

	s.screens[len(s.screens)-1] = scr

	if lifecycle, ok := scr.(Lifecycle); ok {
		return lifecycle.Appeared()
	}

	return scr.Init()
}

// Pop removes the topmost screen and re-appears the screen below it.
func (s *Stack) Pop() tea.Cmd {
	if len(s.screens) == 0 {
		return nil
	}

	top := s.screens[len(s.screens)-1]

	if lifecycle, ok := top.(Lifecycle); ok {
		lifecycle.Disappeared()
	}

	s.screens = s.screens[:len(s.screens)-1]

	if next, ok := s.top(); ok {
		if lifecycle, ok := next.(Lifecycle); ok {
			return lifecycle.Appeared()
		}
	}

	return nil
}

// Update hands msg to the topmost screen and writes it back.
func (s *Stack) Update(msg tea.Msg) tea.Cmd {
	i := len(s.screens) - 1
	if i < 0 {
		return nil
	}

	next, cmd := s.screens[i].Update(msg)
	if next != nil {
		s.screens[i] = next
	}

	return cmd
}

// View renders the topmost screen, composited over what is below it when the
// screen is an overlay.
func (s *Stack) View() string {
	top, ok := s.top()
	if !ok {
		return s.base()
	}

	content := top.View()

	overlay, isOverlay := top.(Overlay)
	if !isOverlay || !overlay.OverlaysBackground() || s.compose == nil {
		return content
	}

	return s.compose(content, s.background())
}

// Resize forwards the terminal size to every screen and to the base view.
func (s *Stack) Resize(width, height int) {
	for _, scr := range s.screens {
		if resizable, ok := scr.(Resizable); ok {
			resizable.Resize(width, height)
		}
	}
}

func (s *Stack) top() (Screen, bool) {
	if len(s.screens) == 0 {
		return nil, false
	}

	return s.screens[len(s.screens)-1], true
}

// background is what an overlay is composited over: the screen directly below it,
// or the base view when the overlay is the only pushed screen.
func (s *Stack) background() string {
	if len(s.screens) < 2 {
		return s.base()
	}

	return s.screens[len(s.screens)-2].View()
}
