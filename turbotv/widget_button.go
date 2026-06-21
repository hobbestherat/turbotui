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
	// than the button can never bleed into neighbouring widgets.
	if b.Shadow && !b.Pressed {
		surface.DrawShadow(abs, b.ShadowColor, b.ShadowStyle)
	}
	fg, bg := focusColors(component.Focused(), b.FG, b.BG, b.FocusFG, b.FocusBG)
	style := tui.Cell{FG: fg, BG: bg, Bold: true}
	faceSurface := surface.WithClip(face)
	faceSurface.Fill(face, style)

	clean, _ := parseMnemonic(b.Label)
	// Focused buttons are wrapped in chevrons so keyboard focus is obvious.
	left, right := "[ ", " ]"
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
	faceSurface.WriteString(face.X, face.Y, left, style)
	drawMnemonicClipped(faceSurface, captionStart, face.Y, b.Label, avail, style, component.mnemonicActive, activeTheme.MnemonicFG)
	faceSurface.WriteString(rightX, face.Y, right, style)
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
