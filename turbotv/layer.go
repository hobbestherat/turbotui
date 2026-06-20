package tv

type Layer struct {
	Name        string
	Root        *VisualComponent
	AcceptInput bool
	FullScreen  bool
	Modal       bool
	// window is the Window this layer hosts (directly or via a Dialog), when any.
	// It lets the desktop hand the window a back-reference so Window.Close and
	// bounds constraints work without the app threading the layer through itself.
	window *Window
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
