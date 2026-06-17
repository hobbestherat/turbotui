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
		FG:       DefaultTheme.WindowFG,
		BG:       DefaultTheme.WindowBG,
		FocusFG:  DefaultTheme.InputFocusFG,
		FocusBG:  DefaultTheme.InputFocusBG,
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
	fg, bg := inputColors(component.HasFocus, c.FG, c.BG, c.FocusFG, c.FocusBG)
	style := tui.Cell{FG: fg, BG: bg}
	surface.Fill(Rect{X: abs.X, Y: abs.Y, W: abs.W, H: 1}, tui.Cell{Ch: ' ', FG: fg, BG: bg})
	box := "[ ] "
	if c.Checked {
		box = "[x] "
	}
	surface.WriteString(abs.X, abs.Y, box, tui.Cell{FG: fg, BG: bg, Bold: true})
	drawMnemonic(surface, abs.X+4, abs.Y, c.Label, style, component.mnemonicActive, DefaultTheme.MnemonicFG)
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
