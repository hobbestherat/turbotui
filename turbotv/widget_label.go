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
	rows := wrapLabelRunes([]rune(clean), abs.W)
	for row := 0; row < abs.H && row < len(rows); row++ {
		r := rows[row]
		y := abs.Y + row
		surface.WriteString(abs.X, y, string(r.runes), style)
		// The mnemonic hot character lands on whichever wrapped row contains it;
		// highlight it there (by display width, so wide/combining runes stay put).
		if component.mnemonicActive && hot >= 0 && r.start <= hot && hot < r.start+len(r.runes) {
			offset := hot - r.start
			col := abs.X + tui.StringWidth(string(r.runes[:offset]))
			surface.SetCell(col, y, tui.Cell{Ch: r.runes[offset], FG: l.HotFG, BG: style.BG, Bold: true, Underline: true})
		}
	}
}

// labelRow is one wrapped display line: a contiguous slice of the source runes
// (clean) and the index in clean at which it begins. Keeping the runes as a
// faithful sub-slice means the mnemonic hot index maps directly onto a row.
type labelRow struct {
	runes []rune
	start int
}

// wrapLabelRunes word-wraps clean into rows no wider than width terminal columns,
// preferring breaks at spaces and hard-splitting words longer than a row. Newlines
// force a break (and are themselves dropped). Each returned row is a contiguous
// sub-slice of clean, so the caller can locate any rune — e.g. the mnemonic hot
// character — by its index in clean.
func wrapLabelRunes(clean []rune, width int) []labelRow {
	if width < 1 {
		width = 1
	}
	var rows []labelRow
	n := len(clean)
	rowStart := 0   // index in clean of the first rune on the current row
	col := 0        // display width consumed on the current row
	lastSpace := -1 // index in clean of the most recent space on the current row
	commit := func(end int) {
		rows = append(rows, labelRow{runes: clean[rowStart:end], start: rowStart})
	}
	for i := 0; i < n; i++ {
		ch := clean[i]
		if ch == '\n' {
			// Hard break: close the row before the newline and resume after it.
			commit(i)
			rowStart = i + 1
			col = 0
			lastSpace = -1
			continue
		}
		cw := tui.RuneWidth(ch)
		if col+cw <= width {
			if ch == ' ' {
				lastSpace = i
			}
			col += cw
			continue
		}
		// The rune does not fit on the current row.
		if ch == ' ' {
			// A space that overflows is never useful at a row end; drop it and break.
			commit(i)
			rowStart = i + 1
			col = 0
			lastSpace = -1
			continue
		}
		if lastSpace >= rowStart {
			// Break just after the last space so the current word starts the next row.
			commit(lastSpace)
			rowStart = lastSpace + 1
			col = 0
			for k := rowStart; k <= i; k++ {
				col += tui.RuneWidth(clean[k])
			}
			lastSpace = -1
			if col <= width {
				continue
			}
			// The word fragment alone is wider than a row: hard-split it below.
		}
		// No breakable space (or a single word longer than a row): hard-split here.
		commit(i)
		rowStart = i
		col = cw
		lastSpace = -1
	}
	commit(n)
	return rows
}
