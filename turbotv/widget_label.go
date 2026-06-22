package tv

import (
	tui "github.com/hobbestherat/turbotui"
)

type Label struct {
	Component *VisualComponent
	Text      string
	FG        tui.Color
	BG        tui.Color
	HotFG     tui.Color
	// Wrap, when true (the default for labels created with NewLabel), word-wraps
	// the text across the label's rows instead of clipping a single line. A label
	// one row tall still renders only its first line; a multi-row label shows as
	// many wrapped lines as fit, so dialog messages and other long text stay
	// readable. The mnemonic marker and double-width runes are honoured while
	// wrapping. Set Wrap = false to restore the legacy single-line clip behaviour.
	Wrap bool
}

func NewLabel(text string, bounds Rect) *Label {
	label := &Label{
		Text:  text,
		FG:    activeTheme.WindowFG,
		BG:    activeTheme.WindowBG,
		HotFG: activeTheme.MnemonicFG,
		Wrap:  true,
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
	style := tui.Cell{FG: l.FG, BG: l.BG}
	if !l.Wrap || abs.W < 1 || abs.H < 1 {
		// Legacy single-line path: write the whole text, clipped to the surface.
		drawMnemonic(surface, abs.X, abs.Y, l.Text, style, component.mnemonicActive, l.HotFG)
		return
	}
	clean, hot := parseMnemonic(l.Text)
	rows := WrapLabelRunes([]rune(clean), abs.W)
	for row := 0; row < abs.H && row < len(rows); row++ {
		r := rows[row]
		y := abs.Y + row
		surface.WriteString(abs.X, y, string(r.Runes), style)
		// The mnemonic hot character lands on whichever wrapped row contains it;
		// highlight it there (by display width, so wide/combining runes stay put).
		if component.mnemonicActive && hot >= 0 && r.Start <= hot && hot < r.Start+len(r.Runes) {
			offset := hot - r.Start
			col := abs.X + tui.StringWidth(string(r.Runes[:offset]))
			surface.SetCell(col, y, tui.Cell{Ch: r.Runes[offset], FG: l.HotFG, BG: style.BG, Bold: true, Underline: true})
		}
	}
}
