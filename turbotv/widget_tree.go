package tv

import (
	"strings"

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
	selected  int
	offset    int
	viewH     int // visible row count from the last draw (for ensureVisible)
	// OnActivate fires on Enter for the selected node; OnSelect fires whenever
	// the selection changes.
	OnActivate func(*TreeNode)
	OnSelect   func(*TreeNode)
}

// NewTree creates an empty tree view.
func NewTree(bounds Rect) *Tree {
	t := &Tree{
		FG:    DefaultTheme.WindowFG,
		BG:    DefaultTheme.WindowBG,
		SelFG: DefaultTheme.SelectionFG,
		SelBG: DefaultTheme.SelectionBG,
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
func (t *Tree) flatten() []treeRow {
	var rows []treeRow
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
			fg, bg = t.SelFG, t.SelBG
		}
		y := abs.Y + row
		surface.Fill(Rect{X: abs.X, Y: y, W: textW, H: 1}, tui.Cell{Ch: ' ', FG: fg, BG: bg})
		marker := " "
		if len(r.node.Children) > 0 {
			if r.node.Expanded {
				marker = "▾"
			} else {
				marker = "▸"
			}
		}
		line := strings.Repeat("  ", r.depth) + marker + " " + r.node.Label
		runes := []rune(line)
		if len(runes) > textW {
			runes = runes[:textW]
		}
		surface.WriteString(abs.X, y, string(runes), tui.Cell{FG: fg, BG: bg})
	}
	if needBar {
		track := Rect{X: abs.X + abs.W - 1, Y: abs.Y, W: 1, H: abs.H}
		drawVScrollbar(surface, track, len(rows), abs.H, t.offset,
			DefaultTheme.WindowBorderFG, t.BG, component.HasFocus)
	}
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
	t.fireSelect(rows)
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
