package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// These tests cover the OnSelectMouse callback added for gogent #302 (approach A):
// a Tree callback that fires ONLY when a selection change came from a mouse click,
// so a host can open something on a single click without also reacting to keyboard
// arrow traversal (which still fires OnSelect). The contract under test:
//
//   - a mouse click on a row fires OnSelectMouse(node) AND OnSelect(node);
//   - keyboard moves fire OnSelect but NEVER OnSelectMouse;
//   - OnActivate keeps its existing semantics (Enter, or a click on an
//     already-selected row);
//   - OnSelectMouse reports the CLICKED node even if an OnSelect handler
//     reentrantly moves the selection (the gogent #302 snap-back);
//   - OnSelectMouse is purely additive: nil OnSelectMouse leaves behaviour
//     unchanged, and it is independent of whether OnSelect is set.

// clickRowUp simulates the commit half of a mouse click (the button release) on a
// tree at the given screen coordinates. handleClick selects on the up event.
func clickRowUp(tree *Tree, x, y int) {
	tree.handleClick(tree.Component, tui.ClickEvent{X: x, Y: y, Button: tui.MouseLeft, Down: false})
}

// clickRow simulates a full mouse click (press then release) at the coordinates.
func clickRow(tree *Tree, x, y int) {
	tree.handleClick(tree.Component, tui.ClickEvent{X: x, Y: y, Button: tui.MouseLeft, Down: true})
	tree.handleClick(tree.Component, tui.ClickEvent{X: x, Y: y, Button: tui.MouseLeft, Down: false})
}

// -----------------------------------------------------------------------------
// Headline contract: click fires OnSelectMouse + OnSelect; keys fire only OnSelect
// -----------------------------------------------------------------------------

// A mouse click on a row reports the clicked node through BOTH OnSelectMouse and
// OnSelect.
func TestTreeClickFiresOnSelectMouseAndOnSelect(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)

	var mouseNode, selNode *TreeNode
	mouseFires, selFires := 0, 0
	tree.OnSelectMouse = func(n *TreeNode) { mouseFires++; mouseNode = n }
	tree.OnSelect = func(n *TreeNode) { selFires++; selNode = n }

	clickRow(tree, 2, 1) // screen row 1 -> "b"

	if mouseFires != 1 || mouseNode == nil || mouseNode.Label != "b" {
		t.Fatalf("OnSelectMouse fired=%d node=%v, want 1 fire on b", mouseFires, mouseNode)
	}
	if selFires != 1 || selNode == nil || selNode.Label != "b" {
		t.Fatalf("OnSelect fired=%d node=%v, want 1 fire on b", selFires, selNode)
	}
	if mouseNode != selNode {
		t.Fatalf("OnSelectMouse and OnSelect reported different nodes: %v vs %v", mouseNode, selNode)
	}
	if tree.selected != 1 {
		t.Fatalf("selected = %d, want 1", tree.selected)
	}
}

// Every keyboard navigation key that fires OnSelect must NOT fire OnSelectMouse —
// this is the whole point of the new callback (gogent must not open a popup while
// arrowing through rows).
func TestTreeKeyboardMovesNeverFireOnSelectMouse(t *testing.T) {
	keys := []struct {
		name  string
		start int
		key   tui.TypeEvent
	}{
		{"down", 0, tui.TypeEvent{Key: tui.KeyDown}},
		{"up", 3, tui.TypeEvent{Key: tui.KeyUp}},
		{"home", 4, tui.TypeEvent{Key: tui.KeyHome}},
		{"end", 0, tui.TypeEvent{Key: tui.KeyEnd}},
		{"pagedown", 0, tui.TypeEvent{Key: tui.KeyPageDown}},
		{"pageup", 4, tui.TypeEvent{Key: tui.KeyPageUp}},
	}
	for _, tc := range keys {
		t.Run(tc.name, func(t *testing.T) {
			tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 3}, "a", "b", "c", "d", "e")
			drawTree(t, tree)
			tree.selected = tc.start

			selFires, mouseFires := 0, 0
			tree.OnSelect = func(*TreeNode) { selFires++ }
			tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }

			tree.handleType(tree.Component, tc.key)

			if selFires == 0 {
				t.Fatalf("%s: OnSelect did not fire; test precondition broken", tc.name)
			}
			if mouseFires != 0 {
				t.Fatalf("%s: OnSelectMouse fired %d times on a keyboard move, want 0", tc.name, mouseFires)
			}
		})
	}
}

// The Left/Right keys also move the selection (collapse/expand-or-descend) and
// fire OnSelect; they must not fire OnSelectMouse either.
func TestTreeLeftRightKeysNeverFireOnSelectMouse(t *testing.T) {
	tree, _ := nestedSelectTree() // root1 expanded: r1, c1a, c1b, r2, r3, c3a
	drawTree(t, tree)

	selFires, mouseFires := 0, 0
	tree.OnSelect = func(*TreeNode) { selFires++ }
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }

	// Right on an expanded parent with children descends into the first child.
	tree.SetSelected(0) // r1 (expanded)
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyRight})
	// Left from a child selects the parent.
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyLeft})

	if selFires == 0 {
		t.Fatal("OnSelect never fired across Right/Left; precondition broken")
	}
	if mouseFires != 0 {
		t.Fatalf("OnSelectMouse fired %d times on Left/Right keys, want 0", mouseFires)
	}
}

// Enter activates via OnActivate and must touch neither OnSelect nor OnSelectMouse.
func TestTreeEnterDoesNotFireOnSelectMouse(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)
	tree.SetSelected(1)

	mouseFires, selFires := 0, 0
	var activated *TreeNode
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }
	tree.OnSelect = func(*TreeNode) { selFires++ }
	tree.OnActivate = func(n *TreeNode) { activated = n }

	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyEnter})

	if activated == nil || activated.Label != "b" {
		t.Fatalf("Enter activated %v, want b", activated)
	}
	if mouseFires != 0 || selFires != 0 {
		t.Fatalf("Enter fired OnSelect=%d OnSelectMouse=%d, want 0/0", selFires, mouseFires)
	}
}

// -----------------------------------------------------------------------------
// OnActivate semantics are unchanged by the addition
// -----------------------------------------------------------------------------

// A click on an already-selected row both opens it (OnActivate, the gogent
// "single click opens" path) and fires OnSelectMouse — the additive callback does
// not disturb the repeat-click activation.
func TestTreeClickAlreadySelectedFiresOnSelectMouseAndActivate(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)
	tree.SetSelected(1) // "b" already selected

	mouseFires, activateFires := 0, 0
	var mouseNode, actNode *TreeNode
	tree.OnSelectMouse = func(n *TreeNode) { mouseFires++; mouseNode = n }
	tree.OnActivate = func(n *TreeNode) { activateFires++; actNode = n }

	clickRow(tree, 2, 1) // re-click the already-selected "b"

	if mouseFires != 1 || mouseNode.Label != "b" {
		t.Fatalf("OnSelectMouse fired=%d node=%v, want 1 on b", mouseFires, mouseNode)
	}
	if activateFires != 1 || actNode.Label != "b" {
		t.Fatalf("OnActivate fired=%d node=%v, want 1 on b (repeat-click activation unchanged)", activateFires, actNode)
	}
}

// A click on a not-yet-selected row selects it but does NOT activate it — only
// OnSelectMouse/OnSelect fire. (Unchanged OnActivate semantics.)
func TestTreeClickNewRowDoesNotActivate(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)
	tree.SetSelected(0)

	mouseFires, activateFires := 0, 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }
	tree.OnActivate = func(*TreeNode) { activateFires++ }

	clickRow(tree, 2, 2) // click "c" (not selected)

	if mouseFires != 1 {
		t.Fatalf("OnSelectMouse fired=%d, want 1", mouseFires)
	}
	if activateFires != 0 {
		t.Fatalf("OnActivate fired=%d on a first click of an unselected row, want 0", activateFires)
	}
}

// -----------------------------------------------------------------------------
// Snap-back: OnSelectMouse reports the CLICKED node, not a reentrantly-moved one
// -----------------------------------------------------------------------------

// gogent #302's real wiring: OnSelect focuses the owning session, which calls
// SelectNode(sessionNode) and snaps t.selected back to the parent row. If
// OnSelectMouse were derived from t.selected *after* OnSelect runs, it would
// report the session node and the sub-agent monologue would never open. It must
// report the node the user actually clicked.
func TestTreeOnSelectMouseSurvivesSnapBack(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 8})
	tree.Component.hasFocus = true
	session := NewTreeNode("session-1")
	session.Expanded = true
	sub := session.AddLeaf("counter-3")
	tree.AddRoot(session)
	drawTree(t, tree) // rows: [0]=session, [1]=sub

	// OnSelect snaps the selection back to the session row, exactly like gogent.
	tree.OnSelect = func(*TreeNode) { tree.SelectNode(session) }

	var mouseNode *TreeNode
	mouseFires := 0
	tree.OnSelectMouse = func(n *TreeNode) { mouseFires++; mouseNode = n }

	clickRow(tree, 4, 1) // click the sub-agent row

	if mouseFires != 1 {
		t.Fatalf("OnSelectMouse fired=%d, want 1", mouseFires)
	}
	if mouseNode != sub {
		t.Fatalf("OnSelectMouse reported %v, want the clicked sub-agent %v (snap-back must not redirect it)", mouseNode, sub)
	}
	// The snap-back itself still took effect (selection is back on the session).
	if tree.Selected() != session {
		t.Fatalf("after snap-back selected=%v, want session (OnSelect behaviour unchanged)", tree.Selected())
	}
}

// Even when OnSelect moves the selection to a DIFFERENT, far-away row, the
// clicked node is reported — guards against any "report current selection" regression.
func TestTreeOnSelectMouseReportsClickedNotReselected(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 6}, "a", "b", "c", "d")
	drawTree(t, tree)

	tree.OnSelect = func(*TreeNode) { tree.SetSelected(3) } // jump to "d"

	var mouseNode *TreeNode
	tree.OnSelectMouse = func(n *TreeNode) { mouseNode = n }

	clickRow(tree, 2, 1) // click "b"

	if mouseNode == nil || mouseNode.Label != "b" {
		t.Fatalf("OnSelectMouse reported %v, want the clicked node b", mouseNode)
	}
}

// -----------------------------------------------------------------------------
// Additivity / nil-safety
// -----------------------------------------------------------------------------

// With OnSelectMouse nil the widget behaves exactly as before: clicks still fire
// OnSelect and OnActivate, keys still fire OnSelect, and nothing panics.
func TestTreeNilOnSelectMouseUnchangedBehaviour(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)
	// OnSelectMouse deliberately left nil.

	selFires, activateFires := 0, 0
	tree.OnSelect = func(*TreeNode) { selFires++ }
	tree.OnActivate = func(*TreeNode) { activateFires++ }

	clickRow(tree, 2, 1)                                             // select b -> OnSelect
	clickRow(tree, 2, 1)                                             // re-click b -> OnSelect + OnActivate
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown}) // b(1)->c(2): OnSelect

	if selFires != 3 {
		t.Fatalf("OnSelect fired=%d, want 3 (two clicks + one key move)", selFires)
	}
	if activateFires != 1 {
		t.Fatalf("OnActivate fired=%d, want 1 (repeat click)", activateFires)
	}
}

// OnSelectMouse is independent of OnSelect: it fires on a click even when OnSelect
// is nil (the two checks are separate).
func TestTreeOnSelectMouseFiresWithNilOnSelect(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)
	// OnSelect nil on purpose.

	mouseFires := 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }

	clickRow(tree, 2, 2)

	if mouseFires != 1 {
		t.Fatalf("OnSelectMouse fired=%d with nil OnSelect, want 1 (independent callbacks)", mouseFires)
	}
}

// -----------------------------------------------------------------------------
// Clicks that do NOT land a row selection must not fire OnSelectMouse
// -----------------------------------------------------------------------------

// The button-press (Down:true) half of a click is a no-op for selection; only the
// release commits. OnSelectMouse must not fire on the press.
func TestTreeMouseDownAloneDoesNotFireOnSelectMouse(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)

	mouseFires, selFires := 0, 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }
	tree.OnSelect = func(*TreeNode) { selFires++ }

	tree.handleClick(tree.Component, tui.ClickEvent{X: 2, Y: 1, Button: tui.MouseLeft, Down: true})

	if mouseFires != 0 || selFires != 0 {
		t.Fatalf("press alone fired OnSelect=%d OnSelectMouse=%d, want 0/0", selFires, mouseFires)
	}
}

// A click in the empty area below the last row selects nothing and fires nothing.
func TestTreeClickBelowRowsDoesNotFireOnSelectMouse(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 6}, "a", "b") // 2 rows, 6-high view
	drawTree(t, tree)

	mouseFires, selFires := 0, 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }
	tree.OnSelect = func(*TreeNode) { selFires++ }

	clickRowUp(tree, 2, 4) // empty row below the two nodes

	if mouseFires != 0 || selFires != 0 {
		t.Fatalf("click below rows fired OnSelect=%d OnSelectMouse=%d, want 0/0", selFires, mouseFires)
	}
}

// A click on the scrollbar column scrolls and returns early, before any selection
// callback — OnSelectMouse must not fire.
func TestTreeScrollbarClickDoesNotFireOnSelectMouse(t *testing.T) {
	// 8 rows in a 3-high view forces a scrollbar in the last column.
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 3}, "a", "b", "c", "d", "e", "f", "g", "h")
	drawTree(t, tree)

	mouseFires, selFires := 0, 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }
	tree.OnSelect = func(*TreeNode) { selFires++ }

	barX := tree.Component.AbsoluteBounds().Right() // last column = scrollbar track
	tree.handleClick(tree.Component, tui.ClickEvent{X: barX, Y: 2, Button: tui.MouseLeft, Down: false})

	if mouseFires != 0 || selFires != 0 {
		t.Fatalf("scrollbar click fired OnSelect=%d OnSelectMouse=%d, want 0/0", selFires, mouseFires)
	}
}

// Clicking an empty tree must not fire OnSelectMouse and must not panic.
func TestTreeClickEmptyTreeIsSafe(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 12, H: 5})
	drawTree(t, tree)

	mouseFires := 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }

	clickRow(tree, 1, 1) // must not panic on an empty tree

	if mouseFires != 0 {
		t.Fatalf("OnSelectMouse fired=%d on an empty tree, want 0", mouseFires)
	}
}

// -----------------------------------------------------------------------------
// Programmatic selection never fires OnSelectMouse
// -----------------------------------------------------------------------------

// SetSelected and SelectNode are external/programmatic drivers; like OnSelect they
// must not fire OnSelectMouse (a host syncing the tree from focus must not see a
// phantom "mouse click").
func TestTreeProgrammaticSelectDoesNotFireOnSelectMouse(t *testing.T) {
	tree, nodes := nestedSelectTree()
	drawTree(t, tree)

	mouseFires := 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }

	tree.SetSelected(2)
	tree.SetSelected(0)
	tree.SelectNode(nodes[4]) // r3
	tree.SelectNode(nodes[1]) // c1a

	if mouseFires != 0 {
		t.Fatalf("programmatic SetSelected/SelectNode fired OnSelectMouse %d times, want 0", mouseFires)
	}
}

// -----------------------------------------------------------------------------
// Expand-marker click mirrors OnSelect (both fire), and still does not activate
// -----------------------------------------------------------------------------

// Clicking a parent's expand marker toggles it and fires OnSelectMouse alongside
// OnSelect (the marker click is still a "mouse selection"), but must NOT activate
// the node — the toggledMarker guard on OnActivate is preserved.
func TestTreeExpandMarkerClickFiresOnSelectMouseNotActivate(t *testing.T) {
	tree, _ := nestedSelectTree() // root1 expanded at row 0, marker at col 0
	drawTree(t, tree)
	tree.SetSelected(0) // r1 already selected so a non-marker click would activate

	mouseFires, selFires, activateFires := 0, 0, 0
	tree.OnSelectMouse = func(*TreeNode) { mouseFires++ }
	tree.OnSelect = func(*TreeNode) { selFires++ }
	tree.OnActivate = func(*TreeNode) { activateFires++ }

	wasExpanded := tree.Roots[0].Expanded
	clickRowUp(tree, 0, 0) // col 0 <= markerCol -> toggles marker

	if tree.Roots[0].Expanded == wasExpanded {
		t.Fatal("clicking the expand marker did not toggle the node; test precondition broken")
	}
	if mouseFires != 1 || selFires != 1 {
		t.Fatalf("marker click fired OnSelect=%d OnSelectMouse=%d, want 1/1 (mirrors OnSelect)", selFires, mouseFires)
	}
	if activateFires != 0 {
		t.Fatalf("marker click fired OnActivate=%d, want 0 (marker toggle never activates)", activateFires)
	}
}

// -----------------------------------------------------------------------------
// gogent #302 end-to-end scenario at the widget level
// -----------------------------------------------------------------------------

// Reproduces the gogent sidebar wiring: a session node with a nested sub-agent
// row. A single mouse click on the sub-agent must reach the host's "open
// monologue" path (OnSelectMouse), while arrowing the keyboard THROUGH the same
// row must not. OnSelect fires on every move/click as before (gogent uses it to
// focus the owning session window).
func TestTreeGogent302SubAgentClickOpensButKeyboardDoesNot(t *testing.T) {
	tree := NewTree(Rect{X: 0, Y: 0, W: 30, H: 8})
	tree.Component.hasFocus = true

	session := NewTreeNode("session-1")
	session.Expanded = true
	subAgent := session.AddLeaf("✓ counter-3 (1)")
	tree.AddRoot(session)
	drawTree(t, tree) // rows: [0]=session-1, [1]=sub-agent

	// Host wiring (mirrors gogent's sidebar): OnSelect focuses the owning session
	// (and snaps the bar back to it); OnSelectMouse opens the clicked monologue.
	focusCalls := 0
	popupsOpened := 0
	var lastPopupNode *TreeNode
	tree.OnSelect = func(*TreeNode) { focusCalls++; tree.SelectNode(session) }
	tree.OnSelectMouse = func(n *TreeNode) { popupsOpened++; lastPopupNode = n }

	// 1) Keyboard arrow DOWN from the session onto the sub-agent: OnSelect fires
	//    (session focus), but NO popup opens. Note the snap-back means the bar
	//    returns to the session, but that does not concern OnSelectMouse here.
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown})
	if popupsOpened != 0 {
		t.Fatalf("keyboard traversal onto the sub-agent opened %d popups, want 0", popupsOpened)
	}
	if focusCalls != 1 {
		t.Fatalf("keyboard move fired OnSelect %d times, want 1", focusCalls)
	}

	// 2) More keyboard moves: still no popups, proving repeated keyboard passes
	//    never open the sub-agent.
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyDown})
	tree.handleType(tree.Component, tui.TypeEvent{Key: tui.KeyUp})
	if popupsOpened != 0 {
		t.Fatalf("repeated keyboard moves opened %d popups, want 0", popupsOpened)
	}

	// 3) A single MOUSE CLICK on the sub-agent row opens its monologue exactly once
	//    and reports the sub-agent node — despite the OnSelect snap-back.
	popupsOpened, focusCalls = 0, 0
	clickRow(tree, 4, 1) // screen row 1 = the sub-agent
	if popupsOpened != 1 || lastPopupNode != subAgent {
		t.Fatalf("single click opened %d popups (node=%v), want 1 on the sub-agent", popupsOpened, lastPopupNode)
	}
	if focusCalls != 1 {
		t.Fatalf("click fired OnSelect %d times, want 1 (session focus still happens)", focusCalls)
	}
}

// Clicking distinct rows in sequence reports each clicked node once through
// OnSelectMouse — no stale/duplicated node from a prior click.
func TestTreeSequentialClicksReportEachNode(t *testing.T) {
	tree := leafTree(Rect{X: 0, Y: 0, W: 12, H: 5}, "a", "b", "c")
	drawTree(t, tree)

	var seen []string
	tree.OnSelectMouse = func(n *TreeNode) { seen = append(seen, n.Label) }

	clickRow(tree, 2, 2) // c
	clickRow(tree, 2, 0) // a
	clickRow(tree, 2, 1) // b

	want := []string{"c", "a", "b"}
	if len(seen) != len(want) {
		t.Fatalf("OnSelectMouse fired %d times (%v), want %d (%v)", len(seen), seen, len(want), want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("click %d reported %q, want %q (full=%v)", i, seen[i], want[i], seen)
		}
	}
}
