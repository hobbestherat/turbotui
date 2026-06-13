package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestMultiLineInputWrapsRows(t *testing.T) {
	input := NewMultiLineInput("abcdef", Rect{X: 0, Y: 0, W: 4, H: 2})
	rows := input.wrappedRows(4)
	if len(rows) != 2 {
		t.Fatalf("expected 2 wrapped rows, got %d", len(rows))
	}
	if string(rows[0].runes) != "abcd" {
		t.Fatalf("unexpected first wrapped row: %q", string(rows[0].runes))
	}
	if string(rows[1].runes) != "ef" {
		t.Fatalf("unexpected second wrapped row: %q", string(rows[1].runes))
	}
}

func TestMultiLineInputUpDownUseWrappedRows(t *testing.T) {
	input := NewMultiLineInput("abcdef", Rect{X: 0, Y: 0, W: 4, H: 2})
	input.moveUp(4)
	if input.CursorY != 0 || input.CursorX != 2 {
		t.Fatalf("expected cursor to move to wrapped first row, got (%d,%d)", input.CursorY, input.CursorX)
	}
	input.moveDown(4)
	if input.CursorY != 0 || input.CursorX != 6 {
		t.Fatalf("expected cursor to move back to wrapped second row, got (%d,%d)", input.CursorY, input.CursorX)
	}
}

func TestMultiLineInputClickSetsWrappedCursor(t *testing.T) {
	input := NewMultiLineInput("abcdef", Rect{X: 0, Y: 0, W: 4, H: 2})
	component := input.Component
	_ = input.handleClick(component, tui.ClickEvent{X: 1, Y: 1, Down: true})
	if input.CursorY != 0 || input.CursorX != 5 {
		t.Fatalf("expected wrapped click cursor at (0,5), got (%d,%d)", input.CursorY, input.CursorX)
	}
}

func TestMultiLineInputSubmitModeEnterDefault(t *testing.T) {
	input := NewMultiLineInput("abc", Rect{X: 0, Y: 0, W: 8, H: 2})
	submits := 0
	input.OnSubmit = func() {
		submits++
	}
	_ = input.handleType(input.Component, tui.TypeEvent{Key: tui.KeyEnter})
	if submits != 1 {
		t.Fatalf("expected plain Enter to submit by default, got submits=%d", submits)
	}
	_ = input.handleType(input.Component, tui.TypeEvent{Key: tui.KeyEnter, Shift: true})
	if submits != 1 {
		t.Fatalf("expected Shift+Enter to insert a newline by default, got submits=%d", submits)
	}
}

func TestMultiLineInputSubmitModeShiftEnter(t *testing.T) {
	input := NewMultiLineInput("abc", Rect{X: 0, Y: 0, W: 8, H: 2})
	input.SubmitMode = MultiLineSubmitOnShiftEnter
	submits := 0
	input.OnSubmit = func() {
		submits++
	}
	_ = input.handleType(input.Component, tui.TypeEvent{Key: tui.KeyEnter})
	if submits != 0 {
		t.Fatalf("expected plain Enter to insert newline in ShiftEnter mode, got submits=%d", submits)
	}
	_ = input.handleType(input.Component, tui.TypeEvent{Key: tui.KeyEnter, Shift: true})
	if submits != 1 {
		t.Fatalf("expected Shift+Enter to submit in ShiftEnter mode, got submits=%d", submits)
	}
}

func TestMultiLineInputPasteSplitsLines(t *testing.T) {
	input := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 5})
	input.handlePaste(input.Component, "one\r\ntwo\nthree")
	if len(input.Lines) != 3 {
		t.Fatalf("expected 3 lines after paste, got %d: %#v", len(input.Lines), input.Lines)
	}
	if input.Lines[0] != "one" || input.Lines[1] != "two" || input.Lines[2] != "three" {
		t.Fatalf("unexpected pasted lines: %#v", input.Lines)
	}
}
