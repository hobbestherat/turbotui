package tv

import tui "github.com/hobbestherat/turbotui"

// Capability interfaces are the opt-in contract for widget behaviour. Instead of
// discovering at runtime that an On*Fn field has the wrong signature or was never
// set, a widget implements the interface for each thing it does — and the compiler
// checks the method signature. The desktop and the bubble dispatchers consult
// these interfaces (e.g. `if t, ok := node.(Typer); ok`), so behaviour is wired by
// implementing a method, not by populating a function field.
//
// *VisualComponent satisfies every capability interface by delegating to its
// matching On*Fn field, so the convenience "assign a closure" style keeps working.
// A widget that would rather implement the interfaces on its own type calls
// VisualComponent.Bind to forward them into those fields (see Bind).
type (
	// Painter draws the component's own content onto surface (children are drawn
	// by the framework afterwards).
	Painter interface {
		Paint(surface Surface)
	}

	// Typer handles a key event, returning true when it consumed it. Unconsumed
	// events bubble to the parent.
	Typer interface {
		HandleType(event tui.TypeEvent) bool
	}

	// Paster handles a bracketed-paste block, returning true when it consumed it.
	Paster interface {
		HandlePaste(text string) bool
	}

	// Clicker handles a mouse press/release, returning true when it consumed it.
	Clicker interface {
		HandleClick(event tui.ClickEvent) bool
	}

	// Scroller handles a scroll-wheel event, returning true when it consumed it.
	Scroller interface {
		HandleScroll(event tui.ScrollEvent) bool
	}

	// Focuser is notified when the component gains (focused=true) or loses focus.
	Focuser interface {
		HandleFocus(focused bool)
	}

	// HitTester decides whether (x, y) — in absolute screen coordinates — counts as
	// inside the component, overriding the default bounds test.
	HitTester interface {
		HitTest(x int, y int) bool
	}

	// Cursorer reports where the hardware text cursor should sit while the
	// component is focused (ok=false hides it).
	Cursorer interface {
		Cursor() (x int, y int, ok bool)
	}

	// Copier returns the text the component would put on the clipboard for Copy,
	// and whether there is anything to copy.
	Copier interface {
		Copy() (text string, ok bool)
	}

	// Cutter removes the component's cuttable text (e.g. a selection) and returns
	// it plus whether anything was cut.
	Cutter interface {
		Cut() (text string, ok bool)
	}
)

// Compile-time assertions that the convenience struct implements every capability.
var (
	_ Painter   = (*VisualComponent)(nil)
	_ Typer     = (*VisualComponent)(nil)
	_ Paster    = (*VisualComponent)(nil)
	_ Clicker   = (*VisualComponent)(nil)
	_ Scroller  = (*VisualComponent)(nil)
	_ Focuser   = (*VisualComponent)(nil)
	_ HitTester = (*VisualComponent)(nil)
	_ Cursorer  = (*VisualComponent)(nil)
	_ Copier    = (*VisualComponent)(nil)
	_ Cutter    = (*VisualComponent)(nil)
)

// Paint draws the component's own content via its DrawFn (a no-op when unset).
func (c *VisualComponent) Paint(surface Surface) {
	if c.DrawFn != nil {
		c.DrawFn(c, surface)
	}
}

// HandleType offers a key event to the component's OnTypeFn.
func (c *VisualComponent) HandleType(event tui.TypeEvent) bool {
	return c.OnTypeFn != nil && c.OnTypeFn(c, event)
}

// HandlePaste offers pasted text to the component's OnPasteFn.
func (c *VisualComponent) HandlePaste(text string) bool {
	return c.OnPasteFn != nil && c.OnPasteFn(c, text)
}

// HandleClick offers a mouse event to the component's OnClickFn.
func (c *VisualComponent) HandleClick(event tui.ClickEvent) bool {
	return c.OnClickFn != nil && c.OnClickFn(c, event)
}

// HandleScroll offers a scroll event to the component's OnScrollFn.
func (c *VisualComponent) HandleScroll(event tui.ScrollEvent) bool {
	return c.OnScrollFn != nil && c.OnScrollFn(c, event)
}

// HandleFocus notifies the component's OnFocusFn of a focus change.
func (c *VisualComponent) HandleFocus(focused bool) {
	if c.OnFocusFn != nil {
		c.OnFocusFn(c, focused)
	}
}

// HitTest reports whether (x, y) lies inside the component, deferring to OnHitTestFn
// when set and otherwise to the absolute bounds.
func (c *VisualComponent) HitTest(x int, y int) bool {
	if c.OnHitTestFn != nil {
		return c.OnHitTestFn(c, x, y)
	}
	return c.AbsoluteBounds().Contains(x, y)
}

// Cursor reports the focused text-cursor position via CursorFn (hidden when unset).
func (c *VisualComponent) Cursor() (int, int, bool) {
	if c.CursorFn != nil {
		return c.CursorFn(c)
	}
	return 0, 0, false
}

// Copy returns the component's clipboard text via CopyFn (nothing when unset).
func (c *VisualComponent) Copy() (string, bool) {
	if c.CopyFn != nil {
		return c.CopyFn(c)
	}
	return "", false
}

// Cut returns the component's cut text via CutFn (nothing when unset).
func (c *VisualComponent) Cut() (string, bool) {
	if c.CutFn != nil {
		return c.CutFn(c)
	}
	return "", false
}

// Bind wires the capability interfaces implemented by behavior into this
// component's On*Fn fields, so a widget can opt into behaviour by implementing a
// method (checked by the compiler) instead of assigning a function field by hand.
// Only the interfaces behavior actually implements are wired; the rest are left
// untouched. behavior is typically the outer widget struct that owns the
// component — do NOT pass the component itself, which would wire a field to call
// back into the same field.
func (c *VisualComponent) Bind(behavior any) {
	if p, ok := behavior.(Painter); ok {
		c.DrawFn = func(_ *VisualComponent, surface Surface) { p.Paint(surface) }
	}
	if t, ok := behavior.(Typer); ok {
		c.OnTypeFn = func(_ *VisualComponent, event tui.TypeEvent) bool { return t.HandleType(event) }
	}
	if p, ok := behavior.(Paster); ok {
		c.OnPasteFn = func(_ *VisualComponent, text string) bool { return p.HandlePaste(text) }
	}
	if cl, ok := behavior.(Clicker); ok {
		c.OnClickFn = func(_ *VisualComponent, event tui.ClickEvent) bool { return cl.HandleClick(event) }
	}
	if s, ok := behavior.(Scroller); ok {
		c.OnScrollFn = func(_ *VisualComponent, event tui.ScrollEvent) bool { return s.HandleScroll(event) }
	}
	if f, ok := behavior.(Focuser); ok {
		c.OnFocusFn = func(_ *VisualComponent, focused bool) { f.HandleFocus(focused) }
	}
	if h, ok := behavior.(HitTester); ok {
		c.OnHitTestFn = func(_ *VisualComponent, x int, y int) bool { return h.HitTest(x, y) }
	}
	if cu, ok := behavior.(Cursorer); ok {
		c.CursorFn = func(_ *VisualComponent) (int, int, bool) { return cu.Cursor() }
	}
	if cp, ok := behavior.(Copier); ok {
		c.CopyFn = func(_ *VisualComponent) (string, bool) { return cp.Copy() }
	}
	if ct, ok := behavior.(Cutter); ok {
		c.CutFn = func(_ *VisualComponent) (string, bool) { return ct.Cut() }
	}
}
