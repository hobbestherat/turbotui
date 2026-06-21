package tui

import (
	"bytes"
	"fmt"
	"testing"
)

// End-to-end tests for the input-event redraw coalescing (gogent#239) that drive
// the REAL run-loop pipeline: a byte read is parsed by inputParser.Feed, every
// parsed event is dispatched to the registered handlers (which now request a
// coalesced redraw), and flushDirty runs once. This mirrors App.Run's
// readChannel case verbatim:
//
//	case data := <-readChannel:
//	    for _, event := range a.parser.Feed(data) { a.dispatchEvent(event) }
//	...
//	a.flushDirty()
//
// so it proves the issue's actual claim — "one 256-byte read carrying 10-20
// motion events collapses to a single compose+Apply" — through the genuine
// machinery, not a stand-in. (The tv.Desktop can't be driven this way from
// package tui because its input handlers are unexported; here the OnClick
// handler stands in for desktop.handleClick, doing exactly what it now does:
// call App.RequestRedraw.)

// sgrMouse encodes one SGR (?1006) mouse report landing on 0-based cell (x,y):
//
//	cb    button code — 0 = left press, 32 = left motion/drag, 3 = release
//	final 'M' = press/motion, 'm' = release
//
// Wire coordinates are 1-based; parseMouse subtracts one (see TestParseMouse).
func sgrMouse(cb, x, y int, final byte) []byte {
	return []byte(fmt.Sprintf("\x1b[<%d;%d;%d%c", cb, x+1, y+1, final))
}

// dispatchRead models one Run iteration's input handling: parse a whole read
// batch, dispatch every event, then flush once. It returns the parsed events so
// a test can assert the parser actually decoded them (a malformed batch would
// otherwise make a coalescing test vacuously pass with zero events).
func dispatchRead(app *App, batch []byte) []any {
	events := app.parser.Feed(batch)
	for _, event := range events {
		app.dispatchEvent(event)
	}
	app.flushDirty()
	return events
}

func TestReadBatchCoalescesToOneFlush(t *testing.T) {
	app := NewWithSize(60, 10, &bytes.Buffer{})
	hits := 0
	// This handler is what desktop.handleClick became: handle the event, then
	// request a COALESCED redraw instead of an immediate compose+flush.
	app.OnClick(func(ClickEvent) { hits++; app.RequestRedraw() })
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// One read carrying a press plus many motion reports — exactly what ?1002h
	// emits during a drag (one event per cell the pointer crosses).
	var batch []byte
	batch = append(batch, sgrMouse(0, 5, 1, 'M')...) // press (left button)
	const motions = 15
	for i := 0; i < motions; i++ {
		batch = append(batch, sgrMouse(32, 6+i, 1, 'M')...) // drag motions
	}

	events := dispatchRead(app, batch)
	if len(events) != motions+1 {
		t.Fatalf("parser decoded %d events, want %d (SGR encoding mismatch?)", len(events), motions+1)
	}
	if hits != motions+1 {
		t.Fatalf("handler should fire once per event in the batch, got %d", hits)
	}
	if redraws != 1 {
		t.Fatalf("one read batch must collapse to a single repaint, got %d", redraws)
	}
}

func TestSingleEventReadBatchFlushesOnce(t *testing.T) {
	app := NewWithSize(60, 10, &bytes.Buffer{})
	app.OnClick(func(ClickEvent) { app.RequestRedraw() })
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// A lone click in its own read still repaints exactly once — no lost final
	// frame for an isolated event.
	events := dispatchRead(app, sgrMouse(0, 3, 2, 'M'))
	if len(events) != 1 {
		t.Fatalf("expected 1 parsed event, got %d", len(events))
	}
	if redraws != 1 {
		t.Fatalf("single-event batch should flush once, got %d", redraws)
	}
}

func TestTwoReadBatchesFlushTwice(t *testing.T) {
	app := NewWithSize(60, 10, &bytes.Buffer{})
	app.OnClick(func(ClickEvent) { app.RequestRedraw() })
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// Two successive reads = two loop iterations. Coalescing must NOT collapse
	// across iterations: each batch that did work gets its own single repaint.
	dispatchRead(app, sgrMouse(0, 1, 1, 'M'))
	dispatchRead(app, sgrMouse(0, 2, 1, 'M'))
	if redraws != 2 {
		t.Fatalf("two reads should produce two repaints (one per iteration), got %d", redraws)
	}
}

func TestMixedReadBatchCoalesces(t *testing.T) {
	app := NewWithSize(60, 10, &bytes.Buffer{})
	clicks, types := 0, 0
	app.OnClick(func(ClickEvent) { clicks++; app.RequestRedraw() })
	app.OnType(func(TypeEvent) { types++; app.RequestRedraw() })
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	// A single read can mix mouse and key events; all of them route their
	// redraw request into the one dirty flag, so the batch still flushes once.
	var batch []byte
	batch = append(batch, sgrMouse(0, 1, 1, 'M')...) // a click
	batch = append(batch, []byte("abc")...)          // three keystrokes
	batch = append(batch, sgrMouse(0, 2, 1, 'M')...) // another click

	events := dispatchRead(app, batch)
	if clicks != 2 || types != 3 {
		t.Fatalf("handlers: clicks=%d types=%d, want 2/3", clicks, types)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 parsed events, got %d", len(events))
	}
	if redraws != 1 {
		t.Fatalf("mixed batch should collapse to one repaint, got %d", redraws)
	}
}

func TestReadBatchWithoutRedrawRequestFlushesNothing(t *testing.T) {
	app := NewWithSize(60, 10, &bytes.Buffer{})
	// A handler that consumes the event but does NOT request a redraw (e.g. a
	// click the app ignored). The batch must produce zero repaints.
	app.OnClick(func(ClickEvent) {})
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })

	var batch []byte
	for i := 0; i < 10; i++ {
		batch = append(batch, sgrMouse(32, 1+i, 1, 'M')...)
	}
	events := dispatchRead(app, batch)
	if len(events) != 10 {
		t.Fatalf("expected 10 parsed events, got %d", len(events))
	}
	if redraws != 0 {
		t.Fatalf("batch with no redraw request should not flush, got %d", redraws)
	}
}
