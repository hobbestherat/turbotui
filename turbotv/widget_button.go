package tv

import tui "github.com/hobbestherat/turbotui"

type Button struct {
	Component   *VisualComponent
	Label       string
	FG          tui.Color
	BG          tui.Color
	FocusFG     tui.Color
	FocusBG     tui.Color
	Shadow      bool
	ShadowColor tui.Color
	Pressed     bool
	OnPress     func()
}

func NewButton(label string, bounds Rect, onPress func()) *Button {
	button := &Button{
		Label:       label,
		FG:          DefaultTheme.ButtonFG,
		BG:          DefaultTheme.ButtonBG,
		FocusFG:     DefaultTheme.ButtonFocusFG,
		FocusBG:     DefaultTheme.ButtonFocusBG,
		Shadow:      true,
		ShadowColor: DefaultTheme.ButtonShadow,
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
	if b.Shadow && !b.Pressed {
		surface.DrawShadow(abs, b.ShadowColor)
	}
	fg, bg := b.FG, b.BG
	if component.HasFocus {
		fg, bg = b.FocusFG, b.FocusBG
	}
	style := tui.Cell{FG: fg, BG: bg, Bold: true}
	surface.Fill(face, style)
	clean, _ := parseMnemonic(b.Label)
	// Focused buttons are wrapped in chevrons so keyboard focus is obvious.
	left, right := "[ ", " ]"
	if component.HasFocus {
		left, right = "►", "◄"
	}
	display := left + clean + right
	start := face.X + (face.W-len([]rune(display)))/2
	if start < face.X {
		start = face.X
	}
	drawMnemonic(surface, start, face.Y, left+b.Label+right, style, component.mnemonicActive, DefaultTheme.MnemonicFG)
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
