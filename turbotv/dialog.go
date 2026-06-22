package tv

import tui "github.com/hobbestherat/turbotui"

// Dialog is a centered, dialog-themed Window for modal layers: a panel that
// closes on Escape by default and seeds its colours from the active theme's
// Dialog* palette. It wraps a Window (reachable through the Window field) so all
// the window machinery — content, buttons, dragging — is available; add it to a
// modal layer with NewModalLayer. See the dialog helpers (ShowConfirmYesNo, …)
// for ready-made dialogs.
type Dialog struct {
	Window *Window
	// autoSpec, when non-nil, is the sizing intent of a dialog created with
	// NewAutoDialog or last passed to Fit. It lets the dialog re-resolve its rect
	// against the current screen — both for Fit and for terminal-resize reflow.
	autoSpec *DialogSpec
	// desktop is the desktop this dialog was sized against (set by NewAutoDialog),
	// used by Fit before the dialog's window has been wired to a desktop via a layer.
	desktop *Desktop
}

// NewDialog creates a dialog-themed window at (x, y) with the given size. The
// returned dialog closes on Escape unless its Window.Component.OnTypeFn is
// replaced; wire default/cancel buttons with Dialog.SetDefaultCancelButtons.
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

// NewAutoDialog creates a dialog sized by spec against the desktop's current
// terminal size, rather than by hand-computed (x, y, w, h). It resolves the rect
// with ResolveDialogRect from desktop.App().Width()/Height() and forwards to
// NewDialog, so the dialog defaults to ~80%×85% of the terminal and shrinks only
// when the spec's content size or caps demand it. The spec is remembered so the
// open dialog re-resolves itself when the terminal is resized (see NewLayer's
// resize wiring). desktop must be non-nil — there is no global app to read the
// screen size from.
func NewAutoDialog(desktop *Desktop, title string, spec DialogSpec) *Dialog {
	if desktop == nil {
		return nil
	}
	x, y, w, h := ResolveDialogRect(spec, desktop.App().Width(), desktop.App().Height())
	dialog := NewDialog(title, x, y, w, h)
	dialog.autoSpec = &spec
	dialog.desktop = desktop
	return dialog
}

// Fit resizes an existing dialog to spec against the current terminal, centering
// it. Use it to add and measure content first, then grow the dialog to suit:
// resolve once with ResolveDialogRect and apply the new bounds via
// Window.Component.SetBounds, which re-runs the window LayoutFn so the content area
// is re-inset. The spec is remembered so a later terminal resize re-resolves it.
//
// Fit needs a desktop to read the screen size: either the dialog was created with
// NewAutoDialog, or it has been added to a desktop via a layer. It is a no-op
// otherwise.
func (d *Dialog) Fit(spec DialogSpec) {
	d.autoSpec = &spec
	// Fit can set autoSpec after the dialog was already placed in a layer (the
	// "build, add, fit-to-content" flow), in which case NewLayer ran before the
	// spec existed and did not install the resize hook. Install it now so a dialog
	// made auto-sized via Fit is just as resize-aware as one from NewAutoDialog. A
	// caller's own OnResize is left untouched.
	if d.Window != nil && d.Window.layer != nil && d.Window.layer.OnResize == nil {
		dialog := d
		d.Window.layer.OnResize = func(Rect) { dialog.reflow() }
	}
	d.reflow()
}

// resolveDesktop returns the desktop this dialog should size against — the one it
// was constructed with, or the one its window was wired to when added to a layer.
func (d *Dialog) resolveDesktop() *Desktop {
	if d.desktop != nil {
		return d.desktop
	}
	if d.Window != nil {
		return d.Window.desktop
	}
	return nil
}

// reflow re-resolves the dialog's rect from its remembered spec against the current
// terminal size and applies it. It is the shared body of Fit and the resize hook,
// and a no-op until the dialog has both a spec and a desktop.
func (d *Dialog) reflow() {
	if d.autoSpec == nil || d.Window == nil {
		return
	}
	desktop := d.resolveDesktop()
	if desktop == nil {
		return
	}
	x, y, w, h := ResolveDialogRect(*d.autoSpec, desktop.App().Width(), desktop.App().Height())
	d.Window.Component.SetBounds(Rect{X: x, Y: y, W: w, H: h})
}
