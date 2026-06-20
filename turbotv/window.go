package tv

import (
	tui "github.com/hobbestherat/turbotui"
	"strings"
)

type Window struct {
	Component   *VisualComponent
	Content     *VisualComponent
	BottomBar   *VisualComponent
	Title       string
	Border      tui.LineKind
	ShowClose   bool
	TitleFG     tui.Color
	TitleBG     tui.Color
	BorderFG    tui.Color
	BorderBG    tui.Color
	CloseFG     tui.Color
	CloseBG     tui.Color
	Shadow      bool
	ShadowColor tui.Color
	OnClose     func(*Window)
	// Resizable, when true, draws a corner grip and lets the user drag the
	// bottom-right corner to resize the window (opt-in).
	Resizable bool
	// Minimizable, when true, adds a [▾]/[▴] button to the title bar that
	// collapses the window to just its title bar and back (opt-in).
	Minimizable bool
	// Maximizable, when true, adds a [▢]/[▣] button to the title bar that expands
	// the window to fill its constraint area (the desktop work area by default)
	// and restores it to its previous bounds (opt-in).
	Maximizable bool
	MinWidth    int
	MinHeight   int
	// ConstrainTo, when non-nil and non-empty, bounds drag, resize and maximize to
	// this rect (in screen coordinates). When nil, the window falls back to the
	// owning desktop's work area (see Desktop.WorkArea / Desktop.SetWorkArea), and
	// is unconstrained only when it has no desktop at all.
	ConstrainTo *Rect
	// OnResize fires after the window is resized via the grip; OnMinimize fires
	// when the minimized state changes (minimized=true when collapsed); OnMaximize
	// fires when the maximized state changes (maximized=true when expanded).
	OnResize         func(*Window)
	OnMinimize       func(window *Window, minimized bool)
	OnMaximize       func(window *Window, maximized bool)
	desktop          *Desktop
	layer            *Layer
	dragging         bool
	dragOffsetX      int
	dragOffsetY      int
	lastMouseDown    bool
	resizing         bool
	minimized        bool
	maximized        bool
	restoreBounds    Rect
	bottomWasVisible bool
}

func NewWindow(title string, bounds Rect, border tui.LineKind) *Window {
	window := &Window{
		Title:       title,
		Border:      border,
		ShowClose:   true,
		TitleFG:     activeTheme.WindowTitleFG,
		TitleBG:     activeTheme.WindowTitleBG,
		BorderFG:    activeTheme.WindowBorderFG,
		BorderBG:    activeTheme.WindowBorderBG,
		CloseFG:     activeTheme.CloseButtonFG,
		CloseBG:     activeTheme.CloseButtonBG,
		Shadow:      true,
		ShadowColor: activeTheme.WindowShadow,
		MinWidth:    12,
		MinHeight:   3,
	}
	window.Component = NewComponent(bounds)
	window.Component.DrawOutside = true
	window.Component.DrawFn = window.draw
	window.Component.LayoutFn = window.layout
	window.Component.OnClickFn = window.handleClick
	// Content/BottomBar are children of the window, so their bounds are
	// window-relative: inset the window's size, not its position.
	inner := Rect{W: bounds.W, H: bounds.H}.Inset(1)
	window.Content = NewComponent(inner)
	window.Content.UseBackground = true
	window.Content.Background = tui.Cell{Ch: ' ', FG: activeTheme.WindowFG, BG: activeTheme.WindowBG}
	window.BottomBar = NewComponent(Rect{X: inner.X, Y: inner.Bottom(), W: inner.W, H: 1})
	window.BottomBar.Visible = false
	window.Component.AddChild(window.Content)
	window.Component.AddChild(window.BottomBar)
	return window
}

// windowRef satisfies the unexported hasWindow interface so that adding a Window
// (directly or via a Dialog) to a layer wires the window back to its layer and
// desktop, enabling Close() and bounds constraints.
func (w *Window) windowRef() *Window { return w }

func (w *Window) Root() *VisualComponent {
	return w.Component
}
func (w *Window) AddContent(child Widget) {
	w.Content.AddChild(child)
}
func (w *Window) AddBottom(child Widget) {
	w.BottomBar.Visible = true
	w.BottomBar.AddChild(child)
	if w.Component.LayoutFn != nil {
		w.Component.LayoutFn(w.Component)
	}
}

// Close removes the window's layer from its desktop (when it was added via a
// layer) and then fires OnClose. Apps that wire the window through
// NewWindowLayer/NewModalLayer + Desktop.AddLayer get one-line teardown; the
// title-bar close button still routes through OnClose only, so apps that want a
// confirmation step keep full control there.
func (w *Window) Close() {
	if w.desktop != nil && w.layer != nil {
		w.desktop.RemoveLayer(w.layer)
	}
	if w.OnClose != nil {
		w.OnClose(w)
	}
}

// IsMinimized reports whether the window is collapsed to its title bar.
func (w *Window) IsMinimized() bool { return w.minimized }

// IsMaximized reports whether the window is expanded to its constraint area.
func (w *Window) IsMaximized() bool { return w.maximized }

// Minimize collapses the window to a single title-bar row. A minimized window is
// never also maximized; the saved restore bounds keep the pre-collapse size.
func (w *Window) Minimize() {
	if w.minimized {
		return
	}
	if !w.maximized {
		w.restoreBounds = w.Component.Bounds
	}
	// Leaving a maximized state to minimize keeps the saved normal bounds.
	w.maximized = false
	w.minimized = true
	w.Content.Visible = false
	w.bottomWasVisible = w.BottomBar.Visible
	w.BottomBar.Visible = false
	cur := w.Component.Bounds
	w.Component.SetBounds(Rect{X: cur.X, Y: cur.Y, W: cur.W, H: 1})
	// The focused child now has a hidden ancestor; move focus out so keystrokes
	// (and the hardware cursor) do not leak to an invisible widget.
	if w.desktop != nil {
		w.desktop.ensureFocusInTopLayer()
	}
	if w.OnMinimize != nil {
		w.OnMinimize(w, true)
	}
}

// Maximize expands the window to fill its constraint area (the desktop work area
// by default), remembering the current bounds so Restore can put it back.
func (w *Window) Maximize() {
	if w.maximized {
		return
	}
	if w.minimized {
		// Un-collapse the chrome but keep the pre-minimize restore bounds.
		w.minimized = false
		w.Content.Visible = true
		w.BottomBar.Visible = w.bottomWasVisible
	} else {
		w.restoreBounds = w.Component.Bounds
	}
	w.maximized = true
	w.Component.SetBounds(w.maximizeArea())
	if w.OnMaximize != nil {
		w.OnMaximize(w, true)
	}
}

// Restore returns a maximized window to its pre-maximize bounds, or expands a
// minimized window back to its title-bar's current position with the saved size.
func (w *Window) Restore() {
	switch {
	case w.maximized:
		w.maximized = false
		w.Component.SetBounds(w.restoreBounds)
		if w.OnMaximize != nil {
			w.OnMaximize(w, false)
		}
	case w.minimized:
		w.minimized = false
		w.Content.Visible = true
		w.BottomBar.Visible = w.bottomWasVisible
		// Honour any drag performed while minimized: restore the saved size at the
		// bar's current position rather than snapping back to the pre-minimize spot.
		cur := w.Component.Bounds
		w.Component.SetBounds(Rect{X: cur.X, Y: cur.Y, W: w.restoreBounds.W, H: w.restoreBounds.H})
		if w.OnMinimize != nil {
			w.OnMinimize(w, false)
		}
	}
}

// ToggleMinimize flips the minimized state.
func (w *Window) ToggleMinimize() {
	if w.minimized {
		w.Restore()
	} else {
		w.Minimize()
	}
}

// ToggleMaximize flips the maximized state.
func (w *Window) ToggleMaximize() {
	if w.maximized {
		w.Restore()
	} else {
		w.Maximize()
	}
}

// constraintRect returns the rect (in screen coordinates) that drag, resize and
// maximize are clamped to, and whether such a constraint exists.
func (w *Window) constraintRect() (Rect, bool) {
	if w.ConstrainTo != nil && !w.ConstrainTo.Empty() {
		return *w.ConstrainTo, true
	}
	if w.desktop != nil {
		return w.desktop.WorkArea(), true
	}
	return Rect{}, false
}

// maximizeArea is the rect a maximized window fills. With no constraint (a
// window not attached to a desktop) it keeps its current bounds.
func (w *Window) maximizeArea() Rect {
	if area, ok := w.constraintRect(); ok {
		return area
	}
	return w.Component.Bounds
}

// clampMove clamps a proposed top-left (x, y) so the title bar stays grabbable
// inside the constraint area: the top and left edges may not leave the area
// (so the title never slides under a top reserve such as the menu bar, nor off
// the left), while the bottom and right may overflow as long as at least a small
// handle of width `keep` remains on screen. This matches modern window managers
// and keeps the existing "drag partly past the bottom edge" behaviour.
func (w *Window) clampMove(x int, y int, width int) (int, int) {
	area, ok := w.constraintRect()
	if !ok {
		return x, y
	}
	keep := w.MinWidth
	if keep > width {
		keep = width
	}
	if keep < 1 {
		keep = 1
	}
	minX := area.X
	maxX := area.Right() - keep + 1
	if maxX < minX {
		maxX = minX
	}
	if x < minX {
		x = minX
	} else if x > maxX {
		x = maxX
	}
	minY := area.Y
	maxY := area.Bottom()
	if maxY < minY {
		maxY = minY
	}
	if y < minY {
		y = minY
	} else if y > maxY {
		y = maxY
	}
	return x, y
}

// clampResize clamps a proposed (width, height) so the window stays within the
// constraint area's right/bottom edges and never shrinks below the minimum.
func (w *Window) clampResize(newW int, newH int) (int, int) {
	if area, ok := w.constraintRect(); ok {
		b := w.Component.Bounds
		if maxW := area.Right() - b.X + 1; newW > maxW {
			newW = maxW
		}
		if maxH := area.Bottom() - b.Y + 1; newH > maxH {
			newH = maxH
		}
	}
	if newW < w.MinWidth {
		newW = w.MinWidth
	}
	if newH < w.MinHeight {
		newH = w.MinHeight
	}
	return newW, newH
}

func (w *Window) layout(component *VisualComponent) {
	if w.minimized {
		return
	}
	if component.Bounds.W < 2 || component.Bounds.H < 2 {
		return
	}
	inner := Rect{W: component.Bounds.W, H: component.Bounds.H}.Inset(1)
	contentHeight := inner.H
	if w.BottomBar.Visible && contentHeight > 1 {
		contentHeight--
		w.BottomBar.SetBounds(Rect{X: inner.X, Y: inner.Bottom(), W: inner.W, H: 1})
	}
	w.Content.SetBounds(Rect{X: inner.X, Y: inner.Y, W: inner.W, H: contentHeight})
}
func (w *Window) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	if w.minimized {
		w.drawMinimizedBar(surface, abs)
		return
	}
	if w.Shadow {
		surface.DrawShadow(abs, w.ShadowColor)
	}
	surface.DrawBox(abs, w.Border, w.BorderFG, w.BorderBG)
	buttons := w.titleButtons(abs)
	title := strings.TrimSpace(w.Title)
	if title != "" && abs.W > 4 {
		w.drawTitle(surface, abs, buttons)
	}
	if buttons.hasMin {
		w.drawMinimize(surface, abs, buttons)
	}
	if buttons.hasMax {
		w.drawMaximize(surface, abs, buttons)
	}
	if buttons.hasClose {
		w.drawClose(surface, abs, buttons)
	}
	if w.Resizable && !w.maximized {
		surface.SetCell(abs.Right(), abs.Bottom(), tui.Cell{Ch: '◢', FG: w.BorderFG, BG: w.BorderBG, Bold: true})
	}
}
func (w *Window) drawMinimizedBar(surface Surface, rect Rect) {
	surface.Fill(Rect{X: rect.X, Y: rect.Y, W: rect.W, H: 1}, tui.Cell{Ch: ' ', FG: w.TitleFG, BG: w.TitleBG})
	buttons := w.titleButtons(rect)
	title := strings.TrimSpace(w.Title)
	if title != "" {
		areaRight := rect.Right() - 1
		if buttons.leftX >= 0 {
			areaRight = buttons.leftX - 2
		}
		maxLen := areaRight - (rect.X + 1) + 1
		if maxLen > 0 {
			if runes := []rune(title); len(runes) > maxLen {
				title = string(runes[:maxLen])
			}
			surface.WriteString(rect.X+1, rect.Y, title, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
		}
	}
	if buttons.hasMin {
		w.drawMinimize(surface, rect, buttons)
	}
	if buttons.hasMax {
		w.drawMaximize(surface, rect, buttons)
	}
	if buttons.hasClose {
		w.drawClose(surface, rect, buttons)
	}
}
func (w *Window) drawTitle(surface Surface, rect Rect, buttons titleButtons) {
	areaLeft := rect.X + 1
	areaRight := rect.Right() - 1
	if buttons.leftX >= 0 {
		// Stop one column short of the leftmost button.
		areaRight = buttons.leftX - 2
	}
	areaW := areaRight - areaLeft + 1
	// Reserve the two spaces that frame the title text.
	maxLen := areaW - 2
	if maxLen < 1 {
		return
	}
	title := w.Title
	if runes := []rune(title); len(runes) > maxLen {
		title = string(runes[:maxLen])
	}
	text := " " + title + " "
	start := areaLeft + (areaW-len([]rune(text)))/2
	if start < areaLeft {
		start = areaLeft
	}
	surface.WriteString(start, rect.Y, text, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
}
func (w *Window) drawClose(surface Surface, rect Rect, buttons titleButtons) {
	r := buttons.closeRect
	if r.X <= rect.X+1 {
		return
	}
	surface.WriteString(r.X, r.Y, "[■]", tui.Cell{FG: w.CloseFG, BG: w.CloseBG, Bold: true})
}
func (w *Window) drawMaximize(surface Surface, rect Rect, buttons titleButtons) {
	r := buttons.maxRect
	if r.X <= rect.X+1 {
		return
	}
	glyph := "[▢]"
	if w.maximized {
		glyph = "[▣]"
	}
	surface.WriteString(r.X, r.Y, glyph, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
}
func (w *Window) drawMinimize(surface Surface, rect Rect, buttons titleButtons) {
	r := buttons.minRect
	if r.X <= rect.X+1 {
		return
	}
	glyph := "[▾]"
	if w.minimized {
		glyph = "[▴]"
	}
	surface.WriteString(r.X, r.Y, glyph, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
}

// titleButtons holds the absolute hit/draw rects for the title-bar buttons that
// are currently shown. They are packed right-to-left in the order close,
// maximize, minimize, each occupying three cells with a one-cell gap. leftX is
// the X of the leftmost shown button (or -1 when no button is shown) so the
// title can reserve exactly the room the buttons need.
type titleButtons struct {
	closeRect Rect
	maxRect   Rect
	minRect   Rect
	hasClose  bool
	hasMax    bool
	hasMin    bool
	leftX     int
}

func (w *Window) titleButtons(abs Rect) titleButtons {
	buttons := titleButtons{leftX: -1}
	slot := 0
	next := func() Rect {
		r := Rect{X: abs.Right() - 5 - 4*slot, Y: abs.Y, W: 3, H: 1}
		slot++
		return r
	}
	if w.ShowClose {
		buttons.closeRect = next()
		buttons.hasClose = true
	}
	if w.Maximizable {
		buttons.maxRect = next()
		buttons.hasMax = true
	}
	if w.Minimizable {
		buttons.minRect = next()
		buttons.hasMin = true
	}
	if slot > 0 {
		buttons.leftX = abs.Right() - 5 - 4*(slot-1)
	}
	return buttons
}

// hitButton reports whether (x, y) falls on the given button rect when shown and
// not crowded off the left edge by the border/title.
func hitButton(abs Rect, r Rect, shown bool, x int, y int) bool {
	return shown && r.X > abs.X+1 && y == r.Y && x >= r.X && x <= r.Right()
}

func (w *Window) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	abs := component.AbsoluteBounds()
	// Resize drag continuity.
	if event.Down && w.resizing {
		newW := event.X - abs.X + 1
		newH := event.Y - abs.Y + 1
		newW, newH = w.clampResize(newW, newH)
		w.Component.SetBounds(Rect{X: w.Component.Bounds.X, Y: w.Component.Bounds.Y, W: newW, H: newH})
		if w.OnResize != nil {
			w.OnResize(w)
		}
		return true
	}
	if !event.Down && w.resizing {
		w.resizing = false
		w.lastMouseDown = false
		return true
	}
	// Move drag continuity.
	if event.Down && w.dragging {
		nx, ny := w.clampMove(event.X-w.dragOffsetX, event.Y-w.dragOffsetY, component.Bounds.W)
		component.SetBounds(Rect{X: nx, Y: ny, W: component.Bounds.W, H: component.Bounds.H})
		return true
	}
	if !event.Down && w.dragging {
		w.lastMouseDown = false
		w.dragging = false
		return true
	}
	if !abs.Contains(event.X, event.Y) {
		w.dragging = false
		w.resizing = false
		return false
	}
	// Start resizing from the bottom-right grip.
	if w.Resizable && !w.minimized && !w.maximized && event.Down && event.X == abs.Right() && event.Y == abs.Bottom() {
		w.resizing = true
		w.lastMouseDown = true
		return true
	}
	buttons := w.titleButtons(abs)
	// Minimize button.
	if event.Down && hitButton(abs, buttons.minRect, buttons.hasMin, event.X, event.Y) {
		w.ToggleMinimize()
		w.dragging = false
		return true
	}
	// Maximize button.
	if event.Down && hitButton(abs, buttons.maxRect, buttons.hasMax, event.X, event.Y) {
		w.ToggleMaximize()
		w.dragging = false
		return true
	}
	// Close button.
	if event.Down && hitButton(abs, buttons.closeRect, buttons.hasClose, event.X, event.Y) {
		if w.OnClose != nil {
			w.OnClose(w)
		}
		w.dragging = false
		return true
	}
	if event.Y != abs.Y {
		if !event.Down {
			w.dragging = false
		}
		return false
	}
	if event.Down && !w.lastMouseDown {
		w.dragging = true
		w.dragOffsetX = event.X - abs.X
		w.dragOffsetY = event.Y - abs.Y
		w.lastMouseDown = true
		return true
	}
	if !event.Down {
		w.lastMouseDown = false
		w.dragging = false
		return true
	}
	return false
}
