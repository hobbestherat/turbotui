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
	// Generic dialogs close on Escape by default (one-line common case). A key
	// reaches the dialog root only when no focused child consumed it first, and
	// callers that want different behaviour just replace this handler.
	window.Component.OnTypeFn = func(_ *VisualComponent, event tui.TypeEvent) bool {
		if event.Key == tui.KeyEscape {
			window.Close()
			return true
		}
		return false
	}
	return &Dialog{Window: window}
}

func (d *Dialog) Root() *VisualComponent {
	return d.Window.Component
}

// windowRef satisfies hasWindow so adding a Dialog to a layer wires the
// underlying window to its layer and desktop (for Close and constraints).
func (d *Dialog) windowRef() *Window { return d.Window }
