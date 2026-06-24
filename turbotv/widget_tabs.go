package tv

import tui "github.com/hobbestherat/turbotui"

// Tab is one labelled page of a Tabs widget: a Title shown in the strip and the
// Content widget rendered (and focusable) only while the tab is active.
type Tab struct {
	Title   string
	Content Widget
}

// Tabs is a tabbed container: a horizontal strip of labels above a content region
// that shows exactly one child Widget — the active tab's content — at a time.
//
// Keyboard, while focus is anywhere inside the active tab: Alt+Left/Alt+Right or
// Ctrl+Tab/Ctrl+Shift+Tab switch tabs; plain Tab/Shift+Tab move focus between the
// focusables WITHIN the active tab and never escape it. The switch chords are
// claimed in the desktop's capture phase (before the focused child sees them), so
// switching works even when the active tab holds a child that itself consumes
// arrow or Tab keys. Mouse: click a label to activate that tab. OnTabChange fires
// on every real change (keyboard, mouse, or SetActive).
//
// It is a normal Widget following the VisualComponent /
// Bind(Painter|Typer|CaptureTyper|Clicker) pattern: the strip is painted by the
// widget itself and each tab's Content is a child component whose visibility tracks
// the active tab. Construct one with NewTabs, then AddTab for each page.
//
// A non-nil *Desktop is required for the full keyboard contract (like Select): the
// widget reads and drives focus through it to trap Tab within the active tab and to
// hand focus to the newly shown tab on a switch. Passing nil yields a Tabs that
// still renders and switches but performs no focus management.
type Tabs struct {
	Component *VisualComponent
	// OnTabChange, when set, is called with the new index after the active tab
	// changes (whether by keyboard, mouse, or SetActive).
	OnTabChange func(index int)
	// FG/BG colour an inactive tab label; ActiveFG/ActiveBG the active one. They are
	// seeded from the active theme and may be overridden before the first draw.
	FG       tui.Color
	BG       tui.Color
	ActiveFG tui.Color
	ActiveBG tui.Color

	desktop *Desktop
	content *VisualComponent
	tabs    []Tab
	active  int
	stripH  int
}

// labelSpan is the absolute horizontal extent [x0, x1) of one tab label in the
// strip, used both to paint the label and to hit-test clicks on it.
type labelSpan struct {
	index int
	x0    int
	x1    int
}

// NewTabs creates an empty Tabs widget at bounds. desktop is the desktop the widget
// is hosted on; it is used to move keyboard focus when the active tab changes.
func NewTabs(desktop *Desktop, bounds Rect) *Tabs {
	t := &Tabs{
		FG:       activeTheme.WindowFG,
		BG:       activeTheme.WindowBG,
		ActiveFG: activeTheme.SelectionFG,
		ActiveBG: activeTheme.SelectionBG,
		desktop:  desktop,
		stripH:   1,
	}
	t.Component = NewComponent(bounds)
	t.Component.LayoutFn = t.layout
	t.Component.Bind(t)
	t.content = NewComponent(Rect{X: 0, Y: t.stripH, W: bounds.W, H: bounds.H - t.stripH})
	t.content.LayoutFn = t.layoutContent
	t.Component.AddChild(t.content)
	return t
}

func (t *Tabs) Root() *VisualComponent { return t.Component }

// AddTab appends a page with the given title and content widget and returns the
// Tabs for chaining. The first tab added becomes the active one. A nil content is
// ignored (no tab is added), so callers never panic on a missing page.
func (t *Tabs) AddTab(title string, content Widget) *Tabs {
	if content == nil {
		return t
	}
	index := len(t.tabs)
	t.tabs = append(t.tabs, Tab{Title: title, Content: content})
	t.content.AddChild(content)
	content.Root().Visible = index == t.active
	// Size the freshly added panel to the current content area.
	t.layoutContent(t.content)
	return t
}

// Count returns the number of tabs.
func (t *Tabs) Count() int { return len(t.tabs) }

// Active returns the index of the active tab (0 when empty).
func (t *Tabs) Active() int { return t.active }

// ActiveContent returns the active tab's content widget, or nil when there are no
// tabs.
func (t *Tabs) ActiveContent() Widget {
	if t.active < 0 || t.active >= len(t.tabs) {
		return nil
	}
	return t.tabs[t.active].Content
}

// SetActive switches to the tab at index (a no-op when out of range or already
// active), firing OnTabChange. It moves focus into the newly shown tab only when
// focus was inside the one being hidden, so a programmatic switch never steals
// focus from an unrelated widget elsewhere on screen.
func (t *Tabs) SetActive(index int) {
	t.setActiveIndex(index, false)
}

// setActiveIndex performs the switch. enterFocus forces focus into the new tab
// (used by the keyboard and mouse paths, where the user deliberately drove the
// switch); otherwise focus only moves when it would otherwise be stranded on the
// now-hidden tab.
func (t *Tabs) setActiveIndex(index int, enterFocus bool) {
	if index < 0 || index >= len(t.tabs) || index == t.active {
		return
	}
	prev := t.active
	t.active = index
	t.syncVisibility()
	// A widget in the tab we just hid cannot keep focus (the desktop stops routing
	// keys to an invisible widget), so pull focus into the newly shown tab whenever
	// the user drove the switch or focus would otherwise be left on a hidden tab.
	if t.desktop != nil && (enterFocus || t.focusInPanel(prev)) {
		t.focusFirstInActive()
	}
	if t.OnTabChange != nil {
		t.OnTabChange(index)
	}
}

// switchBy moves the active tab by delta (wrapping). It reports whether it consumed
// the key: true whenever there is at least one tab, so an Alt+Arrow chord meant for
// tab switching never falls through to spatial focus navigation.
func (t *Tabs) switchBy(delta int) bool {
	n := len(t.tabs)
	if n == 0 {
		return false
	}
	t.setActiveIndex(((t.active+delta)%n+n)%n, true)
	return true
}

func (t *Tabs) syncVisibility() {
	for i, tab := range t.tabs {
		tab.Content.Root().Visible = i == t.active
	}
}

// Paint draws the tab strip; the content panels are drawn by the framework as
// children afterwards.
func (t *Tabs) Paint(surface Surface) {
	abs := t.Component.AbsoluteBounds()
	surface.Fill(Rect{X: abs.X, Y: abs.Y, W: abs.W, H: t.stripH}, tui.Cell{Ch: ' ', FG: t.FG, BG: t.BG})
	for _, span := range t.labelSpans() {
		fg, bg, bold := t.FG, t.BG, false
		if span.index == t.active {
			fg, bg, bold = t.ActiveFG, t.ActiveBG, true
		}
		seg := Rect{X: span.x0, Y: abs.Y, W: span.x1 - span.x0, H: t.stripH}
		surface.Fill(seg, tui.Cell{Ch: ' ', FG: fg, BG: bg})
		surface.WriteString(span.x0, abs.Y, " "+t.tabs[span.index].Title+" ", tui.Cell{FG: fg, BG: bg, Bold: bold})
	}
}

// labelSpans lays the tab labels out left to right in absolute coordinates, each
// padded with a space on either side, stopping once they would overflow the strip.
func (t *Tabs) labelSpans() []labelSpan {
	abs := t.Component.AbsoluteBounds()
	spans := make([]labelSpan, 0, len(t.tabs))
	x := abs.X
	limit := abs.X + abs.W
	for i, tab := range t.tabs {
		w := tui.StringWidth(" " + tab.Title + " ")
		if x >= limit {
			break
		}
		end := x + w
		if end > limit {
			end = limit
		}
		spans = append(spans, labelSpan{index: i, x0: x, x1: end})
		x = end
	}
	return spans
}

// CaptureType claims the tab-switch chords (Alt+Left/Alt+Right, Ctrl+Tab,
// Ctrl+Shift+Tab) during the capture phase — BEFORE the focused content widget sees
// them — so switching always works, even when the active tab holds a child that
// would otherwise consume arrow or Tab keys (a text input, tree, picker, …). This
// is why the contract does not depend on individual child widgets declining keys.
func (t *Tabs) CaptureType(event tui.TypeEvent) bool {
	switch {
	case event.Key == tui.KeyLeft && event.Alt:
		return t.switchBy(-1)
	case event.Key == tui.KeyRight && event.Alt:
		return t.switchBy(1)
	case event.Key == tui.KeyTab && event.Ctrl && !event.Shift:
		return t.switchBy(1)
	case event.Key == tui.KeyTab && event.Ctrl && event.Shift:
		return t.switchBy(-1)
	case event.Key == tui.KeyBackTab && event.Ctrl:
		return t.switchBy(-1)
	}
	return false
}

// HandleType implements plain Tab / Shift+Tab focus traversal WITHIN the active
// tab. It runs as an ancestor of the focused content widget, AFTER that widget
// declines the key (a field that wants Tab keeps it), and consumes Tab so focus
// never escapes the active tab into the strip or a sibling tab.
func (t *Tabs) HandleType(event tui.TypeEvent) bool {
	switch {
	case event.Key == tui.KeyTab && !event.Ctrl && !event.Alt:
		return t.focusWithin(1)
	case event.Key == tui.KeyBackTab && !event.Ctrl && !event.Alt:
		return t.focusWithin(-1)
	}
	return false
}

// focusWithin cycles focus among the active tab's focusables in the given direction
// and consumes the key, so Tab never escapes the active tab into the strip or a
// sibling tab. It declines (returns false) when it cannot scope the move, letting
// the desktop's global Tab traversal take over.
func (t *Tabs) focusWithin(dir int) bool {
	if t.desktop == nil {
		return false
	}
	panel := t.activePanel()
	if panel == nil {
		return false
	}
	var items []*VisualComponent
	collectFocusable(panel, &items)
	sortFocusOrder(items)
	if len(items) == 0 {
		return false
	}
	current := -1
	for i, item := range items {
		if item == t.desktop.focused {
			current = i
			break
		}
	}
	next := 0
	switch {
	case current >= 0:
		next = ((current+dir)%len(items) + len(items)) % len(items)
	case dir < 0:
		next = len(items) - 1
	}
	t.desktop.setFocus(items[next])
	return true
}

// HandleClick activates the tab whose label was clicked. It only acts on the
// release inside the strip; clicks elsewhere are left for the content children.
func (t *Tabs) HandleClick(event tui.ClickEvent) bool {
	abs := t.Component.AbsoluteBounds()
	if event.Y < abs.Y || event.Y >= abs.Y+t.stripH {
		return false
	}
	if event.Down {
		return true
	}
	for _, span := range t.labelSpans() {
		if event.X >= span.x0 && event.X < span.x1 {
			t.setActiveIndex(span.index, true)
			return true
		}
	}
	return true
}

func (t *Tabs) layout(_ *VisualComponent) {
	b := t.Component.Bounds
	h := b.H - t.stripH
	if h < 0 {
		h = 0
	}
	t.content.SetBounds(Rect{X: 0, Y: t.stripH, W: b.W, H: h})
}

func (t *Tabs) layoutContent(c *VisualComponent) {
	fill := Rect{X: 0, Y: 0, W: c.Bounds.W, H: c.Bounds.H}
	for _, child := range c.Children() {
		child.SetBounds(fill)
	}
}

// activePanel returns the root component of the active tab's content (nil when
// empty).
func (t *Tabs) activePanel() *VisualComponent {
	if t.active < 0 || t.active >= len(t.tabs) {
		return nil
	}
	return t.tabs[t.active].Content.Root()
}

// focusInPanel reports whether the desktop's focused component lies inside tab i.
func (t *Tabs) focusInPanel(i int) bool {
	if t.desktop == nil || t.desktop.focused == nil || i < 0 || i >= len(t.tabs) {
		return false
	}
	root := t.tabs[i].Content.Root()
	for node := t.desktop.focused; node != nil; node = node.Parent() {
		if node == root {
			return true
		}
	}
	return false
}

// focusFirstInActive moves focus to the first focusable in the active tab, or
// clears it when the tab has none.
func (t *Tabs) focusFirstInActive() {
	panel := t.activePanel()
	if panel == nil {
		return
	}
	var items []*VisualComponent
	collectFocusable(panel, &items)
	sortFocusOrder(items)
	if len(items) == 0 {
		t.desktop.setFocus(nil)
		return
	}
	t.desktop.setFocus(items[0])
}
