package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// drawTree renders a tree to a fresh root surface sized to its bounds and
// returns the app so individual cells can be inspected.
func drawTree(t *testing.T, tree *Tree) *tui.App {
	t.Helper()
	abs := tree.Component.AbsoluteBounds()
	app := tui.NewWithSize(abs.X+abs.W, abs.Y+abs.H, &bytes.Buffer{})
	surface := newRootSurface(app)
	tree.draw(tree.Component, surface)
	return app
}

func leafTree(bounds Rect, labels ...string) *Tree {
	tree := NewTree(bounds)
	for _, l := range labels {
		tree.AddRoot(NewTreeNode(l))
	}
	return tree
}

// TestTreeDrawSelectionBarFullWidth covers issue #1: the selected row is painted
// as a full-width SelBG bar with SelFG text, while other rows keep the body
// colours.
func TestTreeDrawSelectionBarFullWidth(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 10, H: 5}, "alpha", "beta", "gamma")
	tree.Component.hasFocus = true
	app := drawTree(t, tree)

	// Selected row (index 0): every column must carry the selection background
	// and foreground — a bar, not just the glyphs.
	for x := 0; x < 10; x++ {
		cell := app.ReadCell(x, 0)
		if cell.BG != tree.SelBG {
			t.Fatalf("selected row col %d BG = %+v, want SelBG %+v", x, cell.BG, tree.SelBG)
		}
		if cell.FG != tree.SelFG {
			t.Fatalf("selected row col %d FG = %+v, want SelFG %+v", x, cell.FG, tree.SelFG)
		}
	}
	// A non-selected row keeps the body background across its full width.
	for x := 0; x < 10; x++ {
		if cell := app.ReadCell(x, 1); cell.BG != tree.BG {
			t.Fatalf("unselected row col %d BG = %+v, want body BG %+v", x, cell.BG, tree.BG)
		}
	}
}

// TestTreeSelectionBarFocusDim covers issue #29: the selected row dims when the
// tree is not focused so the active widget is unambiguous.
func TestTreeSelectionBarFocusDim(t *testing.T) {
	if DefaultTheme.SelectionBG == tui.ANSIColor(8) {
		t.Fatal("test precondition: focused and unfocused selection BG must differ")
	}
	tree := leafTree(Rect{X: 0, Y: 0, W: 8, H: 4}, "one", "two")

	tree.Component.hasFocus = true
	focused := drawTree(t, tree)
	if got := focused.ReadCell(0, 0).BG; got != tree.SelBG {
		t.Fatalf("focused selection BG = %+v, want SelBG %+v", got, tree.SelBG)
	}

	tree.Component.hasFocus = false
	unfocused := drawTree(t, tree)
	if got := unfocused.ReadCell(0, 0).BG; got != tree.SelBGUnfocused {
		t.Fatalf("unfocused selection BG = %+v, want SelBGUnfocused %+v", got, tree.SelBGUnfocused)
	}
	if tree.SelBG == tree.SelBGUnfocused {
		t.Fatal("focused and unfocused selection bars are identical")
	}
}

// TestTreeTruncationEllipsis covers issue #30: an overflowing label is cut with
// a trailing ellipsis rather than silently mid-text.
func TestTreeTruncationEllipsis(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 8, H: 3}, "abcdefghijklmnop")
	app := drawTree(t, tree)

	// Width 8: 7 content columns then the ellipsis in the last column.
	if got := app.ReadCell(7, 0).Ch; got != '…' {
		t.Fatalf("last column = %q, want ellipsis", got)
	}

	// A label that fits leaves no ellipsis.
	fits := leafTree(Rect{X: 0, Y: 0, W: 12, H: 3}, "short")
	app2 := drawTree(t, fits)
	for x := 0; x < 12; x++ {
		if app2.ReadCell(x, 0).Ch == '…' {
			t.Fatalf("unexpected ellipsis at col %d for a label that fits", x)
		}
	}
}

// TestTreeNavigationKeys covers issue #31: Home/End/PageUp/PageDown.
func TestTreeNavigationKeys(t *testing.T) {
	labels := make([]string, 20)
	for i := range labels {
		labels[i] = string(rune('a' + i))
	}

	cases := []struct {
		name  string
		start int
		keys  []tui.TypeEvent
		want  int
	}{
		{"home from middle", 10, []tui.TypeEvent{{Key: tui.KeyHome}}, 0},
		{"end from middle", 10, []tui.TypeEvent{{Key: tui.KeyEnd}}, 19},
		{"pagedown one viewport", 0, []tui.TypeEvent{{Key: tui.KeyPageDown}}, 5},
		{"pagedown twice", 0, []tui.TypeEvent{{Key: tui.KeyPageDown}, {Key: tui.KeyPageDown}}, 10},
		{"pagedown clamps at end", 18, []tui.TypeEvent{{Key: tui.KeyPageDown}}, 19},
		{"pageup one viewport", 19, []tui.TypeEvent{{Key: tui.KeyPageUp}}, 14},
		{"pageup clamps at start", 2, []tui.TypeEvent{{Key: tui.KeyPageUp}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Height 5 -> viewH 5, so a page jump is 5 rows.
			tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, labels...)
			drawTree(t, tree) // establishes viewH for page sizing
			tree.selected = tc.start
			for _, ev := range tc.keys {
				if !tree.handleType(tree.Component, ev) {
					t.Fatalf("handleType(%v) returned false", ev.Key)
				}
			}
			if tree.selected != tc.want {
				t.Fatalf("selected = %d, want %d", tree.selected, tc.want)
			}
		})
	}
}

// TestTreeNavigationFiresOnSelect verifies the navigation keys notify OnSelect
// when, and only when, the selection actually moves.
func TestTreeNavigationFiresOnSelect(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c", "d", "e", "f", "g", "h")
	drawTree(t, tree)
	var fired int
	var last *TreeNode
	tree.OnSelect = func(n *TreeNode) { fired++; last = n }

	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyEnd})
	if fired != 1 || last == nil || last.Label != "h" {
		t.Fatalf("End: fired=%d last=%v, want 1 fire on node h", fired, last)
	}
	// Already at the end: End again must not re-fire.
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyEnd})
	if fired != 1 {
		t.Fatalf("End at end re-fired OnSelect (fired=%d)", fired)
	}
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyHome})
	if fired != 2 || last.Label != "a" {
		t.Fatalf("Home: fired=%d last=%v, want 2 fires ending on a", fired, last)
	}
}
