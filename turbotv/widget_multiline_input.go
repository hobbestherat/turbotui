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
	// Lines holds the logical lines of the buffer. NOTE: a line may contain an opaque
	// paste-chip sentinel rune (see paste_chip.go) standing in for a collapsed
	// multi-line paste — use GetText() for the verbatim text and IsPasteChipRune to
	// detect a chip when scanning Lines directly. CursorX is a rune offset into the
	// current line, so a chip (one rune) is one caret stop.
	Lines   []string
	CursorX int
	CursorY int
	ScrollY int
	FG      tui.Color
	BG      tui.Color
	FocusFG tui.Color
	FocusBG tui.Color
	// WordWrap, when true, breaks long logical lines on whitespace so words are
	// kept intact instead of being split mid-word at the right edge. The default
	// (false) keeps the original character-level wrapping, which suits code.
	WordWrap bool
	// OnSubmit, when set, fires according to SubmitMode.
	OnSubmit   func()
	SubmitMode MultiLineSubmitMode

	selAnchorX int
	selAnchorY int // -1 when there is no selection
	selecting  bool
	// draggingThumb is true while the user holds the scrollbar thumb, so motion
	// events keep mapping to the scroll position even off the 1-column track.
	draggingThumb bool
	// pressLine/pressCursor remember where the mouse went down so a selection is
	// only anchored once the pointer actually drags away from that point. A plain
	// click therefore leaves no selection (which previously caused the first
	// typed character to be treated as selected and overwritten).
	pressLine   int
	pressCursor int
	// chips holds the verbatim original text of every collapsed multi-line paste,
	// keyed by the sentinel rune that stands for it in Lines.
	chips chipStore
}

type wrappedLineRow struct {
	line  int
	start int
	runes []rune
}

// runeSpan is a half-open [start, end) range of rune indices into one logical
// line. Spans for a line are contiguous and cover it with no gaps, so a span's
// start is always a valid offset for selection/cursor math regardless of mode.
type runeSpan struct {
	start int
	end   int
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
		FG:         activeTheme.InputFG,
		BG:         activeTheme.InputBG,
		FocusFG:    activeTheme.InputFocusFG,
		FocusBG:    activeTheme.InputFocusBG,
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

// GetText returns the verbatim buffer text: each line's runes with any paste-chip
// sentinel expanded back to its full original (newlines restored), joined with '\n'.
// A chip-free buffer produces exactly the join of Lines.
func (m *MultiLineInput) GetText() string {
	parts := make([]string, len(m.Lines))
	for i, line := range m.Lines {
		parts[i] = m.chips.expand([]rune(line))
	}
	return strings.Join(parts, "\n")
}

// SetText replaces the buffer with text LITERALLY: newlines split into real, editable
// lines exactly as before. It deliberately does NOT collapse multi-line text into a
// chip — history recall of a hand-typed multi-line prompt must stay editable. Use
// SetTextChip to restore a remembered paste AS a chip. Any prior chips are dropped.
func (m *MultiLineInput) SetText(text string) {
	m.chips.reset()
	m.Lines = strings.Split(text, "\n")
	if len(m.Lines) == 0 {
		m.Lines = []string{""}
	}
	m.CursorY = len(m.Lines) - 1
	m.CursorX = len([]rune(m.Lines[m.CursorY]))
	m.ScrollY = 0
	m.selAnchorY = -1
}

// SetTextChip restores text as a single atomic paste chip when it contains a newline
// (so a host can recall a remembered paste as the compact "[pasted N lines]" token
// rather than spilling N editable lines); a newline-free value is set literally via
// SetText. This is the explicit, intent-revealing counterpart to handlePaste — the
// plain SetText never chip-ifies, so it is safe for ordinary recall.
func (m *MultiLineInput) SetTextChip(text string) {
	if !strings.Contains(text, "\n") {
		m.SetText(text)
		return
	}
	m.chips.reset()
	r := m.chips.add(text)
	m.Lines = []string{string(r)}
	m.CursorY = 0
	m.CursorX = 1
	m.ScrollY = 0
	m.selAnchorY = -1
}

func (m *MultiLineInput) Clear() {
	m.SetText("")
}

// pruneChips drops chip-store entries whose sentinel rune is no longer present in the
// buffer, called after edits that may have removed a chip so the store cannot grow
// without bound across paste→delete cycles.
func (m *MultiLineInput) pruneChips() {
	if len(m.chips.byRune) == 0 {
		return
	}
	present := make(map[rune]bool)
	for _, line := range m.Lines {
		for _, r := range line {
			if IsPasteChipRune(r) {
				present[r] = true
			}
		}
	}
	m.chips.keepOnly(present)
}

func (m *MultiLineInput) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := focusColors(component.Focused(), m.FG, m.BG, m.FocusFG, m.FocusBG)
	style := tui.Cell{FG: fg, BG: bg}
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: fg, BG: bg})
	// Reserve the right-hand column for the scrollbar, mirroring TextView, so the
	// text layout stays stable whether or not the bar is currently shown.
	textWidth := m.contentWidth(abs.W)
	// Wrap once and derive the caret row from the same layout, so the whole draw
	// is a single O(rows) pass. Previously this re-derived the layout up to four
	// times (twice through a redundant focused-only ensureScroll).
	rows := m.wrappedRows(textWidth)
	cursorRow, _ := m.cursorRowCol(rows, textWidth)
	m.ensureScrolledTo(abs.H, cursorRow, len(rows))
	for row := 0; row < abs.H; row++ {
		rowIndex := m.ScrollY + row
		if rowIndex < 0 || rowIndex >= len(rows) {
			continue
		}
		wrapped := rows[rowIndex]
		// Walk the row's runes tracking the display column, since a chip occupies more
		// than one column. Ordinary runes paint one cell (col == rune index, as before);
		// a chip paints its themed label across its display width.
		dx := 0
		for i := 0; i < len(wrapped.runes) && dx < textWidth; i++ {
			r := wrapped.runes[i]
			selected := m.isSelected(wrapped.line, wrapped.start+i)
			if IsPasteChipRune(r) {
				label := []rune{r}
				if text, ok := m.chips.text(r); ok {
					label = chipLabelFit(text, textWidth)
				}
				cfg, cbg := activeTheme.PasteChipFG, activeTheme.PasteChipBG
				if selected {
					cfg, cbg = activeTheme.TextSelectionFG, activeTheme.TextSelectionBG
				}
				for _, lr := range label {
					if dx >= textWidth {
						break
					}
					surface.SetCell(abs.X+dx, abs.Y+row, tui.Cell{Ch: lr, FG: cfg, BG: cbg})
					dx++
				}
				continue
			}
			cell := style
			cell.Ch = r
			if selected {
				cell.FG = activeTheme.TextSelectionFG
				cell.BG = activeTheme.TextSelectionBG
			}
			surface.SetCell(abs.X+dx, abs.Y+row, cell)
			dx++
		}
		// Fill the blank tail of a selected, spanned line so a block selection runs
		// to the right edge instead of stopping at each line's last character. The
		// membership is uniform past the row's content, so test the position just after
		// the last rune once and fill the remaining display columns.
		if m.isSelected(wrapped.line, wrapped.start+len(wrapped.runes)) {
			for ; dx < textWidth; dx++ {
				surface.SetCell(abs.X+dx, abs.Y+row, tui.Cell{
					Ch: ' ',
					FG: activeTheme.TextSelectionFG,
					BG: activeTheme.TextSelectionBG,
				})
			}
		}
	}
	// Only show the scrollbar when there is overflow; the reserved column is
	// otherwise left blank.
	if abs.W > 1 && len(rows) > abs.H {
		m.drawScrollbar(surface, abs, component.Focused(), len(rows), bg)
	}
}

// contentWidth is the width available for text: one column narrower than the
// widget so the scrollbar has a home, collapsing to the full width only when the
// widget is too thin (<=1) to host a bar.
func (m *MultiLineInput) contentWidth(width int) int {
	if width > 1 {
		return width - 1
	}
	return width
}

// drawScrollbar paints the right-hand track and thumb via the shared scrollbar
// helper, so the input matches TextView/tree/dropdown. Focused inputs use the
// accent colour; unfocused ones are dimmed.
func (m *MultiLineInput) drawScrollbar(surface Surface, abs Rect, focused bool, total int, bg tui.Color) {
	color := tui.ANSIColor(8)
	if focused {
		color = m.FocusFG
	}
	track := Rect{X: abs.Right(), Y: abs.Y, W: 1, H: abs.H}
	drawVScrollbar(surface, track, total, abs.H, m.ScrollY, color, bg, focused)
}

// cursorPos reports the absolute caret position for the hardware cursor.
func (m *MultiLineInput) cursorPos(component *VisualComponent) (int, int, bool) {
	abs := component.AbsoluteBounds()
	textWidth := m.contentWidth(abs.W)
	// Compute the caret row once and feed it (plus the total) into the scroll
	// clamp, instead of ensureScroll re-deriving cursorVisualPos and then this
	// method calling it a second time.
	cursorVisualY, cursorVisualX := m.cursorVisualPos(textWidth)
	m.ensureScrolledTo(abs.H, cursorVisualY, m.totalVisualRows(textWidth))
	cursorY := cursorVisualY - m.ScrollY
	if cursorY < 0 || cursorY >= abs.H {
		return 0, 0, false
	}
	if cursorVisualX >= textWidth {
		cursorVisualX = textWidth - 1
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
		m.moveUp(m.contentWidth(m.Component.Bounds.W))
		return true
	case tui.KeyDown:
		m.extendOrClear(event.Shift)
		m.moveDown(m.contentWidth(m.Component.Bounds.W))
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
	// Never let a paste-chip sentinel enter the buffer via a keystroke (it could only
	// arrive as a spoofed event); the marker range is reserved for collapsed pastes.
	if IsPasteChipRune(event.Rune) {
		return true
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

// selectionText returns the selected text with any paste-chip sentinel expanded to its
// full original (so copy/cut of a selection touching a chip yields the verbatim
// multi-line content, never the visible label).
func (m *MultiLineInput) selectionText() string {
	if !m.hasSelection() {
		return ""
	}
	y0, x0, y1, x1 := m.selectionOrdered()
	if y0 == y1 {
		runes := []rune(m.Lines[y0])
		x0, x1 = clampRange(len(runes), x0, x1)
		return m.chips.expand(runes[x0:x1])
	}
	var builder strings.Builder
	first := []rune(m.Lines[y0])
	x0 = clampCol(len(first), x0)
	builder.WriteString(m.chips.expand(first[x0:]))
	builder.WriteByte('\n')
	for line := y0 + 1; line < y1; line++ {
		builder.WriteString(m.chips.expand([]rune(m.Lines[line])))
		builder.WriteByte('\n')
	}
	last := []rune(m.Lines[y1])
	x1 = clampCol(len(last), x1)
	builder.WriteString(m.chips.expand(last[:x1]))
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
	m.pruneChips()
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
		// Forward-delete removes one rune; a chip is one rune, so a Delete with the
		// caret immediately before a chip removes the whole pasted block.
		line = append(line[:m.CursorX], line[m.CursorX+1:]...)
		m.Lines[m.CursorY] = string(line)
		m.pruneChips()
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
	max := m.totalVisualRows(m.contentWidth(m.Component.Bounds.W)) - 1
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

// handlePaste inserts pasted text at the cursor, replacing any selection. A paste
// CONTAINING a newline is collapsed into a single atomic "[pasted N lines]" chip (one
// sentinel rune holding the verbatim original) instead of spilling N editable lines; a
// newline-free paste is inserted literally rune-by-rune as before. CR is dropped
// (CRLF→LF) and other control runes are stripped on ingest, so GetText after a chip
// paste equals what a literal multi-line insert would have produced.
func (m *MultiLineInput) handlePaste(_ *VisualComponent, text string) bool {
	m.deleteSelection()
	clean, hasNewline := sanitizePaste(text)
	if hasNewline {
		m.insertRune(m.chips.add(clean))
		return true
	}
	for _, r := range clean {
		m.insertRune(r)
	}
	return true
}

func (m *MultiLineInput) backspace() {
	if m.CursorX > 0 {
		// Backspace removes one rune; a chip is one rune, so a Backspace with the
		// caret immediately after a chip removes the whole pasted block.
		line := []rune(m.Lines[m.CursorY])
		line = append(line[:m.CursorX-1], line[m.CursorX:]...)
		m.Lines[m.CursorY] = string(line)
		m.CursorX--
		m.pruneChips()
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
	rows := m.wrappedRows(width)
	cursorRow, cursorCol := m.cursorRowCol(rows, width)
	if cursorRow <= 0 {
		return
	}
	line, column := m.visualPosToCursorFromRows(rows, cursorRow-1, cursorCol, width)
	m.CursorY = line
	m.CursorX = column
}

func (m *MultiLineInput) moveDown(width int) {
	rows := m.wrappedRows(width)
	cursorRow, cursorCol := m.cursorRowCol(rows, width)
	if cursorRow >= len(rows)-1 {
		return
	}
	line, column := m.visualPosToCursorFromRows(rows, cursorRow+1, cursorCol, width)
	m.CursorY = line
	m.CursorX = column
}

// ensureScrolledTo clamps ScrollY so the caret (at cursorRow of total visual
// rows) stays inside the viewport. It is the allocation-free core of the old
// ensureScroll: callers pass the already-computed caret row and row count so the
// layout is not re-derived inside.
func (m *MultiLineInput) ensureScrolledTo(height int, cursorRow int, total int) {
	if height < 1 {
		height = 1
	}
	if cursorRow < m.ScrollY {
		m.ScrollY = cursorRow
	}
	if cursorRow >= m.ScrollY+height {
		m.ScrollY = cursorRow - height + 1
	}
	if m.ScrollY < 0 {
		m.ScrollY = 0
	}
	max := total - height
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
		for _, span := range m.lineSpans(runes, width) {
			rows = append(rows, wrappedLineRow{
				line:  lineIndex,
				start: span.start,
				runes: runes[span.start:span.end],
			})
		}
	}
	if len(rows) == 0 {
		rows = append(rows, wrappedLineRow{line: 0, start: 0, runes: []rune{}})
	}
	return rows
}

// lineSpans splits one logical line into the contiguous [start, end) rune spans
// that become its visual rows at the given width, honouring WordWrap. Both modes
// preserve every rune (the spans tile the line with no gaps), so a span's start
// remains a valid selection/cursor offset. An empty line yields one empty span.
//
// A line with no paste chip takes the original char-/word-wrap path unchanged (every
// rune is one display column). A line containing a chip routes through chipAwareSpans,
// which measures display width per cell so a multi-column chip wraps as one unbreakable
// token without splitting.
func (m *MultiLineInput) lineSpans(runes []rune, width int) []runeSpan {
	if width < 1 {
		width = 1
	}
	if len(runes) == 0 {
		return []runeSpan{{start: 0, end: 0}}
	}
	if lineHasChip(runes) {
		return m.chipAwareSpans(runes, width)
	}
	if m.WordWrap {
		return wordWrapSpans(runes, width)
	}
	spans := make([]runeSpan, 0, (len(runes)+width-1)/width)
	for start := 0; start < len(runes); start += width {
		end := start + width
		if end > len(runes) {
			end = len(runes)
		}
		spans = append(spans, runeSpan{start: start, end: end})
	}
	return spans
}

// lineHasChip reports whether a line contains any paste-chip sentinel rune.
func lineHasChip(runes []rune) bool {
	for _, r := range runes {
		if IsPasteChipRune(r) {
			return true
		}
	}
	return false
}

// cellWidth is the display width (columns) of one rune in this line: 1 for ordinary
// runes (the widget lays text out one column per rune, as it always has) and the
// width-fitted label width for a chip sentinel. An orphan sentinel with no store entry
// counts as one column.
func (m *MultiLineInput) cellWidth(r rune, width int) int {
	if !IsPasteChipRune(r) {
		return 1
	}
	if text, ok := m.chips.text(r); ok {
		if w := tui.StringWidth(string(chipLabelFit(text, width))); w >= 1 {
			return w
		}
	}
	return 1
}

// runeColWidth sums the display widths of runes[from:to], i.e. the column offset of
// rune index `to` within its visual row that starts at `from`. For a chip-free range
// this is just to-from.
func (m *MultiLineInput) runeColWidth(runes []rune, from, to, width int) int {
	if to > len(runes) {
		to = len(runes)
	}
	col := 0
	for k := from; k < to; k++ {
		col += m.cellWidth(runes[k], width)
	}
	return col
}

// chipAwareSpans wraps a line whose cells have varying display width, packing runes
// into rows no wider than `width` display columns and never splitting a chip. In
// WordWrap mode it prefers to break just after the last whitespace in the row, matching
// the chip-free word-wrap behaviour. A single cell wider than the row still consumes
// its own row (the chip label is truncated to fit at draw time).
func (m *MultiLineInput) chipAwareSpans(runes []rune, width int) []runeSpan {
	n := len(runes)
	spans := make([]runeSpan, 0, n)
	start := 0
	for start < n {
		colW, end := 0, start
		for end < n {
			w := m.cellWidth(runes[end], width)
			if end > start && colW+w > width {
				break
			}
			colW += w
			end++
		}
		if end == start {
			end = start + 1
		}
		if m.WordWrap && end < n {
			breakAt := end
			for i := end - 1; i > start; i-- {
				if runes[i] == ' ' || runes[i] == '\t' {
					breakAt = i + 1
					break
				}
			}
			end = breakAt
		}
		spans = append(spans, runeSpan{start: start, end: end})
		start = end
	}
	return spans
}

// wordWrapSpans breaks runes into contiguous spans no wider than width, breaking
// after whitespace so words stay intact and hard-splitting any single word that
// is itself wider than width. The break whitespace stays at the end of its span,
// keeping the spans contiguous (and thus offset-preserving) rather than dropping
// runes the way a Fields-based wrapper would.
func wordWrapSpans(runes []rune, width int) []runeSpan {
	if width < 1 {
		width = 1
	}
	n := len(runes)
	if n == 0 {
		return []runeSpan{{start: 0, end: 0}}
	}
	var spans []runeSpan
	start := 0
	for start < n {
		if n-start <= width {
			spans = append(spans, runeSpan{start: start, end: n})
			break
		}
		end := start + width
		// Prefer breaking just after the last whitespace within the row so the
		// following word is not split; fall back to a hard cut when there is none.
		breakAt := end
		for i := end - 1; i > start; i-- {
			if runes[i] == ' ' || runes[i] == '\t' {
				breakAt = i + 1
				break
			}
		}
		spans = append(spans, runeSpan{start: start, end: breakAt})
		start = breakAt
	}
	return spans
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
	runes := []rune(line)
	// Word wrap and any line carrying a chip need the real (display-width-aware) span
	// layout; plain char-wrapped text keeps the O(1) arithmetic.
	if m.WordWrap || lineHasChip(runes) {
		return len(m.lineSpans(runes, width))
	}
	length := len(runes)
	if length == 0 {
		return 1
	}
	rows := (length + width - 1) / width
	if rows < 1 {
		rows = 1
	}
	return rows
}

// caretSpanOffsetCol locates the caret within its logical line as a (visual-row
// offset, display column) pair using the actual wrap spans. The column is the summed
// display width of the runes before the caret on its row, so a chip earlier on the row
// contributes its full label width. For a chip-free row this reduces to cursorX minus
// the row start. It is the analogue of the character arithmetic in
// cursorVisualPos/cursorRowCol; callers route through it for word wrap and for any line
// containing a chip so they stay in agreement.
func (m *MultiLineInput) caretSpanOffsetCol(lineIndex, cursorX, width int) (int, int) {
	runes := []rune(m.Lines[lineIndex])
	spans := m.lineSpans(runes, width)
	for i, span := range spans {
		if cursorX < span.end {
			return i, m.runeColWidth(runes, span.start, cursorX, width)
		}
	}
	last := len(spans) - 1
	return last, m.runeColWidth(runes, spans[last].start, cursorX, width)
}

// CaretRowInLine reports the caret's visual-row offset within its own logical
// line and the number of visual rows that line occupies, using the widget's
// actual wrap layout (honouring WordWrap) at its current rendered content width.
// Width is taken from the widget's own bounds, so it always matches what draw
// renders. It lets a host decide whether Up/Down should move the caret within a
// wrapped line or fall through (e.g. recall history) when the caret is on the
// first/last visual row — without re-deriving wrap geometry with char-wrap
// assumptions. (gogent#270)
func (m *MultiLineInput) CaretRowInLine() (rowInLine int, rowsInLine int) {
	width := 1
	if root := m.Root(); root != nil {
		width = m.contentWidth(root.AbsoluteBounds().W)
	}
	if width < 1 {
		width = 1
	}
	if len(m.Lines) == 0 {
		return 0, 1
	}
	y := m.CursorY
	if y < 0 {
		y = 0
	}
	if y >= len(m.Lines) {
		y = len(m.Lines) - 1
	}
	rowsInLine = m.rowsForLine(m.Lines[y], width)
	x := m.CursorX
	if x < 0 {
		x = 0
	}
	if n := len([]rune(m.Lines[y])); x > n {
		x = n
	}
	if m.WordWrap || lineHasChip([]rune(m.Lines[y])) {
		rowInLine, _ = m.caretSpanOffsetCol(y, x, width)
	} else {
		rowInLine = x / width
	}
	if rowInLine < 0 {
		rowInLine = 0
	}
	if rowInLine >= rowsInLine {
		rowInLine = rowsInLine - 1
	}
	return rowInLine, rowsInLine
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
	if m.WordWrap || lineHasChip(lineRunes) {
		offset, col := m.caretSpanOffsetCol(m.CursorY, m.CursorX, width)
		return row + offset, col
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

// cursorRowCol derives the (visual row, column) of the caret from an already
// wrapped layout, clamping CursorY/CursorX into bounds as a side effect (the
// same clamping cursorVisualPos performs). Use it on paths that have already
// called wrappedRows, to avoid re-deriving the layout.
func (m *MultiLineInput) cursorRowCol(rows []wrappedLineRow, width int) (int, int) {
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
	// Count wrapped rows belonging to lines strictly above the cursor line.
	row := 0
	for _, w := range rows {
		if w.line < m.CursorY {
			row++
		}
	}
	if m.WordWrap || lineHasChip(lineRunes) {
		offset, col := m.caretSpanOffsetCol(m.CursorY, m.CursorX, width)
		return row + offset, col
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

// visualPosToCursorFromRows maps a (visual row, display column) back to a (line,
// cursor) using an already-wrapped layout. It is the allocation-free form of the old
// visualPosToCursor, which re-wrapped the whole buffer on every call. The column is a
// display column (chip-aware): a click that lands within a chip snaps to the chip's
// near edge — the caret is placed before the chip for the left half, after it for the
// right half, and can never be placed inside it. For a chip-free row this reduces to
// cursor = start + col exactly as before.
func (m *MultiLineInput) visualPosToCursorFromRows(rows []wrappedLineRow, row, col, width int) (int, int) {
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
	acc := 0
	for i := 0; i < len(target.runes); i++ {
		w := m.cellWidth(target.runes[i], width)
		if col < acc+w {
			cursor := target.start + i
			if IsPasteChipRune(target.runes[i]) && col >= acc+(w+1)/2 {
				cursor++
			}
			return line, cursor
		}
		acc += w
	}
	// Past the row's last cell. For a chip-free row, start+col reproduces the historical
	// behaviour exactly (clamped to the line). For a chip row, display columns and rune
	// offsets diverge, so clamp to the row's end instead of overshooting.
	cursor := target.start + col
	if lineHasChip(target.runes) {
		if max := target.start + len(target.runes); cursor > max {
			cursor = max
		}
	}
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
	textWidth := m.contentWidth(component.Bounds.W)
	if !event.Down {
		m.selecting = false
		m.draggingThumb = false
		return true
	}
	// A scrollbar interaction takes precedence over caret placement so dragging the
	// thumb (or clicking the track/arrows) never moves the cursor or starts a text
	// selection. Mirrors TextView, using the shared scrollbarOffsetForY mapping.
	track := Rect{X: abs.Right(), Y: abs.Y, W: 1, H: abs.H}
	total := m.totalVisualRows(textWidth)
	if m.draggingThumb {
		if offset, ok := scrollbarOffsetForY(track, total, abs.H, m.ScrollY, event.Y); ok {
			m.ScrollY = offset
		}
		return true
	}
	if abs.W > 1 && total > abs.H && event.X == abs.Right() {
		if offset, ok := scrollbarOffsetForY(track, total, abs.H, m.ScrollY, event.Y); ok {
			m.ScrollY = offset
			m.draggingThumb = true
			return true
		}
	}
	// Wrap once and reuse it for the hit mapping (a drag fires many motion
	// events; this avoids re-wrapping the whole buffer on each one).
	rows := m.wrappedRows(textWidth)
	row := m.ScrollY + (event.Y - abs.Y)
	col := event.X - abs.X
	if col < 0 {
		col = 0
	}
	if row < 0 {
		row = 0
	}
	line, cursor := m.visualPosToCursorFromRows(rows, row, col, textWidth)
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
