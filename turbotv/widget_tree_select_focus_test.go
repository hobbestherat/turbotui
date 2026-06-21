package tv

import (
	"fmt"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Round 3: the focus-state dimension of gogent #206. When Workbench.Focus raises
// a session window, focus goes to the WINDOW — the "Sessions & Agents" sidebar
// tree is UNFOCUSED, so the highlight that must follow is the DIM bar
// (SelBGUnfocused), not the bright one. Rounds 1-2 painted every bar with
// hasFocus=true; these tests cover the unfocused state that matches the real bug,
// plus the two minor gaps (OnActivate after a programmatic select, SelectNode
// scrolling back up).

// assertOnlyColoredBarAt asserts that exactly one of the first `count` screen
// rows carries `wantBG`, and it is the row at wantIdx. It is the colour-aware
// generalisation of assertOnlyBarAt (which is hardwired to SelBG), so it can
// demand the dim SelBGUnfocused bar specifically.
func assertOnlyColoredBarAt(t *testing.T, app *tui.App, tree *Tree, count, wantIdx int, wantBG tui.Color) {
	t.Helper()
	bars := 0
	for r := 0; r < count; r++ {
		if app.ReadCell(0, r).BG == wantBG {
			bars++
			if r != wantIdx {
				t.Errorf("coloured bar at screen row %d, want only at %d", r, wantIdx)
			}
		}
	}
	if bars != 1 {
		t.Fatalf("found %d coloured bars among %d rows, want exactly 1 at row %d", bars, count, wantIdx)
	}
}

// requireDistinctBarColors fails loudly if the theme makes the dim bar
// indistinguishable from either the body or the bright bar, so a focus-state
// assertion can never pass vacuously.
func requireDistinctBarColors(t *testing.T, tree *Tree) {
	t.Helper()
	if tree.SelBGUnfocused == tree.BG {
		t.Fatal("precondition: SelBGUnfocused must differ from body BG, else the dim bar is invisible")
	}
	if tree.SelBG == tree.SelBGUnfocused {
		t.Fatal("precondition: focused SelBG must differ from SelBGUnfocused, else focus state is ambiguous")
	}
}

// -----------------------------------------------------------------------------
// Unfocused tree: the dim highlight bar follows a programmatic select
// -----------------------------------------------------------------------------

// With the tree UNFOCUSED (the real sidebar state during Workbench.Focus),
// SetSelected must move the DIM (SelBGUnfocused) bar to the new row and clear the
// old one — not paint nothing, and not leak the bright SelBG.
func TestSetSelectedBarFollowsWhenTreeUnfocused(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 10, H: 5}, "a", "b", "c", "d", "e")
	tree.Component.hasFocus = false // gogent sidebar during a window focus
	requireDistinctBarColors(t, tree)

	tree.SetSelected(0)
	app := drawTree(t, tree)
	if got := barRow(t, app, 0); got != tree.SelBGUnfocused {
		t.Fatalf("baseline unfocused: row 0 BG = %+v, want dim SelBGUnfocused %+v", got, tree.SelBGUnfocused)
	}

	tree.SetSelected(2)
	app = drawTree(t, tree)
	if got := barRow(t, app, 2); got != tree.SelBGUnfocused {
		t.Fatalf("after SetSelected(2) unfocused: row 2 BG = %+v, want dim SelBGUnfocused", got)
	}
	if got := barRow(t, app, 2); got == tree.SelBG {
		t.Fatal("unfocused tree painted the bright SelBG instead of the dim SelBGUnfocused")
	}
	if got := barRow(t, app, 0); got == tree.SelBGUnfocused {
		t.Fatal("row 0 still dim-barred after the bar moved to row 2")
	}
}

// Same guarantee via SelectNode while unfocused.
func TestSelectNodeBarFollowsWhenTreeUnfocused(t *testing.T) {
	rows := make([]*TreeNode, 5)
	tree := NewTree(Rect{X: 0, Y: 0, W: 10, H: 5})
	for i := range rows {
		rows[i] = NewTreeNode(fmt.Sprintf("n%d", i))
		tree.AddRoot(rows[i])
	}
	tree.Component.hasFocus = false
	requireDistinctBarColors(t, tree)

	tree.SetSelected(0)
	drawTree(t, tree)
	if !tree.SelectNode(rows[3]) {
		t.Fatal("SelectNode(rows[3]) returned false")
	}
	app := drawTree(t, tree)
	assertOnlyColoredBarAt(t, app, tree, 5, 3, tree.SelBGUnfocused)
}

// Faithful end-to-end #206 repro: the sidebar tree is UNFOCUSED while session
// focus changes (cycle / menu / close paths), and the dim bar follows the active
// session on every path.
func TestSidebarHighlightFollowsFocusWhileUnfocused(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 8})
	tree.Component.hasFocus = false // the real-world state: window has focus, not the tree
	requireDistinctBarColors(t, tree)

	sessions := map[string]*TreeNode{}
	addSession := func(id string) *TreeNode {
		n := NewTreeNode(id)
		n.Data = nodeRef{sessionID: id}
		tree.AddRoot(n)
		sessions[id] = n
		tree.SelectNode(n) // new-session path selects the added node
		return n
	}
	selectSession := func(id string) bool {
		if n := sessions[id]; n != nil {
			return tree.SelectNode(n)
		}
		return false
	}
	addSession("sess-a")
	addSession("sess-b")
	addSession("sess-c")

	// Menu pick -> sess-c (flat index 2).
	selectSession("sess-c")
	app := drawTree(t, tree)
	assertOnlyColoredBarAt(t, app, tree, 3, 2, tree.SelBGUnfocused)

	// Ctrl+] cycle -> sess-b (index 1).
	selectSession("sess-b")
	app = drawTree(t, tree)
	assertOnlyColoredBarAt(t, app, tree, 3, 1, tree.SelBGUnfocused)

	// Close fallback -> sess-a (index 0).
	selectSession("sess-a")
	app = drawTree(t, tree)
	assertOnlyColoredBarAt(t, app, tree, 3, 0, tree.SelBGUnfocused)
}

// -----------------------------------------------------------------------------
// OnActivate after a programmatic select (gap from round-2 critique)
// -----------------------------------------------------------------------------

// Enter after a SetSelected must activate the programmatically-selected node,
// proving the selection index the input path reads is the one SetSelected set.
func TestSetSelectedThenEnterActivatesRightNode(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	drawTree(t, tree)
	tree.SetSelected(3) // "d"

	var activated *TreeNode
	tree.OnActivate = func(n *TreeNode) { activated = n }
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyEnter})
	if activated == nil || activated.Label != "d" {
		t.Fatalf("Enter activated %v, want d (the programmatically-selected node)", activated)
	}
}

// Same for SelectNode.
func TestSelectNodeThenEnterActivatesRightNode(t *testing.T) {
	tree, nodes := nestedSelectTree()
	drawTree(t, tree)
	tree.SelectNode(nodes[4]) // r3

	var activated *TreeNode
	tree.OnActivate = func(n *TreeNode) { activated = n }
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyEnter})
	if activated == nil || activated.Label != "r3" {
		t.Fatalf("Enter activated %v, want r3", activated)
	}
}

// -----------------------------------------------------------------------------
// SelectNode scrolling back up (gap from round-2 critique)
// -----------------------------------------------------------------------------

// Selecting the first node from a scrolled-down viewport scrolls back to the top
// (round 2 only exercised SelectNode scrolling down).
func TestSelectNodeScrollsUpToFirst(t *testing.T) {
	rows := make([]*TreeNode, 12)
	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 4})
	for i := range rows {
		rows[i] = NewTreeNode(fmt.Sprintf("n%d", i))
		tree.AddRoot(rows[i])
	}
	drawTree(t, tree)
	tree.SetSelected(11) // offset -> 11-4+1 = 8
	if tree.offset != 8 {
		t.Fatalf("precondition offset = %d, want 8", tree.offset)
	}
	if !tree.SelectNode(rows[0]) {
		t.Fatal("SelectNode(rows[0]) returned false")
	}
	if tree.selected != 0 {
		t.Fatalf("selected = %d, want 0", tree.selected)
	}
	if tree.offset != 0 {
		t.Fatalf("SelectNode(first) offset = %d, want 0 (should scroll back to top)", tree.offset)
	}
}
