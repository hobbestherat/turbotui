package tv

import tui "github.com/hobbestherat/turbotui"

type TextBox struct {
	Component *VisualComponent
	Text      []rune
	Cursor    int
	ScrollX   int
	FG        tui.Color
	BG        tui.Color
	FocusFG   tui.Color
	FocusBG   tui.Color
	OnSubmit  func()

	selAnchor int // start of the selection; -1 when there is no selection
	selecting bool
}

func NewTextBox(text string, bounds Rect) *TextBox {
	box := &TextBox{
		Text:      []rune(text),
		Cursor:    len([]rune(text)),
		FG:        DefaultTheme.InputFG,
		BG:        DefaultTheme.InputBG,
		FocusFG:   DefaultTheme.InputFocusFG,
		FocusBG:   DefaultTheme.InputFocusBG,
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
	return box
}

func (t *TextBox) Root() *VisualComponent {
	return t.Component
}

func (t *TextBox) SetText(value string) {
	t.Text = []rune(value)
	t.Cursor = len(t.Text)
	t.selAnchor = -1
}

func (t *TextBox) GetText() string {
	return string(t.Text)
}

func (t *TextBox) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	fg, bg := inputColors(component.HasFocus, t.FG, t.BG, t.FocusFG, t.FocusBG)
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
	for offset := 0; offset < abs.W; offset++ {
		index := visibleStart + offset
		if index < 0 || index >= len(t.Text) {
			break
		}
		cell := textStyle
		cell.Ch = t.Text[index]
		if index >= selLo && index < selHi {
			cell.FG = DefaultTheme.SelectionFG
			cell.BG = DefaultTheme.SelectionBG
		}
		surface.SetCell(abs.X+offset, abs.Y, cell)
	}
	if component.HasFocus {
		t.ensureCursorVisible(abs.W)
	}
}

// cursorPos reports the absolute caret position for the hardware cursor.
func (t *TextBox) cursorPos(component *VisualComponent) (int, int, bool) {
	abs := component.AbsoluteBounds()
	t.ensureCursorVisible(abs.W)
	cursorX := abs.X + (t.Cursor - t.ScrollX)
	if !abs.Contains(cursorX, abs.Y) {
		return 0, 0, false
	}
	return cursorX, abs.Y, true
}

func (t *TextBox) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
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
			t.Text = append(t.Text[:t.Cursor-1], t.Text[t.Cursor:]...)
			t.Cursor--
		}
		return true
	case tui.KeyDelete:
		if t.deleteSelection() {
			return true
		}
		if t.Cursor < len(t.Text) {
			t.Text = append(t.Text[:t.Cursor], t.Text[t.Cursor+1:]...)
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
	t.deleteSelection()
	t.insertRune(event.Rune)
	return true
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
	return true
}

func (t *TextBox) copySelection(_ *VisualComponent) (string, bool) {
	if !t.hasSelection() {
		return "", false
	}
	lo, hi := t.selRange()
	return string(t.Text[lo:hi]), true
}

func (t *TextBox) insertRune(value rune) {
	t.Text = append(t.Text, 0)
	copy(t.Text[t.Cursor+1:], t.Text[t.Cursor:])
	t.Text[t.Cursor] = value
	t.Cursor++
}

// handlePaste inserts pasted text at the cursor, replacing any selection.
// Newlines and other control characters are dropped since a TextBox is
// single-line.
func (t *TextBox) handlePaste(_ *VisualComponent, text string) bool {
	t.deleteSelection()
	for _, r := range text {
		if r < 0x20 {
			continue
		}
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
	if t.Cursor >= t.ScrollX+width {
		t.ScrollX = t.Cursor - width + 1
	}
	if t.ScrollX < 0 {
		t.ScrollX = 0
	}
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
	pos := t.ScrollX + column
	if pos > len(t.Text) {
		pos = len(t.Text)
	}
	if pos < 0 {
		pos = 0
	}
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
