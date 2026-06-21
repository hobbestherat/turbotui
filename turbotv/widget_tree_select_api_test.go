package tv

import (
	"fmt"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the programmatic tree-selection API added for gogent #206:
//   - SetSelected(index int)
//   - SelectNode(node *TreeNode) bool
//
// The bug: the sidebar highlight (tv.Tree.selected) could only be moved by the
// widget's own keyboard/mouse handlers, so gogent could not sync the tree to an
// externally focused session. These two functions are the public counterpart and
// MUST mirror handleClick/handleType (clamp + scroll into view) while NEVER
// firing OnSelect/OnActivate (otherwise a caller listening on OnSelect echoes
// straight back and loops).

// -----------------------------------------------------------------------------
// fixtures
// -----------------------------------------------------------------------------

// nestedSelectTree builds a small, fully-expanded tree whose visible (flattened)
// row order is known by construction:
//
//	r1          (0)
//	  c1a       (1)
//	  c1b       (2)
//	r2          (3)
//	r3          (4)
//	  c3a       (5)
//
// It also returns the nodes so tests can address them by pointer.
func nestedSelectTree() (*Tree, []*TreeNode) {
	root1 := NewTreeNode("r1")
	c1a := root1.AddLeaf("c1a")
	c1b := root1.AddLeaf("c1b")
	root1.Expanded = true

	root2 := NewTreeNode("r2")

	root3 := NewTreeNode("r3")
	c3a := root3.AddLeaf("c3a")
	root3.Expanded = true

	tree := NewTree(Rect{X: 0, Y: 0, W: 20, H: 10})
	tree.AddRoot(root1)
	tree.AddRoot(root2)
	tree.AddRoot(root3)

	return tree, []*TreeNode{root1, c1a, c1b, root2, root3, c3a}
}

// barRow reports the background colour of the first column of screen row y after
// a draw. It is the readout for "where is the highlight bar painted?".
func barRow(t *testing.T, app *tui.App, y int) tui.Color {
	t.Helper()
	return app.ReadCell(0, y).BG
}

// -----------------------------------------------------------------------------
// SetSelected
// -----------------------------------------------------------------------------

// SetSelected's index space must be exactly Selected()'s position in the
// flattened (collapse-aware) row list — not an offset into Roots. A bug that
// indexed into Roots instead would mis-select on any nested tree.
func TestSetSelectedIndexSpaceMatchesSelected(t *testing.T) {
	tree, _ := nestedSelectTree()
	want := []string{"r1", "c1a", "c1b", "r2", "r3", "c3a"}
	for i, label := range want {
		tree.SetSelected(i)
		got := tree.Selected()
		if got == nil {
			t.Fatalf("SetSelected(%d): Selected() is nil, want %q", i, label)
		}
		if got.Label != label {
			t.Fatalf("SetSelected(%d): Selected() = %q, want %q", i, got.Label, label)
		}
	}
}

// SetSelected moves the underlying selected index for a simple leaf tree and
// Selected() reads it back.
func TestSetSelectedMovesHighlight(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	for i := 0; i < 5; i++ {
		tree.SetSelected(i)
		if tree.selected != i {
			t.Fatalf("SetSelected(%d): tree.selected = %d", i, tree.selected)
		}
		if n := tree.Selected(); n == nil || n.Label != string(rune('a'+i)) {
			t.Fatalf("SetSelected(%d): Selected() = %v, want %q", i, n, string(rune('a'+i)))
		}
	}
}

// Negative and over-large indices clamp into range rather than panicking or
// going out of bounds (mirrors clampInt usage in handleType).
func TestSetSelectedClampsOutOfRange(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c")
	drawTree(t, tree) // establishes viewH; not strictly required but realistic

	tree.SetSelected(-10)
	if tree.selected != 0 {
		t.Fatalf("SetSelected(-10): selected = %d, want 0", tree.selected)
	}
	tree.SetSelected(999)
	if tree.selected != 2 {
		t.Fatalf("SetSelected(999): selected = %d, want 2 (last)", tree.selected)
	}
	// Selected() must remain in range after an over-clamp (no nil / no panic).
	if n := tree.Selected(); n == nil || n.Label != "c" {
		t.Fatalf("after SetSelected(999): Selected() = %v, want c", n)
	}
}

// An empty tree is a no-op that pins selection at 0 and never panics, matching
// the documented contract and Selected()'s nil return.
func TestSetSelectedEmptyTreeIsNoOp(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 10, H: 5})
	tree.SetSelected(3)
	if tree.selected != 0 {
		t.Fatalf("empty tree SetSelected(3): selected = %d, want 0", tree.selected)
	}
	if got := tree.Selected(); got != nil {
		t.Fatalf("empty tree Selected() = %v, want nil", got)
	}
}

// A single-row tree clamps every SetSelected to index 0.
func TestSetSelectedSingleRowAlwaysZero(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 8, H: 3}, "only")
	tree.SetSelected(0)
	if tree.selected != 0 || tree.Selected().Label != "only" {
		t.Fatalf("single-row SetSelected(0) mis-selected: %d", tree.selected)
	}
	tree.SetSelected(5)
	if tree.selected != 0 {
		t.Fatalf("single-row SetSelected(5): selected = %d, want 0", tree.selected)
	}
}

// CRITICAL (gogent #206 contract): SetSelected must NOT fire OnSelect or
// OnActivate. A caller that syncs the tree from an OnSelect listener would
// otherwise echo straight back into SetSelected and loop.
func TestSetSelectedDoesNotFireOnSelectOrOnActivate(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	selects, activates := 0, 0
	tree.OnSelect = func(*TreeNode) { selects++ }
	tree.OnActivate = func(*TreeNode) { activates++ }

	tree.SetSelected(2)
	tree.SetSelected(4)
	tree.SetSelected(0)
	if selects != 0 {
		t.Fatalf("SetSelected fired OnSelect %d times, want 0", selects)
	}
	if activates != 0 {
		t.Fatalf("SetSelected fired OnActivate %d times, want 0", activates)
	}
	// Sanity: the callbacks ARE wired (a real move fires exactly once), so a
	// future regression that "forgets" to skip firing is caught rather than
	// passing vacuously because the callbacks were never reachable.
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown})
	if selects != 1 {
		t.Fatalf("control: KeyDown should fire OnSelect once, got %d", selects)
	}
}

// SetSelected scrolls the new selection into view (mirrors ensureVisible in the
// keyboard path), both downward past the bottom and back up to the top.
func TestSetSelectedScrollsIntoView(t *testing.T) {
	// 12 rows, viewport height 4.
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 4},
		"n0", "n1", "n2", "n3", "n4", "n5", "n6", "n7", "n8", "n9", "n10", "n11")
	drawTree(t, tree) // viewH = 4

	// Selecting the last row scrolls it to the bottom of the viewport:
	// offset = 11 - 4 + 1 = 8.
	tree.SetSelected(11)
	if tree.selected != 11 {
		t.Fatalf("SetSelected(11): selected = %d", tree.selected)
	}
	if tree.offset != 8 {
		t.Fatalf("SetSelected(11): offset = %d, want 8 (last row at viewport bottom)", tree.offset)
	}

	// Selecting the first row scrolls back to the top.
	tree.SetSelected(0)
	if tree.offset != 0 {
		t.Fatalf("SetSelected(0): offset = %d, want 0", tree.offset)
	}

	// Selecting a row already inside the viewport does not move the offset.
	tree.SetSelected(5) // offset jumps to 5-4+1 = 2
	tree.SetSelected(4) // row 4 in [2,5] -> already visible, offset unchanged
	if tree.offset != 2 {
		t.Fatalf("SetSelected to an already-visible row moved offset to %d, want 2", tree.offset)
	}
}

// Before any draw, viewH is 0 so ensureVisible cannot scroll. SetSelected must
// still record the (clamped) selection so the widget is in the right state;
// the first draw bounds the offset but — by design — does NOT follow the
// selection (only ensureVisible on input/SetSelected does, and it needs viewH).
// Once a draw has established viewH, a further SetSelected scrolls normally.
func TestSetSelectedBeforeFirstDrawRecordsButDoesNotScroll(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 4},
		"a", "b", "c", "d", "e", "f", "g", "h")
	tree.SetSelected(5)
	if tree.selected != 5 {
		t.Fatalf("pre-draw SetSelected(5): selected = %d, want 5", tree.selected)
	}
	// First draw bounds the offset; it does not follow the (off-screen) selection.
	drawTree(t, tree)
	if tree.offset != 0 {
		t.Fatalf("first draw offset = %d, want 0 (draw never follows selection)", tree.offset)
	}
	if tree.selected != 5 {
		t.Fatalf("draw re-clamped selected to %d, want 5", tree.selected)
	}
	// Now that viewH is established, a follow-up SetSelected scrolls into view.
	tree.SetSelected(5)
	if tree.offset != 2 { // 5 - 4 + 1
		t.Fatalf("post-viewH SetSelected(5): offset = %d, want 2", tree.offset)
	}
	if n := tree.Selected(); n == nil || n.Label != "f" {
		t.Fatalf("Selected() = %v, want f", n)
	}
}

// End-to-end (the actual gogent #206 symptom): after SetSelected the painted
// highlight bar moves to the new row, and leaves the old row.
func TestSetSelectedPaintsHighlightBarAtNewRow(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 10, H: 5}, "alpha", "beta", "gamma")
	tree.Component.hasFocus = true

	tree.SetSelected(2) // gamma
	app := drawTree(t, tree)
	// Row 2 must carry the selection background; row 0 must not.
	if got := barRow(t, app, 2); got != tree.SelBG {
		t.Fatalf("after SetSelected(2) row 2 BG = %+v, want SelBG %+v", got, tree.SelBG)
	}
	if got := barRow(t, app, 0); got == tree.SelBG {
		t.Fatalf("after SetSelected(2) row 0 still painted as SelBG (bar did not move)")
	}
}

// -----------------------------------------------------------------------------
// SelectNode
// -----------------------------------------------------------------------------

// SelectNode moves the highlight to a visible node and returns true; the
// resulting index agrees with the SetSelected/Selected index space.
func TestSelectNodeFindsVisibleNode(t *testing.T) {
	tree, nodes := nestedSelectTree()
	wantIdx := []int{0, 1, 2, 3, 4, 5}
	for i, node := range nodes {
		// Park selection on a different row first so a "true" return reflects a
		// real move, not a no-op on an already-selected node.
		park := wantIdx[i] + 1
		if park >= len(wantIdx) {
			park = 0
		}
		tree.SetSelected(park)
		ok := tree.SelectNode(node)
		if !ok {
			t.Fatalf("SelectNode(%q): returned false, want true", node.Label)
		}
		if tree.selected != wantIdx[i] {
			t.Fatalf("SelectNode(%q): selected = %d, want %d", node.Label, tree.selected, wantIdx[i])
		}
		if got := tree.Selected(); got != node {
			t.Fatalf("SelectNode(%q): Selected() != node", node.Label)
		}
	}
}

// SelectNode matches by POINTER IDENTITY, never by label/Data equality. Two
// distinct nodes with identical content must not be confused — gogent stores
// live *TreeNode pointers per session, so a stale/rebuilt node must not match.
func TestSelectNodeMatchesByPointerIdentity(t *testing.T) {
	tree, nodes := nestedSelectTree()
	live := nodes[2] // "c1b"

	// A freshly built node with the same label and same Data value is NOT live.
	clone := &TreeNode{Label: live.Label, Data: live.Data}
	if ok := tree.SelectNode(clone); ok {
		t.Fatal("SelectNode matched a same-label/same-Data clone, want pointer identity only")
	}
	// The live pointer matches and moves selection.
	tree.SetSelected(0)
	if ok := tree.SelectNode(live); !ok || tree.selected != 2 {
		t.Fatalf("SelectNode(live) ok=%v selected=%d, want ok=true selected=2", ok, tree.selected)
	}
}

// A nil node is a clean no-op: returns false, leaves selection untouched, no
// panic.
func TestSelectNodeNilReturnsFalse(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	tree.SetSelected(2)
	before := tree.selected
	if ok := tree.SelectNode(nil); ok {
		t.Fatal("SelectNode(nil): returned true, want false")
	}
	if tree.selected != before {
		t.Fatalf("SelectNode(nil): moved selection %d -> %d", before, tree.selected)
	}
}

// A node that is not part of this tree (e.g. belongs to another tree) is not
// matched; selection is unchanged.
func TestSelectNodeNotInTreeReturnsFalse(t *testing.T) {
	tree, _ := nestedSelectTree()
	tree.SetSelected(3)

	other := NewTree(Rect{X: 0, Y: 0, W: 12, H: 5})
	stranger := other.AddRoot(NewTreeNode("stranger"))

	if ok := tree.SelectNode(stranger); ok {
		t.Fatal("SelectNode matched a node from a different tree, want false")
	}
	if tree.selected != 3 {
		t.Fatalf("SelectNode(foreign): moved selection to %d, want unchanged 3", tree.selected)
	}
}

// A node hidden inside a COLLAPSED subtree is not visible and therefore not
// matched: SelectNode returns false, leaves selection unchanged, and MUST NOT
// expand the parent to reveal it (the documented contract).
func TestSelectNodeCollapsedSubtreeReturnsFalse(t *testing.T) {
	parent := NewTreeNode("parent")
	hidden := parent.AddLeaf("hidden")
	parent.Expanded = false // hidden is not in the flattened rows

	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 5})
	tree.AddRoot(parent)
	tree.AddRoot(NewTreeNode("sibling"))
	tree.SetSelected(1) // on "sibling"

	if ok := tree.SelectNode(hidden); ok {
		t.Fatal("SelectNode matched a node inside a collapsed subtree, want false")
	}
	if tree.selected != 1 {
		t.Fatalf("SelectNode(hidden): moved selection to %d, want unchanged 1", tree.selected)
	}
	if parent.Expanded {
		t.Fatal("SelectNode must NOT expand a collapsed parent as a side effect")
	}
}

// The sibling of the collapsed-subtree case: an EXPANDED parent's child is
// visible and selectable.
func TestSelectNodeExpandedSubtreeFindsChild(t *testing.T) {
	parent := NewTreeNode("parent")
	child := parent.AddLeaf("child")
	parent.Expanded = true

	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 5})
	tree.AddRoot(parent)
	tree.SetSelected(0) // on parent

	if ok := tree.SelectNode(child); !ok {
		t.Fatal("SelectNode(child of expanded parent) returned false, want true")
	}
	if tree.selected != 1 {
		t.Fatalf("SelectNode(child): selected = %d, want 1", tree.selected)
	}
}

// CRITICAL: SelectNode must NOT fire OnSelect/OnActivate even on a successful
// match (same echo-loop rationale as SetSelected).
func TestSelectNodeDoesNotFireOnSelectOrOnActivate(t *testing.T) {
	tree, nodes := nestedSelectTree()
	selects, activates := 0, 0
	tree.OnSelect = func(*TreeNode) { selects++ }
	tree.OnActivate = func(*TreeNode) { activates++ }

	tree.SetSelected(0)
	tree.SelectNode(nodes[4]) // r3
	tree.SelectNode(nodes[5]) // c3a
	tree.SelectNode(nodes[0]) // r1
	if selects != 0 {
		t.Fatalf("SelectNode fired OnSelect %d times, want 0", selects)
	}
	if activates != 0 {
		t.Fatalf("SelectNode fired OnActivate %d times, want 0", activates)
	}
}

// SelectNode scrolls a far-down node into view.
func TestSelectNodeScrollsIntoView(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 4})
	far := make([]*TreeNode, 12)
	for i := range far {
		far[i] = NewTreeNode(labelI(i))
		tree.AddRoot(far[i])
	}
	drawTree(t, tree) // viewH = 4

	tree.SetSelected(0)
	if ok := tree.SelectNode(far[11]); !ok {
		t.Fatal("SelectNode(last) returned false")
	}
	if tree.selected != 11 {
		t.Fatalf("SelectNode(last): selected = %d, want 11", tree.selected)
	}
	if tree.offset != 8 { // 11 - 4 + 1
		t.Fatalf("SelectNode(last): offset = %d, want 8", tree.offset)
	}
}

// SelectNode reports true and keeps selection on a node that is already
// selected (it is still "found"); it must not be treated as not-found.
func TestSelectNodeAlreadySelectedReturnsTrue(t *testing.T) {
	tree, nodes := nestedSelectTree()
	tree.SetSelected(3) // r2
	if ok := tree.SelectNode(nodes[3]); !ok {
		t.Fatal("SelectNode on the already-selected node returned false, want true")
	}
	if tree.selected != 3 {
		t.Fatalf("SelectNode(already selected): selected = %d, want 3", tree.selected)
	}
}

// End-to-end: SelectNode moves the painted highlight bar to the chosen node's
// row and off the previous row.
func TestSelectNodePaintsHighlightBarAtNewRow(t *testing.T) {
	tree, nodes := nestedSelectTree() // 6 visible rows, viewport H=10 (all fit)
	tree.Component.hasFocus = true
	tree.SetSelected(0)
	app := drawTree(t, tree)
	if got := barRow(t, app, 0); got != tree.SelBG {
		t.Fatalf("precondition: row 0 BG = %+v, want SelBG", got)
	}

	tree.SelectNode(nodes[4]) // r3 at flat index 4
	app = drawTree(t, tree)
	if got := barRow(t, app, 4); got != tree.SelBG {
		t.Fatalf("after SelectNode(r3) row 4 BG = %+v, want SelBG", got)
	}
	if got := barRow(t, app, 0); got == tree.SelBG {
		t.Fatalf("after SelectNode(r3) row 0 still painted as SelBG (bar did not move)")
	}
}

// labelI returns a stable label for index i.
func labelI(i int) string { return fmt.Sprintf("n%d", i) }
