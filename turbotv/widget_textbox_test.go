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

// TestTextBoxPasteCollapsesToChip replaces the former TestTextBoxPasteStripsNewlines:
// as of gogent #501 a multi-line paste into the single-line TextBox collapses to one
// atomic chip (parity with MultiLineInput) instead of stripping the newlines, and
// GetText restores the verbatim original with CR dropped (CRLF→LF).
func TestTextBoxPasteCollapsesToChip(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 40, H: 1})
	box.handlePaste(box.Component, "ab\ncd\r\nef")
	if len(box.Text) != 1 || !IsPasteChipRune(box.Text[0]) {
		t.Fatalf("expected a single chip sentinel, got %q", string(box.Text))
	}
	if got := box.GetText(); got != "ab\ncd\nef" {
		t.Fatalf("expected GetText to restore the verbatim paste, got %q", got)
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

func TestTextBoxSelectAllThenTypeReplaces(t *testing.T) {
	box := NewTextBox("hé🙂", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.SelectAll()
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})

	if got := box.GetText(); got != "X" {
		t.Fatalf("typing after SelectAll: text = %q, want %q", got, "X")
	}
	if box.Cursor != 1 || box.hasSelection() {
		t.Fatalf("typing after SelectAll: cursor=%d selected=%v, want 1, false", box.Cursor, box.hasSelection())
	}
}

func TestTextBoxSelectAllThenRightAppends(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.SelectAll()
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRight})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})

	if got := box.GetText(); got != "helloX" {
		t.Fatalf("Right after SelectAll: text = %q, want %q", got, "helloX")
	}
	if box.Cursor != 6 || box.hasSelection() {
		t.Fatalf("Right after SelectAll: cursor=%d selected=%v, want 6, false", box.Cursor, box.hasSelection())
	}
}

func TestTextBoxSelectAllThenEndAppends(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.SelectAll()
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyEnd})
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})

	if got := box.GetText(); got != "helloX" {
		t.Fatalf("End after SelectAll: text = %q, want %q", got, "helloX")
	}
	if box.Cursor != 6 || box.hasSelection() {
		t.Fatalf("End after SelectAll: cursor=%d selected=%v, want 6, false", box.Cursor, box.hasSelection())
	}
}

func TestTextBoxSelectAllThenBackspaceEmpties(t *testing.T) {
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.SelectAll()
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyBackspace})

	if got := box.GetText(); got != "" {
		t.Fatalf("Backspace after SelectAll: text = %q, want empty", got)
	}
	if box.Cursor != 0 || box.hasSelection() {
		t.Fatalf("Backspace after SelectAll: cursor=%d selected=%v, want 0, false", box.Cursor, box.hasSelection())
	}
}

func TestTextBoxSelectAllEmptyFieldLeavesNoAnchor(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.SelectAll()

	if box.selAnchor != -1 || box.Cursor != 0 || box.hasSelection() {
		t.Fatalf("SelectAll on empty field: anchor=%d cursor=%d selected=%v, want -1, 0, false", box.selAnchor, box.Cursor, box.hasSelection())
	}
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})
	if got := box.GetText(); got != "X" {
		t.Fatalf("first rune after empty SelectAll: text = %q, want %q", got, "X")
	}
	if box.hasSelection() {
		t.Fatal("first rune after empty SelectAll selected itself")
	}
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'Y'})
	if got := box.GetText(); got != "XY" {
		t.Fatalf("second rune after empty SelectAll: text = %q, want %q", got, "XY")
	}
}

func TestTextBoxSelectAllSurvivesSetFocus(t *testing.T) {
	var output bytes.Buffer
	desktop := NewDesktop(tui.NewWithSize(40, 5, &output))
	box := NewTextBox("hello", Rect{X: 0, Y: 0, W: 20, H: 1})
	box.SelectAll()

	desktop.SetFocus(box)

	if !box.Component.Focused() {
		t.Fatal("SetFocus did not focus TextBox")
	}
	lo, hi := box.selRange()
	if !box.hasSelection() || lo != 0 || hi != len(box.Text) {
		t.Fatalf("selection after SetFocus: selected=%v range=(%d,%d), want true and (0,%d)", box.hasSelection(), lo, hi, len(box.Text))
	}
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})
	if got := box.GetText(); got != "X" {
		t.Fatalf("typing after SelectAll and SetFocus: text = %q, want %q", got, "X")
	}
}

func clickEmptyTextBox(t *testing.T, box *TextBox) {
	t.Helper()
	if !box.handleClick(box.Component, tui.ClickEvent{X: 0, Y: 0, Down: true}) {
		t.Fatal("mouse-down inside empty TextBox was not consumed")
	}
	if !box.handleClick(box.Component, tui.ClickEvent{X: 0, Y: 0, Down: false}) {
		t.Fatal("mouse-up inside empty TextBox was not consumed")
	}
	if box.selAnchor != 0 || box.Cursor != 0 || box.hasSelection() {
		t.Fatalf("click precondition: anchor=%d cursor=%d selected=%v, want 0, 0, false", box.selAnchor, box.Cursor, box.hasSelection())
	}
}

func assertTypingAppendsXY(t *testing.T, box *TextBox) {
	t.Helper()
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'X'})
	if box.hasSelection() {
		t.Fatal("first rune selected itself")
	}
	box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'Y'})
	if got := box.GetText(); got != "XY" {
		t.Fatalf("two typed runes: text = %q, want %q", got, "XY")
	}
	if box.hasSelection() {
		t.Fatal("typing left a selection")
	}
}

func TestTextBoxSelectAllClearsStaleAnchorOnEmptyField(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 20, H: 1})
	clickEmptyTextBox(t, box)

	box.SelectAll()

	if box.selAnchor != -1 || box.Cursor != 0 || box.hasSelection() {
		t.Fatalf("SelectAll after click: anchor=%d cursor=%d selected=%v, want -1, 0, false", box.selAnchor, box.Cursor, box.hasSelection())
	}
	assertTypingAppendsXY(t, box)
}

func TestTextBoxCtrlAClearsStaleAnchorOnEmptyField(t *testing.T) {
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 20, H: 1})
	clickEmptyTextBox(t, box)

	if !box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Ctrl: true}) {
		t.Fatal("Ctrl+A on empty field was not consumed")
	}
	if box.selAnchor != -1 || box.Cursor != 0 || box.hasSelection() {
		t.Fatalf("Ctrl+A after click: anchor=%d cursor=%d selected=%v, want -1, 0, false", box.selAnchor, box.Cursor, box.hasSelection())
	}
	assertTypingAppendsXY(t, box)
}

func TestTextBoxCtrlAAndSelectAllAgree(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "empty"},
		{name: "ascii", text: "hello"},
		{name: "unicode", text: "hé🙂"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			viaMethod := NewTextBox(tc.text, Rect{X: 0, Y: 0, W: 20, H: 1})
			viaMethod.Cursor = 0
			viaMethod.SelectAll()

			viaKey := NewTextBox(tc.text, Rect{X: 0, Y: 0, W: 20, H: 1})
			viaKey.Cursor = 0
			viaKey.handleType(viaKey.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: 'a', Ctrl: true})

			if viaKey.selAnchor != viaMethod.selAnchor || viaKey.Cursor != viaMethod.Cursor {
				t.Fatalf("Ctrl+A state=(anchor %d, cursor %d), SelectAll state=(anchor %d, cursor %d)", viaKey.selAnchor, viaKey.Cursor, viaMethod.selAnchor, viaMethod.Cursor)
			}
		})
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
