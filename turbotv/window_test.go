package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// rowHasRune reports whether any cell on row y holds ch.
func rowHasRune(app *tui.App, y int, ch rune) bool {
	for x := 0; x < app.Width(); x++ {
		if app.ReadCell(x, y).Ch == ch {
			return true
		}
	}
	return false
}

// ===== #4: maximize / restore =====

func TestWindowMaximizeRestoreAPI(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	window := NewWindow("max", Rect{X: 10, Y: 6, W: 20, H: 8}, tui.LineSingle)
	window.Maximizable = true
	var events []bool
	window.OnMaximize = func(_ *Window, maximized bool) { events = append(events, maximized) }
	desktop.AddLayer(NewWindowLayer("w", window))

	window.Maximize()
	if !window.IsMaximized() {
		t.Fatal("window should report maximized")
	}
	if got, want := window.Component.Bounds, desktop.WorkArea(); got != want {
		t.Fatalf("maximized bounds = %+v, want work area %+v", got, want)
	}

	window.Restore()
	if window.IsMaximized() {
		t.Fatal("window should report restored")
	}
	if got := window.Component.Bounds; got != (Rect{X: 10, Y: 6, W: 20, H: 8}) {
		t.Fatalf("restored bounds = %+v, want original (10,6,20,8)", got)
	}
	if len(events) != 2 || events[0] != true || events[1] != false {
		t.Fatalf("OnMaximize should fire true then false, got %v", events)
	}
}

func TestWindowMaximizeButtonTogglesViaTitleBar(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	window := NewWindow("max", Rect{X: 10, Y: 6, W: 20, H: 8}, tui.LineSingle)
	window.Maximizable = true // ShowClose defaults true -> close at Right-5, maximize at Right-9
	desktop.AddLayer(NewWindowLayer("w", window))

	abs := window.Component.AbsoluteBounds()
	maxX := abs.Right() - 9
	window.handleClick(window.Component, tui.ClickEvent{X: maxX, Y: abs.Y, Down: true})
	if !window.IsMaximized() {
		t.Fatalf("clicking the maximize button (x=%d) should maximize", maxX)
	}
	// The button now sits on the maximized title bar; clicking restores.
	abs = window.Component.AbsoluteBounds()
	window.handleClick(window.Component, tui.ClickEvent{X: abs.Right() - 9, Y: abs.Y, Down: true})
	if window.IsMaximized() {
		t.Fatal("clicking the restore button should restore")
	}
}

// ===== #5: constrainable bounds / reserved work area =====

func TestReservedWorkAreaClampsDragAndMaximize(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	// Reserve the left 20 columns (e.g. a pinned sidebar); windows keep clear of it.
	desktop.SetWorkArea(Rect{X: 20, Y: 0, W: 60, H: 25})
	window := NewWindow("constrained", Rect{X: 30, Y: 5, W: 20, H: 8}, tui.LineSingle)
	desktop.AddLayer(NewWindowLayer("w", window))

	// Grab the title bar and try to drag it far into the reserved region.
	abs := window.Component.AbsoluteBounds()
	window.handleClick(window.Component, tui.ClickEvent{X: abs.X + 2, Y: abs.Y, Down: true})
	window.handleClick(window.Component, tui.ClickEvent{X: -50, Y: 5, Down: true})
	if window.Component.Bounds.X < 20 {
		t.Fatalf("drag should not cross into the reserved region, got X=%d", window.Component.Bounds.X)
	}
	window.handleClick(window.Component, tui.ClickEvent{X: -50, Y: 5, Down: false})

	// Maximizing fills only the work area, never the reserved columns.
	window.Maximizable = true
	window.Maximize()
	if window.Component.Bounds.X != 20 || window.Component.Bounds != desktop.WorkArea() {
		t.Fatalf("maximize should fill the work area %+v, got %+v", desktop.WorkArea(), window.Component.Bounds)
	}
}

func TestConstrainToOverridesDesktopWorkArea(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	window := NewWindow("c", Rect{X: 30, Y: 10, W: 10, H: 4}, tui.LineSingle)
	box := Rect{X: 25, Y: 8, W: 20, H: 10}
	window.ConstrainTo = &box
	desktop.AddLayer(NewWindowLayer("w", window))

	abs := window.Component.AbsoluteBounds()
	window.handleClick(window.Component, tui.ClickEvent{X: abs.X, Y: abs.Y, Down: true})
	window.handleClick(window.Component, tui.ClickEvent{X: 0, Y: 0, Down: true})
	if window.Component.Bounds.X < box.X || window.Component.Bounds.Y < box.Y {
		t.Fatalf("ConstrainTo should clamp drag to %+v, got %+v", box, window.Component.Bounds)
	}
}

// ===== #44: drag clamp keeps the title grabbable and off the menu bar =====

func TestDragClampedBelowMenuBarAndOnScreen(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 80, H: 1},
		NewSubMenu("&File", NewMenuItem("&Open", func() {})),
	)
	desktop.SetMenuBar(menu)
	window := NewWindow("drag", Rect{X: 10, Y: 5, W: 20, H: 8}, tui.LineSingle)
	desktop.AddLayer(NewWindowLayer("w", window))

	abs := window.Component.AbsoluteBounds()
	// Press the title bar, then try to drag it up under the menu bar and off-left.
	window.handleClick(window.Component, tui.ClickEvent{X: abs.X + 2, Y: abs.Y, Down: true})
	window.handleClick(window.Component, tui.ClickEvent{X: -30, Y: -10, Down: true})

	if window.Component.Bounds.Y < 1 {
		t.Fatalf("title bar must stay below the menu bar (Y>=1), got Y=%d", window.Component.Bounds.Y)
	}
	if window.Component.Bounds.X < 0 {
		t.Fatalf("title bar must stay on screen (X>=0), got X=%d", window.Component.Bounds.X)
	}
}

// ===== #45: minimizing a focused window moves focus out =====

func TestMinimizeMovesFocusOutOfHiddenContent(t *testing.T) {
	desktop := newTestDesktop(t, 40, 12)
	window := NewWindow("min", Rect{X: 1, Y: 1, W: 30, H: 8}, tui.LineSingle)
	window.Minimizable = true
	box := NewTextBox("", Rect{X: 0, Y: 0, W: 10, H: 1})
	window.AddContent(box)
	desktop.AddLayer(NewWindowLayer("w", window))
	desktop.SetFocus(box)

	// Baseline: typing reaches the focused, visible textbox.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
	if box.GetText() != "a" {
		t.Fatalf("expected 'a' to reach the focused textbox, got %q", box.GetText())
	}

	window.Minimize()
	if desktop.focused != nil {
		t.Fatalf("minimize should move focus off the now-hidden content, got %#v", desktop.focused)
	}
	if box.Component.visibleInTree() {
		t.Fatal("a textbox inside hidden content should not be visible-in-tree")
	}

	// Keystrokes must not leak into the hidden textbox.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'b'})
	if box.GetText() != "a" {
		t.Fatalf("keystrokes leaked into hidden textbox, text=%q", box.GetText())
	}
}

// ===== #52: restore honours a drag performed while minimized =====

func TestRestoreHonoursDragWhileMinimized(t *testing.T) {
	desktop := newTestDesktop(t, 80, 25)
	window := NewWindow("min", Rect{X: 2, Y: 3, W: 40, H: 20}, tui.LineSingle)
	window.Minimizable = true
	desktop.AddLayer(NewWindowLayer("w", window))

	window.Minimize()
	// Drag the minimized title bar to a new spot.
	abs := window.Component.AbsoluteBounds()
	window.handleClick(window.Component, tui.ClickEvent{X: abs.X + 2, Y: abs.Y, Down: true})
	window.handleClick(window.Component, tui.ClickEvent{X: 10, Y: 8, Down: true})
	window.handleClick(window.Component, tui.ClickEvent{X: 10, Y: 8, Down: false})

	window.Restore()
	got := window.Component.Bounds
	if got.X != 8 || got.Y != 8 {
		t.Fatalf("restore should keep the dragged position (8,8), got (%d,%d)", got.X, got.Y)
	}
	if got.H != 20 || got.W != 40 {
		t.Fatalf("restore should reapply the saved size 40x20, got %dx%d", got.W, got.H)
	}
}

// ===== #59: self-managing Close() and a default dialog Esc =====

func TestWindowCloseRemovesLayerAndFiresOnClose(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	base := NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16}))
	desktop.AddLayer(base)
	window := NewWindow("w", Rect{X: 5, Y: 2, W: 20, H: 6}, tui.LineSingle)
	layer := NewWindowLayer("win", window)
	desktop.AddLayer(layer)

	closed := false
	window.OnClose = func(*Window) { closed = true }
	window.Close()

	if !closed {
		t.Fatal("Close should fire OnClose")
	}
	if desktop.TopLayer() != base {
		t.Fatal("Close should remove the window's layer from the desktop")
	}
}

func TestDialogEscapeClosesByDefault(t *testing.T) {
	desktop := newTestDesktop(t, 60, 16)
	base := NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16}))
	desktop.AddLayer(base)

	dialog := NewDialog("D", 10, 4, 30, 8)
	button := NewButton("OK", Rect{X: 2, Y: 2, W: 8, H: 1}, nil)
	dialog.Window.AddContent(button)
	desktop.AddLayer(NewModalLayer("dlg", dialog))
	desktop.SetFocus(button)

	desktop.handleType(tui.TypeEvent{Key: tui.KeyEscape})
	if desktop.TopLayer() != base {
		t.Fatal("Escape should close a default dialog and return to the base layer")
	}
}

// ===== #60: title margin reflects the buttons actually shown =====

func TestTitleNotTruncatedWhenNoButtons(t *testing.T) {
	app := tui.NewWithSize(14, 4, &bytes.Buffer{})
	desktop := NewDesktop(app)
	window := NewWindow("Report", Rect{X: 0, Y: 0, W: 12, H: 3}, tui.LineSingle)
	window.ShowClose = false
	window.Minimizable = false
	window.Maximizable = false
	desktop.AddLayer(NewWindowLayer("w", window))
	desktop.Redraw()

	// With no buttons there is no 8-column reserve, so the full title fits.
	if !rowHasRune(app, 0, 't') {
		t.Fatal("expected the full title 'Report' (incl. its final 't') to render")
	}
}
