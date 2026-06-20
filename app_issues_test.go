package tui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/term"
)

// restoreStateForTest is a zero-value terminal state used to flip restoreState on
// without a real tty; term.Restore on a non-tty fd fails and is ignored by Close.
var restoreStateForTest term.State

// --- Issue #8: a stray SS3 sequence must be consumed, not wedge the parser. ---

func TestSS3UnknownIsConsumed(t *testing.T) {
	// ESC O X is not a recognized SS3 final byte. With 3 bytes present it must be
	// consumed (3 bytes, ok=true) and emit no event, rather than asking for more.
	event, consumed, ok := parseEscape([]byte{0x1b, 'O', 'X'})
	if !ok || consumed != 3 {
		t.Fatalf("unknown SS3 should be consumed: ok=%v consumed=%d", ok, consumed)
	}
	if event != nil {
		t.Fatalf("unknown SS3 should emit no event, got %#v", event)
	}
}

func TestSS3IncompleteAsksForMore(t *testing.T) {
	// Only ESC O (2 bytes) is genuinely incomplete.
	if _, consumed, ok := parseEscape([]byte{0x1b, 'O'}); ok || consumed != 0 {
		t.Fatalf("incomplete SS3 should need more bytes: ok=%v consumed=%d", ok, consumed)
	}
}

func TestStraySS3DoesNotWedgeFollowingKeys(t *testing.T) {
	// The regression from issue #8: a stray F1 (ESC O P) followed by a keystroke
	// must not hide that keystroke. Both arrive in one Feed.
	var parser inputParser
	events := parser.Feed([]byte{0x1b, 'O', 'P', 'a'})
	if len(events) != 2 {
		t.Fatalf("expected F1 + 'a', got %d events: %#v", len(events), events)
	}
	if e, ok := events[0].(TypeEvent); !ok || e.Key != KeyF1 {
		t.Fatalf("first event should be F1, got %#v", events[0])
	}
	if e, ok := events[1].(TypeEvent); !ok || e.Key != KeyRune || e.Rune != 'a' {
		t.Fatalf("second event should be rune 'a', got %#v", events[1])
	}
	if len(parser.pending) != 0 {
		t.Fatalf("parser still holding %d bytes; it wedged", len(parser.pending))
	}
}

// --- Issue #61: function keys via SS3 and CSI-tilde. ---

func TestParseSS3FunctionAndKeypadKeys(t *testing.T) {
	cases := map[byte]KeyCode{'P': KeyF1, 'Q': KeyF2, 'R': KeyF3, 'S': KeyF4, 'M': KeyEnter}
	for final, want := range cases {
		event := parseSS3([]byte{0x1b, 'O', final})
		te, ok := event.(TypeEvent)
		if !ok || te.Key != want {
			t.Fatalf("SS3 %q: want key %d, got %#v", final, want, event)
		}
	}
}

func TestParseCSITildeFunctionKeys(t *testing.T) {
	cases := map[string]KeyCode{
		"11": KeyF1, "12": KeyF2, "13": KeyF3, "14": KeyF4,
		"15": KeyF5, "17": KeyF6, "18": KeyF7, "19": KeyF8,
		"20": KeyF9, "21": KeyF10, "23": KeyF11, "24": KeyF12,
	}
	for params, want := range cases {
		event := parseCSI(params, '~')
		te, ok := event.(TypeEvent)
		if !ok || te.Key != want {
			t.Fatalf("CSI %s~: want key %d, got %#v", params, want, event)
		}
	}
}

// --- Issue #12: CSI-u special codepoints. ---

func TestParseCSIUSpecialKeys(t *testing.T) {
	cases := []struct {
		params string
		want   KeyCode
	}{
		{"9", KeyTab},
		{"13", KeyEnter},
		{"27", KeyEscape},
		{"127", KeyBackspace},
		{"57364", KeyF1},  // Kitty functional range
		{"57375", KeyF12}, // top of the Kitty F-key range
	}
	for _, tc := range cases {
		event := parseCSI(tc.params, 'u')
		te, ok := event.(TypeEvent)
		if !ok || te.Key != tc.want {
			t.Fatalf("CSI %su: want key %d, got %#v", tc.params, tc.want, event)
		}
	}
}

func TestParseCSIUModifiedTab(t *testing.T) {
	// Shift+Tab in Kitty mode: codepoint 9, modifier 2 (shift).
	event := parseCSI("9;2", 'u')
	te, ok := event.(TypeEvent)
	if !ok || te.Key != KeyTab || !te.Shift {
		t.Fatalf("expected Shift+Tab, got %#v", event)
	}
}

// --- Issue #11: mouse motion/drag/modifiers. ---

func TestParseMouseDistinguishesPressDragMove(t *testing.T) {
	cases := []struct {
		name             string
		params           string
		final            byte
		down, drag, move bool
	}{
		{"press", "<0;6;4", 'M', true, false, false},
		{"drag", "<32;6;4", 'M', true, true, false},   // motion bit + button held
		{"move", "<35;6;4", 'M', false, false, true},  // motion bit + no button (cb&3==3)
		{"release", "<0;6;4", 'm', false, false, false},
	}
	for _, tc := range cases {
		event := parseMouse(tc.params, tc.final)
		ce, ok := event.(ClickEvent)
		if !ok {
			t.Fatalf("%s: expected ClickEvent, got %T", tc.name, event)
		}
		if ce.Down != tc.down || ce.Drag != tc.drag || ce.Move != tc.move {
			t.Fatalf("%s: down=%v drag=%v move=%v; want %v/%v/%v",
				tc.name, ce.Down, ce.Drag, ce.Move, tc.down, tc.drag, tc.move)
		}
		if ce.X != 5 || ce.Y != 3 {
			t.Fatalf("%s: coordinates %d,%d; want 5,3", tc.name, ce.X, ce.Y)
		}
	}
}

func TestParseMouseModifiers(t *testing.T) {
	// Ctrl+Shift left press: cb = 0 + 4(shift) + 16(ctrl) = 20.
	event := parseMouse("<20;1;1", 'M')
	ce, ok := event.(ClickEvent)
	if !ok || !ce.Down || !ce.Shift || !ce.Ctrl || ce.Alt {
		t.Fatalf("expected Ctrl+Shift press, got %#v", event)
	}
}

// --- Issue #13: a Ch==0 cell in back must not repaint every frame. ---

func TestZeroRuneCellDoesNotRepaint(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(3, 1, &output)
	// Place a zero-rune cell directly into the back buffer (bypassing the public
	// writers that coerce it), simulating an internal path that does so.
	app.back.cells[0] = Cell{Ch: 0, FG: ANSIColor(7), BG: ANSIColor(0)}
	if err := app.Apply(); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	output.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("zero-rune cell repainted on second frame: %q", output.String())
	}
}

// --- Issue #14: adjacent changed cells in a row share a single cursor move. ---

func TestApplyCoalescesHorizontalRuns(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(6, 1, &output)
	app.Clear(DefaultCell())
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}
	app.WriteString(0, 0, "abcdef", style)
	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// A run of contiguous cells must emit exactly one CUP (cursor position, ...H).
	moves := strings.Count(output.String(), "H")
	if moves != 1 {
		t.Fatalf("expected a single coalesced cursor move, got %d: %q", moves, output.String())
	}
}

func TestApplyMovesCursorAcrossGaps(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(6, 1, &output)
	app.Clear(DefaultCell())
	app.Apply() // settle the cleared frame
	output.Reset()
	style := Cell{FG: ANSIColor(15), BG: ANSIColor(0)}
	// Two separated cells: a gap forces a second cursor move.
	app.WriteCell(0, 0, Cell{Ch: 'a', FG: style.FG, BG: style.BG})
	app.WriteCell(4, 0, Cell{Ch: 'b', FG: style.FG, BG: style.BG})
	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if moves := strings.Count(output.String(), "\x1b["); moves < 2 {
		t.Fatalf("expected separate moves across a gap, got %q", output.String())
	}
}

// --- Issue #16: flushes are wrapped in synchronized-output framing. ---

func TestApplyWrapsInSynchronizedOutput(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(3, 1, &output)
	app.Clear(DefaultCell())
	app.WriteCell(0, 0, Cell{Ch: 'x', FG: ANSIColor(15), BG: ANSIColor(0)})
	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got := output.String()
	if !strings.HasPrefix(got, syncBegin) || !strings.HasSuffix(got, syncEnd) {
		t.Fatalf("frame not wrapped in ?2026 begin/end: %q", got)
	}
	// An empty (no-change) flush must write nothing at all — not even the wrapper.
	output.Reset()
	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("empty flush wrote %q; want nothing", output.String())
	}
}

// --- Issue #15: the append helpers allocate nothing in the hot path. ---

func TestFlushHelpersZeroAlloc(t *testing.T) {
	buf := make([]byte, 0, 256)
	cur := styleState{}
	cell := Cell{Ch: 'A', FG: RGBColor(10, 20, 30), BG: ANSIColor(4), Bold: true}
	allocs := testing.AllocsPerRun(100, func() {
		buf = buf[:0]
		buf = appendCursorMove(buf, 12, 7)
		buf = appendStyle(buf, cur, cell)
		buf = appendRune(buf, cell.Ch)
	})
	if allocs != 0 {
		t.Fatalf("flush helpers allocated %v times per run; want 0", allocs)
	}
}

// --- Issue #17 / #20: coalesced redraw and drain mechanics. ---

func TestRequestRedrawCoalesces(t *testing.T) {
	app := NewWithSize(4, 1, &bytes.Buffer{})
	redraws := 0
	app.SetRedrawFn(func() { redraws++ })
	// Several posts each request a redraw; draining then a single flushDirty must
	// repaint exactly once.
	for i := 0; i < 5; i++ {
		app.Post(func() { app.RequestRedraw() })
	}
	app.drainPosts()
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("expected one coalesced redraw, got %d", redraws)
	}
	// With nothing dirty, flushDirty is a no-op.
	app.flushDirty()
	if redraws != 1 {
		t.Fatalf("flushDirty repainted with nothing dirty: %d", redraws)
	}
}

// --- Issue #19: resize defers to a registered handler instead of self-flushing. ---

func TestResizeSkipsTrailingApplyWithHandler(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(4, 2, &output)
	fired := 0
	app.OnResize(func(ResizeEvent) { fired++ }) // a handler that does NOT redraw
	output.Reset()
	app.resize(6, 3)
	if fired != 1 {
		t.Fatalf("expected resize handler to fire once, got %d", fired)
	}
	// The handler owns the repaint, so resize must not emit its own frame: only the
	// clear sequence is written, never a synchronized-output frame.
	if strings.Contains(output.String(), syncBegin) {
		t.Fatalf("resize flushed a redundant frame despite a handler: %q", output.String())
	}
}

func TestResizeSelfFlushesWithoutHandler(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(4, 2, &output)
	app.Clear(DefaultCell())
	app.WriteCell(0, 0, Cell{Ch: 'Z', FG: ANSIColor(15), BG: ANSIColor(0)})
	app.Apply()
	output.Reset()
	// No resize handler: the App preserves content and repaints itself.
	app.resize(6, 3)
	if !strings.Contains(output.String(), syncBegin) {
		t.Fatalf("bare App resize should repaint itself, got %q", output.String())
	}
}

// --- Issue #9: the input reader unblocks on a read deadline (no leak). ---

func TestReadInputUnblocksOnDeadline(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()

	app := &App{in: pr}
	readCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go app.readInput(readCh, errCh)

	// Mirror what Run does on exit: set a past deadline to unblock the read.
	if err := pr.SetReadDeadline(time.Now()); err != nil {
		t.Skipf("SetReadDeadline unsupported here: %v", err)
	}
	select {
	case <-errCh:
		// The goroutine observed the deadline error and returned: no leak.
	case <-time.After(2 * time.Second):
		t.Fatal("readInput did not unblock on read deadline; goroutine leaked")
	}
}

// --- Issue #22: teardown restores the terminal modes on every exit path. ---

func TestTeardownSequenceWrittenOnClose(t *testing.T) {
	pr, pw, _ := os.Pipe()
	defer pr.Close()
	defer pw.Close()
	var output bytes.Buffer
	app := NewWithSize(4, 1, &output)
	app.in = pr
	// Pretend Run set raw mode; term.Restore on a pipe fd fails and is ignored, but
	// the teardown escape sequence must still be written to out.
	app.restoreState = &restoreStateForTest
	app.Close()
	if !strings.Contains(output.String(), teardownSequence) {
		t.Fatalf("Close did not write the teardown sequence: %q", output.String())
	}
}
