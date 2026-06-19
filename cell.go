package tui

import (
	"fmt"
	"strconv"
	"strings"
)

type ColorMode uint8

const (
	ColorDefault ColorMode = iota
	ColorANSI
	ColorRGB
)

type Color struct {
	Mode  ColorMode
	Value uint32
}

func DefaultColor() Color {
	return Color{Mode: ColorDefault}
}

func ANSIColor(index uint8) Color {
	return Color{Mode: ColorANSI, Value: uint32(index)}
}

func RGBColor(r uint8, g uint8, b uint8) Color {
	value := (uint32(r) << 16) | (uint32(g) << 8) | uint32(b)
	return Color{Mode: ColorRGB, Value: value}
}

type Cell struct {
	Ch rune
	// Combining holds any zero-width combining marks that follow Ch (e.g. a
	// U+0301 acute accent) so the whole grapheme renders in this one cell. It is
	// a string rather than a slice so Cell stays comparable for the flush diff.
	Combining string
	FG        Color
	BG        Color
	Bold      bool
	Underline bool
	// cont marks this cell as the right half (continuation) of a double-width
	// glyph occupying the cell to its left. Continuation cells are skipped by the
	// flush so the wide glyph is emitted once and the terminal advances over both
	// columns on its own.
	cont bool
}

func DefaultCell() Cell {
	return Cell{
		Ch: ' ',
		FG: DefaultColor(),
		BG: DefaultColor(),
	}
}

func (c Color) fgCode() string {
	return colorCode(c, true)
}

func (c Color) bgCode() string {
	return colorCode(c, false)
}

func colorCode(c Color, fg bool) string {
	switch c.Mode {
	case ColorANSI:
		index := int(c.Value)
		if index < 0 {
			index = 0
		}
		if index < 8 {
			if fg {
				return strconv.Itoa(30 + index)
			}
			return strconv.Itoa(40 + index)
		}
		if index < 16 {
			if fg {
				return strconv.Itoa(90 + index - 8)
			}
			return strconv.Itoa(100 + index - 8)
		}
		if fg {
			return fmt.Sprintf("38;5;%d", index)
		}
		return fmt.Sprintf("48;5;%d", index)
	case ColorRGB:
		r := (c.Value >> 16) & 0xff
		g := (c.Value >> 8) & 0xff
		b := c.Value & 0xff
		if fg {
			return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
		}
		return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	default:
		if fg {
			return "39"
		}
		return "49"
	}
}

type styleState struct {
	valid     bool
	fg        Color
	bg        Color
	bold      bool
	underline bool
}

func styleCodes(current styleState, cell Cell) []string {
	codes := make([]string, 0, 6)
	if !current.valid {
		codes = append(codes, "0", cell.FG.fgCode(), cell.BG.bgCode())
		if cell.Bold {
			codes = append(codes, "1")
		}
		if cell.Underline {
			codes = append(codes, "4")
		}
		return codes
	}
	if current.bold != cell.Bold {
		if cell.Bold {
			codes = append(codes, "1")
		} else {
			codes = append(codes, "22")
		}
	}
	if current.underline != cell.Underline {
		if cell.Underline {
			codes = append(codes, "4")
		} else {
			codes = append(codes, "24")
		}
	}
	if current.fg != cell.FG {
		codes = append(codes, cell.FG.fgCode())
	}
	if current.bg != cell.BG {
		codes = append(codes, cell.BG.bgCode())
	}
	return codes
}

func moveCursor(x int, y int) string {
	return fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)
}

func sgr(codes []string) string {
	if len(codes) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(codes, ";") + "m"
}

// Styled wraps text in SGR codes for the given colors so it can be printed after
// the TUI is torn down (e.g. in CloseWithMessage). It always resets at the end.
func Styled(text string, fg Color, bg Color, bold bool) string {
	codes := []string{"0", fg.fgCode(), bg.bgCode()}
	if bold {
		codes = append(codes, "1")
	}
	return sgr(codes) + text + "\x1b[0m"
}
