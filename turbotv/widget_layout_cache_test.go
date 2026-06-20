package tv

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// sliceDataPtr returns the address of a slice's backing array, used to tell
// whether two slices share storage (a cache hit) or not (a recompute).
func sliceDataPtr(s interface{}) uintptr {
	return reflect.ValueOf(s).Pointer()
}

func joinRows(rows []renderRow) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.text)
		b.WriteByte('|')
	}
	return b.String()
}

// TestTextViewLayoutRowsCached: an unchanged view returns the same wrapped slice
// across calls (no re-wrap), and a content change forces a recompute.
func TestTextViewLayoutRowsCached(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 10})
	entry := view.AddLine("the quick brown fox jumps over the lazy dog again and again")
	view.Wrap = true

	first := view.layoutRows(10)
	second := view.layoutRows(10)
	if sliceDataPtr(first) != sliceDataPtr(second) {
		t.Fatal("expected layoutRows to return the cached slice when content is unchanged")
	}
	if len(first) < 2 {
		t.Fatalf("expected wrapping to produce multiple rows, got %d", len(first))
	}

	// A mutation bumps the version; the next call must recompute (new storage)
	// and reflect the new text.
	entry.AppendText(" BRANDNEW")
	recomputed := view.layoutRows(10)
	if sliceDataPtr(recomputed) == sliceDataPtr(first) {
		t.Fatal("expected a fresh slice after content changed")
	}
	if !strings.Contains(joinRows(recomputed), "BRANDNEW") {
		t.Fatalf("recomputed rows do not reflect the append: %q", joinRows(recomputed))
	}
}

// TestTextViewLayoutRowsKeyedByWidth: changing the width recomputes even when
// the content version is unchanged.
func TestTextViewLayoutRowsKeyedByWidth(t *testing.T) {
	view := NewTextView("the quick brown fox jumps over the lazy dog", Rect{X: 0, Y: 0, W: 40, H: 10})
	view.Wrap = true

	narrow := view.layoutRows(8)
	wide := view.layoutRows(40)
	if sliceDataPtr(narrow) == sliceDataPtr(wide) {
		t.Fatal("expected different width to trigger a recompute")
	}
	if len(narrow) <= len(wide) {
		t.Fatalf("narrow wrap (%d rows) should yield more rows than wide (%d)", len(narrow), len(wide))
	}
}

// TestTextViewLayoutRowsKeyedByWrap: toggling Wrap via the public field (which
// does not go through touch) is still detected and triggers a recompute.
func TestTextViewLayoutRowsKeyedByWrap(t *testing.T) {
	view := NewTextView("the quick brown fox jumps over the lazy dog", Rect{X: 0, Y: 0, W: 40, H: 10})
	view.Wrap = false

	noWrap := view.layoutRows(10)
	view.Wrap = true
	wrapped := view.layoutRows(10)
	if len(noWrap) != 1 {
		t.Fatalf("expected a single unwrapped row, got %d", len(noWrap))
	}
	if len(wrapped) <= 1 {
		t.Fatalf("expected Wrap=true to wrap into multiple rows, got %d", len(wrapped))
	}
}

// TestTextViewFoldToggleReflected: collapsing a foldable entry changes the row
// count on the next layoutRows (touch invalidates the cache).
func TestTextViewFoldToggleReflected(t *testing.T) {
	view := NewTextView("", Rect{X: 0, Y: 0, W: 40, H: 10})
	parent := view.AddLine("[agent] summary")
	parent.Add("detail one")
	parent.Add("detail two")
	view.Wrap = false

	open := view.layoutRows(40)
	if len(open) != 3 {
		t.Fatalf("expected 3 open rows, got %d", len(open))
	}
	parent.SetCollapsed(true)
	collapsed := view.layoutRows(40)
	if len(collapsed) != 1 {
		t.Fatalf("expected 1 row after collapse, got %d", len(collapsed))
	}
}

// TestTreeFlattenReusesBuffer: the flattened slice reuses its backing array
// across calls (no per-call allocation) yet always reflects the live structure.
func TestTreeFlattenReusesBuffer(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 20, H: 10})
	root := tree.AddRoot(NewTreeNode("root"))
	root.AddLeaf("a")
	root.AddLeaf("b")

	first := tree.flatten()
	second := tree.flatten()
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected collapsed tree to flatten to 1 row, got %d/%d", len(first), len(second))
	}
	if sliceDataPtr(first) != sliceDataPtr(second) {
		t.Fatal("expected flatten to reuse its backing buffer across calls")
	}

	// Mutate structure directly; flatten must reflect it (recomputed each call).
	root.Expanded = true
	grown1 := tree.flatten()
	grown2 := tree.flatten()
	if len(grown1) != 3 || len(grown2) != 3 {
		t.Fatalf("expected expanded tree to flatten to 3 rows, got %d/%d", len(grown1), len(grown2))
	}
	// Steady state: once the buffer has grown large enough to hold the tree, it
	// is reused (no per-call allocation). It may have reallocated to get here, so
	// compare two consecutive post-growth calls rather than against `first`.
	if sliceDataPtr(grown1) != sliceDataPtr(grown2) {
		t.Fatal("expected flatten to reuse its buffer in steady state")
	}
}

// TestMultiLineCursorRowColMatchesCursorVisualPos guards the refactor: the new
// from-rows derivation must return the same (row, col) as the legacy derivation
// across wrapped layouts and caret positions.
func TestMultiLineCursorRowColMatchesCursorVisualPos(t *testing.T) {
	cases := []struct {
		lines []string
		cy    int
		cx    int
		width int
	}{
		{[]string{"abcdef"}, 0, 0, 4},
		{[]string{"abcdef"}, 0, 4, 4},
		{[]string{"abcdef"}, 0, 6, 4}, // caret past last rune -> clamps
		{[]string{"ab", "cdefghij"}, 1, 3, 4},
		{[]string{"hello world foo", "x"}, 0, 7, 5},
		{[]string{"one", "two", "three", "four"}, 3, 2, 3},
	}
	for _, tc := range cases {
		m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: tc.width, H: 5})
		m.Lines = tc.lines
		m.CursorY = tc.cy
		m.CursorX = tc.cx

		// cursorRowCol clamps the caret first; cursorVisualPos then sees the same
		// clamped caret, so the two must agree exactly.
		rows := m.wrappedRows(tc.width)
		gotRow, gotCol := m.cursorRowCol(rows, tc.width)
		wantRow, wantCol := m.cursorVisualPos(tc.width)
		if gotRow != wantRow || gotCol != wantCol {
			t.Fatalf("lines=%v cursor=(%d,%d) w=%d: cursorRowCol=(%d,%d) cursorVisualPos=(%d,%d)",
				tc.lines, tc.cy, tc.cx, tc.width, gotRow, gotCol, wantRow, wantCol)
		}
	}
}

// TestMultiLineDrawScrollsCursorIntoView: the single-pass draw still keeps the
// caret inside the viewport (and renders the scrolled content).
func TestMultiLineDrawScrollsCursorIntoView(t *testing.T) {
	var output bytes.Buffer
	app := tui.NewWithSize(40, 5, &output)
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 5})
	m := NewMultiLineInput("", Rect{X: 0, Y: 0, W: 40, H: 3})
	m.Lines = nil
	for i := 0; i < 10; i++ {
		m.Lines = append(m.Lines, fmt.Sprintf("line%d", i))
	}
	m.CursorY = 9
	m.CursorX = 0
	root.AddChild(m)
	desktop.AddLayer(NewLayer("root", root, true, false))
	desktop.Redraw()

	// Caret at visual row 9 in a 3-row viewport clamps ScrollY to 7.
	if m.ScrollY != 7 {
		t.Fatalf("expected ScrollY=7 after draw, got %d", m.ScrollY)
	}
	// First visible row is "line7".
	if got := app.ReadCell(0, 0).Ch; got != 'l' {
		t.Fatalf("expected first visible row to start with 'l' (line7), got %q", got)
	}
}

// TestMultiLineClickThenDragUsesSingleLayout: clicking and dragging resolves
// caret positions through the shared wrapped layout (regression for the
// visualPosToCursorFromRows path).
func TestMultiLineClickThenDragUsesSingleLayout(t *testing.T) {
	m := NewMultiLineInput("abcdef", Rect{X: 0, Y: 0, W: 4, H: 2})
	// Press at col 1 of wrapped row 1 (the "ef" continuation).
	m.Component.OnClickFn(m.Component, tui.ClickEvent{X: 1, Y: 1, Down: true})
	if m.CursorY != 0 || m.CursorX != 5 {
		t.Fatalf("after click expected cursor (0,5), got (%d,%d)", m.CursorY, m.CursorX)
	}
}
