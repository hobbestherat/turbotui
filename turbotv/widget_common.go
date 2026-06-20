package tv

import (
	"unicode"

	tui "github.com/hobbestherat/turbotui"
)

// inputColors picks the foreground/background pair for an input widget based on
// whether it currently has focus.
func inputColors(focused bool, fg tui.Color, bg tui.Color, focusFG tui.Color, focusBG tui.Color) (tui.Color, tui.Color) {
	if focused {
		return focusFG, focusBG
	}
	return fg, bg
}

// parseMnemonic strips the '&' mnemonic marker from a label and returns the clean
// text plus the rune index that was marked (-1 when none). A literal '&' is
// written as "&&".
func parseMnemonic(label string) (string, int) {
	runes := []rune(label)
	out := make([]rune, 0, len(runes))
	hotIndex := -1
	for index := 0; index < len(runes); index++ {
		if runes[index] == '&' && index+1 < len(runes) {
			if runes[index+1] == '&' {
				out = append(out, '&')
				index++
				continue
			}
			if hotIndex < 0 {
				hotIndex = len(out)
			}
			continue
		}
		out = append(out, runes[index])
	}
	return string(out), hotIndex
}

// labelMnemonic returns the lowercased mnemonic rune of a label, or 0 when none.
func labelMnemonic(label string) rune {
	text, hot := parseMnemonic(label)
	runes := []rune(text)
	if hot < 0 || hot >= len(runes) {
		return 0
	}
	return unicodeLower(runes[hot])
}

// drawMnemonic writes label (with the '&' marker removed) and, when highlight is
// true, paints the hot character in hotFG to advertise its Alt-shortcut.
func drawMnemonic(surface Surface, x int, y int, label string, style tui.Cell, highlight bool, hotFG tui.Color) {
	text, hot := parseMnemonic(label)
	surface.WriteString(x, y, text, style)
	if !highlight || hot < 0 {
		return
	}
	runes := []rune(text)
	if hot >= len(runes) {
		return
	}
	// hot is a rune index, but the highlight must land on a column: sum the
	// display width of everything before it so a leading double-width (CJK) or
	// combining rune doesn't shift the highlight onto the wrong cell.
	col := x + tui.StringWidth(string(runes[:hot]))
	surface.SetCell(col, y, tui.Cell{Ch: runes[hot], FG: hotFG, BG: style.BG, Bold: true, Underline: true})
}

// unicodeLower folds a rune to lower case for mnemonic matching. It uses
// unicode.ToLower so capitals whose lowercase differs in code point (e.g. 'İ',
// 'ẞ', accented Latin) round-trip correctly, not just ASCII A-Z.
func unicodeLower(value rune) rune {
	return unicode.ToLower(value)
}
