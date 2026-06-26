package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the gogent #490 *turbotui half*: a marker-less, body-clickable tree
// node. Covers the two additive primitives:
//
//   - TreeNode.HideMarker — a parent that paints a blank leading column instead
//     of ▸/▾ while keeping indentation/alignment (rendering opt-out).
//   - Tree.OnToggle — a host hook offered each committed row click BEFORE the
//     default marker-column logic; returning true consumes the click (host owns
//     the Expanded flip) and suppresses both the default toggle and the
//     repeat-click OnActivate.
//
// Coordinate convention: tree bounds {X:0,Y:0,W:30,H:10} unless a test needs a
// scrollbar/narrow view. A root node is depth 0 ⇒ marker column = abs.X+depth*2
// = 0. draw() writes content = marker+" "+Label at abs.X+depth*2, so for a root
// the marker is column 0, the separator column 1, Label[0] column 2 (all marker
// glyphs are display-width 1). A "body" click is X past the marker column
// (X=4); a "marker-column" click is X=0. Reuses the shared helpers drawTree,
// leafTree, clickRow, clickRowUp.

// newHideMarkerParent builds a tree whose single root is a HideMarker parent
// (label pfx) with one leaf child (label pfx+"-child"), starting collapsed.
func newHideMarkerParent(pfx string, expanded bool) (*Tree, *TreeNode) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	parent := NewTreeNode(pfx)
	parent.HideMarker = true
	parent.Expanded = expanded
	parent.AddLeaf(pfx + "-child")
	tree.AddRoot(parent)
	return tree, parent
}

// -----------------------------------------------------------------------------
// Rendering: HideMarker paints a blank leading column; alignment preserved.
// -----------------------------------------------------------------------------

func TestHideMarkerExpandedRendersBlankColumn(t *testing.T) {
	tree, _ := newHideMarkerParent("p", true) // expanded parent
	app := drawTree(t, tree)
	// Row 0 = parent. Leading column (depth 0 → col 0) must be blank, NOT ▾.
	if got := app.ReadCell(0, 0).Ch; got != ' ' {
		t.Errorf("HideMarker expanded parent leading col 0 = %q, want ' ' (no ▾)", got)
	}
	// Label still rendered at the leaf column (col 2).
	if got := app.ReadCell(2, 0).Ch; got != 'p' {
		t.Errorf("HideMarker parent label col 2 = %q, want 'p'", got)
	}
	// Expanded child (row 1, depth 1) is still visible — HideMarker must NOT
	// suppress children. Child marker col = 2, label col = 4.
	if got := app.ReadCell(4, 1).Ch; got != 'p' { // "p-child"[0] == 'p'
		t.Errorf("expanded child label col 4 = %q, want 'p' (children must still show)", got)
	}
}

func TestHideMarkerCollapsedRendersBlankColumn(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false)
	app := drawTree(t, tree)
	if got := app.ReadCell(0, 0).Ch; got != ' ' {
		t.Errorf("HideMarker collapsed parent leading col 0 = %q, want ' ' (no ▸)", got)
	}
	if got := app.ReadCell(2, 0).Ch; got != 'p' {
		t.Errorf("collapsed parent label col 2 = %q, want 'p'", got)
	}
	// Collapsed ⇒ child hidden: row 1 is just blank fill.
	if got := app.ReadCell(0, 1).Ch; got != ' ' {
		t.Errorf("collapsed: row 1 col 0 = %q, want ' ' (child must be hidden)", got)
	}
}

// Regression guard: HideMarker=false (the default) still paints ▸/▾ byte-identically.
func TestDefaultParentStillPaintsMarker(t *testing.T) {
	for _, tc := range []struct {
		name     string
		expanded bool
		want     rune
	}{
		{"collapsed", false, '▸'},
		{"expanded", true, '▾'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
			parent := NewTreeNode("p")
			parent.Expanded = tc.expanded // HideMarker left false (default)
			parent.AddLeaf("c")
			tree.AddRoot(parent)
			app := drawTree(t, tree)
			if got := app.ReadCell(0, 0).Ch; got != tc.want {
				t.Errorf("default parent leading col 0 = %q, want %q", got, tc.want)
			}
			if got := app.ReadCell(2, 0).Ch; got != 'p' {
				t.Errorf("default parent label col 2 = %q, want 'p'", got)
			}
		})
	}
}

// Alignment gate: a HideMarker parent, a normal parent, and a leaf — all at the
// same depth — must put their label in the SAME column. Only the leading glyph
// differs (blank for HideMarker/leaf, ▸/▾ for normal). Hiding the marker must
// not shift anything.
func TestHideMarkerLabelAlignmentAcrossNodeTypes(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	hide := NewTreeNode("H")
	hide.HideMarker = true
	hide.AddLeaf("h") // give it children so it's a real parent
	normal := NewTreeNode("N")
	normal.AddLeaf("n")
	leaf := NewTreeNode("L")
	tree.AddRoot(hide)
	tree.AddRoot(normal)
	tree.AddRoot(leaf)
	app := drawTree(t, tree)

	// Leading column differs by type.
	if got := app.ReadCell(0, 0).Ch; got != ' ' {
		t.Errorf("HideMarker parent col 0 = %q, want ' '", got)
	}
	if got := app.ReadCell(0, 1).Ch; got != '▸' {
		t.Errorf("normal collapsed parent col 0 = %q, want '▸'", got)
	}
	if got := app.ReadCell(0, 2).Ch; got != ' ' {
		t.Errorf("leaf col 0 = %q, want ' '", got)
	}
	// …but every label lands on column 2 ⇒ aligned.
	for row, want := range []rune{'H', 'N', 'L'} {
		if got := app.ReadCell(2, row).Ch; got != want {
			t.Errorf("row %d label col 2 = %q, want %q (alignment)", row, got, want)
		}
	}
}

// Leaves are blank regardless of HideMarker (regression: the field must be a
// no-op on leaves).
func TestHideMarkerNoOpOnLeaf(t *testing.T) {
	for _, hide := range []bool{false, true} {
		t.Run("hide="+boolStr(hide), func(t *testing.T) {
			tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
			n := NewTreeNode("L")
			n.HideMarker = hide // leaf — no children
			tree.AddRoot(n)
			app := drawTree(t, tree)
			if got := app.ReadCell(0, 0).Ch; got != ' ' {
				t.Errorf("leaf HideMarker=%v col 0 = %q, want ' '", hide, got)
			}
			if got := app.ReadCell(2, 0).Ch; got != 'L' {
				t.Errorf("leaf label col 2 = %q, want 'L'", got)
			}
		})
	}
}

// Depth-aware: a HideMarker parent nested at depth 1 paints its blank marker at
// its own depth-derived column (col 2), keeping the depth-1 indent.
func TestHideMarkerNestedDepthIndent(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	root := NewTreeNode("A")
	root.Expanded = true
	nested := NewTreeNode("B")
	nested.HideMarker = true
	nested.AddLeaf("C")
	root.AddLeaf("x") // sibling leaf under A, to anchor depth-1 layout
	root.Add(nested)
	tree.AddRoot(root)
	app := drawTree(t, tree)
	// Row 0 = A (depth 0, normal expanded): ▾ at col 0.
	if got := app.ReadCell(0, 0).Ch; got != '▾' {
		t.Errorf("root A col 0 = %q, want '▾'", got)
	}
	// Row 1 = "x" leaf (depth 1); row 2 = nested HideMarker parent (depth 1).
	// Both at depth 1 ⇒ marker col = 2, label col = 4.
	if got := app.ReadCell(2, 2).Ch; got != ' ' {
		t.Errorf("nested HideMarker parent marker col 2 = %q, want ' ' (depth-1 blank)", got)
	}
	if got := app.ReadCell(4, 2).Ch; got != 'B' {
		t.Errorf("nested HideMarker parent label col 4 = %q, want 'B'", got)
	}
	// Same columns as the depth-1 leaf sibling on row 1 (alignment by depth).
	if got := app.ReadCell(4, 1).Ch; got != 'x' {
		t.Errorf("depth-1 leaf label col 4 = %q, want 'x'", got)
	}
}

// Truncation must be unaffected: a HideMarker parent and a leaf with the same
// label truncate to identical glyphs (HideMarker only swaps the leading glyph,
// which is the same width-1 blank for both). Only the rune is compared — the
// rows differ in BG because row 0 is the selected (bar) row, which is unrelated
// to truncation.
func TestHideMarkerTruncationMatchesLeaf(t *testing.T) {
	const label = "ABCDEFGHIJKLMNOP"              // long enough to truncate in W=6
	tree := NewTree(Rect{X: 0, Y: 0, W: 6, H: 5}) // narrow; few rows ⇒ no scrollbar
	hide := NewTreeNode(label)
	hide.HideMarker = true
	hide.AddLeaf("z")
	leaf := NewTreeNode(label)
	tree.AddRoot(hide)
	tree.AddRoot(leaf)
	app := drawTree(t, tree)
	for x := 0; x < 6; x++ {
		if h, l := app.ReadCell(x, 0).Ch, app.ReadCell(x, 1).Ch; h != l {
			t.Errorf("col %d: HideMarker glyph %q != leaf glyph %q (truncation must match)", x, h, l)
		}
	}
}

// -----------------------------------------------------------------------------
// Click path: OnToggle lets a marker-less node toggle from a body click.
// -----------------------------------------------------------------------------

// Core goal: a body click (X past the marker column) on a HideMarker node toggles
// Expanded via the host OnToggle hook.
func TestOnToggleBodyClickTogglesHideMarkerNode(t *testing.T) {
	tree, parent := newHideMarkerParent("p", false) // collapsed
	drawTree(t, tree)
	tree.OnToggle = func(n *TreeNode, _ tui.ClickEvent) bool {
		n.Expanded = !n.Expanded
		return true
	}
	clickRowUp(tree, 4, 0) // body click, X=4 > markerCol(0)
	if !parent.Expanded {
		t.Fatalf("body click via OnToggle: parent.Expanded = false, want true")
	}
	// Toggling back works too.
	clickRowUp(tree, 4, 0)
	if parent.Expanded {
		t.Fatalf("second body click: parent.Expanded = true, want false")
	}
}

// Consuming the click (OnToggle returns true) suppresses the repeat-click
// OnActivate, even when the row was already selected.
func TestOnToggleConsumeSuppressesOnActivate(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false)
	drawTree(t, tree) // selected defaults to row 0
	tree.OnToggle = func(n *TreeNode, _ tui.ClickEvent) bool {
		n.Expanded = !n.Expanded
		return true
	}
	activates := 0
	tree.OnActivate = func(*TreeNode) { activates++ }

	clickRowUp(tree, 4, 0) // body click on already-selected row 0
	if activates != 0 {
		t.Errorf("OnActivate fired %d× on an OnToggle-consumed click, want 0", activates)
	}
}

// The hook is tried BEFORE the default marker-column logic: returning true
// (consume) suppresses the default toggle even on a click that would otherwise
// toggle (the marker column); returning false lets the default run.
func TestOnToggleTriedBeforeDefaultConsumeSuppresses(t *testing.T) {
	// (a) consume without flipping → default suppressed, Expanded stays put.
	t.Run("consume_suppresses_default", func(t *testing.T) {
		tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
		parent := NewTreeNode("p") // HideMarker false; collapsed
		parent.AddLeaf("c")
		tree.AddRoot(parent)
		drawTree(t, tree)
		tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { return true } // consume, no flip
		clickRowUp(tree, 0, 0)                                               // X=0 ⇒ would default-toggle the marker column
		if parent.Expanded {
			t.Fatalf("default marker toggle ran despite OnToggle consume; Expanded = true, want false")
		}
	})
	// (b) return false → default marker-column toggle runs as before.
	t.Run("false_lets_default_run", func(t *testing.T) {
		tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
		parent := NewTreeNode("p")
		parent.AddLeaf("c")
		tree.AddRoot(parent)
		drawTree(t, tree)
		tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { return false }
		clickRowUp(tree, 0, 0) // marker column
		if !parent.Expanded {
			t.Fatalf("OnToggle=false did not let default marker toggle run; Expanded = false, want true")
		}
	})
}

// OnToggle returning false is a full fall-through: on a body click (past the
// marker column) the default does not toggle, and OnActivate may fire on a
// repeat click — exactly the pre-existing behaviour.
func TestOnToggleFalseFallsThroughFully(t *testing.T) {
	tree, parent := newHideMarkerParent("p", false)
	drawTree(t, tree) // row 0 selected
	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { return false }
	activates := 0
	tree.OnActivate = func(*TreeNode) { activates++ }

	clickRowUp(tree, 4, 0) // body click; default can't toggle from body
	if parent.Expanded {
		t.Errorf("OnToggle=false + body click: Expanded = true, want false (default body click never toggles)")
	}
	if activates != 1 {
		t.Errorf("OnActivate fired %d×, want 1 (false ⇒ full default repeat-click activate)", activates)
	}
}

// Regression: with HideMarker=false and OnToggle=nil, a body click does NOT
// toggle (only the marker column does) — the pre-existing behaviour is preserved.
func TestDefaultBodyClickDoesNotToggle(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	parent := NewTreeNode("p")
	parent.AddLeaf("c")
	tree.AddRoot(parent)
	drawTree(t, tree)

	clickRowUp(tree, 4, 0) // body
	if parent.Expanded {
		t.Fatalf("default body click toggled Expanded; want unchanged (only marker column toggles)")
	}
	clickRowUp(tree, 0, 0) // marker column
	if !parent.Expanded {
		t.Fatalf("default marker-column click did not toggle Expanded; want true")
	}
}

// Guards against the relaxed-guard alternative leaking in: with HideMarker=true
// but OnToggle=nil, a body click must NOT toggle (no hook ⇒ no body toggle). The
// (invisible) marker column still toggles by default.
func TestHideMarkerWithoutOnToggleBodyClickDoesNotToggle(t *testing.T) {
	tree, parent := newHideMarkerParent("p", false)
	drawTree(t, tree) // OnToggle stays nil

	clickRowUp(tree, 4, 0) // body — no hook, must not toggle
	if parent.Expanded {
		t.Fatalf("HideMarker + nil OnToggle: body click toggled Expanded; want false (no relaxed guard)")
	}
	clickRowUp(tree, 0, 0) // marker column — default path still works even when hidden
	if !parent.Expanded {
		t.Fatalf("HideMarker marker-column click did not toggle; want true (default unchanged)")
	}
}

// Contract: OnToggle receives the clicked node pointer and the raw click event
// (so the host can do its own suffix-region hit testing).
func TestOnToggleReceivesClickedNodeAndEvent(t *testing.T) {
	tree, parent := newHideMarkerParent("p", false)
	drawTree(t, tree)
	var gotNode *TreeNode
	var gotEv tui.ClickEvent
	tree.OnToggle = func(n *TreeNode, ev tui.ClickEvent) bool {
		gotNode, gotEv = n, ev
		return true
	}
	clickRowUp(tree, 5, 0) // X=5, Y=0
	if gotNode != parent {
		t.Errorf("OnToggle node = %p, want parent %p", gotNode, parent)
	}
	if gotEv.X != 5 || gotEv.Y != 0 {
		t.Errorf("OnToggle event = (X=%d,Y=%d), want (5,0)", gotEv.X, gotEv.Y)
	}
}

// Seam: consuming via OnToggle still moves the selection and still fires
// OnSelect/OnSelectMouse for the clicked row — the host keeps its selection and
// click-detection signals.
func TestOnToggleConsumeStillFiresSelectAndSelectMouse(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false)
	// Second root so the click is a real selection move onto row 0 from row 1.
	tree.AddRoot(NewTreeNode("other"))
	tree.SetSelected(1)
	drawTree(t, tree)

	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { return true }
	selects, mouseSelects := 0, 0
	var mouseNode *TreeNode
	tree.OnSelect = func(*TreeNode) { selects++ }
	tree.OnSelectMouse = func(n *TreeNode) { mouseSelects++; mouseNode = n }

	clickRowUp(tree, 4, 0) // click row 0 (the HideMarker parent)
	if selects != 1 {
		t.Errorf("OnSelect fired %d×, want 1 (selection must still move)", selects)
	}
	if mouseSelects != 1 || mouseNode != tree.Roots[0] {
		t.Errorf("OnSelectMouse = (%d×, %v), want (1×, parent)", mouseSelects, mouseNode)
	}
}

// End-to-end host toggle: OnToggle flips Expanded; a subsequent redraw reveals
// the children (flatten() keys off Expanded, not the marker).
func TestOnToggleHostFlipShowsChildrenOnRedraw(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false)
	drawTree(t, tree)
	tree.OnToggle = func(n *TreeNode, _ tui.ClickEvent) bool {
		n.Expanded = !n.Expanded
		return true
	}
	clickRowUp(tree, 4, 0)

	app := drawTree(t, tree) // redraw
	// Row 1 is now the child (depth 1): label at col 4.
	if got := app.ReadCell(4, 1).Ch; got != 'p' {
		t.Errorf("after host toggle+redraw child label col 4 = %q, want 'p'", got)
	}
}

// The hook is offered only on the committed (release) click — never on the press
// — and exactly once per full click.
func TestOnToggleInvokedOncePerClickNotOnPress(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false)
	drawTree(t, tree)
	calls := 0
	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { calls++; return true }

	tree.handleClick(tree.Component, tui.ClickEvent{X: 4, Y: 0, Button: tui.MouseLeft, Down: true}) // press
	if calls != 0 {
		t.Errorf("OnToggle fired %d× on press, want 0 (release only)", calls)
	}
	clickRow(tree, 4, 0) // press + release
	if calls != 1 {
		t.Errorf("OnToggle fired %d× for a full click, want 1", calls)
	}
}

// OnToggle is mouse-only: keyboard Left/Right/Space toggle Expanded directly and
// never invoke the hook (the host keeps suffix state via its own redraw).
func TestOnToggleNotInvokedFromKeyboard(t *testing.T) {
	tree, parent := newHideMarkerParent("p", false)
	drawTree(t, tree)
	calls := 0
	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { calls++; return true }

	for _, ev := range []tui.TypeEvent{
		{Key: tui.KeyRight},
		{Key: tui.KeyLeft},
		{Key: tui.KeyRune, Rune: ' '},
	} {
		tree.handleType(tree.Component, ev)
	}
	if calls != 0 {
		t.Errorf("OnToggle fired %d× from keyboard, want 0 (mouse-only)", calls)
	}
	// Right expanded, Left collapsed, Space expanded again.
	if !parent.Expanded {
		t.Errorf("keyboard did not toggle HideMarker node; Expanded = false, want true")
	}
}

// Re-entrancy safety: an OnToggle that reentrantly moves the selection (a host
// snap) must not corrupt OnSelectMouse — it still reports the clicked node — and
// must not panic even though it flips Expanded (changing the row count) before
// the stale captured rows are used.
func TestOnToggleReentrantSelectSafe(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	first := NewTreeNode("first")
	first.HideMarker = true
	first.AddLeaf("f-child")
	second := NewTreeNode("second")
	second.HideMarker = true
	second.AddLeaf("s-child")
	tree.AddRoot(first)
	tree.AddRoot(second)
	drawTree(t, tree)

	var mouseNode *TreeNode
	tree.OnSelectMouse = func(n *TreeNode) { mouseNode = n }
	// OnToggle for the clicked node snaps selection to the OTHER node and flips.
	tree.OnToggle = func(n *TreeNode, _ tui.ClickEvent) bool {
		tree.SelectNode(first) // snap away from the clicked 'second'
		n.Expanded = !n.Expanded
		return true
	}

	clickRowUp(tree, 4, 1) // click row 1 = 'second'
	if mouseNode != second {
		t.Errorf("OnSelectMouse reported %v, want the clicked 'second' (snap must not redirect)", mouseNode)
	}
	if tree.Selected() != first {
		t.Errorf("after snap selected = %v, want 'first'", tree.Selected())
	}
}

// A scrollbar-column click (when rows > view height) is handled before the hook:
// OnToggle must not fire.
func TestOnToggleNotInvokedForScrollbarClick(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c", "d", "e", "f", "g", "h") // 8 rows > 5
	drawTree(t, tree)
	calls := 0
	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { calls++; return true }
	clickRowUp(tree, 11, 2) // X = abs.X+abs.W-1 = scrollbar column
	if calls != 0 {
		t.Errorf("OnToggle fired %d× on scrollbar column, want 0", calls)
	}
}

// Out-of-bounds and empty-row clicks never reach OnToggle.
func TestOnToggleNotInvokedForOutOfBoundsOrEmptyClick(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false) // 1 row
	drawTree(t, tree)
	calls := 0
	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { calls++; return true }

	tree.handleClick(tree.Component, tui.ClickEvent{X: 50, Y: 50, Button: tui.MouseLeft, Down: false}) // outside bounds
	if calls != 0 {
		t.Errorf("OnToggle fired %d× on out-of-bounds click, want 0", calls)
	}
	tree.handleClick(tree.Component, tui.ClickEvent{X: 4, Y: 5, Button: tui.MouseLeft, Down: false}) // valid X/Y but empty row (past content)
	if calls != 0 {
		t.Errorf("OnToggle fired %d× on empty row, want 0", calls)
	}
}

// A HideMarker node with zero children behaves as a leaf (blank regardless); no
// panic, no spurious marker.
func TestHideMarkerZeroChildrenBehavesAsLeaf(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 10})
	n := NewTreeNode("solo")
	n.HideMarker = true // but no children
	tree.AddRoot(n)
	app := drawTree(t, tree) // must not panic
	if got := app.ReadCell(0, 0).Ch; got != ' ' {
		t.Errorf("HideMarker childless node col 0 = %q, want ' '", got)
	}
	if got := app.ReadCell(2, 0).Ch; got != 's' {
		t.Errorf("childless node label col 2 = %q, want 's'", got)
	}
}

// OnToggle is offered even for a leaf row: a consuming handler suppresses the
// repeat-click OnActivate (consistent with the parent path).
func TestOnToggleOnLeafRowConsumesAndSuppressesActivate(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 30, H: 10}, "only")
	drawTree(t, tree) // row 0 selected
	calls := 0
	tree.OnToggle = func(*TreeNode, tui.ClickEvent) bool { calls++; return true }
	activates := 0
	tree.OnActivate = func(*TreeNode) { activates++ }

	clickRowUp(tree, 4, 0)
	if calls != 1 {
		t.Errorf("OnToggle fired %d× on leaf row, want 1 (offered for every row)", calls)
	}
	if activates != 0 {
		t.Errorf("OnActivate fired %d× on consumed leaf click, want 0", activates)
	}
}

// -----------------------------------------------------------------------------
// Keyboard: Left/Right/Space keep toggling a HideMarker node; marker stays hidden.
// -----------------------------------------------------------------------------

func TestKeyboardTogglesHideMarkerNode(t *testing.T) {
	tree, parent := newHideMarkerParent("p", false) // collapsed
	drawTree(t, tree)

	// Right on a collapsed parent expands it.
	if !tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyRight}) {
		t.Fatal("KeyRight not handled")
	}
	if !parent.Expanded {
		t.Errorf("KeyRight: Expanded = false, want true")
	}
	// Left on an expanded parent collapses it.
	if !tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyLeft}) {
		t.Fatal("KeyLeft not handled")
	}
	if parent.Expanded {
		t.Errorf("KeyLeft: Expanded = true, want false")
	}
	// Space toggles.
	if !tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: ' '}) {
		t.Fatal("Space not handled")
	}
	if !parent.Expanded {
		t.Errorf("Space: Expanded = false, want true")
	}
}

// A keyboard toggle must not make the marker reappear: HideMarker persists across
// expand/collapse, so the leading column stays blank after a redraw.
func TestKeyboardToggleKeepsMarkerHidden(t *testing.T) {
	tree, _ := newHideMarkerParent("p", false)
	drawTree(t, tree)
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyRight}) // expand
	app := drawTree(t, tree)
	if got := app.ReadCell(0, 0).Ch; got != ' ' {
		t.Errorf("after keyboard expand HideMarker parent col 0 = %q, want ' ' (must stay hidden)", got)
	}
}

// boolStr keeps table-test names readable without importing strconv.
func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
