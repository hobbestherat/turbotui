package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// recorder implements every capability interface and records what it was asked to
// do, so a single value can prove Bind wires each interface into the matching
// On*Fn field. consume is what the bubbling handlers return.
type recorder struct {
	painted   bool
	typed     *tui.TypeEvent
	pasted    *string
	clicked   *tui.ClickEvent
	scrolled  *tui.ScrollEvent
	focus     *bool
	hitCalled bool
	consume   bool
}

func (r *recorder) Paint(Surface)                       { r.painted = true }
func (r *recorder) HandleType(e tui.TypeEvent) bool     { r.typed = &e; return r.consume }
func (r *recorder) HandlePaste(t string) bool           { r.pasted = &t; return r.consume }
func (r *recorder) HandleClick(e tui.ClickEvent) bool   { r.clicked = &e; return r.consume }
func (r *recorder) HandleScroll(e tui.ScrollEvent) bool { r.scrolled = &e; return r.consume }
func (r *recorder) HandleFocus(f bool)                  { r.focus = &f }
func (r *recorder) HitTest(int, int) bool               { r.hitCalled = true; return true }
func (r *recorder) Cursor() (int, int, bool)            { return 7, 9, true }
func (r *recorder) Copy() (string, bool)                { return "copied", true }
func (r *recorder) Cut() (string, bool)                 { return "cut", true }

// Compile-time proof the recorder really satisfies the whole contract.
var (
	_ Painter   = (*recorder)(nil)
	_ Typer     = (*recorder)(nil)
	_ Paster    = (*recorder)(nil)
	_ Clicker   = (*recorder)(nil)
	_ Scroller  = (*recorder)(nil)
	_ Focuser   = (*recorder)(nil)
	_ HitTester = (*recorder)(nil)
	_ Cursorer  = (*recorder)(nil)
	_ Copier    = (*recorder)(nil)
	_ Cutter    = (*recorder)(nil)
)

func TestBindWiresEveryCapability(t *testing.T) {
	app := tui.NewWithSize(20, 10, nil)
	c := NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1})
	r := &recorder{consume: true}
	c.Bind(r)

	c.Paint(newRootSurface(app))
	if !r.painted {
		t.Error("Paint not routed to bound behaviour")
	}
	if !c.HandleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'}) || r.typed == nil || r.typed.Rune != 'a' {
		t.Error("HandleType not routed to bound behaviour")
	}
	if !c.HandlePaste("hi") || r.pasted == nil || *r.pasted != "hi" {
		t.Error("HandlePaste not routed to bound behaviour")
	}
	if !c.HandleClick(tui.ClickEvent{X: 1, Down: true}) || r.clicked == nil {
		t.Error("HandleClick not routed to bound behaviour")
	}
	if !c.HandleScroll(tui.ScrollEvent{Delta: 1}) || r.scrolled == nil {
		t.Error("HandleScroll not routed to bound behaviour")
	}
	c.HandleFocus(true)
	if r.focus == nil || !*r.focus {
		t.Error("HandleFocus not routed to bound behaviour")
	}
	if !c.HitTest(3, 3) || !r.hitCalled {
		t.Error("HitTest not routed to bound behaviour")
	}
	if x, y, ok := c.Cursor(); !ok || x != 7 || y != 9 {
		t.Errorf("Cursor = %d,%d,%v; want 7,9,true", x, y, ok)
	}
	if text, ok := c.Copy(); !ok || text != "copied" {
		t.Errorf("Copy = %q,%v; want copied,true", text, ok)
	}
	if text, ok := c.Cut(); !ok || text != "cut" {
		t.Errorf("Cut = %q,%v; want cut,true", text, ok)
	}
}

// onlyTyper implements a single capability, proving Bind leaves the others nil.
type onlyTyper struct{ called bool }

func (o *onlyTyper) HandleType(tui.TypeEvent) bool { o.called = true; return true }

func TestBindSkipsUnimplementedCapabilities(t *testing.T) {
	c := NewComponent(Rect{W: 1, H: 1})
	o := &onlyTyper{}
	c.Bind(o)

	if c.OnTypeFn == nil {
		t.Fatal("Typer should have been wired")
	}
	if c.OnClickFn != nil || c.OnScrollFn != nil || c.OnPasteFn != nil || c.CopyFn != nil {
		t.Error("Bind wired a capability the behaviour does not implement")
	}
	// Unimplemented capabilities fall through harmlessly.
	if c.HandleClick(tui.ClickEvent{}) {
		t.Error("HandleClick should report not-consumed with no behaviour")
	}
	if _, ok := c.Copy(); ok {
		t.Error("Copy should report nothing with no behaviour")
	}
	if !c.HandleType(tui.TypeEvent{}) || !o.called {
		t.Error("Typer should be invoked")
	}
}

// TestBubbleDispatchUsesCapabilityInterface proves the bubble dispatcher offers
// the event through the Typer interface and that an unhandled event climbs to an
// ancestor that implements it.
func TestBubbleDispatchUsesCapabilityInterface(t *testing.T) {
	root := NewComponent(Rect{W: 10, H: 10})
	child := NewComponent(Rect{W: 4, H: 1})
	root.AddChild(child)

	r := &recorder{consume: true}
	root.Bind(r)

	if !child.BubbleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'z'}) {
		t.Fatal("event should have been consumed by ancestor")
	}
	if r.typed == nil || r.typed.Rune != 'z' {
		t.Error("ancestor Typer was not reached by bubbling")
	}
}

// TestComponentSatisfiesCapabilities is the runtime side of the compile-time
// assertions in capability.go: a plain component delegates each capability to its
// On*Fn field.
func TestComponentSatisfiesCapabilities(t *testing.T) {
	c := NewComponent(Rect{W: 5, H: 1})
	got := ""
	c.OnTypeFn = func(_ *VisualComponent, e tui.TypeEvent) bool { got = string(e.Rune); return true }

	var typer Typer = c
	if !typer.HandleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'q'}) || got != "q" {
		t.Errorf("component as Typer did not delegate to OnTypeFn (got %q)", got)
	}
}

func TestChildrenAccessorReturnsSnapshot(t *testing.T) {
	root := NewComponent(Rect{W: 10, H: 10})
	a := NewComponent(Rect{W: 1, H: 1})
	b := NewComponent(Rect{W: 1, H: 1})
	root.AddChild(a)
	root.AddChild(b)

	kids := root.Children()
	if len(kids) != 2 || kids[0] != a || kids[1] != b {
		t.Fatalf("Children() = %v; want [a b]", kids)
	}
	if a.Parent() != root {
		t.Error("Parent() should report the container")
	}
	// Mutating the snapshot must not alter the tree.
	kids[0] = nil
	if root.Children()[0] != a {
		t.Error("Children() returned an aliased slice; the tree was mutated")
	}

	root.RemoveChild(a)
	if a.Parent() != nil {
		t.Error("RemoveChild should clear the parent link")
	}
	if got := root.Children(); len(got) != 1 || got[0] != b {
		t.Errorf("after RemoveChild Children() = %v; want [b]", got)
	}
}

// TestFocusInvariantSingleFocused proves focus is framework-driven: only setFocus
// flips Focused(), at most one component is focused at a time, and the focus
// notification fires on both the old and new component.
func TestFocusInvariantSingleFocused(t *testing.T) {
	var out bytes.Buffer
	app := tui.NewWithSize(40, 10, &out)
	d := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 10})
	a := NewComponent(Rect{X: 1, Y: 1, W: 5, H: 1})
	b := NewComponent(Rect{X: 1, Y: 3, W: 5, H: 1})
	a.Focusable = true
	b.Focusable = true
	var aFocus, bFocus bool
	a.OnFocusFn = func(_ *VisualComponent, f bool) { aFocus = f }
	b.OnFocusFn = func(_ *VisualComponent, f bool) { bFocus = f }
	root.AddChild(a)
	root.AddChild(b)
	d.AddLayer(NewLayer("l", root, true, false))

	d.SetFocus(a)
	if !a.Focused() || b.Focused() {
		t.Fatalf("after focus a: a=%v b=%v; want true false", a.Focused(), b.Focused())
	}
	if !aFocus {
		t.Error("focus-gained notification did not fire for a")
	}

	d.SetFocus(b)
	if a.Focused() || !b.Focused() {
		t.Fatalf("after focus b: a=%v b=%v; want false true", a.Focused(), b.Focused())
	}
	if aFocus {
		t.Error("focus-lost notification did not clear a's focus flag")
	}
	if !bFocus {
		t.Error("focus-gained notification did not fire for b")
	}

	d.SetFocus(nil)
	if a.Focused() || b.Focused() {
		t.Error("clearing focus should leave nobody focused")
	}
}

func TestSetVisibleSetEnabled(t *testing.T) {
	c := NewComponent(Rect{W: 1, H: 1})
	if !c.Visible || !c.Enabled {
		t.Fatal("NewComponent should start visible and enabled")
	}
	c.SetVisible(false)
	c.SetEnabled(false)
	if c.Visible || c.Enabled {
		t.Error("SetVisible/SetEnabled(false) did not take effect")
	}
	c.SetVisible(true)
	c.SetEnabled(true)
	if !c.Visible || !c.Enabled {
		t.Error("SetVisible/SetEnabled(true) did not take effect")
	}
}

// TestBoundWidgetThroughDesktop is the end-to-end path: a widget that opts in by
// implementing the interfaces (via Bind) receives a key routed by the desktop to
// the focused component.
func TestBoundWidgetThroughDesktop(t *testing.T) {
	var out bytes.Buffer
	app := tui.NewWithSize(40, 10, &out)
	d := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 10})
	field := NewComponent(Rect{X: 1, Y: 1, W: 5, H: 1})
	field.Focusable = true
	r := &recorder{consume: true}
	field.Bind(r)
	root.AddChild(field)
	d.AddLayer(NewLayer("l", root, true, false))
	d.SetFocus(field)

	d.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'k'})
	if r.typed == nil || r.typed.Rune != 'k' {
		t.Error("desktop did not deliver the key to the bound focused widget")
	}
}
