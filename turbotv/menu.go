package tv

import tui "github.com/hobbestherat/turbotui"

type MenuShortcut struct {
	Display string
	Key     tui.KeyCode
	Rune    rune
	Ctrl    bool
}

type MenuItem struct {
	Label    string
	Shortcut *MenuShortcut
	Children []*MenuItem
	OnSelect func()
	Enabled  bool
}

func NewMenuItem(label string, onSelect func()) *MenuItem {
	return &MenuItem{
		Label:    label,
		OnSelect: onSelect,
		Enabled:  true,
	}
}

func NewSubMenu(label string, children ...*MenuItem) *MenuItem {
	return &MenuItem{
		Label:    label,
		Children: children,
		Enabled:  true,
	}
}

func (m *MenuItem) WithShortcut(display string, key tui.KeyCode, r rune, ctrl bool) *MenuItem {
	m.Shortcut = &MenuShortcut{
		Display: display,
		Key:     key,
		Rune:    r,
		Ctrl:    ctrl,
	}
	return m
}

type MenuBar struct {
	Component *VisualComponent
	Menus     []*MenuItem

	FG        tui.Color
	BG        tui.Color
	HotFG     tui.Color
	HotBG     tui.Color
	SelectFG  tui.Color
	SelectBG  tui.Color
	Shadow    bool
	ShadowCol tui.Color

	openPath     []int
	hoverPath    []int
	topRects     []Rect
	popupLayouts []menuPopupLayout
}

type menuPopupLayout struct {
	rect      Rect
	items     []*MenuItem
	itemRects []Rect
	path      []int
}

func NewMenuBar(bounds Rect, menus ...*MenuItem) *MenuBar {
	bar := &MenuBar{
		Menus:     menus,
		FG:        activeTheme.MenuBarFG,
		BG:        activeTheme.MenuBarBG,
		HotFG:     activeTheme.MenuHotFG,
		HotBG:     activeTheme.MenuHotBG,
		SelectFG:  activeTheme.MenuSelectFG,
		SelectBG:  activeTheme.MenuSelectBG,
		Shadow:    true,
		ShadowCol: activeTheme.MenuShadow,
		openPath:  []int{},
		hoverPath: []int{},
	}
	bar.Component = NewComponent(bounds)
	bar.Component.DrawOutside = true
	bar.Component.DrawFn = bar.draw
	bar.Component.OnClickFn = bar.handleClick
	bar.Component.OnHitTestFn = bar.hitTest
	bar.Component.OnMnemonicFn = func(_ *VisualComponent, lower rune) bool {
		return bar.OpenTopByMnemonic(lower)
	}
	return bar
}

func (m *MenuBar) Root() *VisualComponent {
	return m.Component
}

func (m *MenuBar) draw(component *VisualComponent, surface Surface) {
	abs := component.AbsoluteBounds()
	surface.Fill(abs, tui.Cell{Ch: ' ', FG: m.FG, BG: m.BG})
	highlight := component.mnemonicActive
	m.topRects = m.layoutTopRects(abs)
	for idx, item := range m.Menus {
		rect := m.topRects[idx]
		selected := len(m.openPath) > 0 && m.openPath[0] == idx
		style := tui.Cell{FG: m.FG, BG: m.BG}
		if selected {
			style = tui.Cell{FG: m.SelectFG, BG: m.SelectBG, Bold: true}
			// Paint the whole rect (text plus one padding cell each side) so the
			// highlight reads as a solid block, not just the letters.
			surface.Fill(rect, tui.Cell{Ch: ' ', FG: style.FG, BG: style.BG})
		}
		drawMnemonic(surface, rect.X+1, rect.Y, item.Label, style, highlight, m.HotFG)
	}

	m.popupLayouts = m.layoutPopups()
	for _, popup := range m.popupLayouts {
		if m.Shadow {
			surface.DrawShadow(popup.rect, m.ShadowCol)
		}
		surface.Fill(popup.rect, tui.Cell{Ch: ' ', FG: m.FG, BG: m.BG})
		surface.DrawBox(popup.rect, tui.LineSingle, m.FG, m.BG)
		for index, item := range popup.items {
			lineRect := popup.itemRects[index]
			path := append(append([]int{}, popup.path...), index)
			style := tui.Cell{FG: m.FG, BG: m.BG}
			if pathsEqual(path, m.hoverPath) || pathsEqual(path, m.openPath) {
				style = tui.Cell{FG: m.SelectFG, BG: m.SelectBG, Bold: true}
			}
			if !item.Enabled {
				style.FG = tui.ANSIColor(8)
			}
			// Fill the full row so the selection highlight runs edge to edge with
			// no gap between the label and the shortcut hint.
			surface.Fill(lineRect, tui.Cell{Ch: ' ', FG: style.FG, BG: style.BG})
			drawMnemonic(surface, lineRect.X+1, lineRect.Y, item.Label, style, item.Enabled, m.HotFG)
			if item.Shortcut != nil && item.Shortcut.Display != "" {
				shortX := lineRect.Right() - len([]rune(item.Shortcut.Display)) - 1
				if shortX > lineRect.X+1 {
					surface.WriteString(shortX, lineRect.Y, item.Shortcut.Display, style)
				}
			}
			if len(item.Children) > 0 {
				surface.WriteString(lineRect.Right()-1, lineRect.Y, "►", style)
			}
		}
	}
}

func (m *MenuBar) layoutTopRects(abs Rect) []Rect {
	rects := make([]Rect, len(m.Menus))
	x := abs.X
	for idx, item := range m.Menus {
		text, _ := parseMnemonic(item.Label)
		width := len([]rune(text)) + 2
		rects[idx] = Rect{X: x, Y: abs.Y, W: width, H: 1}
		x += width
	}
	return rects
}

func (m *MenuBar) layoutPopups() []menuPopupLayout {
	if len(m.openPath) == 0 || len(m.topRects) == 0 {
		return nil
	}
	topIndex := m.openPath[0]
	if topIndex < 0 || topIndex >= len(m.Menus) {
		return nil
	}
	items := m.Menus[topIndex].Children
	if len(items) == 0 {
		return nil
	}
	layouts := make([]menuPopupLayout, 0, 4)
	anchor := m.topRects[topIndex]
	rect := Rect{X: anchor.X, Y: anchor.Y + 1, W: popupWidth(items), H: len(items) + 2}
	pathPrefix := []int{topIndex}
	level := 0
	for {
		itemRects := make([]Rect, len(items))
		for i := range items {
			itemRects[i] = Rect{X: rect.X + 1, Y: rect.Y + 1 + i, W: rect.W - 2, H: 1}
		}
		layouts = append(layouts, menuPopupLayout{
			rect:      rect,
			items:     items,
			itemRects: itemRects,
			path:      append([]int{}, pathPrefix...),
		})
		if len(m.openPath) <= level+1 {
			break
		}
		selected := m.openPath[level+1]
		if selected < 0 || selected >= len(items) {
			break
		}
		next := items[selected]
		if len(next.Children) == 0 {
			break
		}
		pathPrefix = append(pathPrefix, selected)
		rect = Rect{
			X: rect.Right(),
			Y: rect.Y + 1 + selected,
			W: popupWidth(next.Children),
			H: len(next.Children) + 2,
		}
		items = next.Children
		level++
	}
	return layouts
}

func popupWidth(items []*MenuItem) int {
	width := 8
	for _, item := range items {
		text, _ := parseMnemonic(item.Label)
		rowWidth := len([]rune(text)) + 4
		if item.Shortcut != nil {
			rowWidth += len([]rune(item.Shortcut.Display)) + 2
		}
		if len(item.Children) > 0 {
			rowWidth += 2
		}
		if rowWidth > width {
			width = rowWidth
		}
	}
	return width
}

func (m *MenuBar) hitTest(component *VisualComponent, x int, y int) bool {
	abs := component.AbsoluteBounds()
	if abs.Contains(x, y) {
		return true
	}
	for _, popup := range m.popupLayouts {
		if popup.rect.Contains(x, y) {
			return true
		}
	}
	return false
}

func (m *MenuBar) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	_ = component
	if !event.Down {
		return true
	}
	if len(m.topRects) == 0 {
		return false
	}
	for idx, rect := range m.topRects {
		if rect.Contains(event.X, event.Y) {
			if len(m.openPath) > 0 && m.openPath[0] == idx {
				m.openPath = []int{}
				m.hoverPath = []int{}
				return true
			}
			m.openPath = m.defaultOpenPath(idx)
			m.hoverPath = []int{idx}
			return true
		}
	}
	for _, popup := range m.popupLayouts {
		for row, rect := range popup.itemRects {
			if !rect.Contains(event.X, event.Y) {
				continue
			}
			path := append(append([]int{}, popup.path...), row)
			item := popup.items[row]
			m.hoverPath = path
			if !item.Enabled {
				return true
			}
			if len(item.Children) > 0 {
				m.openPath = path
				return true
			}
			if item.OnSelect != nil {
				item.OnSelect()
			}
			m.openPath = []int{}
			m.hoverPath = []int{}
			return true
		}
	}
	m.openPath = []int{}
	m.hoverPath = []int{}
	return true
}

// IsOpen reports whether a menu is currently dropped down. While open the menubar
// captures the keyboard (see Desktop.handleType).
func (m *MenuBar) IsOpen() bool {
	return len(m.openPath) > 0
}

// HitTest reports whether (x, y) falls on the menu bar itself or any open
// dropdown, so the desktop can route clicks to the (always-on-top) menubar first.
func (m *MenuBar) HitTest(x int, y int) bool {
	return m.hitTest(m.Component, x, y)
}

// CloseMenus collapses any open dropdown.
func (m *MenuBar) CloseMenus() {
	m.openPath = []int{}
	m.hoverPath = []int{}
}

// HandleAccelerator fires Ctrl-style accelerators (e.g. Ctrl+S) declared via
// WithShortcut. It works regardless of whether a menu is open.
func (m *MenuBar) HandleAccelerator(event tui.TypeEvent) bool {
	if m.handleShortcuts(event, m.Menus) {
		m.openPath = []int{}
		m.hoverPath = []int{}
		return true
	}
	return false
}

// HandleKey drives keyboard navigation while a menu is open: arrows, Enter,
// Escape, Alt+mnemonic to switch top menu, and a plain mnemonic letter to pick an
// item at the current level (so "Alt-f f" opens File then Find).
func (m *MenuBar) HandleKey(event tui.TypeEvent) bool {
	if len(m.openPath) == 0 {
		return false
	}
	switch event.Key {
	case tui.KeyEscape:
		m.openPath = []int{}
		m.hoverPath = []int{}
		return true
	case tui.KeyEnter:
		item := m.currentItem()
		if item == nil || !item.Enabled {
			return true
		}
		if len(item.Children) > 0 {
			m.openPath = append(m.openPath, 0)
			return true
		}
		if item.OnSelect != nil {
			item.OnSelect()
		}
		m.openPath = []int{}
		m.hoverPath = []int{}
		return true
	case tui.KeyUp:
		m.moveSelection(-1)
		return true
	case tui.KeyDown:
		m.moveSelection(1)
		return true
	case tui.KeyLeft:
		// Step back out of a nested submenu, otherwise switch to the previous
		// top-level menu (opening its dropdown).
		if len(m.openPath) > 2 {
			m.openPath = m.openPath[:len(m.openPath)-1]
			return true
		}
		if len(m.Menus) > 0 {
			prev := (m.openPath[0] - 1 + len(m.Menus)) % len(m.Menus)
			m.openPath = m.defaultOpenPath(prev)
			m.hoverPath = []int{prev}
			return true
		}
	case tui.KeyRight:
		// Descend into a submenu if the selected item has one, otherwise switch
		// to the next top-level menu (opening its dropdown).
		item := m.currentItem()
		if item != nil && len(item.Children) > 0 {
			m.openPath = append(m.openPath, 0)
			return true
		}
		if len(m.Menus) > 0 {
			next := (m.openPath[0] + 1) % len(m.Menus)
			m.openPath = m.defaultOpenPath(next)
			m.hoverPath = []int{next}
			return true
		}
	case tui.KeyRune:
		if event.Alt {
			// Alt+letter first tries to switch to another top-level menu; if no top
			// menu owns the letter, fall back to selecting an item at the current
			// level, so "Alt+F Alt+X" works just like "Alt+F x".
			if m.OpenTopByMnemonic(unicodeLower(event.Rune)) {
				return true
			}
			m.selectByMnemonic(unicodeLower(event.Rune))
			return true
		}
		if event.Ctrl {
			return false
		}
		m.selectByMnemonic(unicodeLower(event.Rune))
		return true
	}
	return true
}

// selectByMnemonic activates (or descends into) the item at the current popup
// level whose mnemonic matches lower.
func (m *MenuBar) selectByMnemonic(lower rune) bool {
	items := m.itemsAtCurrentDepth()
	if len(items) == 0 {
		return false
	}
	depth := len(m.openPath) - 1
	for index, item := range items {
		if !item.Enabled || labelMnemonic(item.Label) != lower {
			continue
		}
		m.openPath[depth] = index
		if len(item.Children) > 0 {
			m.openPath = append(m.openPath, 0)
			return true
		}
		if item.OnSelect != nil {
			item.OnSelect()
		}
		m.openPath = []int{}
		m.hoverPath = []int{}
		return true
	}
	return false
}

// topMnemonics returns the lowercased mnemonics of the top-level menus so the
// desktop can reserve them when resolving clashes.
func (m *MenuBar) topMnemonics() []rune {
	out := make([]rune, 0, len(m.Menus))
	for _, item := range m.Menus {
		if r := labelMnemonic(item.Label); r != 0 {
			out = append(out, r)
		}
	}
	return out
}

func (m *MenuBar) currentItem() *MenuItem {
	if len(m.openPath) == 0 {
		return nil
	}
	items := m.Menus
	var current *MenuItem
	for _, idx := range m.openPath {
		if idx < 0 || idx >= len(items) {
			return nil
		}
		current = items[idx]
		items = current.Children
	}
	return current
}

func (m *MenuBar) moveSelection(delta int) {
	if len(m.openPath) == 0 {
		return
	}
	items := m.itemsAtCurrentDepth()
	if len(items) == 0 {
		return
	}
	depth := len(m.openPath) - 1
	current := m.openPath[depth]
	m.openPath[depth] = (current + delta + len(items)) % len(items)
}

func (m *MenuBar) itemsAtCurrentDepth() []*MenuItem {
	if len(m.openPath) == 0 {
		return nil
	}
	items := m.Menus
	for depth, idx := range m.openPath {
		if depth == len(m.openPath)-1 {
			return items
		}
		if idx < 0 || idx >= len(items) {
			return nil
		}
		items = items[idx].Children
	}
	return nil
}

func (m *MenuBar) defaultOpenPath(top int) []int {
	if top < 0 || top >= len(m.Menus) {
		return []int{}
	}
	if len(m.Menus[top].Children) > 0 {
		return []int{top, 0}
	}
	return []int{top}
}

func (m *MenuBar) OpenTopByMnemonic(lower rune) bool {
	for idx, item := range m.Menus {
		if labelMnemonic(item.Label) == lower {
			m.openPath = m.defaultOpenPath(idx)
			m.hoverPath = []int{idx}
			return true
		}
	}
	return false
}

func (m *MenuBar) handleShortcuts(event tui.TypeEvent, items []*MenuItem) bool {
	for _, item := range items {
		if item.Shortcut != nil && matchShortcut(event, item.Shortcut) && item.Enabled {
			if item.OnSelect != nil {
				item.OnSelect()
			}
			return true
		}
		if len(item.Children) > 0 {
			if m.handleShortcuts(event, item.Children) {
				return true
			}
		}
	}
	return false
}

func matchShortcut(event tui.TypeEvent, shortcut *MenuShortcut) bool {
	if shortcut == nil {
		return false
	}
	if shortcut.Key != tui.KeyUnknown && event.Key != shortcut.Key {
		return false
	}
	if shortcut.Rune != 0 {
		if event.Key != tui.KeyRune || unicodeLower(event.Rune) != unicodeLower(shortcut.Rune) {
			return false
		}
	}
	if shortcut.Ctrl != event.Ctrl {
		return false
	}
	return true
}

func pathsEqual(a []int, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
