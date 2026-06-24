package tv

// RadioGroup turns a set of Checkboxes into a mutually-exclusive (radio) group:
// checking one box unchecks the rest, and the selected box cannot be unchecked by
// clicking it again (the standard radio contract — selection only moves to another
// option). It owns no component of its own, so the caller lays the checkboxes out
// however it likes (a VBox, a Grid, …) and just registers each with Add.
//
// The member list is owned by the group (registration order is the selection
// index), so it is not exported: add members with Add and read them with Boxes,
// Len, Selected and Value. Promoted from gogent's hand-rolled radio exclusion
// (settings_dialog.go) so any turbotui app can reuse it; pair it with Tabs/Checkbox
// to build single-choice form fields.
type RadioGroup struct {
	// OnChange, when set, is called with the newly selected index after the
	// selection moves (not on a no-op re-click of the current selection).
	OnChange func(index int)

	boxes    []*Checkbox
	selected int
}

// NewRadioGroup creates an empty group with nothing selected.
func NewRadioGroup() *RadioGroup {
	return &RadioGroup{selected: -1}
}

// Add registers cb as a member and returns the group for chaining. A nil cb is
// ignored (not added), so callers never panic on a missing option. It wraps cb's
// OnToggle to enforce exclusion while still invoking any callback already set on
// the box (set the per-box callback before Add), so a member can keep its own
// reaction alongside the group's OnChange. The wrapped callback runs after the
// exclusion logic and sees the box's settled checked state. A box that is already
// checked when added becomes the selection.
func (g *RadioGroup) Add(cb *Checkbox) *RadioGroup {
	if cb == nil {
		return g
	}
	index := len(g.boxes)
	g.boxes = append(g.boxes, cb)
	if cb.Checked {
		g.applySelection(index)
	}
	prev := cb.OnToggle
	cb.OnToggle = func(checked bool) {
		g.onToggle(index, checked)
		if prev != nil {
			prev(cb.Checked)
		}
	}
	return g
}

// onToggle is the exclusion logic run whenever a member box is toggled.
func (g *RadioGroup) onToggle(index int, checked bool) {
	if !checked {
		// Re-clicking the active radio must not clear it; restore the checkmark and
		// keep the current selection.
		g.boxes[index].SetChecked(true)
		return
	}
	if g.selected == index {
		return
	}
	g.applySelection(index)
	if g.OnChange != nil {
		g.OnChange(index)
	}
}

// applySelection checks box index and unchecks every other member without firing
// callbacks.
func (g *RadioGroup) applySelection(index int) {
	for i, box := range g.boxes {
		box.SetChecked(i == index)
	}
	g.selected = index
}

// Len reports the number of member checkboxes.
func (g *RadioGroup) Len() int { return len(g.boxes) }

// Boxes returns a snapshot of the member checkboxes in registration order. It is a
// copy, so mutating the returned slice does not disturb the group's membership.
func (g *RadioGroup) Boxes() []*Checkbox {
	out := make([]*Checkbox, len(g.boxes))
	copy(out, g.boxes)
	return out
}

// Selected returns the selected index, or -1 when nothing is selected.
func (g *RadioGroup) Selected() int { return g.selected }

// SetSelected selects the box at index (out-of-range clears the selection) without
// firing OnChange — the programmatic counterpart to a user click.
func (g *RadioGroup) SetSelected(index int) {
	if index < 0 || index >= len(g.boxes) {
		for _, box := range g.boxes {
			box.SetChecked(false)
		}
		g.selected = -1
		return
	}
	g.applySelection(index)
}

// Value returns the label of the selected box, or "" when nothing is selected.
func (g *RadioGroup) Value() string {
	if g.selected < 0 || g.selected >= len(g.boxes) {
		return ""
	}
	return g.boxes[g.selected].Label
}

// MultiSelect groups Checkboxes whose toggles are independent, exposing the set of
// checked members. Like RadioGroup it owns no component — the caller arranges the
// boxes — and keeps its member list private (registration order is the reported
// index): add with Add and read with Boxes, Len, Selected and SelectedValues. It is
// the multi-select peer of RadioGroup for building checkbox-group form fields.
type MultiSelect struct {
	// OnChange, when set, is called after any member toggles.
	OnChange func()

	boxes []*Checkbox
}

// NewMultiSelect creates an empty multi-select group.
func NewMultiSelect() *MultiSelect { return &MultiSelect{} }

// Add registers cb and returns the group for chaining. A nil cb is ignored (not
// added). It wraps cb's OnToggle so any callback already set on the box (set it
// before Add) still fires, then notifies the group's OnChange; the box's own
// checked state is untouched.
func (m *MultiSelect) Add(cb *Checkbox) *MultiSelect {
	if cb == nil {
		return m
	}
	m.boxes = append(m.boxes, cb)
	prev := cb.OnToggle
	cb.OnToggle = func(checked bool) {
		if prev != nil {
			prev(checked)
		}
		if m.OnChange != nil {
			m.OnChange()
		}
	}
	return m
}

// Len reports the number of member checkboxes.
func (m *MultiSelect) Len() int { return len(m.boxes) }

// Boxes returns a snapshot of the member checkboxes in registration order. It is a
// copy, so mutating the returned slice does not disturb the group's membership.
func (m *MultiSelect) Boxes() []*Checkbox {
	out := make([]*Checkbox, len(m.boxes))
	copy(out, m.boxes)
	return out
}

// Selected returns the indices of the checked boxes in ascending order.
func (m *MultiSelect) Selected() []int {
	out := make([]int, 0, len(m.boxes))
	for i, box := range m.boxes {
		if box.Checked {
			out = append(out, i)
		}
	}
	return out
}

// SelectedValues returns the labels of the checked boxes in order.
func (m *MultiSelect) SelectedValues() []string {
	out := make([]string, 0, len(m.boxes))
	for _, box := range m.boxes {
		if box.Checked {
			out = append(out, box.Label)
		}
	}
	return out
}
