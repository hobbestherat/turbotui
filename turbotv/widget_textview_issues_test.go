package tv

import (
	"io"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// setupTextView builds a w×h desktop whose root layer holds a TextView at the
// given bounds, and returns the desktop (for App()/Redraw()) and the widget.
func setupTextView(w, h int, bounds Rect) (*Desktop, *TextView) {
	app := tui.NewWithSize(w, h, io.Discard)
	desktop := NewDesktop(app)
	view := NewTextView("", bounds)
	root := NewComponent(Rect{X: 0, Y: 0, W: w, H: h})
	root.AddChild(view.Component)
	desktop.AddLayer(NewLayer("test", root, true, false))
	return desktop, view
}

// Issue #25: a scrolling viewer should wrap by default so long lines are not
// silently truncated.
func TestTextViewDefaultsToWrap(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 20, H: 5})
	if !view.Wrap {
		t.Fatal("NewTextView should default Wrap to true")
	}
}

// Issue #25: with wrapping off, a line wider than the view is clipped with a
// trailing ellipsis so the dropped tail is signalled; a line that fits is not.
func TestTextViewNoWrapClipsWithEllipsis(t *testing.T) {
	desktop, view := setupTextView(40, 5, Rect{X: 0, Y: 0, W: 10, H: 3})
	view.Wrap = false
	view.SetText("abcdefghijklmnop") // 16 cols, view is 10 wide, content fits in H so no bar
	desktop.Redraw()
	app := desktop.App()
	// Content fills all 10 columns (no scrollbar reserved), last column is the ellipsis.
	if got := app.ReadCell(0, 0).Ch; got != 'a' {
		t.Fatalf("clipped line should still start at column 0, got %q", got)
	}
	if got := app.ReadCell(9, 0).Ch; got != '…' {
		t.Fatalf("clipped line should end in an ellipsis at the last column, got %q", got)
	}

	// A short line must render verbatim with no ellipsis.
	desktop2, view2 := setupTextView(40, 5, Rect{X: 0, Y: 0, W: 10, H: 3})
	view2.Wrap = false
	view2.SetText("abc")
	desktop2.Redraw()
	app2 := desktop2.App()
	if got := app2.ReadCell(2, 0).Ch; got != 'c' {
		t.Fatalf("short line should render verbatim, got %q at col 2", got)
	}
	for x := 0; x < 10; x++ {
		if app2.ReadCell(x, 0).Ch == '…' {
			t.Fatalf("short line should not be ellipsized (col %d)", x)
		}
	}
}

// Issue #26: wrapText preserves inter-word whitespace, leading indentation and
// tabs instead of collapsing them the way strings.Fields did.
func TestWrapTextPreservesWhitespace(t *testing.T) {
	joined := func(rows []string) string { return strings.Join(rows, "|") }
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"leading indent kept", "    code", 20, []string{"    code"}},
		{"internal runs kept", "a    b", 20, []string{"a    b"}},
		{"all spaces not collapsed", "    ", 20, []string{"    "}},
		{"trailing spaces kept on fitting line", "abc   ", 20, []string{"abc   "}},
		{"tab expands to tab stop", "\tx", 20, []string{"        x"}},
		{"col-aligned tabs preserved", "a\tb", 20, []string{"a       b"}},
		{"long word hard split", "abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"break absorbs separator, no lone-space row", "aaaa bbbb", 4, []string{"aaaa", "bbbb"}},
		{"empty stays one empty row", "", 10, []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapText(tc.text, tc.width)
			if joined(got) != joined(tc.want) {
				t.Fatalf("wrapText(%q, %d) = %q, want %q", tc.text, tc.width, got, tc.want)
			}
		})
	}
}

// Issue #26: a line that fits within the width is emitted verbatim — every
// space survives — so the displayed text matches the source.
func TestWrapTextFittingLineIsVerbatim(t *testing.T) {
	src := "  func main() {\treturn 0 }   "
	rows := wrapText(src, 200)
	if len(rows) != 1 {
		t.Fatalf("expected a single row for a line that fits, got %d: %q", len(rows), rows)
	}
	if rows[0] != expandTabs(src, textViewTabWidth) {
		t.Fatalf("fitting line should be verbatim (tabs expanded): got %q", rows[0])
	}
	// strings.Fields would have collapsed this; ensure runs of spaces survived.
	if !strings.Contains(rows[0], "  func") || !strings.Contains(rows[0], "0 }   ") {
		t.Fatalf("inter-word whitespace was not preserved: %q", rows[0])
	}
}

// Issue #27: when the content fits, no scrollbar column is reserved or drawn,
// matching Tree/Select.
func TestTextViewNoScrollbarWhenContentFits(t *testing.T) {
	desktop, view := setupTextView(40, 10, Rect{X: 0, Y: 0, W: 20, H: 10})
	view.Wrap = false
	view.SetText("one\ntwo\nthree") // 3 rows in a 10-row pane
	view.Component.HasFocus = true

	rows, textWidth, bar := view.metrics(view.Component.AbsoluteBounds())
	if bar {
		t.Fatalf("short content should not reserve a scrollbar (rows=%d)", len(rows))
	}
	if textWidth != 20 {
		t.Fatalf("without a bar the full width should be usable, got %d", textWidth)
	}

	desktop.Redraw()
	app := desktop.App()
	// The would-be scrollbar column (rightmost) must carry no track glyphs.
	for y := 0; y < 10; y++ {
		switch app.ReadCell(19, y).Ch {
		case '▲', '▼', '│', '█':
			t.Fatalf("no scrollbar chrome should be drawn for short content (col 19, row %d)", y)
		}
	}
}

// Issue #27: when the content overflows, the scrollbar column is reserved and
// the bar (arrows) is drawn.
func TestTextViewScrollbarWhenContentOverflows(t *testing.T) {
	desktop, view := setupTextView(40, 3, Rect{X: 0, Y: 0, W: 20, H: 3})
	view.Wrap = false
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "line")
	}
	view.SetText(strings.Join(lines, "\n"))
	view.Component.HasFocus = true

	rows, textWidth, bar := view.metrics(view.Component.AbsoluteBounds())
	if !bar {
		t.Fatalf("overflowing content should reserve a scrollbar (rows=%d, H=3)", len(rows))
	}
	if textWidth != 19 {
		t.Fatalf("a reserved bar should shrink the content width to 19, got %d", textWidth)
	}

	desktop.Redraw()
	app := desktop.App()
	if got := app.ReadCell(19, 0).Ch; got != '▲' {
		t.Fatalf("expected up-arrow at the top of the scrollbar, got %q", got)
	}
	if got := app.ReadCell(19, 2).Ch; got != '▼' {
		t.Fatalf("expected down-arrow at the bottom of the scrollbar, got %q", got)
	}
}

// Issue #28: PageUp/PageDown scroll by a viewport (height-1), not a hardcoded 5.
func TestTextViewPageScrollsByViewport(t *testing.T) {
	const h = 10
	desktop, view := setupTextView(40, h, Rect{X: 0, Y: 0, W: 20, H: h})
	view.Wrap = false
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "row")
	}
	view.SetText(strings.Join(lines, "\n"))
	desktop.Redraw() // records viewH=10 and (following) pins to the bottom

	// 50 rows in a 10-row pane -> max scroll offset is 40.
	if view.scrollY != 40 {
		t.Fatalf("expected scrollY pinned to bottom (40), got %d", view.scrollY)
	}
	// One PageUp must move a full page minus one line of overlap: 10-1 = 9 rows,
	// not the old hardcoded 5.
	view.Component.OnTypeFn(view.Component, tui.TypeEvent{Key: tui.KeyPageUp})
	if view.scrollY != 31 {
		t.Fatalf("PageUp should scroll by viewport-1 (9), expected scrollY=31, got %d", view.scrollY)
	}
	view.Component.OnTypeFn(view.Component, tui.TypeEvent{Key: tui.KeyPageDown})
	if view.scrollY != 40 {
		t.Fatalf("PageDown should return to the bottom (40), got %d", view.scrollY)
	}
}
