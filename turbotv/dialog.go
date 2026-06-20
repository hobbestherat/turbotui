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

// SetDefaultCancelButtons gives the dialog Enter→default / Escape→cancel
// semantics (issue #63). Among the passed buttons it finds the one flagged
// Default and the one flagged Cancel and installs a root key handler: Enter
// presses the default and Escape presses the cancel whenever the keystroke
// reaches the dialog root — that is, when the focused widget did not consume it.
// A focused button therefore still handles its own Enter/Space, while Enter on a
// non-button field activates the default and Escape anywhere activates the cancel.
// The previous root handler (NewDialog's Escape→Close) is chained, so keys with
// no default/cancel still fall through to it.
func (d *Dialog) SetDefaultCancelButtons(buttons ...*Button) {
	var def, cancel *Button
	for _, button := range buttons {
		if button.Default {
			def = button
		}
		if button.Cancel {
			cancel = button
		}
	}
	previous := d.Window.Component.OnTypeFn
	d.Window.Component.OnTypeFn = func(component *VisualComponent, event tui.TypeEvent) bool {
		switch event.Key {
		case tui.KeyEnter:
			if def != nil {
				return def.press()
			}
		case tui.KeyEscape:
			if cancel != nil {
				return cancel.press()
			}
		}
		if previous != nil {
			return previous(component, event)
		}
		return false
	}
}

// windowRef satisfies hasWindow so adding a Dialog to a layer wires the
// underlying window to its layer and desktop (for Close and constraints).
func (d *Dialog) windowRef() *Window { return d.Window }
