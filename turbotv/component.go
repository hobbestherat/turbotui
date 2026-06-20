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
// rectangle, visibility/enabled/focus flags, parent/children links and the
// draw/layout/input callbacks. Its zero value is invisible and inert (Visible
// and Enabled are false), so always construct one with NewComponent.
type VisualComponent struct {
	Bounds      Rect
	Visible     bool
	Enabled     bool
	Focusable   bool
	HasFocus    bool
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

	Parent   *VisualComponent
	Children []*VisualComponent

	DrawFn      DrawFn
	LayoutFn    LayoutFn
	OnTypeFn    TypeHandlerFn
	OnPasteFn   PasteHandlerFn
	OnClickFn   ClickHandlerFn
	OnScrollFn  ScrollHandlerFn
	OnFocusFn   FocusHandlerFn
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

func (c *VisualComponent) SetBounds(bounds Rect) {
	c.Bounds = bounds
	c.invalidateAbs()
	if c.LayoutFn != nil {
		c.LayoutFn(c)
	}
}

func (c *VisualComponent) AddChild(child Widget) {
	root := child.Root()
	root.Parent = c
	root.invalidateAbs()
	c.Children = append(c.Children, root)
}

func (c *VisualComponent) RemoveChild(child Widget) {
	root := child.Root()
	next := make([]*VisualComponent, 0, len(c.Children))
	for _, existing := range c.Children {
		if existing == root {
			existing.Parent = nil
			existing.invalidateAbs()
			continue
		}
		next = append(next, existing)
	}
	c.Children = next
}

// invalidateAbs marks this component's cached absolute bounds — and, since a
// component's absolute position depends on every ancestor, the whole subtree's —
// as stale, so the next AbsoluteBounds() call recomputes it. It is called
// automatically on SetBounds/AddChild/RemoveChild.
func (c *VisualComponent) invalidateAbs() {
	c.absCached = false
	for _, child := range c.Children {
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
	if c.Parent == nil {
		c.abs = c.Bounds
	} else {
		parent := c.Parent.AbsoluteBounds()
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
	for current := c; current != nil; current = current.Parent {
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
	if c.DrawFn != nil {
		c.DrawFn(c, drawSurface)
	}
	for _, child := range c.Children {
		child.Draw(componentSurface)
	}
}

func (c *VisualComponent) HitTestDeep(x int, y int) *VisualComponent {
	if !c.Visible || !c.Enabled {
		return nil
	}
	inside := c.AbsoluteBounds().Contains(x, y)
	if c.OnHitTestFn != nil {
		inside = c.OnHitTestFn(c, x, y)
	}
	if !inside {
		return nil
	}
	for index := len(c.Children) - 1; index >= 0; index-- {
		target := c.Children[index].HitTestDeep(x, y)
		if target != nil {
			return target
		}
	}
	return c
}

func (c *VisualComponent) bubble(handle func(*VisualComponent) bool) bool {
	for current := c; current != nil; current = current.Parent {
		if current.Enabled && handle(current) {
			return true
		}
	}
	return false
}

func (c *VisualComponent) BubbleType(event tui.TypeEvent) bool {
	return c.bubble(func(v *VisualComponent) bool {
		return v.OnTypeFn != nil && v.OnTypeFn(v, event)
	})
}

func (c *VisualComponent) BubblePaste(text string) bool {
	return c.bubble(func(v *VisualComponent) bool {
		return v.OnPasteFn != nil && v.OnPasteFn(v, text)
	})
}

func (c *VisualComponent) BubbleClick(event tui.ClickEvent) bool {
	return c.bubble(func(v *VisualComponent) bool {
		return v.OnClickFn != nil && v.OnClickFn(v, event)
	})
}

func (c *VisualComponent) BubbleScroll(event tui.ScrollEvent) bool {
	return c.bubble(func(v *VisualComponent) bool {
		return v.OnScrollFn != nil && v.OnScrollFn(v, event)
	})
}

func collectFocusable(root *VisualComponent, target *[]*VisualComponent) {
	if root == nil || !root.Visible || !root.Enabled {
		return
	}
	if root.Focusable {
		*target = append(*target, root)
	}
	for _, child := range root.Children {
		collectFocusable(child, target)
	}
}
