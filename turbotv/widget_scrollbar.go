package tv

import tui "github.com/hobbestherat/turbotui"

// This file is the single, shared vertical-scrollbar implementation used by the
// text view, tree and dropdown, so their look and behaviour stay consistent
// instead of each widget re-deriving its own thumb math.

// scrollbarMaxOffset is the largest valid first-visible index for a list of
// total items showing visible at a time.
func scrollbarMaxOffset(total, visible int) int {
	max := total - visible
	if max < 0 {
		max = 0
	}
	return max
}

// drawVScrollbar paints a 1-column vertical scrollbar at column track.X spanning
// track.Y..track.Bottom(): a full-height line with ▲/▼ caps and a single-cell
// thumb whose position reflects offset within [0, total-visible]. offset==0 puts
// the thumb just below the top arrow; offset==max puts it just above the bottom
// arrow.
func drawVScrollbar(surface Surface, track Rect, total, visible, offset int, fg, bg tui.Color, focused bool) {
	if track.H < 1 {
		return
	}
	x := track.X
	for row := 0; row < track.H; row++ {
		surface.SetCell(x, track.Y+row, tui.Cell{Ch: '│', FG: fg, BG: bg})
	}
	surface.SetCell(x, track.Y, tui.Cell{Ch: '▲', FG: fg, BG: bg, Bold: focused})
	surface.SetCell(x, track.Bottom(), tui.Cell{Ch: '▼', FG: fg, BG: bg, Bold: focused})
	span := total - visible
	inner := track.H - 2 // rows between the two arrow caps
	if span <= 0 || inner <= 0 {
		return
	}
	if offset < 0 {
		offset = 0
	}
	if offset > span {
		offset = span
	}
	thumb := offset * (inner - 1) / span
	if thumb < 0 {
		thumb = 0
	}
	if thumb > inner-1 {
		thumb = inner - 1
	}
	surface.SetCell(x, track.Y+1+thumb, tui.Cell{Ch: '█', FG: fg, BG: bg, Bold: focused})
}

// scrollbarOffsetForY maps a pointer Y on a scrollbar track (track.Y..Bottom())
// to a scroll offset in [0, total-visible]. The two arrow rows nudge by one; the
// area between maps proportionally. ok is false when there is nothing to scroll.
func scrollbarOffsetForY(track Rect, total, visible, currentOffset, y int) (int, bool) {
	span := total - visible
	if span <= 0 {
		return currentOffset, false
	}
	if y <= track.Y {
		return clampInt(currentOffset-1, 0, span), true
	}
	if y >= track.Bottom() {
		return clampInt(currentOffset+1, 0, span), true
	}
	inner := track.H - 2
	if inner <= 1 {
		return currentOffset, true
	}
	pos := y - (track.Y + 1)
	return clampInt(pos*span/(inner-1), 0, span), true
}

func clampInt(value, lo, hi int) int {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}
