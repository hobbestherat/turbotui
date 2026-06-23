package tv

import tui "github.com/hobbestherat/turbotui"

type DrawFn func(component *VisualComponent, surface Surface)
type LayoutFn func(component *VisualComponent)
type TypeHandlerFn func(component *VisualComponent, event tui.TypeEvent) bool
type PasteHandlerFn func(component *VisualComponent, text string) bool
type ClickHandlerFn func(component *VisualComponent, event tui.ClickEvent) bool
type ScrollHandlerFn func(component *VisualComponent, event tui.ScrollEvent) bool
type FocusHandlerFn func(component *VisualComponent, focused bool)
type HitTestFn func(component *VisualComponent, x int, y int) bool

// MnemonicFn lets a component claim more than one mnemonic (the menubar). It
// returns true if it consumed the Alt+lower activation.
type MnemonicFn func(component *VisualComponent, lower rune) bool

// Widget is anything that exposes a root VisualComponent so it can be added to
// containers and layers directly, without reaching for the .Component field.
type Widget interface {
	Root() *VisualComponent
}

// VisualComponent is the retained-mode node shared by every tv widget: a bounds
// rectangle, visibility/enabled flags, parent/children links, focus state and the
// draw/layout/input callbacks. Its zero value is invisible and inert (Visible
// and Enabled are false), so always construct one with NewComponent.
//
// Framework-owned state — the parent/children tree links and focus — is private:
// read it through Parent(), Children() and Focused(), and let the desktop drive
// focus. App-owned state (Visible, Enabled, Focusable, Bounds, …) stays public.
//
// A component dispatches input through its On*Fn callback fields, but it also
// satisfies the capability interfaces (Painter, Typer, Clicker, …): a widget can
// implement those interfaces and call Bind to wire them in, getting a
// compile-time-checked contract instead of hand-assigning function fields.
type VisualComponent struct {
	Bounds      Rect
	Visible     bool
	Enabled     bool
	Focusable   bool
	DrawOutside bool
	// TabIndex orders keyboard focus traversal (Tab / Shift+Tab). Components are
	// visited in ascending TabIndex; ties fall back to on-screen reading order
	// (top-to-bottom, then left-to-right) rather than tree/declaration order, so a
	// form laid out by absolute coordinates tabs the way it reads (issue #50). The
	// default 0 leaves every widget in pure reading order.
	TabIndex int
	// Flex is the grow weight used by Box containers when distributing leftover
	// space along their packing axis. Zero (the default) means the child keeps its
	// natural Bounds size.
	Flex int

	// DrawFn paints the component's own content; the desktop calls it during a
	// frame, before the children are drawn. Leave it nil for a container that only
	// arranges children.
	DrawFn DrawFn
	// LayoutFn, when set, runs at the start of each Draw (and on SetBounds) to size
	// or position this component's children before they paint — the hook for
	// responsive layout that depends on the current bounds.
	LayoutFn LayoutFn
	// OnTypeFn is offered each key event while the component (or a descendant that
	// did not consume it) is focused. Return true to consume it; return false to
	// let it bubble to the parent.
	OnTypeFn TypeHandlerFn
	// OnPasteFn is offered a bracketed-paste block delivered to the focused
	// component. Return true to consume it, false to let it bubble.
	OnPasteFn PasteHandlerFn
	// OnClickFn is offered a mouse press/release that hit-tested to this component
	// (or bubbled up from a child). Return true to consume it, false to bubble.
	OnClickFn ClickHandlerFn
	// OnScrollFn is offered a scroll-wheel event over this component. Return true
	// to consume it, false to let it bubble to the parent.
	OnScrollFn ScrollHandlerFn
	// OnFocusFn is called when the desktop gives this component focus
	// (focused=true) or takes it away (focused=false). It is a notification, not a
	// veto — it has no return value.
	OnFocusFn FocusHandlerFn
	// OnHitTestFn, when set, decides whether an absolute (x, y) counts as inside
	// this component, overriding the default bounds test (e.g. a non-rectangular
	// click target). The desktop calls it while routing mouse events.
	OnHitTestFn HitTestFn
	// CursorFn, when set on a focused component, returns the absolute screen
	// position of the text cursor so the desktop can place the real terminal
	// cursor there (ok=false hides it).
	CursorFn func(component *VisualComponent) (int, int, bool)
	// CopyFn, when set, returns the text the component would copy to the clipboard
	// (a selection, or all of its content) and whether there is anything to copy.
	// The desktop calls it on Ctrl+C / Ctrl+Shift+C for the focused component.
	CopyFn func(component *VisualComponent) (string, bool)
	// CutFn, when set, is the clipboard "cut" hook: it removes the selection (or
	// whatever the component considers cuttable) from its own content and returns
	// that text plus whether anything was cut. The desktop calls it on Ctrl+X for
	// the focused component and copies the returned text to the clipboard. When
	// nothing is cuttable it returns ok=false so the keystroke can fall through.
	CutFn         func(component *VisualComponent) (string, bool)
	Background    tui.Cell
	UseBackground bool

	// Mnemonic is the lowercased hot character (declared with '&' in the label),
	// 0 when none. When the component is in the active mnemonic scope, Alt+Mnemonic
	// triggers it: OnActivateFn if set, otherwise focus moves to MnemonicTarget (or
	// the component itself when focusable).
	Mnemonic       rune
	OnActivateFn   func(component *VisualComponent)
	MnemonicTarget *VisualComponent
	OnMnemonicFn   MnemonicFn

	mnemonicActive bool

	// parent/children are the framework-owned tree links. They are private so a
	// caller cannot reseat a node or splice the tree behind the desktop's back and
	// break focus traversal, hit-testing or absolute-bounds caching; mutate them
	// only through AddChild/RemoveChild and read them through Parent()/Children().
	parent   *VisualComponent
	children []*VisualComponent

	// hasFocus is framework-owned focus state. It is private and driven solely by
	// Desktop.setFocus so the invariant "at most one component has focus" cannot be
	// violated by a stray field write; read it through Focused().
	hasFocus bool

	// activatesOnEnter marks a widget whose Enter keystroke triggers a consequential
	// activation (a push Button). The desktop uses it to scope the modal Enter-grace
	// (gogent#347): while a freshly shown modal is in its grace window, Enter is
	// swallowed only for such a focused widget, so a non-button field still bubbles
	// Enter to a dialog's default/cancel handler and a text input still edits. Set by
	// the widget constructor (NewButton); read by the desktop's enter-grace check.
	activatesOnEnter bool

	// abs is the cached AbsoluteBounds() result and absCached marks it valid. It is
	// recomputed lazily (and cached) on the first call after a bounds/parent
	// change, so repeated calls within a frame are O(1) instead of O(depth).
	abs       Rect
	absCached bool
}

// MnemonicActive reports whether this component currently owns its mnemonic and
// should render the highlighted hot character.
func (c *VisualComponent) MnemonicActive() bool {
	return c.mnemonicActive
}

func NewComponent(bounds Rect) *VisualComponent {
	return &VisualComponent{
		Bounds:    bounds,
		Visible:   true,
		Enabled:   true,
		Focusable: false,
	}
}

func (c *VisualComponent) Root() *VisualComponent {
	return c
}

// Focused reports whether the desktop currently routes the keyboard to this
// component. It is the read-only view of the framework-owned focus flag; focus is
// moved with Desktop.SetFocus, never by writing a field.
func (c *VisualComponent) Focused() bool {
	return c.hasFocus
}

// Parent returns the component's parent in the layer tree, or nil for a root. The
// link is read-only: reparent with the parent's AddChild/RemoveChild so the
// absolute-bounds cache and focus traversal stay consistent.
func (c *VisualComponent) Parent() *VisualComponent {
	return c.parent
}

// Children returns a copy of the component's child slice. It is a snapshot, so
// mutating the returned slice does not alter the tree; add or remove children with
// AddChild/RemoveChild.
func (c *VisualComponent) Children() []*VisualComponent {
	out := make([]*VisualComponent, len(c.children))
	copy(out, c.children)
	return out
}

// SetVisible shows or hides the component (and, with it, its subtree). It is the
// named setter counterpart to the public Visible field for callers that prefer a
// method-based API.
func (c *VisualComponent) SetVisible(visible bool) {
	c.Visible = visible
}

// SetEnabled enables or disables the component. A disabled component is skipped by
// hit-testing, focus traversal and input bubbling.
func (c *VisualComponent) SetEnabled(enabled bool) {
	c.Enabled = enabled
}

func (c *VisualComponent) SetBounds(bounds Rect) {
	c.Bounds = bounds
	c.invalidateAbs()
	if c.LayoutFn != nil {
		c.LayoutFn(c)
	}
}

func (c *VisualComponent) AddChild(child Widget) {
	root := child.Root()
	root.parent = c
	root.invalidateAbs()
	c.children = append(c.children, root)
}

func (c *VisualComponent) RemoveChild(child Widget) {
	root := child.Root()
	next := make([]*VisualComponent, 0, len(c.children))
	for _, existing := range c.children {
		if existing == root {
			existing.parent = nil
			existing.invalidateAbs()
			continue
		}
		next = append(next, existing)
	}
	c.children = next
}

// invalidateAbs marks this component's cached absolute bounds — and, since a
// component's absolute position depends on every ancestor, the whole subtree's —
// as stale, so the next AbsoluteBounds() call recomputes it. It is called
// automatically on SetBounds/AddChild/RemoveChild.
func (c *VisualComponent) invalidateAbs() {
	c.absCached = false
	for _, child := range c.children {
		child.invalidateAbs()
	}
}

// AbsoluteBounds returns the component's bounds in screen (root) coordinates by
// walking up the parent chain. The result is memoized per frame: the first call
// after a bounds or parent change does the O(depth) walk and caches the value,
// and every later call (including the ones inside Draw and HitTestDeep) returns
// the cache in O(1).
func (c *VisualComponent) AbsoluteBounds() Rect {
	if c.absCached {
		return c.abs
	}
	if c.parent == nil {
		c.abs = c.Bounds
	} else {
		parent := c.parent.AbsoluteBounds()
		c.abs = Rect{
			X: parent.X + c.Bounds.X,
			Y: parent.Y + c.Bounds.Y,
			W: c.Bounds.W,
			H: c.Bounds.H,
		}
	}
	c.absCached = true
	return c.abs
}

// visibleInTree reports whether the component and every ancestor is visible. A
// focused widget whose container was hidden (e.g. a minimized window's content)
// is therefore not visible-in-tree, so the desktop can stop routing keystrokes
// and the hardware cursor to it.
func (c *VisualComponent) visibleInTree() bool {
	for current := c; current != nil; current = current.parent {
		if !current.Visible {
			return false
		}
	}
	return true
}

func (c *VisualComponent) Draw(surface Surface) {
	if !c.Visible {
		return
	}
	if c.LayoutFn != nil {
		c.LayoutFn(c)
	}
	clip := surface.Clip().Intersect(c.AbsoluteBounds())
	if clip.Empty() {
		return
	}
	componentSurface := surface.WithClip(clip)
	drawSurface := componentSurface
	if c.DrawOutside {
		drawSurface = surface
	}
	if c.UseBackground {
		componentSurface.Fill(c.AbsoluteBounds(), c.Background)
	}
	c.Paint(drawSurface)
	for _, child := range c.children {
		child.Draw(componentSurface)
	}
}

func (c *VisualComponent) HitTestDeep(x int, y int) *VisualComponent {
	if !c.Visible || !c.Enabled {
		return nil
	}
	if !c.HitTest(x, y) {
		return nil
	}
	for index := len(c.children) - 1; index >= 0; index-- {
		target := c.children[index].HitTestDeep(x, y)
		if target != nil {
			return target
		}
	}
	return c
}

func (c *VisualComponent) bubble(handle func(*VisualComponent) bool) bool {
	for current := c; current != nil; current = current.parent {
		if current.Enabled && handle(current) {
			return true
		}
	}
	return false
}

// The Bubble* dispatchers walk up the tree and offer the event to each ancestor
// through its capability interface (Typer, Paster, Clicker, Scroller). A
// *VisualComponent satisfies all of them, delegating to its On*Fn fields, so a
// node that does not implement a capability simply declines the event.
func (c *VisualComponent) BubbleType(event tui.TypeEvent) bool {
	return c.bubble(func(v *VisualComponent) bool {
		t, ok := any(v).(Typer)
		return ok && t.HandleType(event)
	})
}

func (c *VisualComponent) BubblePaste(text string) bool {
	return c.bubble(func(v *VisualComponent) bool {
		p, ok := any(v).(Paster)
		return ok && p.HandlePaste(text)
	})
}

func (c *VisualComponent) BubbleClick(event tui.ClickEvent) bool {
	return c.bubble(func(v *VisualComponent) bool {
		cl, ok := any(v).(Clicker)
		return ok && cl.HandleClick(event)
	})
}

func (c *VisualComponent) BubbleScroll(event tui.ScrollEvent) bool {
	return c.bubble(func(v *VisualComponent) bool {
		s, ok := any(v).(Scroller)
		return ok && s.HandleScroll(event)
	})
}

func collectFocusable(root *VisualComponent, target *[]*VisualComponent) {
	if root == nil || !root.Visible || !root.Enabled {
		return
	}
	if root.Focusable {
		*target = append(*target, root)
	}
	for _, child := range root.children {
		collectFocusable(child, target)
	}
}
