package tv

// DefaultButtonGap is the number of blank cells NewButtonRow places between
// adjacent buttons unless a caller asks for a different gap.
const DefaultButtonGap = 2

// minButtonWidth keeps short captions (OK/Yes/No) from rendering as a cramped
// "[…]" by giving every button a comfortable floor width. It is the floor applied
// by ButtonLabelWidth (turbotv/measure.go).
const minButtonWidth = 10

// buttonLabelWidth is the unexported alias kept for in-package call sites; the
// canonical implementation is ButtonLabelWidth in measure.go.
func buttonLabelWidth(label string) int { return ButtonLabelWidth(label) }

// NewButtonRow lays a row of buttons out from their label widths instead of
// fixed magic offsets: each button is sized to its caption, separated from its
// neighbour by gap cells, and the whole group is start/center/end aligned within
// an interior interiorW cells wide at the vertical offset rowY (all relative to
// the parent the row is added to — typically a dialog's content area). The
// returned HBox is positioned so the group is aligned and clamped to the interior,
// so footers stay clean and in-bounds at any dialog size (issue #7). Callers add
// it with AddContent and, for a footer, pass the content width as interiorW.
func NewButtonRow(rowY int, interiorW int, align Align, gap int, buttons ...*Button) *Box {
	if gap < 0 {
		gap = 0
	}
	total := 0
	for index, button := range buttons {
		width := buttonLabelWidth(button.Label)
		button.Component.SetBounds(Rect{W: width, H: 1})
		if index > 0 {
			total += gap
		}
		total += width
	}
	boxX := 0
	switch align {
	case AlignCenter:
		boxX = (interiorW - total) / 2
	case AlignEnd:
		boxX = interiorW - total
	}
	if boxX < 0 {
		boxX = 0
	}
	boxW := total
	if boxW > interiorW {
		boxW = interiorW
	}
	box := NewHBox(Rect{X: boxX, Y: rowY, W: boxW, H: 1})
	box.Spacing = gap
	for _, button := range buttons {
		box.Add(button)
	}
	return box
}

// longestLineWidth is the unexported alias kept for in-package call sites; the
// canonical implementation is LongestLineWidth in measure.go.
func longestLineWidth(s string) int { return LongestLineWidth(s) }
