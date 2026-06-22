package tv

import (
	"bytes"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// ---- #39: word wrap ----------------------------------------------------------

func TestWordWrapSpansContiguousAndLossless(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  []string
	}{
		{"hello world foo", 8, []string{"hello ", "world ", "foo"}},
		{"hello world", 8, []string{"hello ", "world"}},
		{"abcdefgh", 4, []string{"abcd", "efgh"}}, // single long word hard-splits
		{"ab cd", 10, []string{"ab cd"}},          // fits on one row
		{"a b c d e", 3, []string{"a ", "b ", "c ", "d e"}},
		{"", 4, []string{""}},
	}
	for _, tc := range cases {
		runes := []rune(tc.text)
		spans := wordWrapSpans(runes, tc.width)
		// Spans must tile the line with no gaps and lose no runes.
		if spans[0].start != 0 {
			t.Fatalf("%q: first span must start at 0, got %d", tc.text, spans[0].start)
		}
		var rebuilt strings.Builder
		prevEnd := 0
		var got []string
		for i, s := range spans {
			if s.start != prevEnd {
				t.Fatalf("%q: span %d not contiguous: start=%d prevEnd=%d", tc.text, i, s.start, prevEnd)
			}
			if len([]rune(string(runes[s.start:s.end]))) > tc.width {
				t.Fatalf("%q: span %d wider than width %d", tc.text, i, tc.width)
			}
			rebuilt.WriteString(string(runes[s.start:s.end]))
			got = append(got, string(runes[s.start:s.end]))
			prevEnd = s.end
		}
		if prevEnd != len(runes) {
			t.Fatalf("%q: spans do not cover line (end=%d len=%d)", tc.text, prevEnd, len(runes))
		}
		if rebuilt.String() != tc.text {
			t.Fatalf("%q: rebuilt %q is lossy", tc.text, rebuilt.String())
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Fatalf("%q width=%d: got %v want %v", tc.text, tc.width, got, tc.want)
		}
	}
}

func TestMultiLineWordWrapKeepsWordsIntact(t *testing.T) {
	input := NewMultiLineInput("hello world foo", Rect{X: 0, Y: 0, W: 9, H: 4})
	input.WordWrap = true
	rows := input.wrappedRows(8)
	var got []string
	for _, r := range rows {
		got = append(got, string(r.runes))
	}
	if strings.Join(got, "|") != "hello |world |foo" {
		t.Fatalf("word wrap split words: %v", got)
	}
	// Char wrap (default) for the same buffer splits mid-word.
	input.WordWrap = false
	rows = input.wrappedRows(8)
	if string(rows[0].runes) != "hello wo" {
		t.Fatalf("expected char wrap first row 'hello wo', got %q", string(rows[0].runes))
	}
}

// In word-wrap mode the cursor-row derivations must still agree (they share the
// span locator), and a click must resolve through the wrapped rows correctly.
func TestMultiLineWordWrapCursorAgrees(t *testing.T) {
	cases := []struct {
		cy, cx int
	}{
		{0, 0}, {0, 5}, {0, 6}, {0, 11}, {0, 15},
	}
	for _, tc := range cases {
		m := NewMultiLineInput("hello world foo", Rect{X: 0, Y: 0, W: 9, H: 4})
		m.WordWrap = true
		m.CursorY, m.CursorX = tc.cy, tc.cx
		rows := m.wrappedRows(8)
		gotRow, gotCol := m.cursorRowCol(rows, 8)
		wantRow, wantCol := m.cursorVisualPos(8)
		if gotRow != wantRow || gotCol != wantCol {
			t.Fatalf("cursor=(%d,%d): cursorRowCol=(%d,%d) cursorVisualPos=(%d,%d)",
				tc.cy, tc.cx, gotRow, gotCol, wantRow, wantCol)
		}
	}
}

func TestMultiLineWordWrapClickMapsToWord(t *testing.T) {
	m := NewMultiLineInput("hello world foo", Rect{X: 0, Y: 0, W: 9, H: 4})
	m.WordWrap = true
	// Rows at width 8 are "hello "(start 0), "world "(start 6), "foo"(start 12).
	// Click row 1, col 2 -> logical index 6+2 = 8.
	_ = m.handleClick(m.Component, tui.ClickEvent{X: 2, Y: 1, Down: true})
	if m.CursorY != 0 || m.CursorX != 8 {
		t.Fatalf("expected caret at (0,8), got (%d,%d)", m.CursorY, m.CursorX)
	}
}

// ---- #38: scrollbar ----------------------------------------------------------

func drawInput(t *testing.T, m *MultiLineInput, w, h int) *tui.App {
	t.Helper()
	var output bytes.Buffer
	app := tui.NewWithSize(w, h, &output)
	desktop := NewDesktop(app)
	root := NewComponent(Rect{X: 0, Y: 0, W: w, H: h})
	root.AddChild(m)
	desktop.AddLayer(NewLayer("root", root, true, false))
	desktop.Redraw()
	return app
}

func TestMultiLineScrollbarShownOnOverflow(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 10, H: 3})
	m.Lines = nil
	for i := 0; i < 10; i++ {
		m.Lines = append(m.Lines, "line")
	}
	m.CursorY, m.CursorX = 0, 0
	m.ScrollY = 0
	app := drawInput(t, m, 10, 3)
	// The reserved rightmost column carries the scrollbar: top arrow, then a track.
	if got := app.ReadCell(9, 0).Ch; got != '▲' {
		t.Fatalf("expected top arrow at scrollbar column, got %q", got)
	}
	if got := app.ReadCell(9, 2).Ch; got != '▼' {
		t.Fatalf("expected bottom arrow at scrollbar column, got %q", got)
	}
}

func TestMultiLineNoScrollbarWhenFits(t *testing.T) {
	m := NewMultiLineInput("one\ntwo", Rect{X: 0, Y: 0, W: 10, H: 5})
	app := drawInput(t, m, 10, 5)
	// No overflow: the reserved column stays blank (no arrow glyphs).
	for y := 0; y < 5; y++ {
		if ch := app.ReadCell(9, y).Ch; ch == '▲' || ch == '▼' || ch == '█' {
			t.Fatalf("did not expect a scrollbar glyph at (9,%d), got %q", y, ch)
		}
	}
}

func TestMultiLineScrollbarDragMovesScroll(t *testing.T) {
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 10, H: 4})
	m.Lines = nil
	for i := 0; i < 20; i++ {
		m.Lines = append(m.Lines, "line")
	}
	// Grab the thumb near the bottom of the track and confirm we scroll down.
	abs := m.Component.AbsoluteBounds()
	_ = m.handleClick(m.Component, tui.ClickEvent{X: abs.Right(), Y: abs.Bottom() - 1, Down: true})
	if !m.draggingThumb {
		t.Fatal("expected scrollbar drag to begin")
	}
	if m.ScrollY == 0 {
		t.Fatalf("expected scroll to advance after grabbing thumb low, got %d", m.ScrollY)
	}
	scrolled := m.ScrollY
	// A scrollbar interaction must not move the caret into the text.
	if m.CursorY != 0 || m.CursorX != 0 {
		t.Fatalf("scrollbar drag moved the caret to (%d,%d)", m.CursorY, m.CursorX)
	}
	// Release ends the drag.
	_ = m.handleClick(m.Component, tui.ClickEvent{X: abs.Right(), Y: abs.Bottom() - 1, Down: false})
	if m.draggingThumb {
		t.Fatal("expected release to end the drag")
	}
	if m.ScrollY != scrolled {
		t.Fatalf("release should not change scroll, got %d want %d", m.ScrollY, scrolled)
	}
}

// ---- #40: selection tail fill ------------------------------------------------

func TestMultiLineSelectionFillsTail(t *testing.T) {
	m := NewMultiLineInput("ab\ncd\nef", Rect{X: 0, Y: 0, W: 6, H: 5})
	// Select from line 0 col 1 through line 2 col 1 (spans all three lines).
	m.selAnchorY, m.selAnchorX = 0, 1
	m.CursorY, m.CursorX = 2, 1
	// Focus the input: text selection is only created/edited while focused, and the
	// selection background (TextSelectionBG) is chosen to contrast with the FOCUSED
	// fill (InputFocusBG), not the resting InputBG — so the distinction is only
	// observable on a focused field (gogent#279).
	m.Component.hasFocus = true
	app := drawInput(t, m, 6, 5)

	selBG := activeTheme.TextSelectionBG // input text selection (gogent#279)
	// Line 0 (start line): the blank tail after "ab" must carry the selection BG
	// up to the text width (cols 2..4; col 5 is the scrollbar column).
	if bg := app.ReadCell(2, 0).BG; bg != selBG {
		t.Fatalf("start-line tail not highlighted at (2,0): %+v", bg)
	}
	// Line 1 (interior, fully selected): entire text width highlighted.
	for x := 0; x < 5; x++ {
		if bg := app.ReadCell(x, 1).BG; bg != selBG {
			t.Fatalf("interior line not fully highlighted at (%d,1): %+v", x, bg)
		}
	}
	// Line 2 (end line): selection ends at col 1, so the tail is NOT highlighted.
	if bg := app.ReadCell(2, 2).BG; bg == selBG {
		t.Fatalf("end-line tail should not be highlighted at (2,2)")
	}
}
