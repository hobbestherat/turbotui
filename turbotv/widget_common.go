package tv

import tui "github.com/hobbestherat/turbotui"

// inputColors picks the foreground/background pair for an input widget based on
// whether it currently has focus.
func inputColors(focused bool, fg tui.Color, bg tui.Color, focusFG tui.Color, focusBG tui.Color) (tui.Color, tui.Color) {
	if focused {
		return focusFG, focusBG
	}
	return fg, bg
}

// cursorCell is the block cursor shared by the text input widgets. It inverts the
// surrounding text colors so it stays visible on any theme.
func cursorCell(fg tui.Color, bg tui.Color) tui.Cell {
	return tui.Cell{
		Ch:        '█',
		FG:        bg,
		BG:        fg,
		Bold:      true,
		Underline: true,
	}
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
	surface.SetCell(x+hot, y, tui.Cell{Ch: runes[hot], FG: hotFG, BG: style.BG, Bold: style.Bold})
}

func unicodeLower(value rune) rune {
	if value >= 'A' && value <= 'Z' {
		return value - 'A' + 'a'
	}
	return value
}

