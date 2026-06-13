package tv

import tui "github.com/hobbestherat/turbotui"

type DrawFn func(component *VisualComponent, surface Surface)
type LayoutFn func(component *VisualComponent)
type TypeHandlerFn func(component *VisualComponent, event tui.TypeEvent) bool
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

type VisualComponent struct {
	Bounds      Rect
	Visible     bool
	Enabled     bool
	Focusable   bool
	HasFocus    bool
	DrawOutside bool

	Parent   *VisualComponent
	Children []*VisualComponent

	DrawFn      DrawFn
	LayoutFn    LayoutFn
	OnTypeFn    TypeHandlerFn
	OnClickFn   ClickHandlerFn
	OnScrollFn  ScrollHandlerFn
	OnFocusFn   FocusHandlerFn
	OnHitTestFn HitTestFn
	// CursorFn, when set on a focused component, returns the absolute screen
	// position of the text cursor so the desktop can place the real terminal
	// cursor there (ok=false hides it).
	CursorFn      func(component *VisualComponent) (int, int, bool)
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
	if c.LayoutFn != nil {
		c.LayoutFn(c)
	}
}

func (c *VisualComponent) AddChild(child Widget) {
	root := child.Root()
	root.Parent = c
	c.Children = append(c.Children, root)
}

func (c *VisualComponent) RemoveChild(child Widget) {
	root := child.Root()
	next := make([]*VisualComponent, 0, len(c.Children))
	for _, existing := range c.Children {
		if existing == root {
			existing.Parent = nil
			continue
		}
		next = append(next, existing)
	}
	c.Children = next
}

func (c *VisualComponent) AbsoluteBounds() Rect {
	if c.Parent == nil {
		return c.Bounds
	}
	parent := c.Parent.AbsoluteBounds()
	return Rect{
		X: parent.X + c.Bounds.X,
		Y: parent.Y + c.Bounds.Y,
		W: c.Bounds.W,
		H: c.Bounds.H,
	}
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
