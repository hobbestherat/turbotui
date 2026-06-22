package tv

import (
	"unicode"

	tui "github.com/hobbestherat/turbotui"
)

// focusColors picks the foreground/background pair for a widget based on whether
// it currently has focus. It is shared by every input (TextBox/Select/Checkbox/
// MultiLineInput) and by non-input widgets that still swap colours on focus
// (Button), so the focus-colour pattern is spelled one way everywhere.
func focusColors(focused bool, fg tui.Color, bg tui.Color, focusFG tui.Color, focusBG tui.Color) (tui.Color, tui.Color) {
	if focused {
		return focusFG, focusBG
	}
	return fg, bg
}

// drawMnemonicClipped writes label (with the '&' mnemonic marker removed) at
// (x, y), truncating it to maxWidth terminal columns with a trailing "…" when it
// overflows, and underlines the mnemonic hot character in hotFG when highlight is
// true and it survived truncation. It returns the display width actually drawn.
//
// Unlike drawMnemonic (which never truncates), this is for widgets whose label
// must stay inside a fixed width — Button captions and Checkbox labels — so a long
// label shows an ellipsis instead of bleeding into a neighbour or being silently
// clipped by the surface.
func drawMnemonicClipped(surface Surface, x int, y int, label string, maxWidth int, style tui.Cell, highlight bool, hotFG tui.Color) int {
	clean, hot := parseMnemonic(label)
	if maxWidth <= 0 {
		return 0
	}
	text := clean
	cleanRunes := []rune(clean)
	prefixRunes := len(cleanRunes) // how many clean runes are shown before any ellipsis
	if tui.StringWidth(clean) > maxWidth {
		text = Truncate(clean, maxWidth, "…")
		// Truncate appended a single "…" rune, so the shown clean runes are the
		// truncated text minus that one rune.
		prefixRunes = len([]rune(text)) - len([]rune("…"))
		if prefixRunes < 0 {
			prefixRunes = 0
		}
	}
	surface.WriteString(x, y, text, style)
	if highlight && hot >= 0 && hot < prefixRunes {
		col := x + tui.StringWidth(string(cleanRunes[:hot]))
		surface.SetCell(col, y, tui.Cell{Ch: cleanRunes[hot], FG: mnemonicHotFG(hotFG, style), BG: style.BG, Bold: true, Underline: true})
	}
	return tui.StringWidth(text)
}

// mnemonicHotFG returns the colour to paint the mnemonic hot character in. It is
// normally hotFG (the theme's advertised hot-key colour), but when hotFG is
// structurally identical to the background it would be drawn over, the letter
// would render invisibly (e.g. a theme whose accent doubles as both MenuHotFG and
// the selected-row MenuSelectBG — accent-on-accent). In that case it falls back to
// the label's own foreground (style.FG), which the caller already chose to be
// legible on style.BG, while keeping Bold/Underline so the mnemonic affordance
// survives. The guard fires only on exact equality — when the letter would truly
// vanish — not on merely low contrast, so an intentionally subtle mnemonic hue is
// left untouched.
func mnemonicHotFG(hotFG tui.Color, style tui.Cell) tui.Color {
	if hotFG == style.BG {
		return style.FG
	}
	return hotFG
}

// parseMnemonic is the unexported alias kept for in-package call sites; the
// canonical implementation is ParseMnemonic in measure.go.
func parseMnemonic(label string) (string, int) { return ParseMnemonic(label) }

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
	surface.SetCell(col, y, tui.Cell{Ch: runes[hot], FG: mnemonicHotFG(hotFG, style), BG: style.BG, Bold: true, Underline: true})
}

// unicodeLower folds a rune to lower case for mnemonic matching. It uses
// unicode.ToLower so capitals whose lowercase differs in code point (e.g. 'İ',
// 'ẞ', accented Latin) round-trip correctly, not just ASCII A-Z.
func unicodeLower(value rune) rune {
	return unicode.ToLower(value)
}
