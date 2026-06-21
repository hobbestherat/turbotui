package tv

import (
	"fmt"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Round-2 edge cases: the new SetSelected int-return contract, the painted bar
// under scrolling (the literal gogent #206 symptom, which round 1 only tested in
// fits-in-viewport trees), SelectNode's offset safety on failure, and the
// programmatic-select → keyboard/mouse interplay (no echo, single fire).

// -----------------------------------------------------------------------------
// SetSelected return-value contract (added in fixes-round-1)
// -----------------------------------------------------------------------------

// An in-range SetSelected returns the index it landed on (== the request).
func TestSetSelectedReturnsLandedIndex(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	for i := 0; i < 5; i++ {
		if got := tree.SetSelected(i); got != i {
			t.Fatalf("SetSelected(%d) returned %d, want %d", i, got, i)
		}
	}
}

// An empty tree returns -1 (the "no selectable row" sentinel) and pins selected
// at 0 — the symmetric counterpart to SelectNode's false.
func TestSetSelectedReturnsMinusOneForEmpty(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 10, H: 5})
	if got := tree.SetSelected(3); got != -1 {
		t.Fatalf("empty tree SetSelected(3) returned %d, want -1", got)
	}
	if tree.selected != 0 {
		t.Fatalf("empty tree selected = %d, want 0", tree.selected)
	}
}

// The return value reflects clamping, so a caller can detect an out-of-range
// request without re-reading Selected() (the documented rationale for the int).
func TestSetSelectedReturnReflectsClamp(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c")
	if got := tree.SetSelected(-10); got != 0 {
		t.Fatalf("SetSelected(-10) returned %d, want 0 (clamped low)", got)
	}
	if got := tree.SetSelected(99); got != 2 {
		t.Fatalf("SetSelected(99) returned %d, want 2 (clamped high to last)", got)
	}
	if got := tree.SetSelected(1); got != 1 {
		t.Fatalf("SetSelected(1) returned %d, want 1 (in range)", got)
	}
}

// The returned index is consistent with Selected(): the node at flatten[return]
// is the node Selected() reports. Guards against the return and the index space
// drifting apart on a nested tree.
func TestSetSelectedReturnAgreesWithSelected(t *testing.T) {
	tree, _ := nestedSelectTree()
	want := []string{"r1", "c1a", "c1b", "r2", "r3", "c3a"}
	for i, label := range want {
		got := tree.SetSelected(i)
		if got != i {
			t.Fatalf("SetSelected(%d) returned %d, want %d", i, got, i)
		}
		n := tree.Selected()
		if n == nil || n.Label != label {
			t.Fatalf("after SetSelected(%d): Selected() = %v, want %q", i, n, label)
		}
	}
}

// -----------------------------------------------------------------------------
// Painted highlight bar under scrolling (the actual #206 symptom)
// -----------------------------------------------------------------------------

// After SetSelected scrolls an off-screen row into view, the painted bar must
// land at screen row (selected - offset) — not stay at the old screen row. This
// also exercises the scrollbar path (rows > viewH -> needBar), which round 1's
// paint tests never reached.
func TestSetSelectedBarFollowsAfterScroll(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 4}, // 12 rows > 4 -> scrollbar shown
		"n0", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9", "n10", "n11")
	tree.Component.hasFocus = true

	tree.SetSelected(0)
	app := drawTree(t, tree)
	if got := barRow(t, app, 0); got != tree.SelBG {
		t.Fatalf("baseline: bar should be at screen row 0, BG = %+v", got)
	}

	// Select the last row: offset -> 11-4+1 = 8, so it paints at screen row 3.
	if got := tree.SetSelected(11); got != 11 {
		t.Fatalf("SetSelected(11) returned %d, want 11", got)
	}
	if tree.offset != 8 {
		t.Fatalf("offset = %d, want 8", tree.offset)
	}
	app = drawTree(t, tree)
	wantRow := tree.selected - tree.offset // 11 - 8 = 3
	// Exactly one bar, at the computed screen row, across the whole viewport.
	bars := 0
	for r := 0; r < 4; r++ {
		if barRow(t, app, r) == tree.SelBG {
			bars++
			if r != wantRow {
				t.Errorf("bar at screen row %d, want only %d", r, wantRow)
			}
		}
	}
	if bars != 1 {
		t.Fatalf("found %d selection bars across the viewport, want 1 at row %d", bars, wantRow)
	}
	// The screen row that previously held the bar (row 0) is now body-coloured:
	// it shows n8, not the selection.
	if got := barRow(t, app, 0); got == tree.SelBG {
		t.Fatal("screen row 0 still carries the bar after scrolling (bar did not move)")
	}
}

// Same paint-follows-scroll guarantee via SelectNode.
func TestSelectNodeBarFollowsAfterScroll(t *testing.T) {
	rows := make([]*TreeNode, 12)
	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 4})
	for i := range rows {
		rows[i] = NewTreeNode(fmt.Sprintf("n%d", i))
		tree.AddRoot(rows[i])
	}
	tree.Component.hasFocus = true

	tree.SetSelected(0)
	drawTree(t, tree)
	tree.SelectNode(rows[11])
	if tree.selected != 11 || tree.offset != 8 {
		t.Fatalf("SelectNode(last): selected=%d offset=%d, want 11/8", tree.selected, tree.offset)
	}
	app := drawTree(t, tree)
	wantRow := tree.selected - tree.offset // 3
	if got := barRow(t, app, wantRow); got != tree.SelBG {
		t.Fatalf("bar should be at screen row %d after scroll, BG = %+v", wantRow, got)
	}
	for r := 0; r < 4; r++ {
		if r == wantRow {
			continue
		}
		if got := barRow(t, app, r); got == tree.SelBG {
			t.Fatalf("screen row %d unexpectedly barred after scroll (want only row %d)", r, wantRow)
		}
	}
}

// -----------------------------------------------------------------------------
// SelectNode offset safety on failure / already-visible no-scroll
// -----------------------------------------------------------------------------

// Every SelectNode failure path (nil / foreign node / collapsed-subtree node)
// must leave BOTH selected AND offset untouched, and must not expand a parent.
func TestSelectNodeFailureLeavesOffsetUntouched(t *testing.T) {
	parent := NewTreeNode("parent")
	hidden := parent.AddLeaf("hidden")
	parent.Expanded = false
	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 4})
	tree.AddRoot(parent)
	for i := 0; i < 10; i++ { // 1 (parent) + 10 siblings = 11 visible rows
		tree.AddRoot(NewTreeNode(fmt.Sprintf("s%d", i)))
	}
	drawTree(t, tree)    // viewH = 4
	tree.SetSelected(10) // last sibling -> offset = 10-4+1 = 7
	wantOffset, wantSelected := tree.offset, tree.selected
	if wantOffset != 7 {
		t.Fatalf("precondition offset = %d, want 7", wantOffset)
	}

	other := NewTree(Rect{X: 0, Y: 0, W: 8, H: 3})
	stranger := other.AddRoot(NewTreeNode("stranger"))

	for name, bad := range map[string]*TreeNode{
		"nil":       nil,
		"foreign":   stranger,
		"collapsed": hidden,
	} {
		if ok := tree.SelectNode(bad); ok {
			t.Fatalf("SelectNode(%s) returned true, want false", name)
		}
		if tree.offset != wantOffset {
			t.Fatalf("SelectNode(%s) moved offset %d -> %d", name, wantOffset, tree.offset)
		}
		if tree.selected != wantSelected {
			t.Fatalf("SelectNode(%s) moved selected %d -> %d", name, wantSelected, tree.selected)
		}
	}
	if parent.Expanded {
		t.Fatal("SelectNode(collapsed) expanded the parent as a side effect")
	}
}

// Selecting a node already inside the viewport must not perturb the offset (no
// spurious scroll when nothing is off-screen).
func TestSelectNodeAlreadyVisibleDoesNotScroll(t *testing.T) {
	rows := make([]*TreeNode, 12)
	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 4})
	for i := range rows {
		rows[i] = NewTreeNode(fmt.Sprintf("n%d", i))
		tree.AddRoot(rows[i])
	}
	drawTree(t, tree)
	tree.SetSelected(5) // offset -> 5-4+1 = 2; viewport rows [2,3,4,5]
	if tree.offset != 2 {
		t.Fatalf("precondition offset = %d, want 2", tree.offset)
	}
	// Row 3 is within [2,5] -> already visible.
	if !tree.SelectNode(rows[3]) {
		t.Fatal("SelectNode(visible row 3) returned false")
	}
	if tree.selected != 3 {
		t.Fatalf("selected = %d, want 3", tree.selected)
	}
	if tree.offset != 2 {
		t.Fatalf("SelectNode to an already-visible row moved offset to %d, want 2", tree.offset)
	}
}

// -----------------------------------------------------------------------------
// programmatic-select ↔ keyboard/mouse interplay (no echo, single fire)
// -----------------------------------------------------------------------------

// A programmatic SetSelected must not poison the keyboard path: a subsequent
// KeyDown fires OnSelect exactly once and lands on the next row (no echo from
// the prior SetSelected, no double-fire).
func TestSetSelectedThenKeyboardFiresOnce(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	drawTree(t, tree)
	tree.SetSelected(2)

	fires := 0
	tree.OnSelect = func(*TreeNode) { fires++ }
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown}) // 2 -> 3
	if fires != 1 {
		t.Fatalf("OnSelect fired %d times after one KeyDown, want 1", fires)
	}
	if tree.selected != 3 {
		t.Fatalf("selected = %d, want 3", tree.selected)
	}
}

// Same interplay via SelectNode: the prior programmatic select does not echo and
// the keyboard move fires once.
func TestSelectNodeThenKeyboardFiresOnce(t *testing.T) {
	tree, nodes := nestedSelectTree()
	drawTree(t, tree)
	tree.SelectNode(nodes[1]) // c1a, flat index 1

	fires := 0
	tree.OnSelect = func(*TreeNode) { fires++ }
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown}) // 1 -> 2
	if fires != 1 {
		t.Fatalf("OnSelect fired %d times after one KeyDown, want 1", fires)
	}
	if tree.selected != 2 {
		t.Fatalf("selected = %d, want 2", tree.selected)
	}
}

// A programmatic SetSelected followed by a mouse click on a different row: the
// click commits and fires OnSelect exactly once (the SetSelected contributed
// zero, as guaranteed).
func TestSetSelectedThenClickFiresOnce(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 3}, "a", "b", "c")
	drawTree(t, tree)
	tree.SetSelected(0)

	fires := 0
	tree.OnSelect = func(*TreeNode) { fires++ }
	// Release on screen row 2 -> idx 2.
	tree.handleClick(tree.Component, tui.ClickEvent{X: 0, Y: 2, Down: false})
	if fires != 1 {
		t.Fatalf("OnSelect fired %d times after one click, want 1", fires)
	}
	if tree.selected != 2 {
		t.Fatalf("selected = %d, want 2", tree.selected)
	}
}
