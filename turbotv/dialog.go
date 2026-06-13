package tv

import tui "github.com/hobbestherat/turbotui"

type Dialog struct {
	Window *Window
}

func NewDialog(title string, x int, y int, width int, height int) *Dialog {
	window := NewWindow(title, Rect{X: x, Y: y, W: width, H: height}, tui.LineDouble)
	window.TitleFG = DefaultTheme.DialogFG
	window.TitleBG = DefaultTheme.DialogBG
	window.BorderFG = DefaultTheme.DialogBorderFG
	window.BorderBG = DefaultTheme.DialogBorderBG
	window.Content.Background = tui.Cell{Ch: ' ', FG: DefaultTheme.DialogFG, BG: DefaultTheme.DialogBG}
	window.Shadow = true
	window.ShadowColor = DefaultTheme.WindowShadow
	return &Dialog{Window: window}
}

func (d *Dialog) Root() *VisualComponent {
	return d.Window.Component
}
