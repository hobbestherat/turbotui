package tv

import (
	"strings"
	"unicode"

	tui "github.com/hobbestherat/turbotui"
)

type TextBox struct {
	Component *VisualComponent
	// Text holds the box's runes. NOTE: it may contain an opaque paste-chip sentinel
	// rune (see paste_chip.go) standing in for a collapsed multi-line paste — use
	// GetText() for the verbatim text and IsPasteChipRune to detect a chip when scanning
	// Text directly. Cursor is a rune offset, so a chip (one rune) is one caret stop.
	Text     []rune
	Cursor   int
	ScrollX  int
	FG       tui.Color
	BG       tui.Color
	FocusFG  tui.Color
	FocusBG  tui.Color
	OnSubmit func()

	selAnchor int // start of the selection; -1 when there is no selection
	selecting bool
	// chips holds the verbatim original text of every collapsed multi-line paste,
	// keyed by the sentinel rune that stands for it in Text.
	chips chipStore
}

func NewTextBox(text string, bounds Rect) *TextBox {
	box := &TextBox{
		Text:      []rune(text),
		Cursor:    len([]rune(text)),
		FG:        activeTheme.InputFG,
		BG:        activeTheme.InputBG,
		FocusFG:   activeTheme.InputFocusFG,
		FocusBG:   activeTheme.InputFocusBG,
		selAnchor: -1,
	}
	box.Component = NewComponent(bounds)
	box.Component.Focusable = true
	box.Component.DrawFn = box.draw
	box.Component.OnTypeFn = box.handleType
	box.Component.OnPasteFn = box.handlePaste
	box.Component.OnClickFn = box.handleClick
	box.Component.CursorFn = box.cursorPos
	box.Component.CopyFn = box.copySelection
	box.Component.CutFn = box.cutSelection
	return box
}

func (t *TextBox) Root() *VisualComponent {
	return t.Component
}

// SetText replaces the box content LITERALLY and drops any prior chips. It never
// collapses content into a chip; use SetTextChip to restore a remembered multi-line
// paste as a chip.
func (t *TextBox) SetText(value string) {
	t.chips.reset()
	t.Text = []rune(value)
	t.Cursor = len(t.Text)
	t.selAnchor = -1
}

// SetTextChip restores text as a single atomic paste chip when it contains a newline
// (the single-line parity of MultiLineInput.SetTextChip); a newline-free value is set
// literally via SetText.
func (t *TextBox) SetTextChip(text string) {
	if !strings.Contains(text, "\n") {
		t.SetText(text)
		return
	}
	t.chips.reset()
	r := t.chips.add(text)
	t.Text = []rune{r}
	t.Cursor = 1
	t.ScrollX = 0
	t.selAnchor = -1
}

// GetText returns the verbatim content: the runes with any paste-chip sentinel expanded
// back to its full original (newlines restored). A chip-free box returns its runes as-is.
func (t *TextBox) GetText() string {
	return t.chips.expand(t.Text)
}

// pruneChips drops chip-store entries whose sentinel rune is no longer present in Text,
// called after edits that may have removed a chip.
func (t *TextBox) pruneChips() {
	if len(t.chips.byRune) == 0 {
		return
	}
	present := make(map[rune]bool)
	for _, r := range t.Text {
		if IsPasteChipRune(r) {
			present[r] = true
		}
	}
	t.chips.keepOnly(present)
}

// cellWidth is the display width of one rune: 1 for ordinary runes and the width-fitted
// label width for a chip sentinel.
func (t *TextBox) cellWidth(r rune, width int) int {
	if !IsPasteChipRune(r) {
		return 1
	}
	if text, ok := t.chips.text(r); ok {
		if w := tui.StringWidth(string(chipLabelFit(text, width))); w >= 1 {
			return w
		}
	}
	return 1
}

// dispWidth sums the display widths of Text[from:to].
func (t *TextBox) dispWidth(from, to, width int) int {
	if to > len(t.Text) {
		to = len(t.Text)
	}
	w := 0
	for k := from; k < to; k++ {
		w += t.cellWidth(t.Text[k], width)
	}
	return w
}

func (t *TextBox) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := focusColors(component.Focused(), t.FG, t.BG, t.FocusFG, t.FocusBG)
	textStyle := tui.Cell{FG: fg, BG: bg}
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: fg, BG: bg})
	visibleStart := t.ScrollX
	if visibleStart < 0 {
		visibleStart = 0
	}
	selLo, selHi := -1, -1
	if t.hasSelection() {
		selLo, selHi = t.selRange()
	}
	dx := 0
	for index := visibleStart; index < len(t.Text) && dx < abs.W; index++ {
		r := t.Text[index]
		selected := index >= selLo && index < selHi
		if IsPasteChipRune(r) {
			label := []rune{r}
			if text, ok := t.chips.text(r); ok {
				label = chipLabelFit(text, abs.W)
			}
			cfg, cbg := activeTheme.PasteChipFG, activeTheme.PasteChipBG
			if selected {
				cfg, cbg = activeTheme.TextSelectionFG, activeTheme.TextSelectionBG
			}
			for _, lr := range label {
				if dx >= abs.W {
					break
				}
				surface.SetCell(abs.X+dx, abs.Y, tui.Cell{Ch: lr, FG: cfg, BG: cbg})
				dx++
			}
			continue
		}
		cell := textStyle
		cell.Ch = r
		if selected {
			cell.FG = activeTheme.TextSelectionFG
			cell.BG = activeTheme.TextSelectionBG
		}
		surface.SetCell(abs.X+dx, abs.Y, cell)
		dx++
	}
	if component.Focused() {
		t.ensureCursorVisible(abs.W)
	}
}

// cursorPos reports the absolute caret position for the hardware cursor.
func (t *TextBox) cursorPos(component *VisualComponent) (int, int, bool) {
	abs := component.AbsoluteBounds()
	t.ensureCursorVisible(abs.W)
	cursorX := abs.X + t.dispWidth(t.ScrollX, t.Cursor, abs.W)
	if !abs.Contains(cursorX, abs.Y) {
		return 0, 0, false
	}
	return cursorX, abs.Y, true
}

func (t *TextBox) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	// Ctrl-modified editing shortcuts: select-all, word-wise jump and word-wise
	// delete. Anything else with Ctrl falls through to the regular handling below
	// (so Ctrl+Enter still submits and other Ctrl runes are rejected as before).
	if event.Ctrl && t.handleCtrlShortcut(event) {
		return true
	}
	switch event.Key {
	case tui.KeyEnter:
		if t.OnSubmit == nil {
			return false
		}
		t.OnSubmit()
		return true
	case tui.KeyBackspace:
		if t.deleteSelection() {
			return true
		}
		if t.Cursor > 0 && len(t.Text) > 0 {
			// One rune removed; a chip is one rune, so Backspace after a chip removes
			// the whole pasted block.
			t.Text = append(t.Text[:t.Cursor-1], t.Text[t.Cursor:]...)
			t.Cursor--
			t.pruneChips()
		}
		return true
	case tui.KeyDelete:
		if t.deleteSelection() {
			return true
		}
		if t.Cursor < len(t.Text) {
			t.Text = append(t.Text[:t.Cursor], t.Text[t.Cursor+1:]...)
			t.pruneChips()
		}
		return true
	case tui.KeyLeft:
		t.moveCursor(t.Cursor-1, event.Shift)
		return true
	case tui.KeyRight:
		t.moveCursor(t.Cursor+1, event.Shift)
		return true
	case tui.KeyHome:
		t.moveCursor(0, event.Shift)
		return true
	case tui.KeyEnd:
		t.moveCursor(len(t.Text), event.Shift)
		return true
	}
	if event.Key != tui.KeyRune || event.Ctrl {
		return false
	}
	// Never let a paste-chip sentinel enter via a keystroke; the marker range is
	// reserved for collapsed pastes.
	if IsPasteChipRune(event.Rune) {
		return true
	}
	t.deleteSelection()
	t.insertRune(event.Rune)
	return true
}

// handleCtrlShortcut applies a Ctrl-modified editing shortcut and reports whether
// it recognised the event. It never claims keys it does not handle, so unhandled
// Ctrl combos keep their original behaviour (submit on Ctrl+Enter, rejection of
// other Ctrl runes).
func (t *TextBox) handleCtrlShortcut(event tui.TypeEvent) bool {
	switch event.Key {
	case tui.KeyRune:
		if unicode.ToLower(event.Rune) == 'a' {
			// Select all: anchor at the start, caret at the end.
			t.selAnchor = 0
			t.Cursor = len(t.Text)
			return true
		}
		return false
	case tui.KeyLeft:
		t.moveCursor(wordBoundaryLeft(t.Text, t.Cursor), event.Shift)
		return true
	case tui.KeyRight:
		t.moveCursor(wordBoundaryRight(t.Text, t.Cursor), event.Shift)
		return true
	case tui.KeyBackspace:
		if t.deleteSelection() {
			return true
		}
		start := wordBoundaryLeft(t.Text, t.Cursor)
		if start < t.Cursor {
			t.Text = append(t.Text[:start], t.Text[t.Cursor:]...)
			t.Cursor = start
			t.pruneChips()
		}
		return true
	case tui.KeyDelete:
		if t.deleteSelection() {
			return true
		}
		end := wordBoundaryRight(t.Text, t.Cursor)
		if end > t.Cursor {
			t.Text = append(t.Text[:t.Cursor], t.Text[end:]...)
			t.pruneChips()
		}
		return true
	}
	return false
}

// charClass buckets a non-space rune for word-boundary motion: word characters
// (letters, digits, underscore) form one class, every other non-space rune
// (punctuation, symbols) another, so the caret jumps over a run of the same kind.
func charClass(r rune) int {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
		return 0
	}
	return 1
}

// wordBoundaryLeft returns the start of the word at or before pos: it skips the
// run of spaces to the left, then the run of same-class characters, landing at a
// word boundary (or 0).
func wordBoundaryLeft(runes []rune, pos int) int {
	for pos > 0 && unicode.IsSpace(runes[pos-1]) {
		pos--
	}
	if pos == 0 {
		return 0
	}
	// A paste chip is one indivisible word-unit: stop right before it (and never let a
	// run of ordinary characters absorb it).
	if IsPasteChipRune(runes[pos-1]) {
		return pos - 1
	}
	class := charClass(runes[pos-1])
	for pos > 0 && !unicode.IsSpace(runes[pos-1]) && !IsPasteChipRune(runes[pos-1]) && charClass(runes[pos-1]) == class {
		pos--
	}
	return pos
}

// wordBoundaryRight returns the start of the word at or after pos: it skips the
// run of spaces to the right, then the run of same-class characters, landing at
// the next word boundary (or len(runes)).
func wordBoundaryRight(runes []rune, pos int) int {
	n := len(runes)
	for pos < n && unicode.IsSpace(runes[pos]) {
		pos++
	}
	if pos == n {
		return n
	}
	// A paste chip is one indivisible word-unit: step just past it.
	if IsPasteChipRune(runes[pos]) {
		return pos + 1
	}
	class := charClass(runes[pos])
	for pos < n && !unicode.IsSpace(runes[pos]) && !IsPasteChipRune(runes[pos]) && charClass(runes[pos]) == class {
		pos++
	}
	return pos
}

// moveCursor moves the caret to pos. When extend is true the selection anchor is
// kept (started if needed); otherwise the selection is cleared.
func (t *TextBox) moveCursor(pos int, extend bool) {
	if pos < 0 {
		pos = 0
	}
	if pos > len(t.Text) {
		pos = len(t.Text)
	}
	if extend {
		if t.selAnchor < 0 {
			t.selAnchor = t.Cursor
		}
	} else {
		t.selAnchor = -1
	}
	t.Cursor = pos
}

func (t *TextBox) hasSelection() bool {
	return t.selAnchor >= 0 && t.selAnchor != t.Cursor
}

func (t *TextBox) selRange() (int, int) {
	lo, hi := t.selAnchor, t.Cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi > len(t.Text) {
		hi = len(t.Text)
	}
	return lo, hi
}

func (t *TextBox) deleteSelection() bool {
	if !t.hasSelection() {
		return false
	}
	lo, hi := t.selRange()
	t.Text = append(t.Text[:lo], t.Text[hi:]...)
	t.Cursor = lo
	t.selAnchor = -1
	t.pruneChips()
	return true
}

// copySelection returns the selected text with any paste-chip expanded to its full
// original, so copy yields the verbatim multi-line content rather than the label.
func (t *TextBox) copySelection(_ *VisualComponent) (string, bool) {
	if !t.hasSelection() {
		return "", false
	}
	lo, hi := t.selRange()
	return t.chips.expand(t.Text[lo:hi]), true
}

// cutSelection is the CutFn: it copies the current selection to the clipboard
// (via the desktop) and removes it from the text. With no selection it reports
// nothing to cut so Ctrl+X falls through, mirroring copySelection.
func (t *TextBox) cutSelection(_ *VisualComponent) (string, bool) {
	if !t.hasSelection() {
		return "", false
	}
	lo, hi := t.selRange()
	text := t.chips.expand(t.Text[lo:hi])
	t.Text = append(t.Text[:lo], t.Text[hi:]...)
	t.Cursor = lo
	t.selAnchor = -1
	t.pruneChips()
	return text, true
}

func (t *TextBox) insertRune(value rune) {
	t.Text = append(t.Text, 0)
	copy(t.Text[t.Cursor+1:], t.Text[t.Cursor:])
	t.Text[t.Cursor] = value
	t.Cursor++
}

// handlePaste inserts pasted text at the cursor, replacing any selection. For parity
// with MultiLineInput a paste CONTAINING a newline is collapsed into a single atomic
// "[pasted N lines]" chip (one sentinel rune holding the verbatim original) rather than
// stripping the newlines; a newline-free paste is inserted literally. CR is dropped and
// other control runes are stripped on ingest, so GetText restores the verbatim text.
func (t *TextBox) handlePaste(_ *VisualComponent, text string) bool {
	t.deleteSelection()
	clean, hasNewline := sanitizePaste(text)
	if hasNewline {
		t.insertRune(t.chips.add(clean))
		return true
	}
	for _, r := range clean {
		t.insertRune(r)
	}
	return true
}

func (t *TextBox) ensureCursorVisible(width int) {
	if width < 1 {
		width = 1
	}
	if t.Cursor < t.ScrollX {
		t.ScrollX = t.Cursor
	}
	// Advance the left edge until the caret's display offset fits within the box. For
	// chip-free text dispWidth == Cursor-ScrollX, so this reduces to the prior
	// ScrollX = Cursor-width+1.
	for t.ScrollX < t.Cursor && t.dispWidth(t.ScrollX, t.Cursor, width) > width-1 {
		t.ScrollX++
	}
	if t.ScrollX < 0 {
		t.ScrollX = 0
	}
}

// columnToIndex maps a display column (relative to the box's left edge) to a rune
// offset in Text, accounting for chip widths. A click that lands within a chip snaps to
// the chip's near edge (before for the left half, after for the right half) so the caret
// is never placed inside it. For chip-free text this reduces to ScrollX+column.
func (t *TextBox) columnToIndex(column, width int) int {
	acc := 0
	for i := t.ScrollX; i < len(t.Text); i++ {
		w := t.cellWidth(t.Text[i], width)
		if column < acc+w {
			if IsPasteChipRune(t.Text[i]) && column >= acc+(w+1)/2 {
				return i + 1
			}
			return i
		}
		acc += w
	}
	return len(t.Text)
}

func (t *TextBox) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	abs := component.AbsoluteBounds()
	if !event.Down {
		t.selecting = false
		return true
	}
	column := event.X - abs.X
	if column < 0 {
		column = 0
	}
	pos := t.columnToIndex(column, abs.W)
	if !t.selecting {
		if !abs.Contains(event.X, event.Y) {
			return false
		}
		t.Cursor = pos
		t.selAnchor = pos
		t.selecting = true
		return true
	}
	// Drag motion: extend the selection to the pointer.
	t.Cursor = pos
	return true
}
