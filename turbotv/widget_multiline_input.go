package tv

import (
	"strings"

	tui "github.com/hobbestherat/turbotui"
)

type MultiLineSubmitMode uint8

const (
	// MultiLineSubmitOnEnter is the default: Enter submits (when OnSubmit is set)
	// and Shift+Enter inserts a newline.
	MultiLineSubmitOnEnter MultiLineSubmitMode = iota
	// MultiLineSubmitOnShiftEnter swaps that: Shift+Enter submits, Enter inserts a
	// newline.
	MultiLineSubmitOnShiftEnter
	// MultiLineSubmitOnCtrlEnter submits on Ctrl+Enter and keeps Enter for newline.
	MultiLineSubmitOnCtrlEnter
)

type MultiLineInput struct {
	Component *VisualComponent
	Lines     []string
	CursorX   int
	CursorY   int
	ScrollY   int
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
	FocusBG   tui.Color
	// OnSubmit, when set, fires according to SubmitMode.
	OnSubmit   func()
	SubmitMode MultiLineSubmitMode
}

type wrappedLineRow struct {
	line  int
	start int
	runes []rune
}

func NewMultiLineInput(text string, bounds Rect) *MultiLineInput {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	input := &MultiLineInput{
		Lines:      lines,
		CursorX:    len([]rune(lines[len(lines)-1])),
		CursorY:    len(lines) - 1,
		FG:         DefaultTheme.InputFG,
		BG:         DefaultTheme.InputBG,
		FocusFG:    DefaultTheme.InputFocusFG,
		FocusBG:    DefaultTheme.InputFocusBG,
		SubmitMode: MultiLineSubmitOnEnter,
	}
	input.Component = NewComponent(bounds)
	input.Component.Focusable = true
	input.Component.DrawFn = input.draw
	input.Component.OnTypeFn = input.handleType
	input.Component.OnPasteFn = input.handlePaste
	input.Component.OnScrollFn = input.handleScroll
	input.Component.OnClickFn = input.handleClick
	input.Component.CursorFn = input.cursorPos
	return input
}

func (m *MultiLineInput) Root() *VisualComponent {
	return m.Component
}

func (m *MultiLineInput) GetText() string {
	return strings.Join(m.Lines, "\n")
}

func (m *MultiLineInput) SetText(text string) {
	m.Lines = strings.Split(text, "\n")
	if len(m.Lines) == 0 {
		m.Lines = []string{""}
	}
	m.CursorY = len(m.Lines) - 1
	m.CursorX = len([]rune(m.Lines[m.CursorY]))
	m.ScrollY = 0
}

func (m *MultiLineInput) Clear() {
	m.SetText("")
}

func (m *MultiLineInput) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := inputColors(component.HasFocus, m.FG, m.BG, m.FocusFG, m.FocusBG)
	style := tui.Cell{FG: fg, BG: bg}
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: fg, BG: bg})
	rows := m.wrappedRows(abs.W)
	m.ensureScroll(abs.H, abs.W)
	for row := 0; row < abs.H; row++ {
		rowIndex := m.ScrollY + row
		if rowIndex < 0 || rowIndex >= len(rows) {
			continue
		}
		wrapped := rows[rowIndex]
		for col := 0; col < abs.W && col < len(wrapped.runes); col++ {
			style.Ch = wrapped.runes[col]
			surface.SetCell(abs.X+col, abs.Y+row, style)
		}
	}
	if component.HasFocus {
		m.ensureScroll(abs.H, abs.W)
	}
}

// cursorPos reports the absolute caret position for the hardware cursor.
func (m *MultiLineInput) cursorPos(component *VisualComponent) (int, int, bool) {
	abs := component.AbsoluteBounds()
	m.ensureScroll(abs.H, abs.W)
	cursorVisualY, cursorVisualX := m.cursorVisualPos(abs.W)
	cursorY := cursorVisualY - m.ScrollY
	if cursorY < 0 || cursorY >= abs.H {
		return 0, 0, false
	}
	if cursorVisualX >= abs.W {
		cursorVisualX = abs.W - 1
	}
	if cursorVisualX < 0 {
		return 0, 0, false
	}
	return abs.X + cursorVisualX, abs.Y + cursorY, true
}

func (m *MultiLineInput) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyBackspace:
		m.backspace()
		return true
	case tui.KeyEnter:
		if m.OnSubmit == nil {
			m.newLine()
			return true
		}
		if m.shouldSubmit(event) {
			m.OnSubmit()
			return true
		}
		m.newLine()
		return true
	case tui.KeyLeft:
		m.moveLeft()
		return true
	case tui.KeyRight:
		m.moveRight()
		return true
	case tui.KeyUp:
		m.moveUp(m.Component.Bounds.W)
		return true
	case tui.KeyDown:
		m.moveDown(m.Component.Bounds.W)
		return true
	}
	if event.Key != tui.KeyRune || event.Ctrl {
		return false
	}
	m.insertRune(event.Rune)
	return true
}

func (m *MultiLineInput) handleScroll(_ *VisualComponent, event tui.ScrollEvent) bool {
	m.ScrollY -= event.Delta
	if m.ScrollY < 0 {
		m.ScrollY = 0
	}
	max := m.totalVisualRows(m.Component.Bounds.W) - 1
	if max < 0 {
		max = 0
	}
	if m.ScrollY > max {
		m.ScrollY = max
	}
	return true
}

func (m *MultiLineInput) insertRune(value rune) {
	line := []rune(m.Lines[m.CursorY])
	line = append(line, 0)
	copy(line[m.CursorX+1:], line[m.CursorX:])
	line[m.CursorX] = value
	m.Lines[m.CursorY] = string(line)
	m.CursorX++
}

// handlePaste inserts pasted text at the cursor, splitting on newlines so a
// multi-line paste spans multiple lines. CR is dropped so CRLF pastes behave.
func (m *MultiLineInput) handlePaste(_ *VisualComponent, text string) bool {
	for _, r := range text {
		switch {
		case r == '\r':
			continue
		case r == '\n':
			m.newLine()
		case r < 0x20:
			continue
		default:
			m.insertRune(r)
		}
	}
	return true
}

func (m *MultiLineInput) backspace() {
	if m.CursorX > 0 {
		line := []rune(m.Lines[m.CursorY])
		line = append(line[:m.CursorX-1], line[m.CursorX:]...)
		m.Lines[m.CursorY] = string(line)
		m.CursorX--
		return
	}
	if m.CursorY <= 0 {
		return
	}
	prev := []rune(m.Lines[m.CursorY-1])
	current := []rune(m.Lines[m.CursorY])
	m.CursorX = len(prev)
	m.Lines[m.CursorY-1] = string(append(prev, current...))
	m.Lines = append(m.Lines[:m.CursorY], m.Lines[m.CursorY+1:]...)
	m.CursorY--
}

func (m *MultiLineInput) newLine() {
	line := []rune(m.Lines[m.CursorY])
	left := string(line[:m.CursorX])
	right := string(line[m.CursorX:])
	m.Lines[m.CursorY] = left
	next := append([]string{}, m.Lines[:m.CursorY+1]...)
	next = append(next, right)
	next = append(next, m.Lines[m.CursorY+1:]...)
	m.Lines = next
	m.CursorY++
	m.CursorX = 0
}

func (m *MultiLineInput) moveLeft() {
	if m.CursorX > 0 {
		m.CursorX--
		return
	}
	if m.CursorY <= 0 {
		return
	}
	m.CursorY--
	m.CursorX = len([]rune(m.Lines[m.CursorY]))
}

func (m *MultiLineInput) moveRight() {
	line := []rune(m.Lines[m.CursorY])
	if m.CursorX < len(line) {
		m.CursorX++
		return
	}
	if m.CursorY >= len(m.Lines)-1 {
		return
	}
	m.CursorY++
	m.CursorX = 0
}

func (m *MultiLineInput) moveUp(width int) {
	cursorRow, cursorCol := m.cursorVisualPos(width)
	if cursorRow <= 0 {
		return
	}
	line, column := m.visualPosToCursor(width, cursorRow-1, cursorCol)
	m.CursorY = line
	m.CursorX = column
}

func (m *MultiLineInput) moveDown(width int) {
	cursorRow, cursorCol := m.cursorVisualPos(width)
	if cursorRow >= m.totalVisualRows(width)-1 {
		return
	}
	line, column := m.visualPosToCursor(width, cursorRow+1, cursorCol)
	m.CursorY = line
	m.CursorX = column
}

func (m *MultiLineInput) ensureScroll(height int, width int) {
	if height < 1 {
		height = 1
	}
	cursorRow, _ := m.cursorVisualPos(width)
	if cursorRow < m.ScrollY {
		m.ScrollY = cursorRow
	}
	if cursorRow >= m.ScrollY+height {
		m.ScrollY = cursorRow - height + 1
	}
	if m.ScrollY < 0 {
		m.ScrollY = 0
	}
	max := m.totalVisualRows(width) - height
	if max < 0 {
		max = 0
	}
	if m.ScrollY > max {
		m.ScrollY = max
	}
}

func (m *MultiLineInput) wrappedRows(width int) []wrappedLineRow {
	if width < 1 {
		width = 1
	}
	rows := make([]wrappedLineRow, 0, len(m.Lines))
	for lineIndex, lineText := range m.Lines {
		runes := []rune(lineText)
		if len(runes) == 0 {
			rows = append(rows, wrappedLineRow{line: lineIndex, start: 0, runes: []rune{}})
			continue
		}
		for start := 0; start < len(runes); start += width {
			end := start + width
			if end > len(runes) {
				end = len(runes)
			}
			rows = append(rows, wrappedLineRow{
				line:  lineIndex,
				start: start,
				runes: runes[start:end],
			})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, wrappedLineRow{line: 0, start: 0, runes: []rune{}})
	}
	return rows
}

func (m *MultiLineInput) shouldSubmit(event tui.TypeEvent) bool {
	switch m.SubmitMode {
	case MultiLineSubmitOnEnter:
		return !event.Shift
	case MultiLineSubmitOnCtrlEnter:
		return event.Ctrl
	default:
		return event.Shift
	}
}

func (m *MultiLineInput) totalVisualRows(width int) int {
	if width < 1 {
		width = 1
	}
	total := 0
	for _, line := range m.Lines {
		count := m.rowsForLine(line, width)
		total += count
	}
	if total < 1 {
		total = 1
	}
	return total
}

func (m *MultiLineInput) rowsForLine(line string, width int) int {
	if width < 1 {
		width = 1
	}
	length := len([]rune(line))
	if length == 0 {
		return 1
	}
	rows := (length + width - 1) / width
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *MultiLineInput) cursorVisualPos(width int) (int, int) {
	if width < 1 {
		width = 1
	}
	if len(m.Lines) == 0 {
		return 0, 0
	}
	if m.CursorY < 0 {
		m.CursorY = 0
	}
	if m.CursorY >= len(m.Lines) {
		m.CursorY = len(m.Lines) - 1
	}
	lineRunes := []rune(m.Lines[m.CursorY])
	if m.CursorX < 0 {
		m.CursorX = 0
	}
	if m.CursorX > len(lineRunes) {
		m.CursorX = len(lineRunes)
	}
	row := 0
	for index := 0; index < m.CursorY; index++ {
		row += m.rowsForLine(m.Lines[index], width)
	}
	offset := m.CursorX / width
	col := m.CursorX % width
	lineRows := m.rowsForLine(m.Lines[m.CursorY], width)
	if offset >= lineRows {
		offset = lineRows - 1
		if col >= width {
			col = width - 1
		}
	}
	row += offset
	return row, col
}

func (m *MultiLineInput) visualPosToCursor(width int, row int, col int) (int, int) {
	rows := m.wrappedRows(width)
	if len(rows) == 0 {
		return 0, 0
	}
	if row < 0 {
		row = 0
	}
	if row >= len(rows) {
		row = len(rows) - 1
	}
	target := rows[row]
	line := target.line
	lineRunes := []rune(m.Lines[line])
	cursor := target.start + col
	if cursor > len(lineRunes) {
		cursor = len(lineRunes)
	}
	if cursor < 0 {
		cursor = 0
	}
	return line, cursor
}

func (m *MultiLineInput) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	if !event.Down {
		return true
	}
	abs := component.AbsoluteBounds()
	if !abs.Contains(event.X, event.Y) {
		return false
	}
	row := m.ScrollY + (event.Y - abs.Y)
	col := event.X - abs.X
	line, cursor := m.visualPosToCursor(component.Bounds.W, row, col)
	m.CursorY = line
	m.CursorX = cursor
	return true
}
