package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

// Surface is the clipped drawing target handed to a component's DrawFn. It wraps
// the App's cell buffer together with a clip rectangle, so every write
// (SetCell, WriteString, Fill, DrawBox, DrawShadow) is confined to the
// component's area and can never overdraw a sibling. WithClip narrows the clip
// for a child region; the framework already passes each component a surface
// clipped to its bounds. A Surface is a small value, cheap to copy and pass by
// value.
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

// ReadCell returns the cell currently at (x,y), or the zero Cell when the
// coordinate lies outside the surface's clip rect. It lets a widget recolour a
// cell another pass already drew — e.g. overlaying a selection highlight — while
// preserving its rune and text attributes instead of overwriting them.
func (s Surface) ReadCell(x int, y int) tui.Cell {
	if !s.clip.Contains(x, y) {
		return tui.Cell{}
	}
	return s.app.ReadCell(x, y)
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

// ShadowStyle controls drop-shadow geometry: how far the shadow's corner notch
// is offset from the element's top-left-lit edges and how thick its right and
// bottom bands are. The default (DefaultShadowStyle) follows the classic
// Turbo-Vision proportion — a 2-column right band balancing a 1-row bottom band
// — so the shadow hugs the frame and still reads as balanced despite the ~2:1
// tall:wide terminal cell aspect ratio. Override DefaultShadowStyle before
// building the UI, or set a widget's ShadowStyle field, to adjust it.
type ShadowStyle struct {
	// OffsetX, OffsetY set the corner notch: the right band starts OffsetY rows
	// below the element's top edge and the bottom band starts OffsetX columns to
	// the right of its left edge, so the top-left corner reads as lit. 1 is the
	// snug classic value; larger values open the notch wider.
	OffsetX int
	OffsetY int
	// RightWidth, BottomHeight are the band thicknesses in cells. The bands always
	// hug the frame (their near edge is the cell immediately past the element), so
	// these control how heavy the shadow looks, not how detached it is.
	RightWidth   int
	BottomHeight int
}

// DefaultShadowStyle is the geometry new widgets seed their ShadowStyle from. The
// 2:1 right:bottom proportion compensates for the terminal cell aspect ratio so
// the L-shaped shadow looks balanced and snug.
var DefaultShadowStyle = ShadowStyle{
	OffsetX:      1,
	OffsetY:      1,
	RightWidth:   2,
	BottomHeight: 1,
}

// shadowGlyph is the texture a shadow cell lays down. The light-shade block is
// the classic Turbo-Vision drop-shadow fill.
const shadowGlyph = '░'

// DrawShadow paints an L-shaped drop shadow hugging the element's right and
// bottom edges. The bands always start at the cell immediately past the element
// (so the shadow never reads as detached); style controls their thickness and
// the top-left corner notch. A zero-thickness band is simply omitted.
//
// Each shadow cell owns its glyph and foreground: it always lays down shadowGlyph
// in the shadow colour, so the band's texture is a pure function of its geometry
// and never mirrors whatever rune happens to sit underneath. That is what stops a
// stale or bleed-through letter (drawn into the column on an earlier frame and
// never cleared) from leaking into the shadow as a stray character — the #213
// symptom.
//
// The underlying background colour is deliberately PRESERVED, not owned: the
// shadow reads as a translucent drop shadow that darkens whatever desktop or panel
// it falls on (a themed background keeps its colour, dimmed by the shadowGlyph
// texture) rather than punching a flat hole. The single shadow colour gives no
// second tone to own the background with, and forcing one would break shadows cast
// over a coloured desktop. One consequence: a *stale* background colour left in an
// uncleared cell still shows through — scrubbing it is the compositor's job (clear
// the back buffer before composing), not the shadow's. See drawShadowCell.
func (s Surface) DrawShadow(rect Rect, color tui.Color, style ShadowStyle) {
	right := rect.Right()
	bottom := rect.Bottom()
	// Right band: a vertical strip RightWidth columns wide just past the right
	// edge, started OffsetY rows down so the lit corner stays open.
	for dx := 1; dx <= style.RightWidth; dx++ {
		for y := rect.Y + style.OffsetY; y <= bottom+style.BottomHeight; y++ {
			s.drawShadowCell(right+dx, y, color)
		}
	}
	// Bottom band: a horizontal strip BottomHeight rows tall just past the bottom
	// edge, started OffsetX columns in from the left for the same reason.
	for dy := 1; dy <= style.BottomHeight; dy++ {
		for x := rect.X + style.OffsetX; x <= right+style.RightWidth; x++ {
			s.drawShadowCell(x, bottom+dy, color)
		}
	}
}

// drawShadowCell paints one shadow cell. The shadow owns the cell: it lays down a
// consistent shadowGlyph in the shadow colour, preserving only the underlying
// background, and deliberately does NOT read and re-emit the underlying foreground
// rune. Re-emitting it (the previous behaviour) could blit a stale or
// bleed-through glyph — e.g. an 'e' from a label drawn into this column on an
// earlier frame and never cleared — into the shadow band, where it reads as a
// random stray character instead of shadow. Writing a deterministic cell also
// means that when the back buffer still holds a stale glyph here, the value now
// differs from what the front buffer recorded, so an ordinary Apply repaints it.
// (That heals back-buffer staleness; a terminal that has drifted out of sync with
// the front buffer — front agreeing with back, terminal wrong — still needs
// App.Invalidate to force a full repaint.)
func (s Surface) drawShadowCell(x int, y int, color tui.Color) {
	if !s.clip.Contains(x, y) {
		return
	}
	under := s.app.ReadCell(x, y)
	s.app.WriteCell(x, y, tui.Cell{
		Ch: shadowGlyph,
		FG: color,
		BG: under.BG,
	})
}
