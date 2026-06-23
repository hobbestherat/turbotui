package tv

import (
	tui "github.com/hobbestherat/turbotui"
)

// TreeNode is one node in a Tree. Children with len>0 render an expand marker.
type TreeNode struct {
	Label    string
	Expanded bool
	Children []*TreeNode
	// Data is an optional payload for the caller (ignored by the widget).
	Data interface{}
}

// NewTreeNode creates a leaf node.
func NewTreeNode(label string) *TreeNode { return &TreeNode{Label: label} }

// Add appends a child node and returns it for chaining.
func (n *TreeNode) Add(child *TreeNode) *TreeNode {
	n.Children = append(n.Children, child)
	return child
}

// AddLeaf is a convenience for Add(NewTreeNode(label)).
func (n *TreeNode) AddLeaf(label string) *TreeNode { return n.Add(NewTreeNode(label)) }

type treeRow struct {
	node  *TreeNode
	depth int
}

// Tree is a scrollable, collapsible tree view. Roots holds the top-level nodes.
type Tree struct {
	Component *VisualComponent
	Roots     []*TreeNode
	FG        tui.Color
	BG        tui.Color
	SelFG     tui.Color
	SelBG     tui.Color
	// SelFGUnfocused/SelBGUnfocused paint the selection bar when the tree does
	// not hold focus, so a focused list's bright bar is unambiguous next to an
	// unfocused one (Turbo-Vision convention: dim/hollow bar when inactive).
	SelFGUnfocused tui.Color
	SelBGUnfocused tui.Color
	selected       int
	offset         int
	viewH          int // visible row count from the last draw (for ensureVisible)
	// OnActivate fires on Enter for the selected node; OnSelect fires whenever
	// the selection changes.
	OnActivate func(*TreeNode)
	OnSelect   func(*TreeNode)
	// OnSelectMouse fires on a mouse click that selects a row, reporting the
	// clicked node (it fires on every in-bounds row click, including a re-click of
	// the already-selected row, mirroring OnSelect's click behaviour). It is
	// additive to OnSelect (which still fires for both clicks and keyboard moves):
	// use OnSelectMouse to tell a pointer click apart from keyboard traversal
	// without altering OnSelect/OnActivate semantics. The reported node is the row
	// actually clicked, even if an OnSelect handler reentrantly moves the
	// selection. It never fires from keyboard navigation; when nil the widget
	// behaves exactly as before.
	OnSelectMouse func(*TreeNode)

	// flatBuf is reused across flatten() calls so the visible-rows slice is not
	// reallocated on every draw/click/scroll; it is recomputed (correctly) each
	// call because it just walks the live node tree.
	flatBuf []treeRow
}

// NewTree creates an empty tree view.
func NewTree(bounds Rect) *Tree {
	t := &Tree{
		// BG/FG seed from WindowBG/WindowFG, NOT the new ListBG/ListFG (gogent#327):
		// ListBG defaults to DialogBG, which equals SelectionBG under the stock
		// DefaultTheme, so seeding a list from it would make an un-recoloured tree's
		// selection bar invisible. The List* slots exist for consumers (gogent) that
		// set list.BG/list.FG per instance; the default seed stays on WindowBG so a
		// plain NewTree keeps a visible selection.
		FG:    activeTheme.WindowFG,
		BG:    activeTheme.WindowBG,
		SelFG: activeTheme.SelectionFG,
		SelBG: activeTheme.SelectionBG,
		// Dimmed bar for the unfocused state: a dark-grey background keeps the
		// row distinguishable from the body without competing with the focused
		// tree's bright bar.
		SelFGUnfocused: activeTheme.WindowFG,
		SelBGUnfocused: tui.ANSIColor(8),
	}
	t.Component = NewComponent(bounds)
	t.Component.Focusable = true
	t.Component.DrawFn = t.draw
	t.Component.OnTypeFn = t.handleType
	t.Component.OnClickFn = t.handleClick
	t.Component.OnScrollFn = t.handleScroll
	return t
}
func (t *Tree) Root() *VisualComponent { return t.Component }

// AddRoot appends a top-level node and returns it.
func (t *Tree) AddRoot(node *TreeNode) *TreeNode {
	t.Roots = append(t.Roots, node)
	return node
}

// Selected returns the currently selected node, or nil.
func (t *Tree) Selected() *TreeNode {
	rows := t.flatten()
	if t.selected >= 0 && t.selected < len(rows) {
		return rows[t.selected].node
	}
	return nil
}

// SetSelected moves the highlight bar to the visible row at index, clamping it
// into range and scrolling it into view. The index is in the same space as
// Selected() / the OnSelect callback: an offset into the currently visible
// (flattened, collapse-aware) rows, not into Roots.
//
// It returns the index the selection actually landed on, or -1 if the tree is
// empty (no selectable row). A returned value different from the requested index
// signals that the request was clamped — the symmetric counterpart to
// SelectNode's bool, so a caller need not re-read Selected() to detect a clamp.
//
// This is the programmatic counterpart to keyboard/mouse navigation, for callers
// that drive the selection from outside the widget (e.g. syncing the tree to an
// externally focused item). Unlike handleType/handleClick it deliberately does
// NOT fire OnSelect: a caller that also listens on OnSelect (to push the
// selection outward) would otherwise echo straight back into SetSelected and
// loop. An empty tree leaves the selection pinned at 0 and is otherwise a no-op.
func (t *Tree) SetSelected(index int) int {
	rows := t.flatten()
	if len(rows) == 0 {
		t.selected = 0
		return -1
	}
	t.selected = clampInt(index, 0, len(rows)-1)
	t.ensureVisible()
	return t.selected
}

// SelectNode moves the highlight bar to node, matched by pointer identity among
// the currently visible rows, and scrolls it into view. It reports whether the
// node was found. A nil node, a node not in this tree, or one hidden inside a
// collapsed subtree is not matched and leaves the selection unchanged (returns
// false). Like SetSelected it does NOT fire OnSelect, so it is safe to call from
// an OnSelect handler without looping.
func (t *Tree) SelectNode(node *TreeNode) bool {
	if node == nil {
		return false
	}
	rows := t.flatten()
	for i := range rows {
		if rows[i].node == node {
			t.selected = i
			t.ensureVisible()
			return true
		}
	}
	return false
}

// flatten returns the currently visible rows (depth-first, skipping collapsed
// subtrees). It reuses flatBuf across calls so the slice's backing array is not
// reallocated on every draw/handler; the contents are recomputed each call from
// the live node tree, so it always reflects the current expand/collapse state.
// Callers must not retain the slice across another flatten() call.
func (t *Tree) flatten() []treeRow {
	rows := t.flatBuf[:0]
	var walk func(nodes []*TreeNode, depth int)
	walk = func(nodes []*TreeNode, depth int) {
		for _, n := range nodes {
			rows = append(rows, treeRow{node: n, depth: depth})
			if n.Expanded && len(n.Children) > 0 {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(t.Roots, 0)
	t.flatBuf = rows
	return rows
}
func (t *Tree) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	if abs.W < 1 || abs.H < 1 {
		return
	}
	rows := t.flatten()
	t.viewH = abs.H
	t.clampSelection(len(rows))
	// Only bound the offset here; selection-follow happens on keyboard moves so
	// it never fights an explicit wheel/scrollbar scroll.
	t.offset = clampInt(t.offset, 0, scrollbarMaxOffset(len(rows), abs.H))
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: t.FG, BG: t.BG})
	needBar := len(rows) > abs.H
	textW := abs.W
	if needBar && textW > 1 {
		textW--
	}
	for row := 0; row < abs.H; row++ {
		idx := t.offset + row
		if idx >= len(rows) {
			break
		}
		r := rows[idx]
		fg, bg := t.FG, t.BG
		if idx == t.selected {
			// A focused tree gets the bright selection bar; an unfocused one
			// gets the dim variant so keyboard focus is never ambiguous when
			// several lists share the screen.
			if component.Focused() {
				fg, bg = t.SelFG, t.SelBG
			} else {
				fg, bg = t.SelFGUnfocused, t.SelBGUnfocused
			}
		}
		y := abs.Y + row
		// Paint the entire row width as a bar so the highlight reads at a glance,
		// not just under the glyphs.
		surface.Fill(Rect{X: abs.X, Y: y, W: textW, H: 1}, tui.Cell{Ch: ' ', FG: fg, BG: bg})
		marker := " "
		if len(r.node.Children) > 0 {
			if r.node.Expanded {
				marker = "▾"
			} else {
				marker = "▸"
			}
		}
		// The indent is just blank columns already painted by the row fill above,
		// so write the content at an offset instead of building (and allocating) a
		// "  "-repeated prefix on every visible row each frame.
		indent := r.depth * 2
		avail := textW - indent
		if avail <= 0 {
			continue
		}
		// Truncate with a trailing ellipsis so overflow is signalled rather than
		// silently cutting mid-text. Truncate is display-width aware (wide glyphs
		// are not split).
		content := Truncate(marker+" "+r.node.Label, avail, "…")
		surface.WriteString(abs.X+indent, y, content, tui.Cell{FG: fg, BG: bg})
	}
	if needBar {
		track := Rect{X: abs.X + abs.W - 1, Y: abs.Y, W: 1, H: abs.H}
		drawVScrollbar(surface, track, len(rows), abs.H, t.offset,
			activeTheme.WindowBorderFG, t.BG, component.Focused())
	}
}

// pageStep is the row jump for PageUp/PageDown: one visible viewport, falling
// back to a single row before the first draw establishes viewH.
func (t *Tree) pageStep() int {
	if t.viewH > 1 {
		return t.viewH
	}
	return 1
}

// moveSelection sets the selection to a clamped row index, scrolling it into
// view and firing OnSelect only when it actually changes.
func (t *Tree) moveSelection(to int, rows []treeRow) {
	to = clampInt(to, 0, len(rows)-1)
	if to == t.selected {
		return
	}
	t.selected = to
	t.ensureVisible()
	t.fireSelect(rows)
}

func (t *Tree) clampSelection(total int) {
	if t.selected > total-1 {
		t.selected = total - 1
	}
	if t.selected < 0 {
		t.selected = 0
	}
}

// ensureVisible scrolls the minimum amount so the selection is on screen. It is
// only called from keyboard navigation, leaving wheel/scrollbar scrolling free.
func (t *Tree) ensureVisible() {
	if t.viewH < 1 {
		return
	}
	if t.selected < t.offset {
		t.offset = t.selected
	}
	if t.selected >= t.offset+t.viewH {
		t.offset = t.selected - t.viewH + 1
	}
	if t.offset < 0 {
		t.offset = 0
	}
}
func (t *Tree) fireSelect(rows []treeRow) {
	if t.OnSelect != nil && t.selected >= 0 && t.selected < len(rows) {
		t.OnSelect(rows[t.selected].node)
	}
}

// fireSelectMouse notifies OnSelectMouse of the clicked node. It is called only
// from the mouse-click path so hosts can act on a pointer click without reacting
// to keyboard traversal; the keyboard path never invokes it. It takes the node
// directly rather than re-reading t.selected, so a host's OnSelect that
// reentrantly moves the selection (e.g. a focus snap-back via SelectNode) cannot
// redirect OnSelectMouse to a different row than the one clicked.
func (t *Tree) fireSelectMouse(node *TreeNode) {
	if t.OnSelectMouse != nil {
		t.OnSelectMouse(node)
	}
}
func (t *Tree) handleType(_ *VisualComponent, event tui.TypeEvent) bool {
	rows := t.flatten()
	if len(rows) == 0 {
		return false
	}
	switch event.Key {
	case tui.KeyUp:
		if t.selected > 0 {
			t.selected--
			t.ensureVisible()
			t.fireSelect(rows)
		}
	case tui.KeyDown:
		if t.selected < len(rows)-1 {
			t.selected++
			t.ensureVisible()
			t.fireSelect(rows)
		}
	case tui.KeyHome:
		t.moveSelection(0, rows)
	case tui.KeyEnd:
		t.moveSelection(len(rows)-1, rows)
	case tui.KeyPageUp:
		t.moveSelection(t.selected-t.pageStep(), rows)
	case tui.KeyPageDown:
		t.moveSelection(t.selected+t.pageStep(), rows)
	case tui.KeyRight:
		n := rows[t.selected].node
		if len(n.Children) > 0 && !n.Expanded {
			n.Expanded = true
		} else if len(n.Children) > 0 && t.selected < len(rows)-1 {
			t.selected++
			t.ensureVisible()
			t.fireSelect(rows)
		}
	case tui.KeyLeft:
		n := rows[t.selected].node
		if len(n.Children) > 0 && n.Expanded {
			n.Expanded = false
		} else {
			t.selectParent(rows)
		}
	case tui.KeyEnter:
		if t.OnActivate != nil {
			t.OnActivate(rows[t.selected].node)
		}
	case tui.KeyRune:
		if event.Rune == ' ' {
			n := rows[t.selected].node
			if len(n.Children) > 0 {
				n.Expanded = !n.Expanded
			}
		} else {
			return false
		}
	default:
		return false
	}
	return true
}

// selectParent moves selection to the nearest shallower ancestor row above.
func (t *Tree) selectParent(rows []treeRow) {
	depth := rows[t.selected].depth
	for i := t.selected - 1; i >= 0; i-- {
		if rows[i].depth < depth {
			t.selected = i
			t.ensureVisible()
			t.fireSelect(rows)
			return
		}
	}
}
func (t *Tree) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	abs := component.AbsoluteBounds()
	rows := t.flatten()
	needBar := len(rows) > abs.H
	// Scrollbar interaction (the last column when a bar is shown), handled on
	// both press and drag so the thumb can be dragged.
	if needBar && event.X == abs.X+abs.W-1 {
		track := Rect{X: abs.X + abs.W - 1, Y: abs.Y, W: 1, H: abs.H}
		if off, ok := scrollbarOffsetForY(track, len(rows), abs.H, t.offset, event.Y); ok {
			t.offset = off
		}
		return true
	}
	if event.Down {
		return true
	}
	if !abs.Contains(event.X, event.Y) {
		return true
	}
	idx := t.offset + (event.Y - abs.Y)
	if idx < 0 || idx >= len(rows) {
		return true
	}
	wasSelected := t.selected == idx
	t.selected = idx
	r := rows[idx]
	// Clicking on (or before) the expand marker toggles the node.
	markerCol := abs.X + r.depth*2
	toggledMarker := false
	if len(r.node.Children) > 0 && event.X <= markerCol {
		r.node.Expanded = !r.node.Expanded
		toggledMarker = true
	}
	// Capture the clicked node before OnSelect runs: a host's OnSelect may
	// reentrantly move the selection (e.g. a focus snap-back via SelectNode), and
	// OnSelectMouse must still report the row the user actually clicked.
	clicked := r.node
	t.fireSelect(rows)
	// OnSelectMouse fires alongside OnSelect, but only here on the click path, so
	// hosts can distinguish a pointer click from keyboard traversal.
	t.fireSelectMouse(clicked)
	// A click on an already-selected row (not on its expand marker) also
	// activates it, so callers can open/preview an item on a repeat click in
	// addition to Enter.
	if wasSelected && !toggledMarker && t.OnActivate != nil {
		t.OnActivate(r.node)
	}
	return true
}
func (t *Tree) handleScroll(_ *VisualComponent, event tui.ScrollEvent) bool {
	// Delta is +1 for wheel-up, -1 for wheel-down, so subtracting scrolls the
	// content in the natural direction. The upper bound is enforced in draw().
	t.offset -= event.Delta
	if t.offset < 0 {
		t.offset = 0
	}
	return true
}
