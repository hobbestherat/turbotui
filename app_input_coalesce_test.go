package tui

import (
	"bytes"
	"errors"
	"testing"
)

// Tests pinning the App-level coalesced-redraw seam that tv.Desktop now relies
// on for its input-event handlers (gogent#239): App.RequestRedraw arms a dirty
// flag and App.flushDirty runs the registered redrawFn at most once per loop
// iteration. The run loop dispatches a whole batch of input events (each
// flushing through Desktop.RequestRedraw -> App.RequestRedraw) and only then
// calls flushDirty once, so N events collapse to one repaint.
//
// These cover the corners TestRequestRedrawCoalesces does not: a single event
// still flushes (no lost final frame), the flag is cleared even when the
// repaint itself errors, a missing redrawFn is a safe no-op, and independent
// batches each flush exactly once.

// simulateInputBurst models one run-loop iteration's input dispatch: N input
// events arrive in a single read batch and each handler requests a coalesced
// redraw, then the loop calls flushDirty exactly once. Returns the dirty state
// after the burst (before the flush) so a test can confirm the flag is armed.
func simulateInputBurst(app *App, n int) {
	for i := 0; i < n; i++ {
		app.RequestRedraw()
	}
}

func TestSingleInputRequestFlushesOnce(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// A lone keystroke/click requests one redraw; the single flushDirty the loop
	// runs that iteration must repaint it — deferring must never drop the only
	// frame a single interaction produces (the "lost final frame" risk).
	simulateInputBurst(app, 1)
	if !app.dirty {
		t.Fatalf("RequestRedraw should arm the dirty flag")
	}
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("single request: expected 1 repaint, got %d", redraws)
	}
}

func TestInputBurstCoalescesToOneRepaint(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// A 256-byte read can carry 10-20 mouse-motion events; model the worst case
	// of a large batch. Every event requests a redraw, but one flushDirty repaints
	// exactly once.
	const events = 20
	simulateInputBurst(app, events)
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("burst of %d: expected 1 coalesced repaint, got %d", events, redraws)
	}
	// The flag is cleared, so a follow-up flush with no new requests is a no-op.
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("post-flush dirty should be clear; got %d repaints", redraws)
	}
}

func TestIndependentBatchesEachFlushOnce(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// Two successive read batches (two loop iterations): each drains to one
	// repaint. Coalescing must not collapse ACROSS iterations — each batch that
	// did work gets its own frame.
	simulateInputBurst(app, 5)
	app.flushDirty()
	simulateInputBurst(app, 3)
	app.flushDirty()
	if redraws != 2 {
		t.Fatalf("two batches: expected 2 repaints (one per iteration), got %d", redraws)
	}
}

func TestRequestRedrawNoopWithoutRedrawFn(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	// No SetRedrawFn: a bare App (or one whose desktop detached). RequestRedraw
	// still arms the flag safely, and flushDirty is a documented no-op — it must
	// not panic on a nil redrawFn and must not invoke any repaint.
	app.RequestRedraw()
	if !app.dirty {
		t.Fatalf("RequestRedraw should arm dirty even with no redrawFn")
	}
	app.flushDirty() // must not panic; documented no-op with nil redrawFn

	// Attaching a redrawFn later honors the still-armed flag: the next flush
	// repaints exactly once. (flushDirty itself never clears dirty without a
	// redrawFn, which is harmless — Desktop always installs one before any
	// RequestRedraw — and means a deferred painter is not silently lost.)
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("armed flag should flush once a redrawFn is attached, got %d", redraws)
	}
	if app.dirty {
		t.Fatalf("flushDirty should clear dirty once the repaint has run")
	}
}

func TestFlushClearsDirtyEvenWhenRedrawErrors(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	want := errors.New("boom")
	app.SetRedrawFn(func() { app.applyErr = want })

	// flushDirty clears the flag BEFORE invoking redrawFn, so a failing repaint
	// cannot leave dirty armed and make the loop spin forever re-running it.
	app.RequestRedraw()
	app.flushDirty()
	if app.dirty {
		t.Fatalf("flushDirty must clear dirty even when redrawFn errors")
	}
	if app.LastApplyError() == nil || !errors.Is(app.LastApplyError(), want) {
		t.Fatalf("expected the redraw error to be surfaced, got %v", app.LastApplyError())
	}

	// A fresh request after the failure still coalesces normally.
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })
	app.RequestRedraw()
	app.RequestRedraw()
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("coalescing should work normally after a prior failing flush, got %d", redraws)
	}
}

func TestRequestRedrawDoesNotFlushUntilFlushDirty(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// The whole point of gogent#239: requesting a redraw must not itself repaint
	// or block. Many requests accumulate with zero repaints until the loop flushes.
	for i := 0; i < 50; i++ {
		app.RequestRedraw()
	}
	if redraws != 0 {
		t.Fatalf("RequestRedraw must not repaint synchronously, got %d", redraws)
	}
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("expected a single coalesced repaint, got %d", redraws)
	}
}
