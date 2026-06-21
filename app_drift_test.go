package tui

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// These tests cover the screen-drift healing primitives added for gogent #213:
//
//   - Invalidate forces the next Apply to repaint every cell (and re-issue the
//     cursor), breaking the stall where the front buffer agrees with a terminal
//     that has actually been scrambled by out-of-band writes.
//   - WriteControl serialises a self-contained escape sequence against frame
//     flushes, the notification counterpart of CopyToClipboard, so its bytes
//     cannot splice into an in-flight Apply frame.

// cursorShow is the DECSCUSR show-cursor escape Apply emits when the desired
// cursor state differs from what it last recorded on the terminal.
const cursorShow = "\x1b[?25h"

// TestInvalidateForcesFullRepaint is the core contract: after the grid has
// settled (a no-change Apply writes nothing), Invalidate must make the very next
// Apply repaint the whole screen even though no cell changed. This is the
// primitive that heals a terminal which has drifted out of sync with the front
// buffer.
func TestInvalidateForcesFullRepaint(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(4, 1, &buf)
	app.Clear(DefaultCell())
	app.WriteCell(0, 0, Cell{Ch: 'A', FG: ANSIColor(15), BG: ANSIColor(0)})
	if err := app.Apply(); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Settle: with nothing changed, Apply writes nothing at all.
	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("settle apply: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("no-op Apply wrote %q; want nothing", buf.String())
	}

	// Invalidate discards the front record; the next Apply must repaint.
	app.Invalidate()
	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("post-invalidate apply: %v", err)
	}
	if !strings.Contains(buf.String(), "A") {
		t.Fatalf("Invalidate did not force a full repaint of the cell: %q", buf.String())
	}
	if !strings.HasPrefix(buf.String(), syncBegin) || !strings.HasSuffix(buf.String(), syncEnd) {
		t.Fatalf("post-invalidate repaint was not a framed flush: %q", buf.String())
	}
}

// TestInvalidateReIssuesCursorState checks the other half of invalidateFront: the
// cursor state is also marked stale, so Apply re-emits the show-cursor escape.
func TestInvalidateReIssuesCursorState(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(4, 1, &buf)
	app.Clear(DefaultCell())
	app.SetCursor(2, 0)
	if err := app.Apply(); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Settle: cursor unchanged and grid unchanged -> Apply writes nothing.
	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("settle apply: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("no-op Apply wrote %q; want nothing", buf.String())
	}

	// After Invalidate the cursor state must be re-issued along with the repaint.
	app.Invalidate()
	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("post-invalidate apply: %v", err)
	}
	if !strings.Contains(buf.String(), cursorShow) {
		t.Fatalf("Invalidate did not re-issue the cursor state: %q", buf.String())
	}
}

// TestInvalidateSettlesBackToNoop confirms Invalidate is not sticky: after the
// full repaint it forces, a subsequent Apply with no changes is again a no-op.
// A broken Invalidate that never reset would repaint every frame forever.
func TestInvalidateSettlesBackToNoop(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(3, 1, &buf)
	app.Clear(DefaultCell())
	app.WriteCell(0, 0, Cell{Ch: 'Z', FG: ANSIColor(15), BG: ANSIColor(0)})
	app.Apply()
	app.Invalidate()
	if err := app.Apply(); err != nil {
		t.Fatalf("forced apply: %v", err)
	}

	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("follow-up apply: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("Apply after an Invalidate-triggered repaint wrote %q; want nothing", buf.String())
	}
}

// TestWriteControlWritesSequenceVerbatim checks that self-contained sequences
// (BEL, an OSC 9 desktop notification) reach the output byte-for-byte, unwrapped
// by the synchronized-output framing Apply uses.
func TestWriteControlWritesSequenceVerbatim(t *testing.T) {
	cases := []string{
		"\x07",                      // BEL (terminal bell)
		"\x1b]9;turbotui:hello\x07", // OSC 9 desktop notification
		"\x1b]777;notify;A;B\x07",   // OSC 777 notification
	}
	for _, seq := range cases {
		var buf bytes.Buffer
		app := NewWithSize(4, 1, &buf)
		app.WriteControl(seq)
		if got := buf.String(); got != seq {
			t.Errorf("WriteControl(%q) wrote %q", seq, got)
		}
	}
}

// TestWriteControlEmptyIsNoop: an empty sequence writes nothing.
func TestWriteControlEmptyIsNoop(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(4, 1, &buf)
	app.WriteControl("")
	if buf.Len() != 0 {
		t.Errorf("WriteControl(\"\") wrote %q; want nothing", buf.String())
	}
}

// TestWriteControlDoesNotInvalidateFront mirrors the CopyToClipboard contract: a
// control write is self-contained and must NOT invalidate the front buffer, so the
// next Apply with no grid changes is still a no-op. If WriteControl touched the
// front record, every notification would trigger a full repaint.
func TestWriteControlDoesNotInvalidateFront(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(8, 1, &buf)
	app.Clear(DefaultCell())
	app.WriteString(0, 0, "hello", DefaultCell())
	if err := app.Apply(); err != nil {
		t.Fatalf("settle apply: %v", err)
	}

	buf.Reset()
	app.WriteControl("\x07")
	if !strings.Contains(buf.String(), "\x07") {
		t.Fatalf("control sequence not written: %q", buf.String())
	}

	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("post-control apply: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("WriteControl should not force a repaint, but Apply wrote %q", buf.String())
	}
}

// TestWriteControlSurvivesFailingWriter exercises the error path: a closed/broken
// output must not panic (WriteControl discards the write error, like
// CopyToClipboard). It only needs to return cleanly.
func TestWriteControlSurvivesFailingWriter(t *testing.T) {
	app := NewWithSize(4, 1, failingWriter{})
	app.WriteControl("\x1b]9;n\x07")
	app.WriteControl("") // empty path on a failing writer must also be clean
}

// TestWriteControlConcurrentByteIntegrity drives WriteControl from many goroutines
// at once (without -race, per the repo's test constraints) and asserts every
// sequence survives intact: the writeMu guard serialises each io.WriteString, so no
// sequence is truncated or interleaved with another, and nothing deadlocks. Each
// goroutine emits a distinct fixed-width sequence so interleaving would be
// detectable as a missing/corrupted one.
func TestWriteControlConcurrentByteIntegrity(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(4, 1, &buf)

	const goroutines = 16
	const perG = 50
	seqs := make([]string, goroutines)
	for g := range seqs {
		// Fixed two-hex-digit label so every sequence is the same length.
		seqs[g] = fmt.Sprintf("\x1b]9;%02x\x07", g)
	}

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				app.WriteControl(seqs[g])
			}
		}()
	}
	wg.Wait()

	got := buf.String()
	// Total length must be exactly goroutines*perG copies of a fixed-length sequence.
	wantLen := goroutines * perG * len(seqs[0])
	if len(got) != wantLen {
		t.Fatalf("byte integrity broken: wrote %d bytes, want %d", len(got), wantLen)
	}
	// Every distinct sequence must appear intact exactly perG times; any
	// interleaving would drop or corrupt at least one of these counts.
	for g, seq := range seqs {
		if n := strings.Count(got, seq); n != perG {
			t.Errorf("sequence %d (%q) appeared %d times, want %d (interleaved?)", g, seq, n, perG)
		}
	}
}

// TestWriteControlDoesNotSpliceApplyFrame tests WriteControl's headline contract:
// its bytes must never splice into an in-flight Apply frame. One goroutine drives
// Apply (the sole back-buffer mutator, matching the loop-goroutine contract); several
// background goroutines fire WriteControl concurrently. Both write a.out under
// writeMu, so on the wire every marker lands between frames, never inside one. If
// WriteControl ever stopped sharing writeMu with Apply, a marker would appear inside
// a syncBegin..syncEnd span and this test would catch it.
func TestWriteControlDoesNotSpliceApplyFrame(t *testing.T) {
	var buf bytes.Buffer
	app := NewWithSize(8, 1, &buf)
	app.Clear(DefaultCell())

	const wcGoroutines = 4
	const iters = 200
	marker := "\x1b]9;m\x07"

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		for i := 0; i < iters; i++ {
			ch := rune('A' + (i % 26))
			app.WriteCell(0, 0, Cell{Ch: ch, FG: ANSIColor(15), BG: ANSIColor(0)})
			if err := app.Apply(); err != nil {
				t.Errorf("apply: %v", err)
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for g := 0; g < wcGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				app.WriteControl(marker)
			}
		}()
	}
	wg.Wait()
	<-loopDone

	got := buf.String()
	if n := strings.Count(got, marker); n != wcGoroutines*iters {
		t.Errorf("lost/truncated WriteControl markers: got %d, want %d", n, wcGoroutines*iters)
	}
	if spliced := countMarkersInsideFrames(got, marker); spliced != 0 {
		t.Errorf("%d WriteControl marker(s) spliced into an Apply frame; writeMu is not serialising WriteControl vs Apply", spliced)
	}
}

// countMarkersInsideFrames returns how many copies of marker occur between a
// syncBegin and its matching syncEnd in s — i.e. how many spliced into a frame.
func countMarkersInsideFrames(s, marker string) int {
	inFrame, spliced := 0, 0
	for i := 0; i < len(s); {
		switch {
		case strings.HasPrefix(s[i:], syncBegin):
			inFrame++
			i += len(syncBegin)
		case strings.HasPrefix(s[i:], syncEnd):
			if inFrame > 0 {
				inFrame--
			}
			i += len(syncEnd)
		case strings.HasPrefix(s[i:], marker):
			if inFrame > 0 {
				spliced++
			}
			i += len(marker)
		default:
			i++
		}
	}
	return spliced
}

// TestInvalidateReAssertsHiddenCursor exposes a defect in the current Invalidate
// implementation. Invalidate's doc promises the next Apply "re-issue[s] the cursor
// state" — the whole point of the primitive is to heal a terminal whose cursor state
// drifted out of sync via out-of-band writes. That holds for a VISIBLE cursor (see
// TestInvalidateReIssuesCursorState) but NOT for a HIDDEN one: invalidateFront only
// sets frontCursorVisible=false, so when the cursor is meant to be hidden,
// appendCursorEscapes hits its `!frontCursorVisible` early-return and emits no
// \x1b[?25l. A drift that left the real cursor visible is therefore not healed.
//
// This test is EXPECTED TO FAIL until Invalidate forces the cursor to be re-asserted
// regardless of visibility (e.g. via a separate "cursor dirty" bit / generation).
func TestInvalidateReAssertsHiddenCursor(t *testing.T) {
	const cursorHide = "\x1b[?25l"
	var buf bytes.Buffer
	app := NewWithSize(4, 1, &buf)
	app.Clear(DefaultCell())
	app.SetCursor(1, 0)
	if err := app.Apply(); err != nil {
		t.Fatalf("show apply: %v", err)
	}
	app.HideCursor()
	if err := app.Apply(); err != nil {
		t.Fatalf("hide apply: %v", err)
	}

	app.Invalidate()
	buf.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("post-invalidate apply: %v", err)
	}
	if !strings.Contains(buf.String(), cursorHide) {
		t.Errorf("Invalidate did not re-assert the HIDDEN cursor after a full repaint; output=%q", buf.String())
	}
}
