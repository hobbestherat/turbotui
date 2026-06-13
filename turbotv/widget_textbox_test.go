package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestTextBoxSubmitOnEnter(t *testing.T) {
	box := NewTextBox("abc", Rect{X: 0, Y: 0, W: 8, H: 1})
	submits := 0
	box.OnSubmit = func() {
		submits++
	}
	if !box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyEnter}) {
		t.Fatalf("expected Enter to be consumed when OnSubmit is set")
	}
	if submits != 1 {
		t.Fatalf("expected submit callback once, got %d", submits)
	}
}

func TestTextBoxEnterBubblesWithoutSubmit(t *testing.T) {
	box := NewTextBox("abc", Rect{X: 0, Y: 0, W: 8, H: 1})
	if box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyEnter}) {
		t.Fatalf("expected Enter to bubble when OnSubmit is nil")
	}
}

func TestTextBoxPasteStripsNewlines(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "ab\ncd\r\nef")
	if string(box.Text) != "abcdef" {
		t.Fatalf("expected newlines stripped, got %q", string(box.Text))
	}
}

func TestTextBoxShiftSelectAndCopy(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.Cursor = 0
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	if !box.hasSelection() {
		t.Fatalf("expected a selection after shift+right")
	}
	text, ok := box.copySelection(box.Component)
	if !ok || text != "he" {
		t.Fatalf("expected copy 'he', got %q ok=%v", text, ok)
	}
}

func TestTextBoxTypingReplacesSelection(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.Cursor = 0
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})
	if string(box.Text) != "Xllo" {
		t.Fatalf("expected selection replaced, got %q", string(box.Text))
	}
}

func TestTextBoxBackspaceDeletesSelection(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.Cursor = 1
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyBackspace})
	if string(box.Text) != "hlo" {
		t.Fatalf("expected 'hlo' after deleting selection, got %q", string(box.Text))
	}
}
