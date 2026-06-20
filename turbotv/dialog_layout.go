package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// DefaultButtonGap is the number of blank cells NewButtonRow places between
// adjacent buttons unless a caller asks for a different gap.
const DefaultButtonGap = 2

// minButtonWidth keeps short captions (OK/Yes/No) from rendering as a cramped
// "[…]" by giving every button a comfortable floor width.
const minButtonWidth = 10

// buttonLabelWidth is the natural cell width a button needs to show its label
// with the "[ … ]" / "► … ◄" chrome and stay readable. It strips the mnemonic
// marker first so "&Yes" measures as "Yes", and clamps up to minButtonWidth.
func buttonLabelWidth(label string) int {
	clean, _ := parseMnemonic(label)
	width := tui.StringWidth(clean) + 4 // two cells of chrome on each side
	if width < minButtonWidth {
		width = minButtonWidth
	}
	return width
}

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

// longestLineWidth returns the display width of the widest hard line (split on
// '\n') in s, used to size a dialog to its message before wrapping.
func longestLineWidth(s string) int {
	widest := 0
	for _, line := range strings.Split(s, "\n") {
		if width := tui.StringWidth(line); width > widest {
			widest = width
		}
	}
	return widest
}
