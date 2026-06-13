package tv

import tui "github.com/hobbestherat/turbotui"

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

func (s Surface) WriteString(x int, y int, text string, style tui.Cell) {
	column := x
	for _, ch := range text {
		if s.clip.Contains(column, y) {
			style.Ch = ch
			s.app.WriteCell(column, y, style)
		}
		column++
	}
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
