package tv

import "time"

type Layer struct {
	Name        string
	Root        *VisualComponent
	AcceptInput bool
	FullScreen  bool
	Modal       bool
	// OnClickOutside, when set on a modal layer, is invoked when a click lands
	// outside the layer's root while it is on top. A modal swallows such clicks
	// (they never reach lower layers, issue #42); this hook lets the app react —
	// flash the dialog, beep, or dismiss it.
	OnClickOutside func(*Layer)
	// OnResize, when set, is invoked with the layer root's (re-clamped) bounds
	// whenever the terminal is resized, so windowed layers can reflow their
	// contents. FullScreen layers are stretched automatically and do not receive
	// it (issue #71).
	OnResize func(Rect)
	// window is the Window this layer hosts (directly or via a Dialog), when any.
	// It lets the desktop hand the window a back-reference so Window.Close and
	// bounds constraints work without the app threading the layer through itself.
	window *Window
	// armedAt is the modal Enter-grace timestamp (gogent#347). The desktop stamps it
	// from its (injectable) clock when a Modal layer is added, marking the instant the
	// modal appeared. While the desktop's enter-grace window has not yet elapsed since
	// armedAt, Desktop.handleType swallows Enter for the focused widget so a keystroke
	// the user had already begun before the modal popped up cannot activate its button.
	// Zero means "never armed" (no suppression). See Desktop.SetEnterGrace.
	armedAt time.Time
}

// Arm sets the layer's Enter-grace timestamp (gogent#347). Modal layers are armed
// automatically by Desktop.AddLayer from the desktop clock; call this to re-arm an
// already-shown modal (e.g. when it is re-presented) or to arm deterministically in a
// test by passing a fixed time. The grace duration itself is set on the desktop with
// SetEnterGrace.
func (l *Layer) Arm(at time.Time) {
	l.armedAt = at
}

// ArmedAt reports the layer's current Enter-grace timestamp (zero when never armed).
func (l *Layer) ArmedAt() time.Time {
	return l.armedAt
}

// hasWindow is implemented by roots that own a Window (Window itself and Dialog),
// so NewLayer can discover and wire the hosted window.
type hasWindow interface {
	windowRef() *Window
}

func NewLayer(name string, root Widget, acceptInput bool, fullScreen bool) *Layer {
	layer := &Layer{
		Name:        name,
		Root:        root.Root(),
		AcceptInput: acceptInput,
		FullScreen:  fullScreen,
	}
	if owner, ok := root.(hasWindow); ok {
		if window := owner.windowRef(); window != nil {
			layer.window = window
			window.layer = layer
		}
	}
	// An auto-sized dialog re-resolves its rect against the new terminal size when
	// the desktop reports a resize, so an open dialog stays ~80%×85% of the screen
	// instead of remaining a fixed box on a now-larger terminal. Callers may still
	// override OnResize afterwards for non-auto layouts.
	if dialog, ok := root.(*Dialog); ok && dialog.autoSpec != nil {
		layer.OnResize = func(Rect) { dialog.reflow() }
	}
	return layer
}

// NewFullscreenLayer creates an input-accepting layer stretched to the desktop size.
func NewFullscreenLayer(name string, root Widget) *Layer {
	return NewLayer(name, root, true, true)
}

// NewWindowLayer creates a normal, non-modal windowed layer. Menu shortcuts from
// lower layers stay active while it is on top.
func NewWindowLayer(name string, root Widget) *Layer {
	return NewLayer(name, root, true, false)
}

// NewModalLayer creates a modal windowed layer (dialogs). While it is on top it
// captures all mnemonics/shortcuts; lower layers (including the menubar) are inert.
func NewModalLayer(name string, root Widget) *Layer {
	layer := NewLayer(name, root, true, false)
	layer.Modal = true
	return layer
}
