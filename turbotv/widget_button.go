package tv

import tui "github.com/hobbestherat/turbotui"

// Button is a focusable push button. It activates on Enter/Space or a click,
// calling OnPress; focus is shown by wrapping the caption in chevrons (►Label◄).
// A label declared with '&' (e.g. "&Quit") gives the button a mnemonic, and
// Default/Cancel mark the buttons that a dialog's Enter/Escape trigger when no
// focused widget consumed the key. Construct one with NewButton.
type Button struct {
	Component   *VisualComponent
	Label       string
	FG          tui.Color
	BG          tui.Color
	FocusFG     tui.Color
	FocusBG     tui.Color
	Shadow      bool
	ShadowColor tui.Color
	ShadowStyle ShadowStyle
	Pressed     bool
	OnPress     func()
	// Default marks the button that Enter activates when the keystroke reaches the
	// dialog root (i.e. no focused widget consumed it); Cancel marks the one Escape
	// activates. They are wired by Dialog.SetDefaultCancelButtons. A focused button
	// still handles its own Enter/Space, so these only matter when focus is on a
	// non-button widget (or for Escape, which buttons never consume themselves).
	Default bool
	Cancel  bool
}

// NewButton creates a button with the given label and bounds. onPress (which may
// be nil) is called when the button is activated by Enter, Space, a click or its
// mnemonic. The button seeds its colours and shadow from the active theme.
func NewButton(label string, bounds Rect, onPress func()) *Button {
	button := &Button{
		Label:       label,
		FG:          activeTheme.ButtonFG,
		BG:          activeTheme.ButtonBG,
		FocusFG:     activeTheme.ButtonFocusFG,
		FocusBG:     activeTheme.ButtonFocusBG,
		Shadow:      true,
		ShadowColor: activeTheme.ButtonShadow,
		ShadowStyle: DefaultShadowStyle,
		OnPress:     onPress,
	}
	button.Component = NewComponent(bounds)
	button.Component.Focusable = true
	button.Component.activatesOnEnter = true
	button.Component.DrawOutside = true
	button.Component.DrawFn = button.draw
	button.Component.OnTypeFn = button.handleType
	button.Component.OnClickFn = button.handleClick
	button.Component.Mnemonic = labelMnemonic(label)
	button.Component.OnActivateFn = func(_ *VisualComponent) {
		button.press()
	}
	return button
}

func (b *Button) Root() *VisualComponent {
	return b.Component
}

func (b *Button) SetLabel(label string) {
	b.Label = label
	b.Component.Mnemonic = labelMnemonic(label)
}

func (b *Button) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	face := abs
	if b.Pressed {
		face = Rect{X: abs.X + 1, Y: abs.Y + 1, W: abs.W, H: abs.H}
	}
	// The shadow sits outside the button bounds, so it must draw through the
	// parent's clip (the component opts out via DrawOutside). The face and label
	// do NOT: draw them through a face-bounds-clipped surface so a caption longer
	// than the button can never bleed into neighbouring widgets. The shadow keys
	// off abs.Right()/abs.Bottom(), so it hugs the bottom edge at any face height.
	if b.Shadow && !b.Pressed {
		surface.DrawShadow(abs, b.ShadowColor, b.ShadowStyle)
	}
	fg, bg := focusColors(component.Focused(), b.FG, b.BG, b.FocusFG, b.FocusBG)
	style := tui.Cell{FG: fg, BG: bg, Bold: true}
	faceSurface := surface.WithClip(face)
	// Fill paints every row of the face, so a tall button reads as a solid block.
	faceSurface.Fill(face, style)

	clean, _ := parseMnemonic(b.Label)
	// Height comes purely from bounds: brackets and the box face run the full
	// height for visual weight (a solid "[ … ]" block), while the caption and the
	// focus chevrons sit on the single vertically-centred row. centerY rounds down
	// on even heights (H=2 -> the lower of the two rows). At H==1 the centre row is
	// the only row, so this renders identically to a one-row button (gogent#529).
	centerY := face.Y + face.H/2

	// Box brackets frame every row; they stay plain even when focused so the box
	// outline reads continuously and only the caption row carries the chevrons.
	boxLeft, boxRight := "[ ", " ]"
	boxRightW := tui.StringWidth(boxRight)
	boxRightX := face.X + face.W - boxRightW
	if boxRightX < face.X {
		boxRightX = face.X
	}

	// Caption-row chrome: focused buttons swap the brackets for chevrons so
	// keyboard focus is obvious.
	left, right := boxLeft, boxRight
	if component.Focused() {
		left, right = "►", "◄"
	}
	leftW := tui.StringWidth(left)
	rightW := tui.StringWidth(right)
	avail := face.W - leftW - rightW
	if avail < 0 {
		avail = 0
	}
	// Caption width after truncation, for centring. When the label fits it keeps
	// its full width; otherwise it is ellipsised down to the available width.
	captionW := tui.StringWidth(clean)
	if captionW > avail {
		captionW = avail
	}
	// Pin the brackets flush to the face bounds and float only the caption between
	// them, so face.W alone controls the visible "[ … ]" box width: buttons given
	// equal bounds paint equal boxes regardless of label length (gogent#259). The
	// caption keeps the exact X it had when the whole group was centred —
	// face.X+leftW+(avail-captionW)/2 equals the old start+leftW — so buttons sized
	// to buttonWidth(label) render identically; only wider faces change, spreading
	// the brackets to the edges instead of leaving a narrower outline.
	captionStart := face.X + leftW + (avail-captionW)/2
	rightX := face.X + face.W - rightW
	if rightX < face.X {
		rightX = face.X // face narrower than the right bracket: clamp, never go left of the face
	}
	for y := face.Y; y <= face.Bottom(); y++ {
		if y == centerY {
			// The centred row carries the caption between the (possibly chevron)
			// brackets.
			faceSurface.WriteString(face.X, y, left, style)
			drawMnemonicClipped(faceSurface, captionStart, y, b.Label, avail, style, component.mnemonicActive, activeTheme.MnemonicFG)
			faceSurface.WriteString(rightX, y, right, style)
			continue
		}
		// Every other row draws only the plain box brackets at the face edges.
		faceSurface.WriteString(face.X, y, boxLeft, style)
		faceSurface.WriteString(boxRightX, y, boxRight, style)
	}
}

func (b *Button) press() bool {
	if b.OnPress == nil {
		return true
	}
	b.OnPress()
	return true
}

func (b *Button) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	if event.Key == tui.KeyEnter || (event.Key == tui.KeyRune && event.Rune == ' ') {
		return b.press()
	}
	return false
}

func (b *Button) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	inside := component.AbsoluteBounds().Contains(event.X, event.Y)
	if event.Down {
		b.Pressed = inside
		return true
	}
	wasPressed := b.Pressed
	b.Pressed = false
	if !inside || !wasPressed {
		return true
	}
	return b.press()
}
