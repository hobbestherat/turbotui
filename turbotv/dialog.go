package tv

import tui "github.com/hobbestherat/turbotui"

type Dialog struct {
	Window *Window
}

func NewDialog(title string, x int, y int, width int, height int) *Dialog {
	window := NewWindow(title, Rect{X: x, Y: y, W: width, H: height}, tui.LineDouble)
	window.TitleFG = activeTheme.DialogFG
	window.TitleBG = activeTheme.DialogBG
	window.BorderFG = activeTheme.DialogBorderFG
	window.BorderBG = activeTheme.DialogBorderBG
	window.Content.Background = tui.Cell{Ch: ' ', FG: activeTheme.DialogFG, BG: activeTheme.DialogBG}
	window.Shadow = true
	window.ShadowColor = activeTheme.WindowShadow
	return &Dialog{Window: window}
}

func (d *Dialog) Root() *VisualComponent {
	return d.Window.Component
}
