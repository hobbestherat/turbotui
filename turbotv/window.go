package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

type Window struct {
	Component *VisualComponent
	Content   *VisualComponent
	BottomBar *VisualComponent

	Title         string
	Border        tui.LineKind
	ShowClose     bool
	TitleFG       tui.Color
	TitleBG       tui.Color
	BorderFG      tui.Color
	BorderBG      tui.Color
	CloseFG       tui.Color
	CloseBG       tui.Color
	Shadow        bool
	ShadowColor   tui.Color
	OnClose       func(*Window)
	dragging      bool
	dragOffsetX   int
	dragOffsetY   int
	lastMouseDown bool
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

func (w *Window) layout(component *VisualComponent) {
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
	if w.Shadow {
		surface.DrawShadow(abs, w.ShadowColor)
	}
	surface.DrawBox(abs, w.Border, w.BorderFG, w.BorderBG)
	title := strings.TrimSpace(w.Title)
	if title != "" && abs.W > 8 {
		w.drawTitle(surface, abs)
	}
	if w.ShowClose && abs.W > 8 {
		w.drawClose(surface, abs)
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

// closeButtonRect is the 3-cell "[■]" hit/draw region in the window title bar.
func closeButtonRect(abs Rect) Rect {
	return Rect{X: abs.Right() - 5, Y: abs.Y, W: 3, H: 1}
}

func (w *Window) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	abs := component.AbsoluteBounds()
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
		return false
	}
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
