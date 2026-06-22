package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// CaretRowInLine (gogent#270) must report the caret's visual row within its
// logical line and that line's total visual rows, from the widget's real wrap
// layout — so a host can detect "caret on first/last visual row" under word wrap,
// not just character wrap. Bounds W=11 → contentWidth 10.

func newCaretRowInput(t *testing.T, text string, wordWrap bool) *MultiLineInput {
	t.Helper()
	_ = tui.NewWithSize(40, 10, &bytes.Buffer{}) // ensures the tui package is initialised
	in := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 11, H: 5})
	in.WordWrap = wordWrap
	in.SetText(text)
	return in
}

func TestCaretRowInLineWordWrap(t *testing.T) {
	// "aaaa bbbb cccc" wraps at width 10 into ["aaaa bbbb ", "cccc"] → 2 rows.
	cases := []struct {
		name     string
		cursorX  int
		wantRow  int
		wantRows int
	}{
		{"start of line, first row", 0, 0, 2},
		{"within first row", 5, 0, 2},
		{"into second row", 12, 1, 2},
		{"end of line, last row", 14, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := newCaretRowInput(t, "aaaa bbbb cccc", true)
			in.CursorY = 0
			in.CursorX = tc.cursorX
			row, rows := in.CaretRowInLine()
			if row != tc.wantRow || rows != tc.wantRows {
				t.Fatalf("CaretRowInLine() = (%d,%d), want (%d,%d)", row, rows, tc.wantRow, tc.wantRows)
			}
		})
	}
}

func TestCaretRowInLineCharWrap(t *testing.T) {
	// Char wrap at width 10: "abcdefghijklmn" (14) → rows ["abcdefghij","klmn"] (2).
	in := newCaretRowInput(t, "abcdefghijklmn", false)
	in.CursorY = 0
	in.CursorX = 12 // 12/10 = row 1
	row, rows := in.CaretRowInLine()
	if row != 1 || rows != 2 {
		t.Fatalf("char-wrap CaretRowInLine() = (%d,%d), want (1,2)", row, rows)
	}
	in.CursorX = 3 // row 0
	if row, rows = in.CaretRowInLine(); row != 0 || rows != 2 {
		t.Fatalf("char-wrap first row = (%d,%d), want (0,2)", row, rows)
	}
}

func TestCaretRowInLineShortLineSingleRow(t *testing.T) {
	in := newCaretRowInput(t, "hi", true)
	in.CursorY = 0
	in.CursorX = 1
	if row, rows := in.CaretRowInLine(); row != 0 || rows != 1 {
		t.Fatalf("short line = (%d,%d), want (0,1)", row, rows)
	}
}
