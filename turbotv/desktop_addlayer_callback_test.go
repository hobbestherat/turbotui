package tv

import (
	"bytes"
	"testing"
	"time"

	tui "github.com/hobbestherat/turbotui"
)

// AddLayer reaches two kinds of application code: a fullscreen layer's LayoutFn (via
// SetBounds) and the injectable clock that arms a modal's Enter grace. Neither may be
// allowed to leave the desktop's paint lock held — the first because the callback is
// free to call back into AddLayer, the second because a panic the application recovers
// must not poison every later AddLayer and Redraw. Both defects deadlock rather than
// misbehave, so these tests run the call under a deadline instead of hanging the
// package until the test binary's own timeout.

// mustNotBlock runs fn on its own goroutine and fails if it has not returned within a
// deadline generous enough that only a real deadlock trips it. The goroutine is left
// parked on failure; the test binary is on its way down anyway.
func mustNotBlock(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s blocked: the desktop held a lock across an application callback", what)
	}
}

func TestAddLayerFullScreenLayoutFnMayAddLayer(t *testing.T) {
	app := tui.NewWithSize(20, 8, &bytes.Buffer{})
	d := NewDesktop(app)

	overlay := NewLayer("overlay", NewComponent(Rect{X: 2, Y: 2, W: 6, H: 2}), true, false)
	root := NewComponent(Rect{X: 0, Y: 0, W: 1, H: 1})
	layouts := 0
	root.LayoutFn = func(*VisualComponent) {
		layouts++
		if layouts == 1 {
			d.AddLayer(overlay)
		}
	}

	background := NewLayer("background", root, true, true)
	mustNotBlock(t, "AddLayer of a fullscreen layer whose LayoutFn adds a layer", func() {
		d.AddLayer(background)
	})

	if layouts == 0 {
		t.Fatalf("expected the fullscreen layer's LayoutFn to run")
	}
	if want := (Rect{X: 0, Y: 0, W: 20, H: 8}); root.Bounds != want {
		t.Fatalf("fullscreen bounds: got %v, want %v", root.Bounds, want)
	}
	got := d.layerSnapshot()
	if len(got) != 2 || got[0] != background || got[1] != overlay {
		t.Fatalf("stack after re-entrant AddLayer: got %v, want [background overlay]", got)
	}
}

func TestAddLayerSurvivesRecoveredLayoutFnPanic(t *testing.T) {
	app := tui.NewWithSize(20, 8, &bytes.Buffer{})
	d := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 1, H: 1})
	root.LayoutFn = func(*VisualComponent) { panic("layout error") }
	mustPanic(t, "AddLayer with a panicking LayoutFn", func() {
		d.AddLayer(NewLayer("broken", root, true, true))
	})
	root.LayoutFn = nil

	assertDesktopUsable(t, d, "a recovered LayoutFn panic")
}

func TestAddLayerSurvivesRecoveredClockPanic(t *testing.T) {
	app := tui.NewWithSize(20, 8, &bytes.Buffer{})
	d := NewDesktop(app)

	// The clock runs inside the critical section, so only unlocking on the way out of
	// it keeps a panicking clock from poisoning the desktop.
	d.SetClock(func() time.Time { panic("clock error") })
	modal := NewLayer("modal", NewComponent(Rect{X: 0, Y: 0, W: 4, H: 1}), true, false)
	modal.Modal = true
	mustPanic(t, "AddLayer with a panicking clock", func() { d.AddLayer(modal) })
	d.SetClock(nil)

	assertDesktopUsable(t, d, "a recovered clock panic")
}

// assertDesktopUsable checks that the desktop still serves the calls that take its
// paint lock, i.e. that the recovered panic released it on the way out.
func assertDesktopUsable(t *testing.T, d *Desktop, after string) {
	t.Helper()
	mustNotBlock(t, "AddLayer after "+after, func() {
		d.AddLayer(NewLayer("after", NewComponent(Rect{X: 0, Y: 0, W: 4, H: 1}), true, false))
	})
	mustNotBlock(t, "Redraw after "+after, d.Redraw)
}

// mustPanic asserts fn panics and recovers it the way an application with an outer
// recovery boundary would, so the test can go on to check the desktop still works.
func mustPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected %s to panic", what)
		}
	}()
	fn()
}
