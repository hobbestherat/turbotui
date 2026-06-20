package tv

import tui "github.com/hobbestherat/turbotui"

type Label struct {
	Component *VisualComponent
	Text      string
	FG        tui.Color
	BG        tui.Color
	HotFG     tui.Color
}

func NewLabel(text string, bounds Rect) *Label {
	label := &Label{
		Text:  text,
		FG:    activeTheme.WindowFG,
		BG:    activeTheme.WindowBG,
		HotFG: activeTheme.MnemonicFG,
	}
	label.Component = NewComponent(bounds)
	label.Component.DrawFn = label.draw
	label.Component.Mnemonic = labelMnemonic(text)
	return label
}

func (l *Label) Root() *VisualComponent {
	return l.Component
}

func (l *Label) SetText(text string) {
	l.Text = text
	l.Component.Mnemonic = labelMnemonic(text)
}

func (l *Label) GetText() string {
	clean, _ := parseMnemonic(l.Text)
	return clean
}

// SetTarget links this label's mnemonic to another widget, so Alt+<hot> moves
// focus there (e.g. an "&Name" label above its input field).
func (l *Label) SetTarget(target Widget) {
	l.Component.MnemonicTarget = target.Root()
}

func (l *Label) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	drawMnemonic(surface, abs.X, abs.Y, l.Text, tui.Cell{FG: l.FG, BG: l.BG}, component.mnemonicActive, l.HotFG)
}
