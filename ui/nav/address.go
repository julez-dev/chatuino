package nav

import tea "charm.land/bubbletea/v2"

// Envelope is a message addressed to one item of a registry.
type Envelope struct {
	To  string
	Msg any
}

// Address wraps msg for delivery to the item with the given id.
func Address(id string, msg any) Envelope {
	return Envelope{To: id, Msg: msg}
}

// To returns a command that delivers msg to the item with the given id and to no
// other. Children never see messages addressed elsewhere, so they do not need to
// re-check a target field.
func To(id string, msg any) tea.Cmd {
	return func() tea.Msg {
		return Address(id, msg)
	}
}

// Dispatch delivers an addressed message to its item. ok is false when msg is not
// an envelope or its target is not in the registry, in which case the caller
// should keep handling it.
func (r *Registry[T]) Dispatch(msg tea.Msg, update func(T, tea.Msg) (T, tea.Cmd)) (tea.Cmd, bool) {
	envelope, ok := msg.(Envelope)
	if !ok {
		return nil, false
	}

	return r.UpdateOne(envelope.To, envelope.Msg, update)
}
