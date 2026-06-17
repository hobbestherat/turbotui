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

	selAnchorX int
	selAnchorY int // -1 when there is no selection
	selecting  bool
	// pressLine/pressCursor remember where the mouse went down so a selection is
	// only anchored once the pointer actually drags away from that point. A plain
	// click therefore leaves no selection (which previously caused the first
	// typed character to be treated as selected and overwritten).
	pressLine   int
	pressCursor int
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
		selAnchorY: -1,
	}
	input.Component = NewComponent(bounds)
	input.Component.Focusable = true
	input.Component.DrawFn = input.draw
	input.Component.OnTypeFn = input.handleType
	input.Component.OnPasteFn = input.handlePaste
	input.Component.OnScrollFn = input.handleScroll
	input.Component.OnClickFn = input.handleClick
	input.Component.CursorFn = input.cursorPos
	input.Component.CopyFn = input.copySelection
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
	m.selAnchorY = -1
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
			cell := style
			cell.Ch = wrapped.runes[col]
			if m.isSelected(wrapped.line, wrapped.start+col) {
				cell.FG = DefaultTheme.SelectionFG
				cell.BG = DefaultTheme.SelectionBG
			}
			surface.SetCell(abs.X+col, abs.Y+row, cell)
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
		if m.deleteSelection() {
			return true
		}
		m.backspace()
		return true
	case tui.KeyDelete:
		if m.deleteSelection() {
			return true
		}
		m.forwardDelete()
		return true
	case tui.KeyEnter:
		if m.OnSubmit != nil && m.shouldSubmit(event) {
			m.OnSubmit()
			return true
		}
		m.deleteSelection()
		m.newLine()
		return true
	case tui.KeyLeft:
		m.extendOrClear(event.Shift)
		m.moveLeft()
		return true
	case tui.KeyRight:
		m.extendOrClear(event.Shift)
		m.moveRight()
		return true
	case tui.KeyUp:
		m.extendOrClear(event.Shift)
		m.moveUp(m.Component.Bounds.W)
		return true
	case tui.KeyDown:
		m.extendOrClear(event.Shift)
		m.moveDown(m.Component.Bounds.W)
		return true
	case tui.KeyHome:
		m.extendOrClear(event.Shift)
		m.CursorX = 0
		return true
	case tui.KeyEnd:
		m.extendOrClear(event.Shift)
		m.CursorX = len([]rune(m.Lines[m.CursorY]))
		return true
	}
	if event.Key != tui.KeyRune || event.Ctrl {
		return false
	}
	m.deleteSelection()
	m.insertRune(event.Rune)
	return true
}

// extendOrClear starts/keeps the selection anchor when extend is true (shift held)
// or clears the selection otherwise, before the caller moves the cursor.
func (m *MultiLineInput) extendOrClear(extend bool) {
	if extend {
		if m.selAnchorY < 0 {
			m.selAnchorX = m.CursorX
			m.selAnchorY = m.CursorY
		}
		return
	}
	m.selAnchorY = -1
}

func (m *MultiLineInput) hasSelection() bool {
	return m.selAnchorY >= 0 && (m.selAnchorY != m.CursorY || m.selAnchorX != m.CursorX)
}

func (m *MultiLineInput) selectionOrdered() (int, int, int, int) {
	ay, ax, cy, cx := m.selAnchorY, m.selAnchorX, m.CursorY, m.CursorX
	if ay > cy || (ay == cy && ax > cx) {
		ay, ax, cy, cx = cy, cx, ay, ax
	}
	return ay, ax, cy, cx
}

func (m *MultiLineInput) isSelected(line int, col int) bool {
	if !m.hasSelection() {
		return false
	}
	y0, x0, y1, x1 := m.selectionOrdered()
	if line < y0 || line > y1 {
		return false
	}
	if y0 == y1 {
		return col >= x0 && col < x1
	}
	if line == y0 {
		return col >= x0
	}
	if line == y1 {
		return col < x1
	}
	return true
}

func (m *MultiLineInput) selectionText() string {
	if !m.hasSelection() {
		return ""
	}
	y0, x0, y1, x1 := m.selectionOrdered()
	if y0 == y1 {
		runes := []rune(m.Lines[y0])
		x0, x1 = clampRange(len(runes), x0, x1)
		return string(runes[x0:x1])
	}
	var builder strings.Builder
	first := []rune(m.Lines[y0])
	x0 = clampCol(len(first), x0)
	builder.WriteString(string(first[x0:]))
	builder.WriteByte('\n')
	for line := y0 + 1; line < y1; line++ {
		builder.WriteString(m.Lines[line])
		builder.WriteByte('\n')
	}
	last := []rune(m.Lines[y1])
	x1 = clampCol(len(last), x1)
	builder.WriteString(string(last[:x1]))
	return builder.String()
}

func (m *MultiLineInput) deleteSelection() bool {
	if !m.hasSelection() {
		return false
	}
	y0, x0, y1, x1 := m.selectionOrdered()
	first := []rune(m.Lines[y0])
	last := []rune(m.Lines[y1])
	// Selection columns can outlive the text they referenced (e.g. after the
	// cursor moves to a shorter line), so clamp before slicing to avoid panics.
	x0 = clampCol(len(first), x0)
	x1 = clampCol(len(last), x1)
	merged := string(first[:x0]) + string(last[x1:])
	newLines := append([]string{}, m.Lines[:y0]...)
	newLines = append(newLines, merged)
	newLines = append(newLines, m.Lines[y1+1:]...)
	m.Lines = newLines
	m.CursorY = y0
	m.CursorX = x0
	m.selAnchorY = -1
	return true
}

// clampCol bounds a column index to [0, length].
func clampCol(length, col int) int {
	if col < 0 {
		return 0
	}
	if col > length {
		return length
	}
	return col
}

// clampRange bounds a same-line [x0, x1) selection to [0, length] and ensures
// x0 <= x1.
func clampRange(length, x0, x1 int) (int, int) {
	x0 = clampCol(length, x0)
	x1 = clampCol(length, x1)
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	return x0, x1
}

func (m *MultiLineInput) forwardDelete() {
	line := []rune(m.Lines[m.CursorY])
	if m.CursorX < len(line) {
		line = append(line[:m.CursorX], line[m.CursorX+1:]...)
		m.Lines[m.CursorY] = string(line)
		return
	}
	if m.CursorY >= len(m.Lines)-1 {
		return
	}
	m.Lines[m.CursorY] = string(line) + m.Lines[m.CursorY+1]
	m.Lines = append(m.Lines[:m.CursorY+1], m.Lines[m.CursorY+2:]...)
}

func (m *MultiLineInput) copySelection(_ *VisualComponent) (string, bool) {
	text := m.selectionText()
	return text, text != ""
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

// handlePaste inserts pasted text at the cursor, replacing any selection and
// splitting on newlines so a multi-line paste spans multiple lines. CR is
// dropped so CRLF pastes behave.
func (m *MultiLineInput) handlePaste(_ *VisualComponent, text string) bool {
	m.deleteSelection()
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
	abs := component.AbsoluteBounds()
	if !event.Down {
		m.selecting = false
		return true
	}
	row := m.ScrollY + (event.Y - abs.Y)
	col := event.X - abs.X
	if col < 0 {
		col = 0
	}
	if row < 0 {
		row = 0
	}
	line, cursor := m.visualPosToCursor(component.Bounds.W, row, col)
	if !m.selecting {
		if !abs.Contains(event.X, event.Y) {
			return false
		}
		// Place the caret and clear any existing selection. Do NOT anchor a
		// selection yet: a selection is only started if the pointer drags.
		m.CursorY = line
		m.CursorX = cursor
		m.pressLine = line
		m.pressCursor = cursor
		m.selAnchorY = -1
		m.selecting = true
		return true
	}
	// Drag motion: the first time the pointer leaves the press point, anchor the
	// selection there; then extend it to the current pointer position.
	if line != m.pressLine || cursor != m.pressCursor {
		if m.selAnchorY < 0 {
			m.selAnchorX = m.pressCursor
			m.selAnchorY = m.pressLine
		}
		m.CursorY = line
		m.CursorX = cursor
	}
	return true
}
