package tv

type Layer struct {
	Name        string
	Root        *VisualComponent
	AcceptInput bool
	FullScreen  bool
	Modal       bool
}

func NewLayer(name string, root Widget, acceptInput bool, fullScreen bool) *Layer {
	return &Layer{
		Name:        name,
		Root:        root.Root(),
		AcceptInput: acceptInput,
		FullScreen:  fullScreen,
	}
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
