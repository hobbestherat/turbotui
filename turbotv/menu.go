package tv

import tui "github.com/hobbestherat/turbotui"

type MenuShortcut struct {
	Display string
	Key     tui.KeyCode
	Rune    rune
	Ctrl    bool
	// Shift and Alt extend accelerators beyond Ctrl combos so chords like Shift+F1
	// or Ctrl+Shift+S (and bare function-key accelerators, with Key set to KeyF1…)
	// can be bound (issue #61).
	Shift bool
	Alt   bool
}

type MenuItem struct {
	Label string
	// Shortcut is the accelerator HINT drawn at the right of the item; it is
	// display-only. The menu bar no longer owns a BindingRegistry and does not
	// register accelerators from its tree — global accelerators live on the
	// Desktop's single registry (see Desktop.Bindings) and the menu reads the
	// rendered Display string from there. A nil Shortcut draws no hint.
	Shortcut *MenuShortcut
	Children []*MenuItem
	OnSelect func()
	Enabled  bool

	// ActionID is an opaque identifier for the action this item triggers — the
	// opaque contract turbotui shares with the application. It is optional and empty
	// by default. The menu bar no longer registers bindings from its tree, so this
	// is purely a label an application can use to associate a menu item with the
	// matching binding it registers on the Desktop registry (e.g. to sync the
	// Shortcut display when the binding is rebound). The item always fires through
	// OnSelect, so leaving this empty changes nothing.
	ActionID ActionID

	// Separator draws a non-selectable horizontal rule instead of a label and is
	// skipped by keyboard/mouse navigation.
	Separator bool

	// Checkable items render a √/○ glyph in the label gutter; activating one flips
	// Checked and calls OnToggle (instead of OnSelect).
	Checkable bool
	Checked   bool
	OnToggle  func(checked bool)

	// RightAligned packs this top-level menu from the right edge inward (to the left
	// of the status slot) instead of left-packing it (issue #500). It only affects a
	// top-level menu (an entry of MenuBar.Menus); it is ignored on child items. The
	// zero value is false, so a menu is left-packed exactly as before unless opted in.
	RightAligned bool
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

// NewSeparator returns a non-selectable divider for use between menu items.
func NewSeparator() *MenuItem {
	return &MenuItem{Separator: true}
}

// NewCheckMenuItem returns a checkable (toggle) menu item. Activating it flips its
// Checked state and invokes onToggle with the new value.
func NewCheckMenuItem(label string, checked bool, onToggle func(checked bool)) *MenuItem {
	return &MenuItem{
		Label:     label,
		Enabled:   true,
		Checkable: true,
		Checked:   checked,
		OnToggle:  onToggle,
	}
}

// WithShortcut sets the item's accelerator HINT (the string drawn at the right of
// the item). It is display-only: the menu bar does not register bindings from its
// tree, so this does not make the chord fire — the application registers the
// matching Global accelerator on the Desktop registry (see Desktop.Bindings). The
// key/rune/ctrl fields back MenuShortcut.Chord so a caller can derive the chord the
// hint represents.
func (m *MenuItem) WithShortcut(display string, key tui.KeyCode, r rune, ctrl bool) *MenuItem {
	m.Shortcut = &MenuShortcut{
		Display: display,
		Key:     key,
		Rune:    r,
		Ctrl:    ctrl,
	}
	return m
}

// WithShortcutMod is the modifier-aware form of WithShortcut: it sets a display
// hint carrying any combination of Ctrl/Shift/Alt, describing chords such as Shift+F1
// or Ctrl+Shift+S and bare function-key accelerators (pass key = tui.KeyF1…, r = 0).
// Like WithShortcut it is display-only — it does not register a binding; the
// application registers the matching accelerator on the Desktop registry.
func (m *MenuItem) WithShortcutMod(display string, key tui.KeyCode, r rune, ctrl bool, shift bool, alt bool) *MenuItem {
	m.Shortcut = &MenuShortcut{
		Display: display,
		Key:     key,
		Rune:    r,
		Ctrl:    ctrl,
		Shift:   shift,
		Alt:     alt,
	}
	return m
}

// WithActionID tags the item with an opaque ActionID labelling the action it
// triggers. It is additive and display-oriented: the menu bar no longer registers
// bindings from its tree, so this only labels the item (an application can use it to
// keep the Shortcut hint in sync with the binding it registered on the Desktop
// registry). Items without an ActionID fire through OnSelect exactly as before.
func (m *MenuItem) WithActionID(id ActionID) *MenuItem {
	m.ActionID = id
	return m
}

// AlignRight marks a top-level menu to be packed from the right edge inward, to the
// left of the menu bar's status slot (issue #500), instead of left-packed with the
// rest. It is the fluent form of setting RightAligned and is additive: an item that
// is never marked stays left-packed exactly as before. It has no effect on a child
// (submenu) item.
func (m *MenuItem) AlignRight() *MenuItem {
	m.RightAligned = true
	return m
}

// Chord returns the key combination this shortcut's fields describe, decoupled from
// its Display string. It bridges the MenuShortcut representation to the first-class
// Chord used by the BindingRegistry, so an application can derive the Chord a menu
// item's hint represents. A nil shortcut yields the zero Chord; note the zero Chord
// is not inert — with no named key and no rune to compare it matches any
// modifier-free event (this mirrors matchShortcut, which guards Shortcut != nil).
func (s *MenuShortcut) Chord() Chord {
	if s == nil {
		return Chord{}
	}
	return Chord{Key: s.Key, Rune: s.Rune, Ctrl: s.Ctrl, Shift: s.Shift, Alt: s.Alt}
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
	ShadowSty ShadowStyle

	// StatusText is a right-anchored status label drawn flush-right within the bar
	// (issue #500). The empty string (the default) draws nothing, so a bar that never
	// sets it renders exactly as before. It is measured by display width (so glyphs
	// like ●/○/◐ and any wide rune size correctly) and rendered literally — it is NOT
	// '&'-mnemonic-parsed and shows no hot-key underline. Update it with SetStatus.
	StatusText string
	// StatusFG/StatusBG colour the whole status string; a zero value falls back to the
	// bar's FG/BG. There is no per-glyph colouring in v1 — the single pair applies to
	// the entire string.
	StatusFG tui.Color
	StatusBG tui.Color

	openPath     []int
	hoverPath    []int
	topRects     []Rect
	popupLayouts []menuPopupLayout
	// statusSlotW is the clamped width (status text + 1 pad cell) reserved at the right
	// edge for StatusText, computed once in layoutTopRects and read back by draw so the
	// reserved slot and the painted text can never drift. It is 0 when no status shows.
	statusSlotW int
}

type menuPopupLayout struct {
	rect      Rect
	items     []*MenuItem
	itemRects []Rect // one per item; off-screen (scrolled-out) rows are the zero Rect
	path      []int
	offset    int  // index of the first visible item when the popup scrolls
	scrollbar bool // true when the list is taller than the visible area
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
		ShadowSty: DefaultShadowStyle,
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

// SetStatus updates the right-anchored status text (issue #500). Like the other
// MenuBar/widget mutators (SetVisible/SetEnabled/…) it is a pure state change and does
// NOT repaint by itself: call it on the event-loop goroutine and pair it with a redraw,
// i.e. from a background goroutine use Desktop.Post(func(){ bar.SetStatus(...) }) (Post
// requests the coalesced redraw), or follow it with Desktop.Redraw(). The empty string
// clears the slot.
func (m *MenuBar) SetStatus(text string) {
	m.StatusText = text
}

// SetStatusColors overrides the colours of the status text. A zero Color falls back to
// the bar's FG/BG, so SetStatusColors(tui.Color{}, tui.Color{}) restores the defaults.
// The pair applies to the whole status string (no per-glyph colouring in v1). Same
// threading/redraw contract as SetStatus.
func (m *MenuBar) SetStatusColors(fg tui.Color, bg tui.Color) {
	m.StatusFG = fg
	m.StatusBG = bg
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
		if !item.Enabled {
			style.FG = tui.ANSIColor(8)
		}
		drawMnemonic(surface, rect.X+1, rect.Y, item.Label, style, highlight && item.Enabled, m.HotFG)
	}

	// Right-anchored status slot (issue #500), drawn AFTER the left/right top items so
	// the left items always own their cells. statusSlotW was clamped in layoutTopRects
	// (0 when there is no room or no StatusText). The text is right-aligned with exactly
	// one pad cell at the bar's last column, measured/truncated by display width so wide
	// runes (●/○/◐ and CJK) size correctly and are never split. Overflow truncates with
	// an ellipsis and then hides — it never wraps to a second row.
	if m.statusSlotW > 0 {
		statusFG := m.StatusFG
		if statusFG == (tui.Color{}) {
			statusFG = m.FG
		}
		statusBG := m.StatusBG
		if statusBG == (tui.Color{}) {
			statusBG = m.BG
		}
		textCols := m.statusSlotW - 1 // reserve one pad cell on the right
		text := Truncate(m.StatusText, textCols, "…")
		startX := abs.Right() - tui.StringWidth(text) // pad cell falls at abs.Right()
		surface.WriteString(startX, abs.Y, text, tui.Cell{FG: statusFG, BG: statusBG})
	}

	// The popup geometry is clamped/flipped against the full screen so right-edge
	// menus and their submenus stay on-screen, so it needs the surface dimensions.
	clip := surface.Clip()
	m.popupLayouts = m.layoutPopups(clip.X+clip.W, clip.Y+clip.H)
	for _, popup := range m.popupLayouts {
		if m.Shadow {
			surface.DrawShadow(popup.rect, m.ShadowCol, m.ShadowSty)
		}
		surface.Fill(popup.rect, tui.Cell{Ch: ' ', FG: m.FG, BG: m.BG})
		surface.DrawBox(popup.rect, tui.LineSingle, m.FG, m.BG)
		gutter := menuGutter(popup.items)
		for index, item := range popup.items {
			lineRect := popup.itemRects[index]
			if lineRect.Empty() {
				continue // scrolled out of view
			}
			if item.Separator {
				for x := popup.rect.X + 1; x < popup.rect.Right(); x++ {
					surface.SetCell(x, lineRect.Y, tui.Cell{Ch: '─', FG: m.FG, BG: m.BG})
				}
				continue
			}
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
			labelX := lineRect.X + 1
			if gutter > 0 {
				if item.Checkable {
					glyph := '○'
					if item.Checked {
						glyph = '√'
					}
					surface.SetCell(lineRect.X+1, lineRect.Y, tui.Cell{Ch: glyph, FG: style.FG, BG: style.BG, Bold: style.Bold})
				}
				labelX = lineRect.X + 1 + gutter
			}
			drawMnemonic(surface, labelX, lineRect.Y, item.Label, style, item.Enabled, m.HotFG)
			if item.Shortcut != nil && item.Shortcut.Display != "" {
				shortX := lineRect.Right() - len([]rune(item.Shortcut.Display)) - 1
				if shortX > labelX {
					surface.WriteString(shortX, lineRect.Y, item.Shortcut.Display, style)
				}
			}
			if len(item.Children) > 0 {
				surface.WriteString(lineRect.Right()-1, lineRect.Y, "►", style)
			}
		}
		if popup.scrollbar {
			visible := popup.rect.H - 2
			track := Rect{X: popup.rect.Right() - 1, Y: popup.rect.Y + 1, W: 1, H: visible}
			drawVScrollbar(surface, track, len(popup.items), visible, popup.offset, m.FG, m.BG, true)
		}
	}
}

// menuGutter returns the width reserved at the left of a popup's labels for the
// check glyph: 2 columns when the popup has any checkable item, else 0.
func menuGutter(items []*MenuItem) int {
	for _, item := range items {
		if item.Checkable {
			return 2
		}
	}
	return 0
}

// topItemWidth returns the cell width of a top-level menu's label box. It mirrors the
// historical formula EXACTLY — strip the '&' mnemonic marker, then len([]rune)+2 (one
// pad cell each side) — so existing left-packed bars stay byte-for-byte unchanged. The
// parseMnemonic step is load-bearing: every real label carries a mnemonic ("&File",
// "&Daemon", …) and measuring the raw label would count the '&' and shift the whole
// bar. Rune count (not display width) is kept deliberately so the measurement is
// identical to before; only the new status slot measures by display width (see draw).
func topItemWidth(item *MenuItem) int {
	text, _ := parseMnemonic(item.Label)
	return len([]rune(text)) + 2
}

// layoutTopRects positions every top-level menu, returning one rect per Menus entry,
// index-aligned with Menus (so hit-testing, popups and mnemonic dispatch can keep
// indexing Menus[i] ↔ topRects[i]). Left-aligned items pack from the left as before;
// RightAligned items pack from the right edge inward, to the left of a right-anchored
// status slot (issue #500). Precedence on the row, left → right:
//
//	[ left-packed menus ] … gutter … [ right-aligned menus ] [ status slot (≥1 pad) ]
//
// Priority when space is tight: left menus own their cells unconditionally; the status
// slot yields first (it shrinks, then hides); right menus yield next (clamped so they
// never start left of the left menus). All arithmetic uses the exclusive end barEnd so
// it stays correct against the inclusive Rect.Right(). With no RightAligned item and an
// empty StatusText this reduces to the original left-pack and produces identical rects.
func (m *MenuBar) layoutTopRects(abs Rect) []Rect {
	rects := make([]Rect, len(m.Menus))
	barEnd := abs.X + abs.W // exclusive right end; columns are [abs.X, barEnd)

	// (1) Left-pack left-aligned items from abs.X, exactly as before.
	x := abs.X
	for idx, item := range m.Menus {
		if item.RightAligned {
			continue
		}
		width := topItemWidth(item)
		rects[idx] = Rect{X: x, Y: abs.Y, W: width, H: 1}
		x += width
	}
	leftEnd := x // first free column after the left-packed menus

	// (2) Reserve the status slot (text + exactly one pad cell). It is the lowest
	// priority: clamp it to whatever is left after the left and right menus.
	desiredSlotW := 0
	if m.StatusText != "" {
		desiredSlotW = tui.StringWidth(m.StatusText) + 1
	}
	rightMenusW := 0
	for _, item := range m.Menus {
		if item.RightAligned {
			rightMenusW += topItemWidth(item)
		}
	}
	free := barEnd - leftEnd
	maxSlot := free - rightMenusW
	if maxSlot < 0 {
		maxSlot = 0
	}
	slotW := desiredSlotW
	if slotW > maxSlot {
		slotW = maxSlot
	}
	if slotW < 0 {
		slotW = 0
	}
	m.statusSlotW = slotW

	// (3) Right-pack right-aligned items, iterating in REVERSE so declared
	// left-to-right reading order is preserved (last-declared sits nearest the slot).
	cursor := barEnd - slotW // first column the slot owns
	for idx := len(m.Menus) - 1; idx >= 0; idx-- {
		item := m.Menus[idx]
		if !item.RightAligned {
			continue
		}
		width := topItemWidth(item)
		cursor -= width
		rx := cursor
		if rx < leftEnd {
			rx = leftEnd // never start left of the left-packed menus
		}
		rects[idx] = Rect{X: rx, Y: abs.Y, W: width, H: 1}
	}
	return rects
}

// layoutPopups computes the rect (and per-item rects) of every open dropdown,
// clamping/flipping each one to stay within appW x appH and capping its height
// with scrolling so long menus remain reachable. appW/appH are the screen bounds.
func (m *MenuBar) layoutPopups(appW int, appH int) []menuPopupLayout {
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
	pathPrefix := []int{topIndex}
	level := 0

	width := popupWidth(items)
	// Top-level dropdown drops below its label; flip left when it would overflow
	// the right edge so the shortcut hints and ► glyphs stay on-screen.
	x := anchor.X
	if x+width > appW {
		x = appW - width
	}
	if x < 0 {
		x = 0
	}
	y := anchor.Y + 1
	topLevel := true

	for {
		// Slide the box up so it fits on screen; the top-level dropdown stays
		// anchored to the bar and only caps its height (scrolling instead).
		boxY := y
		boxH := len(items) + 2
		if !topLevel && boxY+boxH > appH {
			boxY = appH - boxH
		}
		if boxY < 0 {
			boxY = 0
		}
		if boxY+boxH > appH {
			boxH = appH - boxY
		}
		if boxH < 3 {
			boxH = 3
		}
		rect := Rect{X: x, Y: boxY, W: width, H: boxH}

		visible := rect.H - 2
		selected := -1
		if len(m.openPath) > level+1 {
			selected = m.openPath[level+1]
		}
		offset, scrollbar := popupScroll(visible, len(items), selected)

		contentW := rect.W - 2
		if scrollbar {
			contentW--
		}
		itemRects := make([]Rect, len(items))
		for i := range items {
			row := i - offset
			if row < 0 || row >= visible {
				itemRects[i] = Rect{} // scrolled out of view
				continue
			}
			itemRects[i] = Rect{X: rect.X + 1, Y: rect.Y + 1 + row, W: contentW, H: 1}
		}
		layouts = append(layouts, menuPopupLayout{
			rect:      rect,
			items:     items,
			itemRects: itemRects,
			path:      append([]int{}, pathPrefix...),
			offset:    offset,
			scrollbar: scrollbar,
		})

		if len(m.openPath) <= level+1 {
			break
		}
		if selected < 0 || selected >= len(items) {
			break
		}
		next := items[selected]
		if len(next.Children) == 0 {
			break
		}
		selRect := itemRects[selected]
		if selRect.Empty() {
			break
		}
		childWidth := popupWidth(next.Children)
		// Submenus open to the right of the parent popup, flipping to its left when
		// they would overflow the right edge.
		x = rect.Right() + 1
		if x+childWidth > appW {
			x = rect.X - childWidth
		}
		if x < 0 {
			x = 0
		}
		y = selRect.Y
		pathPrefix = append(pathPrefix, selected)
		items = next.Children
		width = childWidth
		topLevel = false
		level++
	}
	return layouts
}

// popupScroll returns the first-visible index and whether a scrollbar is needed
// so that the selected row stays within a window of visible rows.
func popupScroll(visible int, count int, selected int) (int, bool) {
	if visible < 1 || count <= visible {
		return 0, false
	}
	offset := 0
	if selected >= visible {
		offset = selected - visible + 1
	}
	maxOff := count - visible
	if offset > maxOff {
		offset = maxOff
	}
	if offset < 0 {
		offset = 0
	}
	return offset, true
}

func popupWidth(items []*MenuItem) int {
	width := 8
	gutter := menuGutter(items)
	for _, item := range items {
		if item.Separator {
			continue
		}
		text, _ := parseMnemonic(item.Label)
		rowWidth := len([]rune(text)) + 4 + gutter
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

// handleClick implements the press-to-open, release-to-activate gesture: a press
// opens/switches menus and highlights leaves, and a release over an enabled leaf
// fires it. This makes "press the bar, drag onto an item, release" work and
// cancels cleanly when the release lands outside any item. TV activates on release.
func (m *MenuBar) handleClick(component *VisualComponent, event tui.ClickEvent) bool {
	_ = component
	if len(m.topRects) == 0 {
		return false
	}
	if event.Down {
		return m.handlePress(event)
	}
	return m.handleRelease(event)
}

func (m *MenuBar) handlePress(event tui.ClickEvent) bool {
	for idx, rect := range m.topRects {
		if !rect.Contains(event.X, event.Y) {
			continue
		}
		if !m.Menus[idx].Enabled {
			return true // disabled top menus cannot be opened
		}
		if len(m.openPath) > 0 && m.openPath[0] == idx {
			m.CloseMenus()
			return true
		}
		m.openPath = m.defaultOpenPath(idx)
		m.hoverPath = []int{idx}
		return true
	}
	for _, popup := range m.popupLayouts {
		for row, rect := range popup.itemRects {
			if rect.Empty() || !rect.Contains(event.X, event.Y) {
				continue
			}
			item := popup.items[row]
			if item.Separator {
				return true
			}
			path := append(append([]int{}, popup.path...), row)
			m.hoverPath = path
			if !item.Enabled {
				return true
			}
			if len(item.Children) > 0 {
				m.openPath = path
			}
			// Leaves only highlight on press; they fire on the matching release.
			return true
		}
	}
	m.CloseMenus()
	return true
}

func (m *MenuBar) handleRelease(event tui.ClickEvent) bool {
	for _, popup := range m.popupLayouts {
		for row, rect := range popup.itemRects {
			if rect.Empty() || !rect.Contains(event.X, event.Y) {
				continue
			}
			item := popup.items[row]
			if item.Separator || !item.Enabled {
				return true
			}
			if len(item.Children) > 0 {
				m.openPath = append(append([]int{}, popup.path...), row)
				m.hoverPath = m.openPath
				return true
			}
			m.activateLeaf(item)
			return true
		}
	}
	// Released on a top menu or outside any item: keep the menu open (cancel).
	return true
}

// activateLeaf performs a leaf item's action — toggling a checkable item or
// invoking OnSelect — and then closes the menu. Shared by mouse and keyboard.
func (m *MenuBar) activateLeaf(item *MenuItem) {
	if item.Checkable {
		item.Checked = !item.Checked
		if item.OnToggle != nil {
			item.OnToggle(item.Checked)
		}
	} else if item.OnSelect != nil {
		item.OnSelect()
	}
	m.CloseMenus()
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
		if item == nil || item.Separator || !item.Enabled {
			return true
		}
		if len(item.Children) > 0 {
			m.openPath = append(m.openPath, firstSelectable(item.Children))
			return true
		}
		m.activateLeaf(item)
		return true
	case tui.KeyUp:
		m.moveSelection(-1)
		return true
	case tui.KeyDown:
		m.moveSelection(1)
		return true
	case tui.KeyLeft:
		// Step back out of a nested submenu, otherwise switch to the previous
		// enabled top-level menu (opening its dropdown).
		if len(m.openPath) > 2 {
			m.openPath = m.openPath[:len(m.openPath)-1]
			return true
		}
		if prev := m.adjacentTop(m.openPath[0], -1); prev >= 0 {
			m.openPath = m.defaultOpenPath(prev)
			m.hoverPath = []int{prev}
		}
		return true
	case tui.KeyRight:
		// Descend into a submenu if the selected item has one, otherwise switch
		// to the next enabled top-level menu (opening its dropdown).
		item := m.currentItem()
		if item != nil && len(item.Children) > 0 {
			m.openPath = append(m.openPath, firstSelectable(item.Children))
			return true
		}
		if next := m.adjacentTop(m.openPath[0], 1); next >= 0 {
			m.openPath = m.defaultOpenPath(next)
			m.hoverPath = []int{next}
		}
		return true
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
	// Genuinely unhandled keys (e.g. Ctrl combos, function keys) bubble out so the
	// desktop can still fire global accelerators / unhandledKeyFn while a menu is open.
	return false
}

// adjacentTop returns the index of the nearest enabled top-level menu starting
// from start and stepping by delta (wrapping), or -1 when none is enabled.
func (m *MenuBar) adjacentTop(start int, delta int) int {
	n := len(m.Menus)
	if n == 0 {
		return -1
	}
	idx := start
	for i := 0; i < n; i++ {
		idx = (idx + delta + n) % n
		if m.Menus[idx].Enabled {
			return idx
		}
	}
	return -1
}

// firstSelectable returns the index of the first non-separator item, or 0.
func firstSelectable(items []*MenuItem) int {
	for i, item := range items {
		if !item.Separator {
			return i
		}
	}
	return 0
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
		if item.Separator || !item.Enabled || labelMnemonic(item.Label) != lower {
			continue
		}
		m.openPath[depth] = index
		if len(item.Children) > 0 {
			m.openPath = append(m.openPath, firstSelectable(item.Children))
			return true
		}
		m.activateLeaf(item)
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
	n := len(items)
	// Step over separators so the highlight always lands on a real item.
	for i := 0; i < n; i++ {
		current = (current + delta + n) % n
		if !items[current].Separator {
			m.openPath[depth] = current
			return
		}
	}
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
	item := m.Menus[top]
	if !item.Enabled {
		return []int{} // disabled top menus do not open
	}
	if len(item.Children) > 0 {
		return []int{top, firstSelectable(item.Children)}
	}
	return []int{top}
}

func (m *MenuBar) OpenTopByMnemonic(lower rune) bool {
	for idx, item := range m.Menus {
		if item.Enabled && labelMnemonic(item.Label) == lower {
			m.openPath = m.defaultOpenPath(idx)
			m.hoverPath = []int{idx}
			return true
		}
	}
	return false
}

// matchShortcut reports whether event would trigger shortcut. It defers to
// Chord.Matches so a MenuShortcut and the BindingRegistry compare an event the same
// way and can never drift (the dispatch tests pin this contract). A nil shortcut
// never matches.
func matchShortcut(event tui.TypeEvent, shortcut *MenuShortcut) bool {
	if shortcut == nil {
		return false
	}
	return shortcut.Chord().Matches(event)
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
