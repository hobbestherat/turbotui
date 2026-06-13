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
}

func NewTextBox(text string, bounds Rect) *TextBox {
	box := &TextBox{
		Text:    []rune(text),
		Cursor:  len([]rune(text)),
		FG:      DefaultTheme.InputFG,
		BG:      DefaultTheme.InputBG,
		FocusFG: DefaultTheme.InputFocusFG,
		FocusBG: DefaultTheme.InputFocusBG,
	}
	box.Component = NewComponent(bounds)
	box.Component.Focusable = true
	box.Component.DrawFn = box.draw
	box.Component.OnTypeFn = box.handleType
	box.Component.OnPasteFn = box.handlePaste
	box.Component.OnClickFn = box.handleClick
	box.Component.CursorFn = box.cursorPos
	return box
}

func (t *TextBox) Root() *VisualComponent {
	return t.Component
}

func (t *TextBox) SetText(value string) {
	t.Text = []rune(value)
	t.Cursor = len(t.Text)
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
	for offset := 0; offset < abs.W; offset++ {
		index := visibleStart + offset
		if index < 0 || index >= len(t.Text) {
			break
		}
		textStyle.Ch = t.Text[index]
		surface.SetCell(abs.X+offset, abs.Y, textStyle)
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
	if event.Key == tui.KeyEnter {
		if t.OnSubmit == nil {
			return false
		}
		t.OnSubmit()
		return true
	}
	if event.Key == tui.KeyBackspace {
		if t.Cursor <= 0 || len(t.Text) == 0 {
			return true
		}
		t.Text = append(t.Text[:t.Cursor-1], t.Text[t.Cursor:]...)
		t.Cursor--
		return true
	}
	if event.Key == tui.KeyLeft {
		if t.Cursor > 0 {
			t.Cursor--
		}
		return true
	}
	if event.Key == tui.KeyRight {
		if t.Cursor < len(t.Text) {
			t.Cursor++
		}
		return true
	}
	if event.Key != tui.KeyRune || event.Ctrl {
		return false
	}
	t.insertRune(event.Rune)
	return true
}

func (t *TextBox) insertRune(value rune) {
	t.Text = append(t.Text, 0)
	copy(t.Text[t.Cursor+1:], t.Text[t.Cursor:])
	t.Text[t.Cursor] = value
	t.Cursor++
}

// handlePaste inserts pasted text at the cursor. Newlines and other control
// characters are dropped since a TextBox is single-line.
func (t *TextBox) handlePaste(_ *VisualComponent, text string) bool {
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
	if !event.Down {
		return true
	}
	abs := component.AbsoluteBounds()
	if !abs.Contains(event.X, event.Y) {
		return false
	}
	clickColumn := event.X - abs.X
	if clickColumn < 0 {
		clickColumn = 0
	}
	t.Cursor = t.ScrollX + clickColumn
	if t.Cursor > len(t.Text) {
		t.Cursor = len(t.Text)
	}
	return true
}
