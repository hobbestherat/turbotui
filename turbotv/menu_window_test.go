package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// down/up build press/release mouse events at a point.
func down(x, y int) tui.ClickEvent { return tui.ClickEvent{X: x, Y: y, Down: true} }
func up(x, y int) tui.ClickEvent   { return tui.ClickEvent{X: x, Y: y, Down: false} }

// ===== #43: the menu bar must be unclickable while a modal dialog is open =====

func TestModalLayerBlocksMenuClick(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", func() {})),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	// Without a modal, a click on the File label (row 0) opens the menu.
	desktop.handleClick(down(1, 0))
	if !menu.IsOpen() {
		t.Fatalf("expected click on File to open the menu with no modal up")
	}
	desktop.handleClick(down(40, 8)) // click away to close
	if menu.IsOpen() {
		t.Fatalf("expected click outside to close the menu")
	}

	// With a modal dialog on top, the same click must NOT reach the menu bar.
	dialog := NewComponent(Rect{X: 5, Y: 5, W: 20, H: 5})
	desktop.AddLayer(NewModalLayer("modal", dialog))
	desktop.handleClick(down(1, 0))
	if menu.IsOpen() {
		t.Fatalf("expected modal layer to block the menu-bar click")
	}
}

// ===== #46: popups are clamped to the screen and submenus flip near the edge ====

func TestPopupClampedToRightEdge(t *testing.T) {
	const appW, appH = 80, 24
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: appW, H: 1},
		NewSubMenu("Help", NewMenuItem("About", nil), NewMenuItem("Licence", nil)),
	)
	// Pretend the Help label sits hard against the right edge.
	menu.topRects = []Rect{{X: 74, Y: 0, W: 6, H: 1}}
	menu.openPath = []int{0}

	layouts := menu.layoutPopups(appW, appH)
	if len(layouts) != 1 {
		t.Fatalf("expected 1 popup, got %d", len(layouts))
	}
	r := layouts[0].rect
	if r.X < 0 || r.X+r.W > appW {
		t.Fatalf("popup not clamped horizontally: %+v (appW=%d)", r, appW)
	}
}

func TestSubmenuFlipsLeftNearRightEdge(t *testing.T) {
	const appW, appH = 80, 24
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: appW, H: 1},
		NewSubMenu("Help",
			NewSubMenu("More", NewMenuItem("X", nil), NewMenuItem("Y", nil)),
		),
	)
	menu.topRects = []Rect{{X: 74, Y: 0, W: 6, H: 1}}
	menu.openPath = []int{0, 0} // open Help, descend into More

	layouts := menu.layoutPopups(appW, appH)
	if len(layouts) != 2 {
		t.Fatalf("expected parent + submenu popups, got %d", len(layouts))
	}
	parent, child := layouts[0].rect, layouts[1].rect
	if child.X < 0 || child.X+child.W > appW {
		t.Fatalf("submenu not clamped: %+v (appW=%d)", child, appW)
	}
	if child.X >= parent.X {
		t.Fatalf("expected submenu to flip to the left of the parent: parent=%+v child=%+v", parent, child)
	}
}

// ===== #47: long menus cap their height and scroll to the selection =====

func TestLongMenuCapsHeightAndScrolls(t *testing.T) {
	const appW, appH = 80, 24
	children := make([]*MenuItem, 30)
	for i := range children {
		children[i] = NewMenuItem("item", nil)
	}
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: appW, H: 1}, NewSubMenu("Big", children...))
	menu.topRects = []Rect{{X: 0, Y: 0, W: 5, H: 1}}
	menu.openPath = []int{0, 25} // item 25 highlighted

	layouts := menu.layoutPopups(appW, appH)
	p := layouts[0]
	if p.rect.H > appH {
		t.Fatalf("popup height %d exceeds screen height %d", p.rect.H, appH)
	}
	if !p.scrollbar {
		t.Fatalf("expected a scrollbar for a 30-item menu in a %d-row screen", appH)
	}
	if p.itemRects[25].Empty() {
		t.Fatalf("the highlighted item 25 must be scrolled into view")
	}
	if !p.itemRects[0].Empty() {
		t.Fatalf("item 0 should be scrolled out of view when item 25 is selected")
	}
}

// ===== #53: leaf items activate on release, not on press =====

func TestMenuLeafActivatesOnRelease(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	opened := 0
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", func() { opened++ })),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	desktop.handleType(altRune('f')) // open File
	if !menu.IsOpen() {
		t.Fatalf("expected File menu open")
	}
	// Input handlers now request a coalesced redraw (gogent#239); the run loop
	// would compose once per iteration. Drive that compose explicitly here so the
	// popup layout the test reads is populated, as the loop would do.
	desktop.compose()
	leaf := menu.popupLayouts[0].itemRects[0]
	cx, cy := leaf.X, leaf.Y

	desktop.handleClick(down(cx, cy))
	if opened != 0 {
		t.Fatalf("leaf must not fire on press, got opened=%d", opened)
	}
	desktop.handleClick(up(cx, cy))
	if opened != 1 {
		t.Fatalf("leaf must fire on release, got opened=%d", opened)
	}
	if menu.IsOpen() {
		t.Fatalf("menu should close after activating a leaf")
	}
}

func TestMenuLeafReleaseOutsideCancels(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	opened := 0
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", func() { opened++ })),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	desktop.handleType(altRune('f'))
	desktop.compose() // coalesced redraw is loop-driven now (gogent#239)
	leaf := menu.popupLayouts[0].itemRects[0]
	desktop.handleClick(down(leaf.X, leaf.Y))
	desktop.handleClick(up(40, 10)) // release far away
	if opened != 0 {
		t.Fatalf("release outside the item must not activate it, got opened=%d", opened)
	}
	if !menu.IsOpen() {
		t.Fatalf("releasing outside should leave the menu open (cancel only)")
	}
}

// ===== #54: HandleKey lets genuinely unhandled keys bubble out =====

func TestHandleKeyBubblesUnhandledKeys(t *testing.T) {
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", nil)),
	)
	menu.openPath = []int{0, 0}

	if menu.HandleKey(tui.TypeEvent{Key: tui.KeyDown}) != true {
		t.Fatalf("Down should be handled while a menu is open")
	}
	if menu.HandleKey(tui.TypeEvent{Key: tui.KeyRune, Rune: 'q', Ctrl: true}) != false {
		t.Fatalf("Ctrl combos should bubble out (return false)")
	}
	if menu.HandleKey(tui.TypeEvent{Key: tui.KeyHome}) != false {
		t.Fatalf("unmapped keys should bubble out (return false)")
	}
}

func TestAcceleratorFiresWhileMenuOpen(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	quit := 0
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File",
			NewMenuItem("&Quit", func() { quit++ }).WithShortcut("Ctrl+Q", tui.KeyRune, 'q', true),
		),
	)
	desktop.SetMenuBar(menu)
	desktop.Bindings().Register(
		KeyBinding{Chord: Chord{Key: tui.KeyRune, Rune: 'q', Ctrl: true}, ActionID: "app.quit", Scope: ScopeGlobal},
		func() bool { quit++; return true },
	)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	desktop.handleType(altRune('f')) // open the menu
	if !menu.IsOpen() {
		t.Fatalf("expected File menu open")
	}
	// Ctrl+Q must still fire (and close the menu) even though a dropdown is open.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'q', Ctrl: true})
	if quit != 1 {
		t.Fatalf("expected Ctrl+Q accelerator to fire while menu open, got %d", quit)
	}
	if menu.IsOpen() {
		t.Fatalf("accelerator should close the menu")
	}
}

func TestUnhandledKeyFnFiresWhileMenuOpen(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", nil)),
	)
	desktop.SetMenuBar(menu)
	base := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})
	desktop.AddLayer(NewFullscreenLayer("base", base))

	got := 0
	desktop.SetUnhandledKeyFn(func(_ tui.TypeEvent) { got++ })
	desktop.handleType(altRune('f'))
	desktop.handleType(tui.TypeEvent{Key: tui.KeyHome})
	if got != 1 {
		t.Fatalf("expected unhandledKeyFn to fire for F1 while menu open, got %d", got)
	}
}

// ===== #55: disabled top-level menus cannot be opened =====

func TestDisabledTopMenuCannotOpen(t *testing.T) {
	disabled := NewSubMenu("&File", NewMenuItem("&Open", nil))
	disabled.Enabled = false
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1}, disabled)

	if menu.OpenTopByMnemonic('f') {
		t.Fatalf("disabled top menu should not open via mnemonic")
	}
	if menu.IsOpen() {
		t.Fatalf("disabled top menu must stay closed")
	}
	if len(menu.defaultOpenPath(0)) != 0 {
		t.Fatalf("defaultOpenPath must be empty for a disabled top menu")
	}

	menu.topRects = []Rect{{X: 0, Y: 0, W: 6, H: 1}}
	menu.handlePress(down(1, 0))
	if menu.IsOpen() {
		t.Fatalf("clicking a disabled top menu must not open it")
	}
}

func TestArrowSkipsDisabledTopMenu(t *testing.T) {
	mid := NewSubMenu("&Edit", NewMenuItem("&Cut", nil))
	mid.Enabled = false
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", nil)),
		mid,
		NewSubMenu("&View", NewMenuItem("&Zoom", nil)),
	)
	menu.openPath = []int{0, 0} // File open

	menu.HandleKey(tui.TypeEvent{Key: tui.KeyRight})
	if len(menu.openPath) == 0 || menu.openPath[0] != 2 {
		t.Fatalf("Right should skip the disabled Edit menu and land on View, got %v", menu.openPath)
	}
}

// ===== #62: separators and checkable items =====

func TestMenuSeparatorSkippedByNavigation(t *testing.T) {
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&View",
			NewMenuItem("&Alpha", nil),
			NewSeparator(),
			NewMenuItem("&Beta", nil),
		),
	)
	menu.openPath = []int{0, 0}
	menu.moveSelection(1)
	if menu.openPath[1] != 2 {
		t.Fatalf("Down should skip the separator (index 1) and land on Beta (index 2), got %d", menu.openPath[1])
	}
	menu.moveSelection(-1)
	if menu.openPath[1] != 0 {
		t.Fatalf("Up should skip the separator and land back on Alpha (index 0), got %d", menu.openPath[1])
	}
}

func TestCheckableMenuItemToggles(t *testing.T) {
	var states []bool
	check := NewCheckMenuItem("&Wrap", false, func(c bool) { states = append(states, c) })
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1}, NewSubMenu("&View", check))

	menu.activateLeaf(check)
	if !check.Checked {
		t.Fatalf("expected checkable item to become checked")
	}
	menu.activateLeaf(check)
	if check.Checked {
		t.Fatalf("expected second activation to uncheck")
	}
	if len(states) != 2 || states[0] != true || states[1] != false {
		t.Fatalf("OnToggle should receive the new state each time, got %v", states)
	}
}

func TestPopupWidthIgnoresSeparators(t *testing.T) {
	items := []*MenuItem{
		NewMenuItem("&Alpha", nil),
		NewSeparator(),
		NewCheckMenuItem("&Wrap", true, nil),
	}
	w := popupWidth(items)
	if w < 8 {
		t.Fatalf("popupWidth should never go below the minimum, got %d", w)
	}
	// A popup with a checkable item reserves a 2-column gutter.
	if menuGutter(items) != 2 {
		t.Fatalf("expected a 2-column gutter when a checkable item is present")
	}
}
