package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
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
	// Degrade the logical colour to what the active terminal can render
	// (NO_COLOR strips it entirely; truecolor passes through unchanged).
	c = adaptColor(c, GetColorLevel())
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

// The append* helpers below build escape sequences directly into a caller-owned
// []byte using strconv.AppendInt instead of fmt.Sprintf/strings.Join. Apply keeps
// one such buffer across frames so a full repaint allocates nothing per cell (see
// issues #14/#15).

// appendCursorMove appends a CUP (cursor position) sequence for (x, y) to buf.
func appendCursorMove(buf []byte, x int, y int) []byte {
	buf = append(buf, 0x1b, '[')
	buf = strconv.AppendInt(buf, int64(y+1), 10)
	buf = append(buf, ';')
	buf = strconv.AppendInt(buf, int64(x+1), 10)
	buf = append(buf, 'H')
	return buf
}

// appendRune appends the UTF-8 encoding of r to buf.
func appendRune(buf []byte, r rune) []byte {
	return utf8.AppendRune(buf, r)
}

// appendColorCode appends the SGR sub-parameters that select c (e.g. "38;2;r;g;b"
// for a foreground truecolor) to buf. It mirrors colorCode but writes integers
// directly rather than allocating via fmt.Sprintf.
func appendColorCode(buf []byte, c Color, fg bool) []byte {
	c = adaptColor(c, GetColorLevel())
	switch c.Mode {
	case ColorANSI:
		index := int(c.Value)
		if index < 0 {
			index = 0
		}
		if index < 8 {
			base := 40
			if fg {
				base = 30
			}
			return strconv.AppendInt(buf, int64(base+index), 10)
		}
		if index < 16 {
			base := 100
			if fg {
				base = 90
			}
			return strconv.AppendInt(buf, int64(base+index-8), 10)
		}
		if fg {
			buf = append(buf, "38;5;"...)
		} else {
			buf = append(buf, "48;5;"...)
		}
		return strconv.AppendInt(buf, int64(index), 10)
	case ColorRGB:
		r := (c.Value >> 16) & 0xff
		g := (c.Value >> 8) & 0xff
		b := c.Value & 0xff
		if fg {
			buf = append(buf, "38;2;"...)
		} else {
			buf = append(buf, "48;2;"...)
		}
		buf = strconv.AppendInt(buf, int64(r), 10)
		buf = append(buf, ';')
		buf = strconv.AppendInt(buf, int64(g), 10)
		buf = append(buf, ';')
		buf = strconv.AppendInt(buf, int64(b), 10)
		return buf
	default:
		if fg {
			return append(buf, '3', '9')
		}
		return append(buf, '4', '9')
	}
}

// appendStyle appends the minimal SGR transition needed to move the terminal from
// cur to cell's style to buf. When cur is invalid it emits a full reset+colors;
// otherwise only the attributes that differ. It appends nothing when there is no
// change so adjacent same-style cells share one SGR (issue #14).
func appendStyle(buf []byte, cur styleState, cell Cell) []byte {
	if !cur.valid {
		buf = append(buf, 0x1b, '[', '0', ';')
		buf = appendColorCode(buf, cell.FG, true)
		buf = append(buf, ';')
		buf = appendColorCode(buf, cell.BG, false)
		if cell.Bold {
			buf = append(buf, ';', '1')
		}
		if cell.Underline {
			buf = append(buf, ';', '4')
		}
		return append(buf, 'm')
	}
	start := len(buf)
	buf = append(buf, 0x1b, '[')
	body := len(buf)
	if cur.bold != cell.Bold {
		if len(buf) > body {
			buf = append(buf, ';')
		}
		if cell.Bold {
			buf = append(buf, '1')
		} else {
			buf = append(buf, '2', '2')
		}
	}
	if cur.underline != cell.Underline {
		if len(buf) > body {
			buf = append(buf, ';')
		}
		if cell.Underline {
			buf = append(buf, '4')
		} else {
			buf = append(buf, '2', '4')
		}
	}
	if cur.fg != cell.FG {
		if len(buf) > body {
			buf = append(buf, ';')
		}
		buf = appendColorCode(buf, cell.FG, true)
	}
	if cur.bg != cell.BG {
		if len(buf) > body {
			buf = append(buf, ';')
		}
		buf = appendColorCode(buf, cell.BG, false)
	}
	if len(buf) == body {
		return buf[:start] // nothing changed; drop the tentative "\x1b["
	}
	return append(buf, 'm')
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
