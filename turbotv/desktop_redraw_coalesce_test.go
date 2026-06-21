package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for gogent#239 / the turbotui redraw-coalescing fix on the input path.
//
// Button-event mouse tracking (?1002h) emits a motion event for every cell a
// drag crosses, and a single input read carries 10-20 of them. The four desktop
// input handlers (handleClick, handleScroll, handleType, handlePaste) used to
// call Desktop.Redraw() directly — a full synchronous compose + a BLOCKING
// App.Apply() terminal write — so one read batch did 10-20 full redraws and the
// event queue grew faster than it drained, making a dragged window trail the
// cursor. They now call Desktop.RequestRedraw(), which only sets the dirty flag;
// the run loop's flushDirty() composes + applies at most once per iteration.
//
// These tests pin that behaviour. The observable signal is the number of
// terminal frames written: App.Apply() performs exactly one io.Writer.Write per
// changed frame and zero when nothing changed, so a Write-call counter on the
// app's output counts real terminal flushes one-for-one. A deferred (coalesced)
// redraw writes nothing during the burst; one run-loop flush afterwards writes
// exactly one frame containing the final state.

// frameCounter is an io.Writer that counts Apply flushes. Apply issues a single
// Write per changed frame and no Write at all when the diff is empty, so this is
// a precise count of terminal flushes (the blocking writes gogent#239 is about).
type frameCounter struct{ frames int }

func (c *frameCounter) Write(p []byte) (int, error) {
	c.frames++
	return len(p), nil
}

// newCoalesceDesktop builds a fully-wired desktop whose terminal output goes to
// c, so a test can assert how many frames an event sequence produced. NewDesktop
// registers the production coalesced redrawFn (compose + updateCursor + Apply),
// so this exercises the real machinery, not a stub.
func newCoalesceDesktop(t *testing.T, c *frameCounter, w, h int) *Desktop {
	t.Helper()
	app := tui.NewWithSize(w, h, c)
	return NewDesktop(app)
}

// newMutableWidget is a focusable widget that advances a marker one cell each
// time any of its input handlers runs and paints that marker. Every handler
// invocation therefore changes the screen, so a *synchronous* Redraw would emit
// a fresh terminal frame — which is exactly what lets these tests tell a
// coalesced (deferred) redraw apart from the old per-event synchronous one. The
// returned *int counts handler invocations.
func newMutableWidget(bounds Rect) (*VisualComponent, *int) {
	hits := new(int)
	markX := 0
	width := bounds.W
	widget := NewComponent(bounds)
	widget.Focusable = true
	bump := func() bool {
		*hits++
		markX = *hits % width
		return true
	}
	widget.OnClickFn = func(_ *VisualComponent, _ tui.ClickEvent) bool { return bump() }
	widget.OnTypeFn = func(_ *VisualComponent, _ tui.TypeEvent) bool { return bump() }
	widget.OnScrollFn = func(_ *VisualComponent, _ tui.ScrollEvent) bool { return bump() }
	widget.OnPasteFn = func(_ *VisualComponent, _ string) bool { return bump() }
	widget.DrawFn = func(vc *VisualComponent, s Surface) {
		abs := vc.AbsoluteBounds()
		for x := 0; x < abs.W; x++ {
			ch := ' '
			if x == markX {
				ch = '#'
			}
			s.SetCell(abs.X+x, abs.Y, tui.Cell{Ch: ch, FG: tui.ANSIColor(15)})
		}
	}
	return widget, hits
}

// resetFrames zeroes the counter after the setup frames (AddLayer etc.) have
// been written, so subsequent counts reflect only the events under test.
func resetFrames(c *frameCounter) { c.frames = 0 }

// simulateLoopFlush stands in for the single flushDirty() the run loop runs at
// the end of an iteration. flushDirty invokes the registered redrawFn, which for
// a Desktop is compose + updateCursor + Apply — byte-for-byte the same work
// Desktop.Redraw() does. Calling Redraw() once therefore faithfully models one
// coalesced repaint (the dirty-flag bookkeeping itself is covered by the
// tui-package RequestRedraw/flushDirty tests).
func simulateLoopFlush(d *Desktop) { d.Redraw() }

// ===== The headline regression: a real window drag bursts into one flush =====

func TestWindowDragBurstCoalescesToOneFlush(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 80, 25)
	window := NewWindow("drag", Rect{X: 10, Y: 6, W: 20, H: 8}, tui.LineSingle)
	desktop.AddLayer(NewWindowLayer("win", window)) // writes the initial frame
	resetFrames(counter)

	// A press on the title bar captures the window, then a burst of motion
	// reports while the button stays held — exactly what ?1002h emits per cell
	// crossed during a drag. One input read can carry all of these.
	desktop.handleClick(tui.ClickEvent{X: 12, Y: 6, Button: tui.MouseLeft, Down: true})
	const motions = 12
	for i := 0; i < motions; i++ {
		desktop.handleClick(tui.ClickEvent{X: 13 + i, Y: 6, Button: tui.MouseLeft, Down: true, Drag: true})
	}
	if counter.frames != 0 {
		t.Fatalf("expected 0 synchronous flushes during a %d-motion drag burst (coalesced), got %d",
			motions, counter.frames)
	}
	// The drag must actually have moved the window — otherwise "0 flushes" would
	// be vacuously true. Each motion advanced the window one column.
	if window.Component.Bounds.X <= 10 {
		t.Fatalf("expected the drag to move the window right of x=10, got x=%d (test is vacuous)",
			window.Component.Bounds.X)
	}
	// The loop flushes the whole burst as a single repaint.
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("expected exactly one repaint after the drag burst, got %d", counter.frames)
	}
}

// ===== Each of the four input handlers defers its redraw =====

// freshBurstDesktop builds an independent desktop + mutable widget for a single
// subtest, so marker state and the front buffer never leak between subtests.
// AddLayer paints the initial frame (marker at column 0); the returned counter
// is zeroed so the subtest counts only its own events.
func freshBurstDesktop(t *testing.T) (*Desktop, *VisualComponent, *int, *frameCounter) {
	t.Helper()
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	widget, hits := newMutableWidget(Rect{X: 2, Y: 2, W: 20, H: 1})
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false)) // initial frame (marker@0)
	desktop.SetFocus(widget)
	resetFrames(counter)
	return desktop, widget, hits, counter
}

func TestInputHandlerBurstsCoalesce(t *testing.T) {
	// Each subtest gets its own desktop + widget so the marker position and the
	// already-painted front buffer start clean for every handler.
	const burst = 15

	t.Run("click", func(t *testing.T) {
		desktop, _, hits, counter := freshBurstDesktop(t)
		// A press captures the widget; subsequent presses keep firing the
		// handler (mouseCapture stays set), each advancing the marker.
		for i := 0; i < burst; i++ {
			desktop.handleClick(tui.ClickEvent{X: 3 + (i % 5), Y: 2, Button: tui.MouseLeft, Down: true})
		}
		if counter.frames != 0 {
			t.Fatalf("click burst: expected 0 synchronous flushes, got %d", counter.frames)
		}
		if *hits != burst {
			t.Fatalf("click burst: handler should have run %d times, got %d", burst, *hits)
		}
		simulateLoopFlush(desktop)
		if counter.frames != 1 {
			t.Fatalf("click burst: expected 1 flush after coalescing, got %d", counter.frames)
		}
	})

	t.Run("type", func(t *testing.T) {
		desktop, _, hits, counter := freshBurstDesktop(t)
		for i := 0; i < burst; i++ {
			desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
		}
		if counter.frames != 0 {
			t.Fatalf("type burst: expected 0 synchronous flushes, got %d", counter.frames)
		}
		if *hits != burst {
			t.Fatalf("type burst: handler should have run %d times, got %d", burst, *hits)
		}
		simulateLoopFlush(desktop)
		if counter.frames != 1 {
			t.Fatalf("type burst: expected 1 flush after coalescing, got %d", counter.frames)
		}
	})

	t.Run("scroll", func(t *testing.T) {
		desktop, _, hits, counter := freshBurstDesktop(t)
		for i := 0; i < burst; i++ {
			desktop.handleScroll(tui.ScrollEvent{X: 4, Y: 2, Delta: 1})
		}
		if counter.frames != 0 {
			t.Fatalf("scroll burst: expected 0 synchronous flushes, got %d", counter.frames)
		}
		if *hits != burst {
			t.Fatalf("scroll burst: handler should have run %d times, got %d", burst, *hits)
		}
		simulateLoopFlush(desktop)
		if counter.frames != 1 {
			t.Fatalf("scroll burst: expected 1 flush after coalescing, got %d", counter.frames)
		}
	})

	t.Run("paste", func(t *testing.T) {
		desktop, _, hits, counter := freshBurstDesktop(t)
		for i := 0; i < burst; i++ {
			desktop.handlePaste(tui.PasteEvent{Text: "x"})
		}
		if counter.frames != 0 {
			t.Fatalf("paste burst: expected 0 synchronous flushes, got %d", counter.frames)
		}
		if *hits != burst {
			t.Fatalf("paste burst: handler should have run %d times, got %d", burst, *hits)
		}
		simulateLoopFlush(desktop)
		if counter.frames != 1 {
			t.Fatalf("paste burst: expected 1 flush after coalescing, got %d", counter.frames)
		}
	})
}

// ===== A burst mixing all four handlers collapses into one flush =====

func TestMixedHandlerBurstCoalesces(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	widget, _ := newMutableWidget(Rect{X: 2, Y: 2, W: 20, H: 1})
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false))
	desktop.SetFocus(widget)
	resetFrames(counter)

	// Interleave the handler types the way a noisy read batch might, each
	// requesting a coalesced redraw into the same dirty flag.
	for i := 0; i < 6; i++ {
		desktop.handleClick(tui.ClickEvent{X: 3, Y: 2, Button: tui.MouseLeft, Down: true})
		desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
		desktop.handleScroll(tui.ScrollEvent{X: 4, Y: 2, Delta: 1})
		desktop.handlePaste(tui.PasteEvent{Text: "x"})
	}
	if counter.frames != 0 {
		t.Fatalf("mixed burst: expected 0 synchronous flushes, got %d", counter.frames)
	}
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("mixed burst: expected 1 flush after coalescing, got %d", counter.frames)
	}
}

// ===== No "lost final frame": a single event still produces exactly one flush =====

func TestSingleEventProducesOneFlush(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	widget, _ := newMutableWidget(Rect{X: 2, Y: 2, W: 20, H: 1})
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false))
	desktop.SetFocus(widget)
	resetFrames(counter)

	// A lone keystroke must still repaint once the loop flushes — deferring must
	// not drop the only frame a single interaction produces.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
	if counter.frames != 0 {
		t.Fatalf("single type: expected 0 synchronous flushes, got %d", counter.frames)
	}
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("single type: expected exactly 1 flush (no lost final frame), got %d", counter.frames)
	}
}

// ===== The single coalesced frame must show the FINAL state, not a stale one =====
//
// The count-based tests above prove the burst collapses to one repaint, but a
// repaint that composed stale/intermediate state (e.g. snapping to the first
// event instead of the last) would still pass them. This test reads back the
// painted buffer after the single flush and asserts the marker sits at the LAST
// event's column and the first column is cleared — i.e. the one frame reflects
// the final burst state, which is the whole point of coalescing a drag.

func TestCoalescedFlushPaintsFinalStateNotStale(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	const widgetX, widgetY, widgetW = 2, 2, 20
	widget, _ := newMutableWidget(Rect{X: widgetX, Y: widgetY, W: widgetW, H: 1})
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false)) // paints marker at column 0
	desktop.SetFocus(widget)

	// Initial frame: marker sits at the widget's first column.
	if got := desktop.App().ReadCell(widgetX, widgetY).Ch; got != '#' {
		t.Fatalf("initial marker should be at col %d, got %q", widgetX, got)
	}

	resetFrames(counter)
	// A burst that advances the marker `burst` times. newMutableWidget sets
	// markX = hits % widgetW each handler run, so the final marker column (within
	// the widget) is burst % widgetW.
	const burst = 7
	for i := 0; i < burst; i++ {
		desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
	}
	if counter.frames != 0 {
		t.Fatalf("expected 0 synchronous flushes during burst, got %d", counter.frames)
	}
	finalCol := widgetX + (burst % widgetW) // absolute screen column of the marker
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("expected exactly 1 coalesced flush, got %d", counter.frames)
	}
	// The single frame must reflect the LAST event: marker at finalCol, and the
	// originally-marked first column cleared back to a space.
	if got := desktop.App().ReadCell(finalCol, widgetY).Ch; got != '#' {
		t.Fatalf("coalesced frame should paint the marker at the FINAL column %d, got %q (stale state?)",
			finalCol, got)
	}
	if got := desktop.App().ReadCell(widgetX, widgetY).Ch; got != ' ' {
		t.Fatalf("coalesced frame should have cleared the start column %d, got %q", widgetX, got)
	}
}

// ===== Large burst (many read batches worth) still collapses to one flush =====

func TestLargeBurstCoalescesToOneFlush(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 80, 25)
	window := NewWindow("drag", Rect{X: 10, Y: 6, W: 20, H: 8}, tui.LineSingle)
	desktop.AddLayer(NewWindowLayer("win", window))
	resetFrames(counter)

	desktop.handleClick(tui.ClickEvent{X: 12, Y: 6, Button: tui.MouseLeft, Down: true})
	// 200 motion events — an order of magnitude more than a single read carries,
	// so this spans many read batches in the worst case. It must still produce
	// zero synchronous flushes during the burst and one afterwards.
	const motions = 200
	for i := 0; i < motions; i++ {
		desktop.handleClick(tui.ClickEvent{X: 13 + (i % 40), Y: 6, Button: tui.MouseLeft, Down: true, Drag: true})
	}
	if counter.frames != 0 {
		t.Fatalf("large burst: expected 0 synchronous flushes, got %d", counter.frames)
	}
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("large burst: expected 1 flush after coalescing, got %d", counter.frames)
	}
}

// ===== Desktop.RequestRedraw is deferred: it writes nothing by itself =====

func TestRequestRedrawDoesNotFlushImmediately(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	widget, _ := newMutableWidget(Rect{X: 2, Y: 2, W: 20, H: 1})
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false)) // initial frame (marker@0)
	resetFrames(counter)

	// Mutate on-screen content out-of-band (as a background Post would) and arm
	// the coalesced flag directly via the public RequestRedraw seam — the one the
	// input handlers now use. RequestRedraw must not compose or write by itself.
	widget.DrawFn = func(vc *VisualComponent, s Surface) {
		abs := vc.AbsoluteBounds()
		s.SetCell(abs.X+3, abs.Y, tui.Cell{Ch: 'Q', FG: tui.ANSIColor(11)})
	}
	for i := 0; i < 10; i++ {
		desktop.RequestRedraw()
	}
	if counter.frames != 0 {
		t.Fatalf("RequestRedraw flushed synchronously: got %d frames, want 0", counter.frames)
	}
	// The armed flag drains as a single repaint that paints the new content.
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("expected 1 flush after RequestRedraw, got %d", counter.frames)
	}
}

// ===== Desktop.Redraw stays synchronous for the structural callers that need it =====

func TestRedrawStillFlushesSynchronously(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 20, 4)
	bg := NewComponent(Rect{X: 0, Y: 0, W: 20, H: 4})
	desktop.AddLayer(NewFullscreenLayer("base", bg)) // initial frame
	resetFrames(counter)

	// Mutating the screen and calling Redraw directly must still emit a frame in
	// the same call — the synchronous path the issue deliberately keeps for
	// AddLayer/RaiseLayer/handleResize and friends.
	bg.DrawFn = func(vc *VisualComponent, s Surface) {
		abs := vc.AbsoluteBounds()
		s.SetCell(abs.X, abs.Y, tui.Cell{Ch: 'Z', FG: tui.ANSIColor(11)})
	}
	desktop.Redraw()
	if counter.frames != 1 {
		t.Fatalf("Redraw should still flush synchronously, got %d frames", counter.frames)
	}
	// A second synchronous Redraw with no further change writes nothing (no
	// spurious flushes once the screen has converged).
	desktop.Redraw()
	if counter.frames != 1 {
		t.Fatalf("converged Redraw should not flush again, got %d frames", counter.frames)
	}
}

// ===== After a burst is flushed, the converged screen does not keep flushing =====

func TestNoSpuriousFlushesAfterConvergence(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	widget, _ := newMutableWidget(Rect{X: 2, Y: 2, W: 20, H: 1})
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false))
	desktop.SetFocus(widget)
	resetFrames(counter)

	for i := 0; i < 10; i++ {
		desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'a'})
	}
	simulateLoopFlush(desktop) // first flush paints the final state
	if counter.frames != 1 {
		t.Fatalf("expected 1 flush after burst, got %d", counter.frames)
	}
	// Re-flushing with no new requests paints nothing: the idle UI does no
	// spurious redraws (acceptance criterion #4 in gogent#239).
	simulateLoopFlush(desktop)
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("expected no extra flushes after convergence, got %d", counter.frames)
	}
}

// ===== Edge: a handler that declines the event requests no redraw =====
//
// handleScroll asks the target to scroll and only requests a redraw when the
// target consumes it. When the target declines, the burst must neither paint nor
// leave pending work (the loop would have nothing to flush). We observe the
// "consumed" side directly: a declining widget's handler runs but returns false,
// so a follow-up flush writes nothing because nothing on screen changed.

func TestScrollOverDecliningWidgetRequestsNoRedraw(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)

	offers := new(int)
	widget := NewComponent(Rect{X: 2, Y: 2, W: 20, H: 1})
	widget.OnScrollFn = func(_ *VisualComponent, _ tui.ScrollEvent) bool {
		*offers++
		return false // decline: nothing to scroll, nothing to repaint
	}
	root := NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6})
	root.AddChild(widget)
	desktop.AddLayer(NewLayer("top", root, true, false))
	resetFrames(counter)

	for i := 0; i < 8; i++ {
		desktop.handleScroll(tui.ScrollEvent{X: 4, Y: 2, Delta: 1})
	}
	if *offers != 8 {
		t.Fatalf("declining widget should still be offered each scroll, got %d", *offers)
	}
	if counter.frames != 0 {
		t.Fatalf("declined scroll burst should not flush, got %d", counter.frames)
	}
	// Nothing changed, so a flush paints nothing.
	simulateLoopFlush(desktop)
	if counter.frames != 0 {
		t.Fatalf("declined scroll burst left no screen change, but flushed %d frames", counter.frames)
	}
}

// ===== Edge: a click that misses every layer (release on empty space) =====
//
// handleClick always reaches its trailing RequestRedraw on a non-Down event, so
// even a release over empty space arms the flag — but it paints nothing until the
// loop flushes, and flushes nothing if the screen is unchanged.

func TestReleaseOnEmptySpaceDefersAndPaintsNothing(t *testing.T) {
	counter := &frameCounter{}
	desktop := newCoalesceDesktop(t, counter, 40, 6)
	desktop.AddLayer(NewLayer("top", NewComponent(Rect{X: 0, Y: 0, W: 40, H: 6}), true, false))
	resetFrames(counter)

	for i := 0; i < 5; i++ {
		desktop.handleClick(tui.ClickEvent{X: 30, Y: 5, Button: tui.MouseLeft, Down: false})
	}
	if counter.frames != 0 {
		t.Fatalf("empty-space release burst should not flush synchronously, got %d", counter.frames)
	}
	// No widget changed, so the coalesced flush writes nothing.
	simulateLoopFlush(desktop)
	if counter.frames != 0 {
		t.Fatalf("empty-space release changed nothing, but flushed %d frames", counter.frames)
	}
}

// ===== Menu handleType paths coalesce (closing the gap the driver's manual
// compose() calls left open in menu_window_test.go) =====
//
// handleType has three RequestRedraw sites while a menu is involved: opening via
// an Alt+mnemonic, navigating an open menu via HandleKey, and firing a Ctrl
// accelerator. The driver's menu tests force desktop.compose() by hand, which
// bypasses the coalesced flush — so they would still pass if the redraw request
// were dropped. These pin each path through the counting-writer harness instead:
// zero synchronous flushes during the keystroke, exactly one after the loop flush.

func newMenuDesktop(t *testing.T, counter *frameCounter) (*Desktop, *MenuBar) {
	t.Helper()
	desktop := newCoalesceDesktop(t, counter, 60, 16)
	menu := NewMenuBar(Rect{X: 0, Y: 0, W: 60, H: 1},
		NewSubMenu("&File",
			NewMenuItem("&Open", func() {}),
			NewMenuItem("&Save", func() {}),
			NewMenuItem("&Quit", func() {}).WithShortcut("Ctrl+Q", tui.KeyRune, 'q', true),
		),
	)
	desktop.SetMenuBar(menu)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 60, H: 16})))
	return desktop, menu
}

func TestMenuOpenViaMnemonicCoalesces(t *testing.T) {
	counter := &frameCounter{}
	desktop, menu := newMenuDesktop(t, counter)
	resetFrames(counter)

	// Alt+F opens the File menu via dispatchMnemonic — the popup appears, so a
	// synchronous redraw would have flushed. It must defer instead.
	desktop.handleType(altRune('f'))
	if !menu.IsOpen() {
		t.Fatalf("expected File menu open")
	}
	if counter.frames != 0 {
		t.Fatalf("menu open: expected 0 synchronous flushes, got %d", counter.frames)
	}
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("menu open: expected 1 flush after coalescing, got %d", counter.frames)
	}
}

func TestMenuNavigateViaHandleKeyCoalesces(t *testing.T) {
	counter := &frameCounter{}
	desktop, menu := newMenuDesktop(t, counter)
	desktop.handleType(altRune('f')) // open
	if !menu.IsOpen() {
		t.Fatalf("expected File menu open")
	}
	resetFrames(counter)

	// Arrow-down navigates the open menu via menuBar.HandleKey, moving the
	// highlight onto the next item — a visible change, so it must defer and then
	// flush once.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyDown})
	if counter.frames != 0 {
		t.Fatalf("menu navigate: expected 0 synchronous flushes, got %d", counter.frames)
	}
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("menu navigate: expected 1 flush after coalescing, got %d", counter.frames)
	}
}

func TestMenuAcceleratorCoalesces(t *testing.T) {
	counter := &frameCounter{}
	desktop, menu := newMenuDesktop(t, counter)
	fired := 0
	// Observe the accelerator firing via the leaf's OnSelect (Children[2] = Quit).
	menu.Menus[0].Children[2].OnSelect = func() { fired++ }
	desktop.handleType(altRune('f')) // open
	if !menu.IsOpen() {
		t.Fatalf("expected File menu open")
	}
	simulateLoopFlush(desktop) // iteration 1: paint the open popup
	resetFrames(counter)

	// Ctrl+Q fires the accelerator and closes the menu (iteration 2). Closing a
	// popup that is now on screen is a visible change, so it must defer and then
	// flush once. (If open+close shared one batch the popup would never paint and
	// the flush would rightly write nothing — so we settle the open first.)
	desktop.handleType(tui.TypeEvent{Key: tui.KeyRune, Rune: 'q', Ctrl: true})
	if fired != 1 {
		t.Fatalf("expected the Ctrl+Q accelerator to fire, got %d", fired)
	}
	if menu.IsOpen() {
		t.Fatalf("expected the accelerator to close the menu")
	}
	if counter.frames != 0 {
		t.Fatalf("accelerator: expected 0 synchronous flushes, got %d", counter.frames)
	}
	simulateLoopFlush(desktop)
	if counter.frames != 1 {
		t.Fatalf("accelerator: expected 1 flush after coalescing, got %d", counter.frames)
	}
}
