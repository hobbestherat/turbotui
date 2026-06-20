package tv

import tui "github.com/hobbestherat/turbotui"

// Checkbox is a single on/off toggle rendered as "[x] Label" / "[ ] Label".
// It is focusable and toggles on Space, Enter, mouse click, or its mnemonic.
type Checkbox struct {
	Component *VisualComponent
	Label     string
	Checked   bool
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
	FocusBG   tui.Color
	// OnToggle, when set, is called with the new checked state after every change.
	OnToggle func(checked bool)
}

// NewCheckbox creates a checkbox. label may declare a mnemonic with '&'.
func NewCheckbox(label string, bounds Rect, onToggle func(bool)) *Checkbox {
	cb := &Checkbox{
		Label:    label,
		FG:       activeTheme.WindowFG,
		BG:       activeTheme.WindowBG,
		FocusFG:  activeTheme.InputFocusFG,
		FocusBG:  activeTheme.InputFocusBG,
		OnToggle: onToggle,
	}
	cb.Component = NewComponent(bounds)
	cb.Component.Focusable = true
	cb.Component.DrawFn = cb.draw
	cb.Component.OnTypeFn = cb.handleType
	cb.Component.OnClickFn = cb.handleClick
	cb.Component.Mnemonic = labelMnemonic(label)
	cb.Component.OnActivateFn = func(_ *VisualComponent) { cb.toggle() }
	return cb
}
func (c *Checkbox) Root() *VisualComponent { return c.Component }

// SetChecked sets the state without firing OnToggle.
func (c *Checkbox) SetChecked(checked bool) { c.Checked = checked }

// IsChecked reports the current state.
func (c *Checkbox) IsChecked() bool { return c.Checked }
func (c *Checkbox) toggle() bool {
	c.Checked = !c.Checked
	if c.OnToggle != nil {
		c.OnToggle(c.Checked)
	}
	return true
}
func (c *Checkbox) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := focusColors(component.HasFocus, c.FG, c.BG, c.FocusFG, c.FocusBG)
	style := tui.Cell{FG: fg, BG: bg}
	// Fill the whole bounds (not just the first row) so the focus highlight and
	// the click hit area (handleClick toggles anywhere in abs) match what is drawn
	// when a caller sizes the checkbox taller than one row.
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: fg, BG: bg})
	box := "[ ] "
	if c.Checked {
		box = "[x] "
	}
	surface.WriteString(abs.X, abs.Y, box, tui.Cell{FG: fg, BG: bg, Bold: true})
	// Truncate the label to the space after the "[x] " marker so a long caption
	// shows an ellipsis instead of being silently cut by the surface clip.
	drawMnemonicClipped(surface, abs.X+4, abs.Y, c.Label, abs.W-4, style, component.mnemonicActive, activeTheme.MnemonicFG)
}
func (c *Checkbox) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	if event.Key == tui.KeyEnter || (event.Key == tui.KeyRune && event.Rune == ' ') {
		return c.toggle()
	}
	return false
}
func (c *Checkbox) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	if event.Down {
		return true
	}
	if component.AbsoluteBounds().Contains(event.X, event.Y) {
		return c.toggle()
	}
	return true
}
