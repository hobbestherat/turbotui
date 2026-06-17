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
	MinWidth    int
	MinHeight   int
	// OnResize fires after the window is resized via the grip; OnMinimize fires
	// when the minimized state changes (minimized=true when collapsed).
	OnResize         func(*Window)
	OnMinimize       func(window *Window, minimized bool)
	dragging         bool
	dragOffsetX      int
	dragOffsetY      int
	lastMouseDown    bool
	resizing         bool
	minimized        bool
	restoreBounds    Rect
	bottomWasVisible bool
}

func NewWindow(title string, bounds Rect, border tui.LineKind) *Window {
	window := &Window{
		Title:       title,
		Border:      border,
		ShowClose:   true,
		TitleFG:     DefaultTheme.WindowTitleFG,
		TitleBG:     DefaultTheme.WindowTitleBG,
		BorderFG:    DefaultTheme.WindowBorderFG,
		BorderBG:    DefaultTheme.WindowBorderBG,
		CloseFG:     DefaultTheme.CloseButtonFG,
		CloseBG:     DefaultTheme.CloseButtonBG,
		Shadow:      true,
		ShadowColor: DefaultTheme.WindowShadow,
		MinWidth:    12,
		MinHeight:   3,
	}
	window.Component = NewComponent(bounds)
	window.Component.DrawOutside = true
	window.Component.DrawFn = window.draw
	window.Component.LayoutFn = window.layout
	window.Component.OnClickFn = window.handleClick
	window.Content = NewComponent(Rect{X: 1, Y: 1, W: bounds.W - 2, H: bounds.H - 2})
	window.Content.UseBackground = true
	window.Content.Background = tui.Cell{Ch: ' ', FG: DefaultTheme.WindowFG, BG: DefaultTheme.WindowBG}
	window.BottomBar = NewComponent(Rect{X: 1, Y: bounds.H - 2, W: bounds.W - 2, H: 1})
	window.BottomBar.Visible = false
	window.Component.AddChild(window.Content)
	window.Component.AddChild(window.BottomBar)
	return window
}
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

// IsMinimized reports whether the window is collapsed to its title bar.
func (w *Window) IsMinimized() bool { return w.minimized }

// Minimize collapses the window to a single title-bar row.
func (w *Window) Minimize() {
	if w.minimized {
		return
	}
	w.minimized = true
	w.restoreBounds = w.Component.Bounds
	w.Content.Visible = false
	w.bottomWasVisible = w.BottomBar.Visible
	w.BottomBar.Visible = false
	w.Component.SetBounds(Rect{X: w.Component.Bounds.X, Y: w.Component.Bounds.Y, W: w.Component.Bounds.W, H: 1})
	if w.OnMinimize != nil {
		w.OnMinimize(w, true)
	}
}

// Restore expands a minimized window back to its previous bounds.
func (w *Window) Restore() {
	if !w.minimized {
		return
	}
	w.minimized = false
	w.Content.Visible = true
	w.BottomBar.Visible = w.bottomWasVisible
	w.Component.SetBounds(w.restoreBounds)
	if w.OnMinimize != nil {
		w.OnMinimize(w, false)
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
func (w *Window) layout(component *VisualComponent) {
	if w.minimized {
		return
	}
	if component.Bounds.W < 2 || component.Bounds.H < 2 {
		return
	}
	contentHeight := component.Bounds.H - 2
	if w.BottomBar.Visible && contentHeight > 1 {
		contentHeight--
		w.BottomBar.SetBounds(Rect{X: 1, Y: component.Bounds.H - 2, W: component.Bounds.W - 2, H: 1})
	}
	w.Content.SetBounds(Rect{X: 1, Y: 1, W: component.Bounds.W - 2, H: contentHeight})
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
	title := strings.TrimSpace(w.Title)
	if title != "" && abs.W > 8 {
		w.drawTitle(surface, abs)
	}
	if w.Minimizable && abs.W > 12 {
		w.drawMinimize(surface, abs)
	}
	if w.ShowClose && abs.W > 8 {
		w.drawClose(surface, abs)
	}
	if w.Resizable {
		surface.SetCell(abs.Right(), abs.Bottom(), tui.Cell{Ch: '◢', FG: w.BorderFG, BG: w.BorderBG, Bold: true})
	}
}
func (w *Window) drawMinimizedBar(surface Surface, rect Rect) {
	surface.Fill(Rect{X: rect.X, Y: rect.Y, W: rect.W, H: 1}, tui.Cell{Ch: ' ', FG: w.TitleFG, BG: w.TitleBG})
	title := strings.TrimSpace(w.Title)
	maxLen := rect.W - 8
	if maxLen > 0 && title != "" {
		if len([]rune(title)) > maxLen {
			title = string([]rune(title)[:maxLen])
		}
		surface.WriteString(rect.X+1, rect.Y, title, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
	}
	if w.Minimizable {
		w.drawMinimize(surface, rect)
	}
	if w.ShowClose {
		w.drawClose(surface, rect)
	}
}
func (w *Window) drawTitle(surface Surface, rect Rect) {
	title := w.Title
	maxLen := rect.W - 8
	if maxLen < 1 {
		return
	}
	if len([]rune(title)) > maxLen {
		titleRunes := []rune(title)
		title = string(titleRunes[:maxLen])
	}
	text := " " + title + " "
	start := rect.X + (rect.W-len([]rune(text)))/2
	if start < rect.X+1 {
		start = rect.X + 1
	}
	surface.WriteString(start, rect.Y, text, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
}
func (w *Window) drawClose(surface Surface, rect Rect) {
	closeRect := closeButtonRect(rect)
	if closeRect.X <= rect.X+1 {
		return
	}
	surface.WriteString(closeRect.X, closeRect.Y, "[■]", tui.Cell{FG: w.CloseFG, BG: w.CloseBG, Bold: true})
}
func (w *Window) drawMinimize(surface Surface, rect Rect) {
	r := w.minimizeButtonRect(rect)
	if r.X <= rect.X+1 {
		return
	}
	glyph := "[▾]"
	if w.minimized {
		glyph = "[▴]"
	}
	surface.WriteString(r.X, r.Y, glyph, tui.Cell{FG: w.TitleFG, BG: w.TitleBG, Bold: true})
}

// closeButtonRect is the 3-cell "[■]" hit/draw region in the window title bar.
func closeButtonRect(abs Rect) Rect {
	return Rect{X: abs.Right() - 5, Y: abs.Y, W: 3, H: 1}
}

// minimizeButtonRect is the 3-cell minimize button, placed left of the close
// button when both are shown.
func (w *Window) minimizeButtonRect(abs Rect) Rect {
	x := abs.Right() - 5
	if w.ShowClose {
		x = abs.Right() - 9
	}
	return Rect{X: x, Y: abs.Y, W: 3, H: 1}
}
func (w *Window) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	abs := component.AbsoluteBounds()
	// Resize drag continuity.
	if event.Down && w.resizing {
		newW := event.X - abs.X + 1
		newH := event.Y - abs.Y + 1
		if newW < w.MinWidth {
			newW = w.MinWidth
		}
		if newH < w.MinHeight {
			newH = w.MinHeight
		}
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
		component.SetBounds(Rect{
			X: event.X - w.dragOffsetX,
			Y: event.Y - w.dragOffsetY,
			W: component.Bounds.W,
			H: component.Bounds.H,
		})
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
	if w.Resizable && !w.minimized && event.Down && event.X == abs.Right() && event.Y == abs.Bottom() {
		w.resizing = true
		w.lastMouseDown = true
		return true
	}
	// Minimize button.
	if w.Minimizable && event.Down && event.Y == abs.Y {
		r := w.minimizeButtonRect(abs)
		if r.X > abs.X+1 && event.X >= r.X && event.X <= r.Right() {
			w.ToggleMinimize()
			w.dragging = false
			return true
		}
	}
	// Close button.
	closeRect := closeButtonRect(abs)
	if event.Down && w.ShowClose && event.Y == abs.Y && event.X >= closeRect.X && event.X <= closeRect.Right() {
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
