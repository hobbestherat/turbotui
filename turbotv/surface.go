package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

type Surface struct {
	app  *tui.App
	clip Rect
}

func newRootSurface(app *tui.App) Surface {
	return Surface{
		app: app,
		clip: Rect{
			X: 0,
			Y: 0,
			W: app.Width(),
			H: app.Height(),
		},
	}
}

func (s Surface) WithClip(rect Rect) Surface {
	return Surface{
		app:  s.app,
		clip: s.clip.Intersect(rect),
	}
}

func (s Surface) Clip() Rect {
	return s.clip
}

func (s Surface) SetCell(x int, y int, cell tui.Cell) {
	if !s.clip.Contains(x, y) {
		return
	}
	s.app.WriteCell(x, y, cell)
}

// WriteString draws text at (x,y), clipped to the surface's clip rect. It is
// display-width aware: double-width glyphs advance two columns, combining marks
// fold into the preceding glyph, and a wide glyph that would straddle the clip
// edge is replaced by a blank so neighbouring widgets are never overdrawn.
func (s Surface) WriteString(x int, y int, text string, style tui.Cell) {
	column := x
	lastBase := -1
	for _, ch := range text {
		width := tui.RuneWidth(ch)
		if width == 0 {
			if lastBase >= 0 && s.clip.Contains(lastBase, y) {
				base := s.app.ReadCell(lastBase, y)
				base.Combining += string(ch)
				s.app.WriteCell(lastBase, y, base)
			}
			continue
		}
		cell := style
		cell.Combining = ""
		switch {
		case width >= 2 && s.clip.Contains(column, y) && s.clip.Contains(column+1, y):
			cell.Ch = ch
			s.app.WriteCell(column, y, cell)
			lastBase = column
		case width >= 2 && s.clip.Contains(column, y):
			// The wide glyph straddles the clip boundary; blank the visible half.
			cell.Ch = ' '
			s.app.WriteCell(column, y, cell)
			lastBase = -1
		case width == 1 && s.clip.Contains(column, y):
			cell.Ch = ch
			s.app.WriteCell(column, y, cell)
			lastBase = column
		default:
			lastBase = -1
		}
		column += width
	}
}

// WriteStringClipped draws text but never beyond maxWidth terminal columns
// (in addition to the surface clip), cutting on a glyph boundary so a
// double-width character is never split. For an ellipsis on overflow, pass the
// result of Truncate to WriteString instead.
func (s Surface) WriteStringClipped(x int, y int, maxWidth int, text string, style tui.Cell) {
	if maxWidth <= 0 {
		return
	}
	s.WriteString(x, y, Truncate(text, maxWidth, ""), style)
}

// Truncate shortens text so it occupies at most maxWidth terminal columns,
// appending ellipsis when it had to cut. It is width-aware and never splits a
// double-width glyph across the boundary; combining marks stay with their base.
func Truncate(text string, maxWidth int, ellipsis string) string {
	if maxWidth <= 0 {
		return ""
	}
	if tui.StringWidth(text) <= maxWidth {
		return text
	}
	ellipsisWidth := tui.StringWidth(ellipsis)
	if ellipsisWidth > maxWidth {
		ellipsis = ""
		ellipsisWidth = 0
	}
	budget := maxWidth - ellipsisWidth
	var b strings.Builder
	used := 0
	for _, r := range text {
		w := tui.RuneWidth(r)
		if used+w > budget {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String() + ellipsis
}

func (s Surface) Fill(rect Rect, cell tui.Cell) {
	target := s.clip.Intersect(rect)
	if target.Empty() {
		return
	}
	for y := target.Y; y < target.Y+target.H; y++ {
		for x := target.X; x < target.X+target.W; x++ {
			s.app.WriteCell(x, y, cell)
		}
	}
}

func (s Surface) DrawBox(rect Rect, line tui.LineKind, fg tui.Color, bg tui.Color) {
	if rect.W < 2 || rect.H < 2 {
		return
	}
	chars := tui.BorderStyleFor(line)
	right := rect.Right()
	bottom := rect.Bottom()
	for x := rect.X + 1; x < right; x++ {
		s.SetCell(x, rect.Y, tui.Cell{Ch: chars.Horizontal, FG: fg, BG: bg, Bold: true})
		s.SetCell(x, bottom, tui.Cell{Ch: chars.Horizontal, FG: fg, BG: bg, Bold: true})
	}
	for y := rect.Y + 1; y < bottom; y++ {
		s.SetCell(rect.X, y, tui.Cell{Ch: chars.Vertical, FG: fg, BG: bg, Bold: true})
		s.SetCell(right, y, tui.Cell{Ch: chars.Vertical, FG: fg, BG: bg, Bold: true})
	}
	s.SetCell(rect.X, rect.Y, tui.Cell{Ch: chars.TopLeft, FG: fg, BG: bg, Bold: true})
	s.SetCell(right, rect.Y, tui.Cell{Ch: chars.TopRight, FG: fg, BG: bg, Bold: true})
	s.SetCell(rect.X, bottom, tui.Cell{Ch: chars.BottomLeft, FG: fg, BG: bg, Bold: true})
	s.SetCell(right, bottom, tui.Cell{Ch: chars.BottomRight, FG: fg, BG: bg, Bold: true})
}

func (s Surface) DrawShadow(rect Rect, color tui.Color) {
	for y := rect.Y + 1; y <= rect.Bottom()+1; y++ {
		s.drawShadowCell(rect.Right()+1, y, color)
	}
	for x := rect.X + 1; x <= rect.Right()+1; x++ {
		s.drawShadowCell(x, rect.Bottom()+1, color)
	}
}

func (s Surface) drawShadowCell(x int, y int, color tui.Color) {
	if !s.clip.Contains(x, y) {
		return
	}
	under := s.app.ReadCell(x, y)
	ch := under.Ch
	if ch == 0 || ch == ' ' {
		ch = '░'
	}
	s.app.WriteCell(x, y, tui.Cell{
		Ch: ch,
		FG: color,
		BG: under.BG,
	})
}
