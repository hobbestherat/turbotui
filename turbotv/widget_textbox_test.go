package tv

import (
	"bytes"
	"encoding/base64"
	"strings"
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

func TestTextBoxCtrlASelectAll(t *testing.T) {
	box := NewTextBox("hello world", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.Cursor = 0
	if !box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Ctrl: true}) {
		t.Fatalf("expected Ctrl+A to be consumed")
	}
	if !box.hasSelection() {
		t.Fatalf("expected a selection after Ctrl+A")
	}
	text, ok := box.copySelection(box.Component)
	if !ok || text != "hello world" {
		t.Fatalf("expected select-all to cover the whole text, got %q ok=%v", text, ok)
	}
	if box.Cursor != len(box.Text) {
		t.Fatalf("expected caret at end after select-all, got %d", box.Cursor)
	}
}

func TestTextBoxCtrlArrowWordJump(t *testing.T) {
	box := NewTextBox("hello world foo", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Cursor = len(box.Text) // end

	// Ctrl+Left walks back one word boundary at a time: end -> 12 -> 6 -> 0.
	for _, want := range []int{12, 6, 0} {
		box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyLeft, Ctrl: true})
		if box.Cursor != want {
			t.Fatalf("Ctrl+Left: cursor = %d, want %d", box.Cursor, want)
		}
	}

	// Ctrl+Right walks forward: 0 -> 5 -> 11 -> 15.
	for _, want := range []int{5, 11, 15} {
		box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Ctrl: true})
		if box.Cursor != want {
			t.Fatalf("Ctrl+Right: cursor = %d, want %d", box.Cursor, want)
		}
	}
}

func TestTextBoxCtrlArrowExtendsSelection(t *testing.T) {
	box := NewTextBox("hello world", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Cursor = len(box.Text)
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyLeft, Ctrl: true, Shift: true})
	if !box.hasSelection() {
		t.Fatalf("expected Shift+Ctrl+Left to extend the selection")
	}
	if text, _ := box.copySelection(box.Component); text != "world" {
		t.Fatalf("expected selection 'world', got %q", text)
	}
}

func TestTextBoxCtrlBackspaceDeletesWord(t *testing.T) {
	box := NewTextBox("hello world", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Cursor = len(box.Text)
	if !box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyBackspace, Ctrl: true}) {
		t.Fatalf("expected Ctrl+Backspace to be consumed")
	}
	if string(box.Text) != "hello " {
		t.Fatalf("expected 'hello ' after Ctrl+Backspace, got %q", string(box.Text))
	}
	if box.Cursor != 6 {
		t.Fatalf("expected cursor at 6, got %d", box.Cursor)
	}
}

func TestTextBoxCtrlBackspaceDeletesSelection(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Cursor = 1
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyBackspace, Ctrl: true})
	if string(box.Text) != "hlo" {
		t.Fatalf("expected Ctrl+Backspace on a selection to delete it, got %q", string(box.Text))
	}
}

func TestTextBoxCtrlDeleteDeletesWordForward(t *testing.T) {
	box := NewTextBox("hello world", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Cursor = 0
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyDelete, Ctrl: true})
	if string(box.Text) != " world" {
		t.Fatalf("expected ' world' after Ctrl+Delete, got %q", string(box.Text))
	}
	if box.Cursor != 0 {
		t.Fatalf("expected cursor to stay at 0, got %d", box.Cursor)
	}
}

func TestTextBoxCtrlAThenTypeReplacesAll(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Ctrl: true}) // select all
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})
	if string(box.Text) != "X" {
		t.Fatalf("expected typing after select-all to replace everything, got %q", string(box.Text))
	}
}

// TestTextBoxCtrlEnterSubmits guards the refactor: Ctrl+Enter (the LF "submit" key)
// must still fire OnSubmit even though Ctrl is now intercepted for editing shortcuts.
func TestTextBoxCtrlEnterSubmits(t *testing.T) {
	box := NewTextBox("abc", Rect{X: 0, Y: 0, W: 8, H: 1})
	submits := 0
	box.OnSubmit = func() { submits++ }
	if !box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyEnter, Ctrl: true}) {
		t.Fatalf("expected Ctrl+Enter to be consumed (submit)")
	}
	if submits != 1 {
		t.Fatalf("expected submit once on Ctrl+Enter, got %d", submits)
	}
}

func TestTextBoxCutSelection(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 30, H: 1})
	box.Cursor = 1
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight, Shift: true})
	text, ok := box.cutSelection(box.Component)
	if !ok || text != "el" {
		t.Fatalf("expected cut to return 'el', got %q ok=%v", text, ok)
	}
	if string(box.Text) != "hlo" {
		t.Fatalf("expected 'hlo' after cut, got %q", string(box.Text))
	}
	if box.hasSelection() {
		t.Fatalf("expected no selection remaining after cut")
	}
}

func TestTextBoxCutWithoutSelectionDoesNothing(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 30, H: 1})
	text, ok := box.cutSelection(box.Component)
	if ok {
		t.Fatalf("expected cut with no selection to report nothing, got %q", text)
	}
	if string(box.Text) != "hello" {
		t.Fatalf("expected text unchanged, got %q", string(box.Text))
	}
}

// TestDesktopCtrlXCutsToClipboard is an end-to-end check that Ctrl+X on a focused
// TextBox removes its selection and writes the cut text to the clipboard (OSC 52).
func TestDesktopCtrlXCutsToClipboard(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(40, 5, &output)
	app.SetClipboardBackend(tui.ClipboardOSC52Only) // deterministic: OSC 52 only, no native shell-out
	desktop := NewDesktop(app)
	box := NewTextBox("hello", Rect{X: 1, Y: 1, W: 10, H: 1})
	desktop.AddLayer(NewLayer("root", box.Component, false, false))
	desktop.SetFocus(box)
	// Select "he" (columns 0..1).
	box.Cursor = 0
	box.selAnchor = 2

	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'x', Ctrl: true})

	if string(box.Text) != "llo" {
		t.Fatalf("expected Ctrl+X to remove the selection, got %q", string(box.Text))
	}
	// The cut text "he" must reach the clipboard via an OSC 52 escape.
	want := base64.StdEncoding.EncodeToString([]byte("he"))
	if !strings.Contains(output.String(), "\x1b]52;c;"+want+"\a") {
		t.Fatalf("expected OSC 52 clipboard escape with %q in output", want)
	}
}
