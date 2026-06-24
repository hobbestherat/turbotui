package tv

// RadioGroup turns a set of Checkboxes into a mutually-exclusive (radio) group:
// checking one box unchecks the rest, and the selected box cannot be unchecked by
// clicking it again (the standard radio contract — selection only moves to another
// option). It owns no component of its own, so the caller lays the checkboxes out
// however it likes (a VBox, a Grid, …) and just registers each with Add.
//
// Promoted from gogent's hand-rolled radio exclusion (settings_dialog.go) so any
// turbotui app can reuse it. Pair it with Tabs/Checkbox to build single-choice
// form fields.
type RadioGroup struct {
	// Boxes are the member checkboxes in the order they were added; the index of a
	// box in this slice is its selection index.
	Boxes []*Checkbox
	// OnChange, when set, is called with the newly selected index after the
	// selection moves (not on a no-op re-click of the current selection).
	OnChange func(index int)

	selected int
}

// NewRadioGroup creates an empty group with nothing selected.
func NewRadioGroup() *RadioGroup {
	return &RadioGroup{selected: -1}
}

// Add registers cb as a member and returns the group for chaining. It wraps cb's
// OnToggle to enforce exclusion while still invoking any callback already set on
// the box (set the per-box callback before Add), so a member can keep its own
// reaction alongside the group's OnChange. The wrapped callback runs after the
// exclusion logic and sees the box's settled checked state. A box that is already
// checked when added becomes the selection.
func (g *RadioGroup) Add(cb *Checkbox) *RadioGroup {
	index := len(g.Boxes)
	g.Boxes = append(g.Boxes, cb)
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
		g.Boxes[index].SetChecked(true)
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
	for i, box := range g.Boxes {
		box.SetChecked(i == index)
	}
	g.selected = index
}

// Selected returns the selected index, or -1 when nothing is selected.
func (g *RadioGroup) Selected() int { return g.selected }

// SetSelected selects the box at index (out-of-range clears the selection) without
// firing OnChange — the programmatic counterpart to a user click.
func (g *RadioGroup) SetSelected(index int) {
	if index < 0 || index >= len(g.Boxes) {
		for _, box := range g.Boxes {
			box.SetChecked(false)
		}
		g.selected = -1
		return
	}
	g.applySelection(index)
}

// Value returns the label of the selected box, or "" when nothing is selected.
func (g *RadioGroup) Value() string {
	if g.selected < 0 || g.selected >= len(g.Boxes) {
		return ""
	}
	return g.Boxes[g.selected].Label
}

// MultiSelect groups Checkboxes whose toggles are independent, exposing the set of
// checked members. Like RadioGroup it owns no component — the caller arranges the
// boxes — and is the multi-select peer of RadioGroup for building checkbox-group
// form fields.
type MultiSelect struct {
	// Boxes are the member checkboxes in the order they were added.
	Boxes []*Checkbox
	// OnChange, when set, is called after any member toggles.
	OnChange func()
}

// NewMultiSelect creates an empty multi-select group.
func NewMultiSelect() *MultiSelect { return &MultiSelect{} }

// Add registers cb and returns the group for chaining. It wraps cb's OnToggle so
// any callback already set on the box (set it before Add) still fires, then
// notifies the group's OnChange; the box's own checked state is untouched.
func (m *MultiSelect) Add(cb *Checkbox) *MultiSelect {
	m.Boxes = append(m.Boxes, cb)
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

// Selected returns the indices of the checked boxes in ascending order.
func (m *MultiSelect) Selected() []int {
	out := make([]int, 0, len(m.Boxes))
	for i, box := range m.Boxes {
		if box.Checked {
			out = append(out, i)
		}
	}
	return out
}

// SelectedValues returns the labels of the checked boxes in order.
func (m *MultiSelect) SelectedValues() []string {
	out := make([]string, 0, len(m.Boxes))
	for _, box := range m.Boxes {
		if box.Checked {
			out = append(out, box.Label)
		}
	}
	return out
}
