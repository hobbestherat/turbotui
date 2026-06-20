package tv

import (
	"bytes"
	"sync"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// --- Issue #42: a modal layer captures all input, even outside its rect. ---

func TestModalSwallowsClickOutsideItsBounds(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)

	// Background window with a focusable widget the click would otherwise activate.
	base := NewWindow("base", Rect{X: 0, Y: 0, W: 30, H: 10}, tui.LineSingle)
	input := NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1})
	input.Focusable = true
	base.Content.AddChild(input)
	baseLayer := NewWindowLayer("base", base)
	desktop.AddLayer(baseLayer)

	// Small modal off to one side, with a focusable child of its own.
	modalRoot := NewComponent(Rect{X: 50, Y: 15, W: 10, H: 5})
	modalChild := NewComponent(Rect{X: 1, Y: 1, W: 3, H: 1})
	modalChild.Focusable = true
	modalRoot.AddChild(modalChild)
	modalLayer := NewModalLayer("modal", modalRoot)
	outside := 0
	modalLayer.OnClickOutside = func(*Layer) { outside++ }
	desktop.AddLayer(modalLayer)

	// Click over the background window's input, well outside the modal.
	desktop.handleClick(tui.ClickEvent{X: 2, Y: 1, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: 2, Y: 1, Button: tui.MouseLeft, Down: false})

	if desktop.TopLayer() != modalLayer {
		t.Fatalf("modal must stay on top; the background window was raised")
	}
	if input.HasFocus {
		t.Fatalf("a click outside the modal must not focus a background widget")
	}
	if outside != 1 {
		t.Fatalf("expected OnClickOutside to fire once, got %d", outside)
	}

	// A click that DOES land on the modal still reaches its child.
	childAbs := modalChild.AbsoluteBounds()
	desktop.handleClick(tui.ClickEvent{X: childAbs.X, Y: childAbs.Y, Button: tui.MouseLeft, Down: true})
	desktop.handleClick(tui.ClickEvent{X: childAbs.X, Y: childAbs.Y, Button: tui.MouseLeft, Down: false})
	if !modalChild.HasFocus {
		t.Fatalf("a click inside the modal must reach its children")
	}
	if outside != 1 {
		t.Fatalf("a click inside the modal must not count as outside, got %d", outside)
	}
}

// --- Issue #50: Tab traversal follows reading order, with TabIndex override. ---

func TestTabFollowsReadingOrder(t *testing.T) {
	app := tui.NewWithSize(40, 20, &bytes.Buffer{})
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 20})
	// Deliberately add them out of visual order (bottom, then top, then middle) to
	// prove order comes from coordinates, not AddChild order.
	bottom := NewComponent(Rect{X: 5, Y: 10, W: 4, H: 1})
	bottom.Focusable = true
	top := NewComponent(Rect{X: 5, Y: 2, W: 4, H: 1})
	top.Focusable = true
	middle := NewComponent(Rect{X: 5, Y: 6, W: 4, H: 1})
	middle.Focusable = true
	root.AddChild(bottom)
	root.AddChild(top)
	root.AddChild(middle)
	desktop.AddLayer(NewLayer("top", root, true, false))

	want := []*VisualComponent{top, middle, bottom}
	for i, expected := range want {
		desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
		if desktop.focused != expected {
			t.Fatalf("tab %d: expected reading-order target, got %p (want %p)", i, desktop.focused, expected)
		}
	}
}

func TestTabIndexOverridesReadingOrder(t *testing.T) {
	app := tui.NewWithSize(40, 20, &bytes.Buffer{})
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 20})
	top := NewComponent(Rect{X: 5, Y: 2, W: 4, H: 1})
	top.Focusable = true
	top.TabIndex = 2
	bottom := NewComponent(Rect{X: 5, Y: 10, W: 4, H: 1})
	bottom.Focusable = true
	bottom.TabIndex = 1
	root.AddChild(top)
	root.AddChild(bottom)
	desktop.AddLayer(NewLayer("top", root, true, false))

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if desktop.focused != bottom {
		t.Fatalf("expected lower TabIndex (bottom) to be visited first")
	}
	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})
	if desktop.focused != top {
		t.Fatalf("expected higher TabIndex (top) to be visited second")
	}
}

// --- Issue #51: arrow navigation targets the widget actually in that direction. ---

func TestDirectionScore(t *testing.T) {
	tests := []struct {
		name        string
		key         tui.KeyCode
		dx, dy      int
		wantPrimary int
		wantPerp    int
		wantOK      bool
	}{
		{"right direct", tui.KeyRight, 20, 0, 20, 0, true},
		{"right diagonal", tui.KeyRight, 3, 10, 3, 10, true},
		{"right behind", tui.KeyRight, -5, 0, -5, 0, false},
		{"left", tui.KeyLeft, -8, 2, 8, 2, true},
		{"up", tui.KeyUp, -1, -9, 9, 1, true},
		{"down", tui.KeyDown, 4, 6, 6, 4, true},
		{"down wrong way", tui.KeyDown, 0, -3, -3, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			primary, perp, ok := directionScore(tc.key, tc.dx, tc.dy)
			if primary != tc.wantPrimary || perp != tc.wantPerp || ok != tc.wantOK {
				t.Fatalf("directionScore(%v,%d,%d) = (%d,%d,%v), want (%d,%d,%v)",
					tc.key, tc.dx, tc.dy, primary, perp, ok, tc.wantPrimary, tc.wantPerp, tc.wantOK)
			}
		})
	}
}

func TestArrowFocusPrefersDirectOverDiagonal(t *testing.T) {
	app := tui.NewWithSize(60, 30, &bytes.Buffer{})
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 60, H: 30})
	origin := NewComponent(Rect{X: 0, Y: 0, W: 2, H: 1})
	origin.Focusable = true
	// Directly to the right but far; the old squared-distance heuristic would skip
	// it in favour of the nearer down-and-slightly-right widget.
	direct := NewComponent(Rect{X: 20, Y: 0, W: 2, H: 1})
	direct.Focusable = true
	diagonal := NewComponent(Rect{X: 3, Y: 10, W: 2, H: 1})
	diagonal.Focusable = true
	root.AddChild(origin)
	root.AddChild(direct)
	root.AddChild(diagonal)
	desktop.AddLayer(NewLayer("top", root, true, false))

	desktop.setFocus(origin)
	if !desktop.moveFocusDirection(tui.KeyRight) {
		t.Fatalf("expected → to move focus")
	}
	if desktop.focused != direct {
		t.Fatalf("expected → to land on the directly-right widget, not the diagonal one")
	}

	// Down should still reach the diagonal widget (it is the only one below).
	desktop.setFocus(origin)
	if !desktop.moveFocusDirection(tui.KeyDown) || desktop.focused != diagonal {
		t.Fatalf("expected ↓ to reach the lower widget")
	}
}

// --- Issue #56: the layer stack is mutex-guarded against off-loop mutation. ---

func TestLayerSnapshotIsIndependent(t *testing.T) {
	app := tui.NewWithSize(20, 10, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewLayer("a", NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1}), true, false))

	snap := desktop.layerSnapshot()
	desktop.AddLayer(NewLayer("b", NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1}), true, false))

	if len(snap) != 1 {
		t.Fatalf("snapshot must not observe a later AddLayer, got %d layers", len(snap))
	}
	if got := desktop.layerSnapshot(); len(got) != 2 {
		t.Fatalf("a fresh snapshot should see both layers, got %d", len(got))
	}
}

func TestConcurrentAddLayerKeepsEveryLayer(t *testing.T) {
	app := tui.NewWithSize(20, 10, newSyncWriter())
	desktop := NewDesktop(app)

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			desktop.AddLayer(NewLayer("x", NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1}), true, false))
		}()
	}
	wg.Wait()

	if got := len(desktop.layerSnapshot()); got != n {
		t.Fatalf("mutex-guarded AddLayer lost layers under contention: got %d, want %d", got, n)
	}
}

// syncWriter serializes writes so the test's concurrency exercise targets the
// desktop's layer mutex rather than racing on the output buffer itself.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newSyncWriter() *syncWriter { return &syncWriter{} }

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// --- Issue #58: close/min/max presses never leave a dangling drag state. ---

func TestWindowButtonPressLeavesNoClickMode(t *testing.T) {
	app := tui.NewWithSize(40, 12, &bytes.Buffer{})
	desktop := NewDesktop(app)

	closed := 0
	win := NewWindow("w", Rect{X: 0, Y: 0, W: 20, H: 8}, tui.LineSingle)
	win.OnClose = func(*Window) { closed++ }
	desktop.AddLayer(NewLayer("w", win.Component, true, false))

	abs := win.Component.AbsoluteBounds()
	buttons := win.titleButtons(abs)
	desktop.handleClick(tui.ClickEvent{X: buttons.closeRect.X, Y: buttons.closeRect.Y, Button: tui.MouseLeft, Down: true})

	if closed != 1 {
		t.Fatalf("expected close button to fire once, got %d", closed)
	}
	if win.mode != clickNone {
		t.Fatalf("a close press must not enter a drag/resize mode, got %d", win.mode)
	}

	// A later move with no active drag must not relocate the window.
	before := win.Component.Bounds
	desktop.handleClick(tui.ClickEvent{X: 8, Y: 5, Button: tui.MouseLeft, Down: true})
	if win.Component.Bounds != before {
		t.Fatalf("window moved without an active drag: %+v -> %+v", before, win.Component.Bounds)
	}
}

func TestWindowTitleDragUsesClickMode(t *testing.T) {
	app := tui.NewWithSize(40, 12, &bytes.Buffer{})
	desktop := NewDesktop(app)

	win := NewWindow("w", Rect{X: 2, Y: 2, W: 20, H: 6}, tui.LineSingle)
	desktop.AddLayer(NewLayer("w", win.Component, true, false))

	desktop.handleClick(tui.ClickEvent{X: 4, Y: 2, Button: tui.MouseLeft, Down: true})
	if win.mode != clickDrag {
		t.Fatalf("a title-bar press should start a drag, got mode %d", win.mode)
	}
	desktop.handleClick(tui.ClickEvent{X: 4, Y: 2, Button: tui.MouseLeft, Down: false})
	if win.mode != clickNone {
		t.Fatalf("releasing should clear the click mode, got %d", win.mode)
	}
}

// --- Issue #71: resize clamps windowed layers and fires the OnResize hooks. ---

func TestResizeClampsAndNotifiesWindowedLayer(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)

	win := NewWindow("w", Rect{X: 70, Y: 20, W: 8, H: 4}, tui.LineSingle)
	layer := NewWindowLayer("w", win)
	var layerResized Rect
	layer.OnResize = func(r Rect) { layerResized = r }
	desktop.AddLayer(layer)

	deskHook := 0
	desktop.OnResize(func() { deskHook++ })

	app.Resize(30, 10)

	b := win.Component.Bounds
	if b.X > 29 || b.Y > 9 {
		t.Fatalf("window was not clamped into the 30x10 viewport: %+v", b)
	}
	if b.W != 8 || b.H != 4 {
		t.Fatalf("clamping must not change the window size: %+v", b)
	}
	if layerResized != b {
		t.Fatalf("layer OnResize got %+v, expected the clamped bounds %+v", layerResized, b)
	}
	if deskHook != 1 {
		t.Fatalf("desktop OnResize hook should fire once, got %d", deskHook)
	}
}

func TestResizeClampsPlainLayer(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)

	root := NewComponent(Rect{X: 60, Y: 20, W: 10, H: 4})
	desktop.AddLayer(NewLayer("panel", root, true, false))

	app.Resize(20, 8)

	if root.Bounds.X > 19 || root.Bounds.Y > 7 {
		t.Fatalf("plain layer not clamped into 20x8 viewport: %+v", root.Bounds)
	}
}
