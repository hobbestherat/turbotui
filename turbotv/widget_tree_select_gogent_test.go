package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// nodeRef mirrors gogent's sidebar node payload: the tree node for a session
// carries its session id in Data, and gogent keeps a sessionID -> *TreeNode map.
type nodeRef struct {
	sessionID string
}

// -----------------------------------------------------------------------------
// echo-loop safety (the core reason SetSelected/SelectNode must not fire)
// -----------------------------------------------------------------------------

// gogent's OnSelect handler pushes the tree selection outward (e.g. focuses the
// matching window). If that handler also re-syncs the tree with SetSelected, and
// SetSelected fired OnSelect, the call would recurse without bound. It must not.
func TestSetSelectedFromOnSelectDoesNotLoop(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	drawTree(t, tree) // viewH known
	fires := 0
	tree.OnSelect = func(*TreeNode) {
		fires++
		// Echo back to row 0 every time; a re-firing SetSelected recurses forever.
		tree.SetSelected(0)
	}
	// KeyDown: 0 -> 1, fires OnSelect once; the handler's SetSelected(0) must
	// take effect without re-firing.
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown})
	if fires != 1 {
		t.Fatalf("OnSelect fired %d times, want 1 (SetSelected must not echo)", fires)
	}
	if tree.selected != 0 {
		t.Fatalf("handler SetSelected(0) did not take effect; selected = %d", tree.selected)
	}
}

// Same echo-loop guarantee for SelectNode.
func TestSelectNodeFromOnSelectDoesNotLoop(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 8}, "a", "b", "c", "d", "e")
	first := tree.Roots[0]
	drawTree(t, tree)
	fires := 0
	tree.OnSelect = func(*TreeNode) {
		fires++
		tree.SelectNode(first) // would recurse forever if SelectNode fired OnSelect
	}
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown})
	if fires != 1 {
		t.Fatalf("OnSelect fired %d times, want 1 (SelectNode must not echo)", fires)
	}
	if tree.selected != 0 {
		t.Fatalf("handler SelectNode(first) did not take effect; selected = %d", tree.selected)
	}
}

// -----------------------------------------------------------------------------
// gogent #206 integration: the painted highlight bar follows external focus
// -----------------------------------------------------------------------------

// assertOnlyBarAt asserts that, among the first `count` screen rows, exactly one
// carries the selection bar and it is the row at flat index wantIdx. Catches
// both "bar did not move" and "bar painted on two rows" regressions.
func assertOnlyBarAt(t *testing.T, app *tui.App, tree *Tree, count, wantIdx int) {
	t.Helper()
	bars := 0
	for r := 0; r < count; r++ {
		if barRow(t, app, r) == tree.SelBG {
			bars++
			if r != wantIdx {
				t.Errorf("selection bar at screen row %d, want only at %d", r, wantIdx)
			}
		}
	}
	if bars != 1 {
		t.Fatalf("found %d selection-bar rows among %d, want exactly 1 at row %d", bars, count, wantIdx)
	}
}

// TestSidebarHighlightFollowsFocusedSession reproduces gogent #206 at the widget
// level. An external focus change (new session / Ctrl+] cycle / close / Session
// menu pick) drives SelectNode on the stored session node; the painted highlight
// bar must follow — on every path, and without OnSelect echoing back.
func TestSidebarHighlightFollowsFocusedSession(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 8})
	tree.Component.hasFocus = true

	sessions := map[string]*TreeNode{}
	// addSession mirrors gogent's sidebar.addSession: append the node, register it
	// in the id map, then (per the plan) select the node just added.
	addSession := func(id string) *TreeNode {
		n := NewTreeNode(id)
		n.Data = nodeRef{sessionID: id}
		tree.AddRoot(n)
		sessions[id] = n
		tree.SelectNode(n) // new-session path selects the freshly added node
		return n
	}
	// selectSession mirrors gogent's sidebar.selectSession(id): exact-pointer
	// lookup, no-op for unknown ids.
	selectSession := func(id string) bool {
		if n := sessions[id]; n != nil {
			return tree.SelectNode(n)
		}
		return false
	}

	addSession("sess-a")
	addSession("sess-b")
	// Focus sess-c directly via the Session menu (no keyboard interaction).
	addSession("sess-c")

	app := drawTree(t, tree)
	assertOnlyBarAt(t, app, tree, 3, 2) // sess-c = flat index 2

	// Ctrl+] cycle lands on sess-b: the tree follows via selectSession.
	if !selectSession("sess-b") {
		t.Fatal("selectSession(sess-b) returned false for a known session")
	}
	app = drawTree(t, tree)
	assertOnlyBarAt(t, app, tree, 3, 1)

	// Close-session fallback lands on sess-a.
	selectSession("sess-a")
	app = drawTree(t, tree)
	assertOnlyBarAt(t, app, tree, 3, 0)

	// A new session is appended and immediately selected.
	addSession("sess-d")
	app = drawTree(t, tree)
	assertOnlyBarAt(t, app, tree, 4, 3)

	// Unknown id (sub-agent-only / removed session) is a no-op: bar stays put.
	if selectSession("no-such-session") {
		t.Fatal("selectSession on an unknown id returned true, want false")
	}
	app = drawTree(t, tree)
	assertOnlyBarAt(t, app, tree, 4, 3)
}

// The same scenario but verifying OnSelect is NOT re-fired by the programmatic
// selects — the echo that gogent's Workbench.Focus path must avoid.
func TestSidebarSelectSessionDoesNotFireOnSelect(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 8})
	tree.Component.hasFocus = true
	sessions := map[string]*TreeNode{}
	addSession := func(id string) *TreeNode {
		n := NewTreeNode(id)
		tree.AddRoot(n)
		sessions[id] = n
		return n
	}
	addSession("a")
	addSession("b")
	addSession("c")

	fires := 0
	tree.OnSelect = func(*TreeNode) { fires++ }

	// Driving every focus path via SelectNode must not trigger OnSelect at all.
	tree.SelectNode(sessions["c"])
	tree.SelectNode(sessions["a"])
	tree.SelectNode(sessions["b"])
	if fires != 0 {
		t.Fatalf("programmatic SelectNode fired OnSelect %d times, want 0 (echo loop risk)", fires)
	}
}
