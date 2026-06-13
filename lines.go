package tui

type LineKind uint8

const (
	LineNone LineKind = iota
	LineSingle
	LineDouble
)

type LineOrientation uint8

const (
	Horizontal LineOrientation = iota
	Vertical
)

type lineCell struct {
	up    LineKind
	right LineKind
	down  LineKind
	left  LineKind
	fg    Color
	bg    Color
}

func (a *App) AddLinePiece(x int, y int, orientation LineOrientation, kind LineKind, fg Color, bg Color) {
	if kind == LineNone {
		return
	}
	switch orientation {
	case Horizontal:
		if !a.back.inBounds(x, y) || !a.back.inBounds(x+1, y) {
			return
		}
		a.setLineEdge(x, y, 'R', kind, fg, bg)
		a.setLineEdge(x+1, y, 'L', kind, fg, bg)
		a.updateLineCell(x, y)
		a.updateLineCell(x+1, y)
	case Vertical:
		if !a.back.inBounds(x, y) || !a.back.inBounds(x, y+1) {
			return
		}
		a.setLineEdge(x, y, 'D', kind, fg, bg)
		a.setLineEdge(x, y+1, 'U', kind, fg, bg)
		a.updateLineCell(x, y)
		a.updateLineCell(x, y+1)
	}
}

func (a *App) DrawHorizontalLine(x int, y int, width int, kind LineKind, fg Color, bg Color) {
	if width < 2 {
		return
	}
	for offset := 0; offset < width-1; offset++ {
		a.AddLinePiece(x+offset, y, Horizontal, kind, fg, bg)
	}
}

func (a *App) DrawVerticalLine(x int, y int, height int, kind LineKind, fg Color, bg Color) {
	if height < 2 {
		return
	}
	for offset := 0; offset < height-1; offset++ {
		a.AddLinePiece(x, y+offset, Vertical, kind, fg, bg)
	}
}

func (a *App) DrawBox(x int, y int, width int, height int, kind LineKind, fg Color, bg Color) {
	if width < 2 || height < 2 {
		return
	}
	a.DrawHorizontalLine(x, y, width, kind, fg, bg)
	a.DrawHorizontalLine(x, y+height-1, width, kind, fg, bg)
	a.DrawVerticalLine(x, y, height, kind, fg, bg)
	a.DrawVerticalLine(x+width-1, y, height, kind, fg, bg)
}

func (a *App) setLineEdge(x int, y int, dir rune, kind LineKind, fg Color, bg Color) {
	idx := a.lineIndex(x, y)
	if idx < 0 {
		return
	}
	state := a.lines[idx]
	switch dir {
	case 'U':
		state.up = kind
	case 'R':
		state.right = kind
	case 'D':
		state.down = kind
	case 'L':
		state.left = kind
	}
	state.fg = fg
	state.bg = bg
	a.lines[idx] = state
}

func (a *App) updateLineCell(x int, y int) {
	idx := a.lineIndex(x, y)
	if idx < 0 {
		return
	}
	state := a.lines[idx]
	ch := lineRune(state)
	if ch == 0 {
		return
	}
	a.back.set(x, y, Cell{
		Ch: ch,
		FG: state.fg,
		BG: state.bg,
	})
}

// BorderStyle holds the glyphs for drawing a standalone box of a given LineKind.
type BorderStyle struct {
	Horizontal  rune
	Vertical    rune
	TopLeft     rune
	TopRight    rune
	BottomLeft  rune
	BottomRight rune
}

// BorderStyleFor returns the box-drawing glyphs for the given line kind. It is
// the single source of truth for border runes shared by higher-level toolkits.
func BorderStyleFor(kind LineKind) BorderStyle {
	if kind == LineDouble {
		return BorderStyle{
			Horizontal:  '═',
			Vertical:    '║',
			TopLeft:     '╔',
			TopRight:    '╗',
			BottomLeft:  '╚',
			BottomRight: '╝',
		}
	}
	return BorderStyle{
		Horizontal:  '─',
		Vertical:    '│',
		TopLeft:     '┌',
		TopRight:    '┐',
		BottomLeft:  '└',
		BottomRight: '┘',
	}
}

// lineRune resolves the glyph for a junction cell. Fully specified junctions are
// looked up in lineRuneMap; the fallback below handles partial states (e.g. a
// dangling stub) that the map does not enumerate.
func lineRune(state lineCell) rune {
	key := [4]LineKind{state.up, state.right, state.down, state.left}
	if r, ok := lineRuneMap[key]; ok {
		return r
	}
	hasVertical := state.up != LineNone || state.down != LineNone
	hasHorizontal := state.left != LineNone || state.right != LineNone
	if hasHorizontal && !hasVertical {
		if state.left == LineDouble || state.right == LineDouble {
			return '═'
		}
		return '─'
	}
	if hasVertical && !hasHorizontal {
		if state.up == LineDouble || state.down == LineDouble {
			return '║'
		}
		return '│'
	}
	if hasHorizontal && hasVertical {
		if state.left == LineDouble || state.right == LineDouble || state.up == LineDouble || state.down == LineDouble {
			if (state.left == LineSingle || state.right == LineSingle) && (state.up == LineDouble || state.down == LineDouble) {
				return '╫'
			}
			if (state.left == LineDouble || state.right == LineDouble) && (state.up == LineSingle || state.down == LineSingle) {
				return '╪'
			}
			return '╬'
		}
		return '┼'
	}
	return ' '
}

var lineRuneMap = map[[4]LineKind]rune{
	{0, 1, 0, 1}: '─',
	{1, 0, 1, 0}: '│',
	{0, 1, 1, 0}: '┌',
	{0, 0, 1, 1}: '┐',
	{1, 1, 0, 0}: '└',
	{1, 0, 0, 1}: '┘',
	{1, 1, 1, 0}: '├',
	{1, 0, 1, 1}: '┤',
	{0, 1, 1, 1}: '┬',
	{1, 1, 0, 1}: '┴',
	{1, 1, 1, 1}: '┼',
	{0, 2, 0, 2}: '═',
	{2, 0, 2, 0}: '║',
	{0, 2, 2, 0}: '╔',
	{0, 0, 2, 2}: '╗',
	{2, 2, 0, 0}: '╚',
	{2, 0, 0, 2}: '╝',
	{2, 2, 2, 0}: '╠',
	{2, 0, 2, 2}: '╣',
	{0, 2, 2, 2}: '╦',
	{2, 2, 0, 2}: '╩',
	{2, 2, 2, 2}: '╬',
	{0, 2, 1, 0}: '╒',
	{0, 1, 2, 0}: '╓',
	{0, 0, 1, 2}: '╕',
	{0, 0, 2, 1}: '╖',
	{1, 2, 0, 0}: '╘',
	{2, 1, 0, 0}: '╙',
	{1, 0, 0, 2}: '╛',
	{2, 0, 0, 1}: '╜',
	{1, 2, 1, 0}: '╞',
	{2, 1, 2, 0}: '╟',
	{1, 0, 1, 2}: '╡',
	{2, 0, 2, 1}: '╢',
	{0, 2, 1, 2}: '╤',
	{0, 1, 2, 1}: '╥',
	{1, 2, 0, 2}: '╧',
	{2, 1, 0, 1}: '╨',
	{1, 2, 1, 2}: '╪',
	{2, 1, 2, 1}: '╫',
}
