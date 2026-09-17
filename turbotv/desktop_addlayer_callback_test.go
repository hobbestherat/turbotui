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

// TestAddLayerFullScreenLayoutFnNotifiesInTopOrder pins the ordering that makes the
// re-entrant fullscreen path honest: every OnActiveLayerChange invocation must name the
// layer that is actually on top when it is delivered. Running a layer-adding LayoutFn
// before the outer notification instead makes that call report the background as active
// while the overlay it just spawned is the real top.
func TestAddLayerFullScreenLayoutFnNotifiesInTopOrder(t *testing.T) {
	app := tui.NewWithSize(20, 8, &bytes.Buffer{})
	d := NewDesktop(app)

	overlay := NewLayer("overlay", NewComponent(Rect{X: 2, Y: 2, W: 6, H: 2}), true, false)
	root := NewComponent(Rect{X: 0, Y: 0, W: 1, H: 1})
	background := NewLayer("background", root, true, true)

	type notification struct {
		got, top   *Layer
		rootBounds Rect
	}
	var seen []notification
	d.OnActiveLayerChange(func(got *Layer) {
		seen = append(seen, notification{got: got, top: d.TopLayer(), rootBounds: root.Bounds})
	})
	layouts := 0
	root.LayoutFn = func(*VisualComponent) {
		layouts++
		if layouts == 1 {
			d.AddLayer(overlay)
		}
	}

	mustNotBlock(t, "AddLayer of a fullscreen layer whose LayoutFn adds a layer", func() {
		d.AddLayer(background)
	})

	want := []*Layer{background, overlay}
	if len(seen) != len(want) {
		t.Fatalf("notifications: got %d, want %d", len(seen), len(want))
	}
	for i, n := range seen {
		if n.got != want[i] {
			t.Errorf("notification %d: got layer %p, want %p", i, n.got, want[i])
		}
		// The whole point: the argument is the layer that is genuinely on top when the
		// callback runs, not one a nested add has already displaced.
		if n.got != n.top {
			t.Errorf("notification %d reported %p while TopLayer was %p", i, n.got, n.top)
		}
	}
	// The desktop-owned part of the fullscreen stretch is applied before the callback,
	// so a callback reading the new top's bounds does not see the pre-stretch rect.
	if full := (Rect{X: 0, Y: 0, W: 20, H: 8}); seen[0].rootBounds != full {
		t.Errorf("bounds seen by the fullscreen layer's callback: got %v, want %v", seen[0].rootBounds, full)
	}
	if d.lastNotifiedTop != overlay || d.TopLayer() != overlay {
		t.Fatalf("bookkeeping did not converge: lastNotifiedTop=%p TopLayer=%p want %p",
			d.lastNotifiedTop, d.TopLayer(), overlay)
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

// TestAddLayerLayoutFnSurvivesCallbackClearingIt pins the consequence of AddLayer
// deliberately running the active-layer notification between the moment a fullscreen
// root's LayoutFn is checked and the moment it is called: the callback may mutate the
// layer it is handed, including the public LayoutFn field. The scheduled layout must
// still be the function that was checked — re-reading the field at invocation time
// panics on nil and silently runs a replacement otherwise.
func TestAddLayerLayoutFnSurvivesCallbackClearingIt(t *testing.T) {
	app := tui.NewWithSize(20, 8, &bytes.Buffer{})
	d := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 1, H: 1})
	layouts := 0
	root.LayoutFn = func(*VisualComponent) { layouts++ }
	d.OnActiveLayerChange(func(top *Layer) { top.Root.LayoutFn = nil })

	d.AddLayer(NewLayer("fullscreen", root, true, true))

	if layouts != 1 {
		t.Fatalf("LayoutFn calls: got %d, want 1", layouts)
	}
}

// TestAddLayerLayoutFnIgnoresCallbackReplacingIt is the other half: a callback that
// swaps in a different LayoutFn does not get it run in place of the one the stretch
// scheduled. The assertion is on the first layout to run after the notification —
// later ones come from the repaint AddLayer ends with, which re-stretches and redraws
// every fullscreen root and so legitimately runs whatever LayoutFn the field holds by
// then. Re-reading the field at invocation time instead makes the replacement the
// first (and the original never run at all).
func TestAddLayerLayoutFnIgnoresCallbackReplacingIt(t *testing.T) {
	app := tui.NewWithSize(20, 8, &bytes.Buffer{})
	d := NewDesktop(app)

	root := NewComponent(Rect{X: 0, Y: 0, W: 1, H: 1})
	var ran []string
	root.LayoutFn = func(*VisualComponent) { ran = append(ran, "original") }
	d.OnActiveLayerChange(func(top *Layer) {
		top.Root.LayoutFn = func(*VisualComponent) { ran = append(ran, "replacement") }
	})

	d.AddLayer(NewLayer("fullscreen", root, true, true))

	if len(ran) == 0 || ran[0] != "original" {
		t.Fatalf("first layout after the notification: got %v, want original first", ran)
	}
	originals := 0
	for _, which := range ran {
		if which == "original" {
			originals++
		}
	}
	if originals != 1 {
		t.Fatalf("original LayoutFn calls: got %d, want 1 (the scheduled one) in %v", originals, ran)
	}
}
