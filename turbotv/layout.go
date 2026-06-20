package tv

// Align controls how a child of a Box is placed within spare space on the box's
// cross axis (perpendicular to the direction of packing). AlignStretch (the
// default) makes the child fill the cross axis; the others keep the child's
// natural cross size and position it.
type Align uint8

const (
	AlignStretch Align = iota
	AlignStart
	AlignCenter
	AlignEnd
)

// Box is a single-row (HBox) or single-column (VBox) flex container. It lays its
// children out in its LayoutFn, so it integrates with the standard
// SetBounds → LayoutFn propagation: whenever the box is resized, its children are
// repositioned automatically.
//
// On the main axis (width for HBox, height for VBox) each child's size is:
//   - equal share of the available space when Homogeneous is true;
//   - otherwise its own Bounds.W/H, unless Flex > 0, in which case it shares the
//     space left over after every non-flex child has taken its natural size
//     (weighted by Flex).
//
// Spacing is added between adjacent children. Align positions children on the
// cross axis.
type Box struct {
	Component   *VisualComponent
	Horizontal  bool
	Spacing     int
	Homogeneous bool
	Align       Align
}

// NewHBox creates a horizontal Box (children packed left to right).
func NewHBox(bounds Rect) *Box {
	return newBox(bounds, true)
}

// NewVBox creates a vertical Box (children packed top to bottom).
func NewVBox(bounds Rect) *Box {
	return newBox(bounds, false)
}

func newBox(bounds Rect, horizontal bool) *Box {
	box := &Box{Horizontal: horizontal, Align: AlignStretch}
	box.Component = NewComponent(bounds)
	box.Component.LayoutFn = box.layout
	return box
}

func (b *Box) Root() *VisualComponent { return b.Component }

// Add appends a child widget and returns the box for chaining.
func (b *Box) Add(child Widget) *Box {
	b.Component.AddChild(child)
	return b
}

func (b *Box) layout(component *VisualComponent) {
	children := component.children
	if len(children) == 0 {
		return
	}
	spacing := b.Spacing
	if spacing < 0 {
		spacing = 0
	}
	// Children are positioned relative to this component's own origin, so only
	// the component's size (not its X/Y position) drives the layout.
	if b.Horizontal {
		sizes := boxMainSizes(children, component.Bounds.W, spacing, b.Homogeneous)
		x := 0
		for index, child := range children {
			y, h := crossPlace(b.Align, component.Bounds.H, child.Bounds.H)
			child.SetBounds(Rect{X: x, Y: y, W: sizes[index], H: h})
			x += sizes[index] + spacing
		}
		return
	}
	sizes := boxMainSizes(children, component.Bounds.H, spacing, b.Homogeneous)
	y := 0
	for index, child := range children {
		x, w := crossPlace(b.Align, component.Bounds.W, child.Bounds.W)
		child.SetBounds(Rect{X: x, Y: y, W: w, H: sizes[index]})
		y += sizes[index] + spacing
	}
}

// boxMainSizes resolves each child's size along the packing axis from the given
// available length. It implements the homogeneous / natural / flex rules shared
// by HBox and VBox.
func boxMainSizes(children []*VisualComponent, available, spacing int, homogeneous bool) []int {
	count := len(children)
	sizes := make([]int, count)
	totalSpacing := spacing * (count - 1)
	if totalSpacing < 0 {
		totalSpacing = 0
	}
	room := available - totalSpacing
	if room < 0 {
		room = 0
	}
	if homogeneous {
		each := room / count
		for index := range sizes {
			sizes[index] = each
		}
		return sizes
	}
	totalFlex := 0
	fixed := 0
	for _, child := range children {
		if child.Flex > 0 {
			totalFlex += child.Flex
			continue
		}
		natural := child.naturalMain(true)
		fixed += natural
	}
	if totalFlex == 0 {
		for index, child := range children {
			sizes[index] = child.naturalMain(true)
		}
		return sizes
	}
	free := room - fixed
	if free < 0 {
		free = 0
	}
	allocated := 0
	lastFlex := -1
	for index, child := range children {
		if child.Flex > 0 {
			sizes[index] = free * child.Flex / totalFlex
			allocated += sizes[index]
			lastFlex = index
			continue
		}
		sizes[index] = child.naturalMain(true)
	}
	if lastFlex >= 0 {
		sizes[lastFlex] += free - allocated
		if sizes[lastFlex] < 0 {
			sizes[lastFlex] = 0
		}
	}
	return sizes
}

// naturalMain returns the child's natural size along the packing axis: Bounds.W
// for a horizontal box, Bounds.H for a vertical one. The horizontal flag is taken
// from the owning box; negative values clamp at zero.
func (c *VisualComponent) naturalMain(horizontal bool) int {
	size := c.Bounds.W
	if !horizontal {
		size = c.Bounds.H
	}
	if size < 0 {
		size = 0
	}
	return size
}

// crossPlace resolves the (origin, size) of a child on the cross axis given the
// box's cross extent, the child's natural cross size and the alignment. The
// origin is relative to the box (0 = the box's near edge on that axis).
func crossPlace(align Align, boxSize, natural int) (int, int) {
	size := natural
	if size > boxSize {
		size = boxSize
	}
	if size < 0 {
		size = 0
	}
	switch align {
	case AlignStart:
		return 0, size
	case AlignCenter:
		return (boxSize - size) / 2, size
	case AlignEnd:
		return boxSize - size, size
	default: // AlignStretch
		if boxSize < 0 {
			boxSize = 0
		}
		return 0, boxSize
	}
}

// Grid arranges its children into a fixed Cols × Rows grid of equal cells. Child
// k is placed at column k%Cols, row k/Cols. Padding insets the whole grid and
// Spacing is added between cells. Like Box it lays out in its LayoutFn.
type Grid struct {
	Component *VisualComponent
	Cols      int
	Rows      int
	Spacing   int
	Padding   int
}

// NewGrid creates a Grid with the given number of columns and rows.
func NewGrid(bounds Rect, cols int, rows int) *Grid {
	grid := &Grid{Cols: cols, Rows: rows}
	grid.Component = NewComponent(bounds)
	grid.Component.LayoutFn = grid.layout
	return grid
}

func (g *Grid) Root() *VisualComponent { return g.Component }

// Add appends a child widget and returns the grid for chaining.
func (g *Grid) Add(child Widget) *Grid {
	g.Component.AddChild(child)
	return g
}

func (g *Grid) layout(component *VisualComponent) {
	children := component.children
	if len(children) == 0 || g.Cols < 1 || g.Rows < 1 {
		return
	}
	spacing := g.Spacing
	if spacing < 0 {
		spacing = 0
	}
	// Cells are laid out relative to the grid's own origin; inset the size only.
	inner := Rect{W: component.Bounds.W, H: component.Bounds.H}.Inset(g.Padding)
	cellW := (inner.W - spacing*(g.Cols-1)) / g.Cols
	cellH := (inner.H - spacing*(g.Rows-1)) / g.Rows
	if cellW < 0 {
		cellW = 0
	}
	if cellH < 0 {
		cellH = 0
	}
	for index, child := range children {
		row := index / g.Cols
		if row >= g.Rows {
			break
		}
		col := index % g.Cols
		x := inner.X + col*(cellW+spacing)
		y := inner.Y + row*(cellH+spacing)
		child.SetBounds(Rect{X: x, Y: y, W: cellW, H: cellH})
	}
}
