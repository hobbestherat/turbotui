package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// activeLayerRecorder records every OnActiveLayerChange callback invocation so a
// test can assert how many times it fired and with which top layer.
type activeLayerRecorder struct {
	calls []*Layer
}

func (r *activeLayerRecorder) record(top *Layer) {
	r.calls = append(r.calls, top)
}

func (r *activeLayerRecorder) count() int { return len(r.calls) }

func (r *activeLayerRecorder) last() *Layer {
	if len(r.calls) == 0 {
		return nil
	}
	return r.calls[len(r.calls)-1]
}

// newActiveLayerDesktop builds a desktop on an 80x25 off-screen app.
func newActiveLayerDesktop(t *testing.T) *Desktop {
	t.Helper()
	var output bytes.Buffer
	app := tui.NewWithSize(80, 25, &output)
	return NewDesktop(app)
}

// newTestWindowLayer builds a windowed layer with one focusable child so click
// raising and focus moves can be exercised. Returns the window and its layer.
func newTestWindowLayer(name string, r Rect) (*Window, *Layer) {
	w := NewWindow(name, r, tui.LineSingle)
	input := NewComponent(Rect{X: 0, Y: 0, W: 5, H: 1})
	input.Focusable = true
	w.Content.AddChild(input)
	return w, NewWindowLayer(name, w)
}

// clickTitle simulates a complete press+release click gesture on a window's title
// bar (a non-focusable region), which is the reported activation path.
func clickTitle(d *Desktop, r Rect) {
	d.handleClick(tui.ClickEvent{X: r.X + 2, Y: r.Y, Button: tui.MouseLeft, Down: true})
	d.handleClick(tui.ClickEvent{X: r.X + 2, Y: r.Y, Button: tui.MouseLeft, Down: false})
}

// --- Click-to-raise (the reported path) ------------------------------------

func TestOnActiveLayerChange_ClickRaiseFiresOnceWithThatLayer(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rectA := Rect{X: 0, Y: 0, W: 20, H: 8}
	rectB := Rect{X: 40, Y: 0, W: 20, H: 8}
	_, layerA := newTestWindowLayer("A", rectA)
	_, layerB := newTestWindowLayer("B", rectB)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B starts on top.

	// Register only after the stack is built so the AddLayer notifications do not
	// pollute the recorder; lastNotifiedTop is already seeded to B.
	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	clickTitle(d, rectA)

	if rec.count() != 1 {
		t.Fatalf("expected exactly one notification on click-raise, got %d", rec.count())
	}
	if rec.last() != layerA {
		t.Fatalf("expected notification to carry the raised layer A, got %v", rec.last())
	}
	if d.TopLayer() != layerA {
		t.Fatalf("expected layer A to be on top after click")
	}
}

func TestOnActiveLayerChange_ClickAlreadyTopDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rectA := Rect{X: 0, Y: 0, W: 20, H: 8}
	rectB := Rect{X: 40, Y: 0, W: 20, H: 8}
	_, layerA := newTestWindowLayer("A", rectA)
	_, layerB := newTestWindowLayer("B", rectB)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	// Click B, which is already the top layer: the top does not change.
	clickTitle(d, rectB)

	if rec.count() != 0 {
		t.Fatalf("expected no notification when clicking the already-top layer, got %d (%v)", rec.count(), rec.calls)
	}
}

func TestOnActiveLayerChange_ClickEmptyDesktopDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	// A fullscreen background plus a window on top, mirroring gogent's layout.
	bgRoot := NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})
	bgLayer := NewFullscreenLayer("bg", bgRoot)
	d.AddLayer(bgLayer)

	winRect := Rect{X: 10, Y: 5, W: 20, H: 8}
	_, winLayer := newTestWindowLayer("win", winRect)
	d.AddLayer(winLayer)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	// Click a spot covered only by the fullscreen background (away from the window).
	d.handleClick(tui.ClickEvent{X: 70, Y: 20, Button: tui.MouseLeft, Down: true})
	d.handleClick(tui.ClickEvent{X: 70, Y: 20, Button: tui.MouseLeft, Down: false})

	if rec.count() != 0 {
		t.Fatalf("expected no notification when clicking the background (top unchanged), got %d", rec.count())
	}
	if d.TopLayer() != winLayer {
		t.Fatalf("expected the window to remain on top after a background click")
	}
}

func TestOnActiveLayerChange_DragGestureFiresOnce(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rectA := Rect{X: 0, Y: 0, W: 20, H: 8}
	rectB := Rect{X: 40, Y: 0, W: 20, H: 8}
	_, layerA := newTestWindowLayer("A", rectA)
	_, layerB := newTestWindowLayer("B", rectB)
	d.AddLayer(layerA)
	d.AddLayer(layerB)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	// A single press-drag-release gesture on background window A's title bar:
	// press, several motion reports (Down with capture held), then release.
	d.handleClick(tui.ClickEvent{X: 2, Y: 0, Button: tui.MouseLeft, Down: true})
	d.handleClick(tui.ClickEvent{X: 3, Y: 0, Button: tui.MouseLeft, Down: true})
	d.handleClick(tui.ClickEvent{X: 5, Y: 0, Button: tui.MouseLeft, Down: true})
	d.handleClick(tui.ClickEvent{X: 5, Y: 0, Button: tui.MouseLeft, Down: false})

	if rec.count() != 1 {
		t.Fatalf("expected a single coalesced notification for one drag gesture, got %d (%v)", rec.count(), rec.calls)
	}
	if rec.last() != layerA {
		t.Fatalf("expected the drag gesture notification to carry layer A, got %v", rec.last())
	}
}

func TestOnActiveLayerChange_DistinctGesturesEachFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rectA := Rect{X: 0, Y: 0, W: 20, H: 8}
	rectB := Rect{X: 40, Y: 0, W: 20, H: 8}
	_, layerA := newTestWindowLayer("A", rectA)
	_, layerB := newTestWindowLayer("B", rectB)
	d.AddLayer(layerA)
	d.AddLayer(layerB)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	clickTitle(d, rectA) // raise A
	clickTitle(d, rectB) // raise B
	clickTitle(d, rectA) // raise A again

	if rec.count() != 3 {
		t.Fatalf("expected three notifications for three distinct raising gestures, got %d", rec.count())
	}
	want := []*Layer{layerA, layerB, layerA}
	for i, w := range want {
		if rec.calls[i] != w {
			t.Fatalf("notification %d: expected %v, got %v", i, w, rec.calls[i])
		}
	}
}

// --- Programmatic RaiseLayer -----------------------------------------------

func TestOnActiveLayerChange_RaiseLayerFiresWithNewTop(t *testing.T) {
	d := newActiveLayerDesktop(t)

	winRootA := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", winRootA)
	winRootB := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", winRootB)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RaiseLayer(layerA)

	if rec.count() != 1 {
		t.Fatalf("expected one notification from programmatic RaiseLayer, got %d", rec.count())
	}
	if rec.last() != layerA {
		t.Fatalf("expected RaiseLayer notification to carry layer A, got %v", rec.last())
	}
}

func TestOnActiveLayerChange_RaiseAlreadyTopDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	winRootA := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", winRootA)
	winRootB := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", winRootB)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RaiseLayer(layerB) // already top: no-op reorder.

	if rec.count() != 0 {
		t.Fatalf("expected no notification when raising the already-top layer, got %d", rec.count())
	}
}

func TestOnActiveLayerChange_RaiseFullscreenLayerDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	bgRoot := NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})
	bgLayer := NewFullscreenLayer("bg", bgRoot)
	d.AddLayer(bgLayer)

	winRoot := NewComponent(Rect{X: 10, Y: 5, W: 20, H: 8})
	winLayer := NewWindowLayer("win", winRoot)
	d.AddLayer(winLayer) // window on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	// Fullscreen (background) layers are never raised above real windows.
	d.RaiseLayer(bgLayer)

	if rec.count() != 0 {
		t.Fatalf("expected no notification when raising a fullscreen background layer, got %d", rec.count())
	}
}

func TestOnActiveLayerChange_RaiseKeepsModalOnTopDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	winRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layer := NewWindowLayer("win", winRoot)
	otherRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	otherLayer := NewWindowLayer("other", otherRoot)
	modalRoot := NewComponent(Rect{X: 5, Y: 5, W: 10, H: 4})
	modalLayer := NewModalLayer("modal", modalRoot)
	d.AddLayer(layer)
	d.AddLayer(otherLayer)
	d.AddLayer(modalLayer) // modal on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	// Raising a window below a modal keeps the modal on top, so the top layer does
	// not change.
	d.RaiseLayer(layer)

	if d.TopLayer() != modalLayer {
		t.Fatalf("precondition: expected modal to remain on top")
	}
	if rec.count() != 0 {
		t.Fatalf("expected no notification when the modal stays on top, got %d (%v)", rec.count(), rec.calls)
	}
}

// --- AddLayer ---------------------------------------------------------------

func TestOnActiveLayerChange_AddLayerFiresWithNewTop(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	winRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layer := NewWindowLayer("win", winRoot)
	d.AddLayer(layer)

	if rec.count() != 1 {
		t.Fatalf("expected AddLayer of the first layer to fire once, got %d", rec.count())
	}
	if rec.last() != layer {
		t.Fatalf("expected AddLayer notification to carry the new layer, got %v", rec.last())
	}
}

func TestOnActiveLayerChange_AddSecondLayerFires(t *testing.T) {
	d := newActiveLayerDesktop(t)

	firstRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	first := NewWindowLayer("first", firstRoot)
	d.AddLayer(first)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	secondRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	second := NewWindowLayer("second", secondRoot)
	d.AddLayer(second)

	if rec.count() != 1 {
		t.Fatalf("expected adding a second (new top) layer to fire once, got %d", rec.count())
	}
	if rec.last() != second {
		t.Fatalf("expected notification to carry the newly added top layer, got %v", rec.last())
	}
}

// --- RemoveTopLayer / RemoveLayer ------------------------------------------

func TestOnActiveLayerChange_RemoveTopLayerFiresWithNewTop(t *testing.T) {
	d := newActiveLayerDesktop(t)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	bRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", bRoot)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RemoveTopLayer() // removes B; A becomes top.

	if rec.count() != 1 {
		t.Fatalf("expected RemoveTopLayer to fire once, got %d", rec.count())
	}
	if rec.last() != layerA {
		t.Fatalf("expected RemoveTopLayer notification to carry the new top A, got %v", rec.last())
	}
}

func TestOnActiveLayerChange_RemoveTopLayerToEmptyFiresWithNil(t *testing.T) {
	d := newActiveLayerDesktop(t)

	onlyRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	only := NewWindowLayer("only", onlyRoot)
	d.AddLayer(only)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RemoveTopLayer() // stack becomes empty.

	if rec.count() != 1 {
		t.Fatalf("expected removing the only layer to fire once, got %d", rec.count())
	}
	if rec.last() != nil {
		t.Fatalf("expected nil top after emptying the stack, got %v", rec.last())
	}
}

func TestOnActiveLayerChange_RemoveTopLayerEmptyStackDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RemoveTopLayer() // nothing to remove.

	if rec.count() != 0 {
		t.Fatalf("expected no notification when popping an empty stack, got %d", rec.count())
	}
}

func TestOnActiveLayerChange_RemoveTopMostViaRemoveLayerFires(t *testing.T) {
	d := newActiveLayerDesktop(t)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	bRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", bRoot)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RemoveLayer(layerB) // removing the top: A becomes top.

	if rec.count() != 1 {
		t.Fatalf("expected RemoveLayer of the top to fire once, got %d", rec.count())
	}
	if rec.last() != layerA {
		t.Fatalf("expected new top A after removing top B, got %v", rec.last())
	}
}

func TestOnActiveLayerChange_RemoveNonTopLayerDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	bRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", bRoot)
	d.AddLayer(layerA)
	d.AddLayer(layerB) // B on top.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RemoveLayer(layerA) // removing the BOTTOM layer: top (B) unchanged.

	if d.TopLayer() != layerB {
		t.Fatalf("precondition: expected B to remain on top")
	}
	if rec.count() != 0 {
		t.Fatalf("expected no notification when removing a non-top layer, got %d (%v)", rec.count(), rec.calls)
	}
}

func TestOnActiveLayerChange_RemoveAbsentLayerDoesNotFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	absentRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	absent := NewWindowLayer("absent", absentRoot)
	d.RemoveLayer(absent) // not in the stack.

	if rec.count() != 0 {
		t.Fatalf("expected no notification when removing a layer not in the stack, got %d", rec.count())
	}
}

func TestOnActiveLayerChange_RemoveLayerNilIsSafe(t *testing.T) {
	d := newActiveLayerDesktop(t)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	d.RemoveLayer(nil) // early return, no mutation.

	if rec.count() != 0 {
		t.Fatalf("expected no notification for RemoveLayer(nil), got %d", rec.count())
	}
}

// --- Observability contract: handler sees the updated top ------------------

func TestOnActiveLayerChange_HandlerSeesUpdatedTopLayer(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rectA := Rect{X: 0, Y: 0, W: 20, H: 8}
	rectB := Rect{X: 40, Y: 0, W: 20, H: 8}
	_, layerA := newTestWindowLayer("A", rectA)
	_, layerB := newTestWindowLayer("B", rectB)
	d.AddLayer(layerA)
	d.AddLayer(layerB)

	var seenTop *Layer
	var argTop *Layer
	d.OnActiveLayerChange(func(top *Layer) {
		argTop = top
		seenTop = d.TopLayer() // a handler must observe the already-updated stack.
	})

	clickTitle(d, rectA)

	if argTop != layerA {
		t.Fatalf("expected callback arg to be layer A, got %v", argTop)
	}
	if seenTop != layerA {
		t.Fatalf("expected TopLayer() inside the handler to already be A, got %v", seenTop)
	}
}

func TestOnActiveLayerChange_FiresSynchronouslyOnEventLoop(t *testing.T) {
	d := newActiveLayerDesktop(t)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA)

	bRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", bRoot)

	fired := false
	d.OnActiveLayerChange(func(*Layer) { fired = true })

	d.AddLayer(layerB)

	// The contract requires the callback to run on the event loop (no goroutine),
	// so it must have already fired by the time the mutator returns.
	if !fired {
		t.Fatalf("expected OnActiveLayerChange to fire synchronously before AddLayer returned")
	}
}

// --- Callback registration semantics ---------------------------------------

func TestOnActiveLayerChange_NilCallbackIsSafe(t *testing.T) {
	d := newActiveLayerDesktop(t)

	d.OnActiveLayerChange(nil)

	// Exercise every mutator with a nil callback registered; none may panic.
	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA)
	bRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", bRoot)
	d.AddLayer(layerB)
	d.RaiseLayer(layerA)
	d.RemoveLayer(layerA)
	d.RemoveTopLayer()
}

func TestOnActiveLayerChange_NoCallbackRegisteredIsSafe(t *testing.T) {
	d := newActiveLayerDesktop(t)

	// Never register a callback; mutators must still work and track the top so a
	// later registration is not fired spuriously.
	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA)
	d.RemoveTopLayer()
}

func TestOnActiveLayerChange_LaterRegistrationReplacesEarlier(t *testing.T) {
	d := newActiveLayerDesktop(t)

	first := &activeLayerRecorder{}
	second := &activeLayerRecorder{}
	d.OnActiveLayerChange(first.record)
	d.OnActiveLayerChange(second.record) // replaces first.

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA)

	if first.count() != 0 {
		t.Fatalf("expected the replaced callback never to fire, got %d", first.count())
	}
	if second.count() != 1 || second.last() != layerA {
		t.Fatalf("expected the latest callback to fire once with layer A, got count=%d last=%v", second.count(), second.last())
	}
}

func TestOnActiveLayerChange_NilAfterRegistrationDisablesNotification(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	aRoot := NewComponent(Rect{X: 0, Y: 0, W: 10, H: 4})
	layerA := NewWindowLayer("A", aRoot)
	d.AddLayer(layerA) // fires once.

	d.OnActiveLayerChange(nil) // disable.

	bRoot := NewComponent(Rect{X: 20, Y: 0, W: 10, H: 4})
	layerB := NewWindowLayer("B", bRoot)
	d.AddLayer(layerB) // would change top, but no callback.

	if rec.count() != 1 {
		t.Fatalf("expected exactly the one pre-disable notification, got %d", rec.count())
	}
}

// TestOnActiveLayerChange_RegisterAfterLayersNoSpuriousFire verifies the seeding
// behavior documented on notifyActiveLayerChange: the last-notified top is tracked
// even before a callback exists, so registering one after layers are present and
// then performing a no-op activation does not fire spuriously.
func TestOnActiveLayerChange_RegisterAfterLayersNoSpuriousFire(t *testing.T) {
	d := newActiveLayerDesktop(t)

	rect := Rect{X: 0, Y: 0, W: 20, H: 8}
	_, only := newTestWindowLayer("only", rect)
	d.AddLayer(only) // no callback registered yet; lastNotifiedTop seeded to `only`.

	rec := &activeLayerRecorder{}
	d.OnActiveLayerChange(rec.record)

	// Click the only (already-top) window: a no-op activation.
	clickTitle(d, rect)
	// And a programmatic raise of the already-top layer.
	d.RaiseLayer(only)

	if rec.count() != 0 {
		t.Fatalf("expected no spurious notification after registering on an existing stack, got %d (%v)", rec.count(), rec.calls)
	}
}
