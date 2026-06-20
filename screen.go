package tui

// screen is one fixed-size grid of cells: the App keeps two of them (a front
// buffer holding what is currently on the terminal and a back buffer being drawn
// into) so Apply can diff them and emit only the cells that changed.
type screen struct {
	width  int
	height int
	cells  []Cell
}

func newScreen(width int, height int, cell Cell) screen {
	s := screen{
		width:  width,
		height: height,
		cells:  make([]Cell, width*height),
	}
	s.fill(cell)
	return s
}

func (s *screen) fill(cell Cell) {
	for i := range s.cells {
		s.cells[i] = cell
	}
}

func (s *screen) inBounds(x int, y int) bool {
	if x < 0 || y < 0 {
		return false
	}
	if x >= s.width || y >= s.height {
		return false
	}
	return true
}

func (s *screen) index(x int, y int) int {
	return y*s.width + x
}

func (s *screen) get(x int, y int) Cell {
	if !s.inBounds(x, y) {
		return DefaultCell()
	}
	return s.cells[s.index(x, y)]
}

func (s *screen) set(x int, y int, cell Cell) {
	if !s.inBounds(x, y) {
		return
	}
	if cell.Ch == 0 {
		cell.Ch = ' '
	}
	s.cells[s.index(x, y)] = cell
}
