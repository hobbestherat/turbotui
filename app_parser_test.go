package tui

import (
	"testing"
)

// Tests for the input parser's C0 control-byte decoding (gogent issue #208, fixed
// in turbotui). Terminals send chords like Ctrl+] as single C0 bytes (0x1D);
// parseOneInput must turn them into TypeEvent{Key: KeyRune, Rune: <printable>,
// Ctrl: true} via caret notation (head ^ 0x40). The previous letter-only offset
// (head + 'a' - 1) was valid only for 0x01..0x1A and mis-mapped 0x1C..0x1F — e.g.
// Ctrl+] (0x1D) became Rune '}' instead of ']', so the accelerator never matched.
//
// These tests exercise the parser through both parseOneInput (per-byte unit) and
// inputParser.Feed (buffering/ordering), covering the whole 0x00..0x1F range, the
// bytes intercepted before the control-decode branch, ESC handling, and the Ctrl+]
// regression that gogent's WithShortcut("Ctrl+]", KeyRune, ']', true) relies on.

// wantCtrlEvent is the TypeEvent a decoded Ctrl+<rune> control byte must produce.
func wantCtrlEvent(r rune) TypeEvent {
	return TypeEvent{Key: KeyRune, Rune: r, Ctrl: true}
}

// asTypeEvent asserts ev is a TypeEvent and returns it.
func asTypeEvent(t *testing.T, ev any) TypeEvent {
	t.Helper()
	te, ok := ev.(TypeEvent)
	if !ok {
		t.Fatalf("expected TypeEvent, got %T: %#v", ev, ev)
	}
	return te
}

// TestParseOneInputControlRange exhaustively pins the decoder output for every C0
// control byte 0x00..0x1F. The bytes intercepted earlier (\t, \n, \r, ESC) are
// recorded with their real outcomes so a future re-ordering of the special-case
// checks in parseOneInput is caught here.
func TestParseOneInputControlRange(t *testing.T) {
	tests := []struct {
		byte     byte
		ok       bool
		consumed int
		event    TypeEvent // meaningful only when hasEvent
		hasEvent bool
	}{
		{0x00, true, 1, wantCtrlEvent('@'), true},                   // ^@ NUL — caret notation (head^0x40 = '@')
		{0x01, true, 1, wantCtrlEvent('a'), true},                   // ^A
		{0x02, true, 1, wantCtrlEvent('b'), true},                   // ^B
		{0x03, true, 1, wantCtrlEvent('c'), true},                   // ^C
		{0x04, true, 1, wantCtrlEvent('d'), true},                   // ^D
		{0x05, true, 1, wantCtrlEvent('e'), true},                   // ^E
		{0x06, true, 1, wantCtrlEvent('f'), true},                   // ^F
		{0x07, true, 1, wantCtrlEvent('g'), true},                   // ^G (BEL)
		{0x08, true, 1, wantCtrlEvent('h'), true},                   // ^H (BS / Ctrl+H)
		{0x09, true, 1, TypeEvent{Key: KeyTab}, true},               // \t — intercepted
		{0x0A, true, 1, TypeEvent{Key: KeyEnter, Ctrl: true}, true}, // \n — intercepted (Ctrl+Enter)
		{0x0B, true, 1, wantCtrlEvent('k'), true},                   // ^K
		{0x0C, true, 1, wantCtrlEvent('l'), true},                   // ^L
		{0x0D, true, 1, TypeEvent{Key: KeyEnter}, true},             // \r — intercepted
		{0x0E, true, 1, wantCtrlEvent('n'), true},                   // ^N
		{0x0F, true, 1, wantCtrlEvent('o'), true},                   // ^O
		{0x10, true, 1, wantCtrlEvent('p'), true},                   // ^P
		{0x11, true, 1, wantCtrlEvent('q'), true},                   // ^Q
		{0x12, true, 1, wantCtrlEvent('r'), true},                   // ^R
		{0x13, true, 1, wantCtrlEvent('s'), true},                   // ^S
		{0x14, true, 1, wantCtrlEvent('t'), true},                   // ^T
		{0x15, true, 1, wantCtrlEvent('u'), true},                   // ^U
		{0x16, true, 1, wantCtrlEvent('v'), true},                   // ^V
		{0x17, true, 1, wantCtrlEvent('w'), true},                   // ^W
		{0x18, true, 1, wantCtrlEvent('x'), true},                   // ^X
		{0x19, true, 1, wantCtrlEvent('y'), true},                   // ^Y
		{0x1A, true, 1, wantCtrlEvent('z'), true},                   // ^Z
		{0x1B, false, 0, TypeEvent{}, false},                        // ESC — held as escape introducer
		{0x1C, true, 1, wantCtrlEvent('\\'), true},                  // ^\  (was '{')
		{0x1D, true, 1, wantCtrlEvent(']'), true},                   // ^]  (was '}') — gogent #208
		{0x1E, true, 1, wantCtrlEvent('^'), true},                   // ^^
		{0x1F, true, 1, wantCtrlEvent('_'), true},                   // ^_
	}
	for _, tc := range tests {
		ev, consumed, ok := parseOneInput([]byte{tc.byte})
		if ok != tc.ok {
			t.Errorf("0x%02x: ok = %v, want %v", tc.byte, ok, tc.ok)
		}
		if consumed != tc.consumed {
			t.Errorf("0x%02x: consumed = %d, want %d (must advance to avoid wedging)", tc.byte, consumed, tc.consumed)
		}
		if tc.hasEvent {
			te, isType := ev.(TypeEvent)
			if !isType {
				t.Errorf("0x%02x: expected TypeEvent, got %T (%#v)", tc.byte, ev, ev)
				continue
			}
			if te != tc.event {
				t.Errorf("0x%02x: event = %+v, want %+v", tc.byte, te, tc.event)
			}
		} else if ev != nil {
			t.Errorf("0x%02x: expected nil event, got %#v", tc.byte, ev)
		}
	}
}

// TestParseCtrlBracketMatchesGogentShortcut is the headline gogent #208
// regression: feeding byte 0x1D must yield exactly the event the gogent
// accelerator WithShortcut("Ctrl+]", KeyRune, ']', true) is matched against.
func TestParseCtrlBracketMatchesGogentShortcut(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{0x1d})
	if len(events) != 1 {
		t.Fatalf("Ctrl+] should produce 1 event, got %d: %#v", len(events), events)
	}
	got := asTypeEvent(t, events[0])
	want := TypeEvent{Key: KeyRune, Rune: ']', Ctrl: true}
	if got != want {
		t.Fatalf("Ctrl+] event = %+v, want %+v", got, want)
	}
	// Explicitly guard against the pre-fix buggy rune (0x1D + 'a' - 1 = '}' = 125).
	if got.Rune == '}' {
		t.Fatalf("regression: Ctrl+] decoded to buggy '}' rune (gogent #208)")
	}
}

// TestFeedControlPunctuationRange feeds the whole corrected range 0x1C..0x1F in
// one chunk and checks order, exact runes, the Ctrl flag, and that nothing is left
// pending (no wedging).
func TestFeedControlPunctuationRange(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{0x1c, 0x1d, 0x1e, 0x1f})
	want := []TypeEvent{
		wantCtrlEvent('\\'),
		wantCtrlEvent(']'),
		wantCtrlEvent('^'),
		wantCtrlEvent('_'),
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events, want %d: %#v", len(events), len(want), events)
	}
	for i, ev := range events {
		got := asTypeEvent(t, ev)
		if got != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, got, want[i])
		}
		if got.Alt || got.Shift {
			t.Errorf("event %d %+v: stray Alt/Shift modifier", i, got)
		}
	}
	if len(parser.pending) != 0 {
		t.Fatalf("parser left %d bytes pending after the range", len(parser.pending))
	}
}

// TestParseCtrlLettersUnchanged checks that the already-working Ctrl+<letter>
// shortcuts (gogent registers F, K, N, Q, W — all in 0x01..0x1A) still decode to
// their historical lower-case runes. The fix folds A..Z back to a..z to preserve
// this; these would break if the fold were dropped.
func TestParseCtrlLettersUnchanged(t *testing.T) {
	letters := map[byte]rune{
		0x06: 'f', // Ctrl+F
		0x0B: 'k', // Ctrl+K
		0x0E: 'n', // Ctrl+N
		0x11: 'q', // Ctrl+Q
		0x17: 'w', // Ctrl+W
		0x01: 'a', // Ctrl+A
		0x1A: 'z', // Ctrl+Z (top of the letter range)
	}
	for b, want := range letters {
		ev, consumed, ok := parseOneInput([]byte{b})
		if !ok || consumed != 1 {
			t.Errorf("Ctrl+%c (0x%02x): ok=%v consumed=%d", want, b, ok, consumed)
			continue
		}
		got := asTypeEvent(t, ev)
		if got != wantCtrlEvent(want) {
			t.Errorf("Ctrl+%c (0x%02x): got %+v, want lower-case Rune %q with Ctrl", want, b, got, want)
		}
	}
}

// TestEscapeNotDecodedAsControlRune ensures ESC (0x1B) — which the old buggy
// formula would have mapped to '{' had it reached the control branch — is routed
// to the escape-sequence path instead: a lone ESC is held briefly, then flushed as
// a bare KeyEscape with no Ctrl modifier and no rune.
func TestEscapeNotDecodedAsControlRune(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{0x1b})
	if len(events) != 0 {
		t.Fatalf("lone ESC should be held pending, got events: %#v", events)
	}
	if !parser.pendingLoneEscape() {
		t.Fatalf("lone ESC should be pending (1 byte), got %d pending bytes", len(parser.pending))
	}

	flushed := parser.flushLoneEscape()
	if len(flushed) != 1 {
		t.Fatalf("flushLoneEscape should emit 1 event, got %#v", flushed)
	}
	esc := asTypeEvent(t, flushed[0])
	if esc.Key != KeyEscape {
		t.Errorf("ESC should flush as KeyEscape, got key %d", esc.Key)
	}
	if esc.Ctrl {
		t.Errorf("lone ESC must not carry Ctrl, got %+v", esc)
	}
	if esc.Rune != 0 {
		t.Errorf("lone ESC must not carry a rune, got %q", esc.Rune)
	}
}

// TestEscapeAltSequence checks ESC used as a meta/Alt prefix before a plain rune.
func TestEscapeAltSequence(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{0x1b, 'a'})
	if len(events) != 1 {
		t.Fatalf("ESC a should produce 1 event, got %d: %#v", len(events), events)
	}
	got := asTypeEvent(t, events[0])
	want := TypeEvent{Key: KeyRune, Rune: 'a', Alt: true}
	if got != want {
		t.Fatalf("ESC a = %+v, want %+v", got, want)
	}
}

// TestEscapeStartsCSISequence checks ESC still introduces CSI sequences (cursor
// keys), proving the escape path was not disturbed by the control-byte fix.
func TestEscapeStartsCSISequence(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{0x1b, '[', 'A'}) // Up arrow
	if len(events) != 1 {
		t.Fatalf("ESC [ A should produce 1 event, got %d: %#v", len(events), events)
	}
	got := asTypeEvent(t, events[0])
	if got.Key != KeyUp {
		t.Fatalf("ESC [ A should be KeyUp, got %+v", got)
	}
	if len(parser.pending) != 0 {
		t.Fatalf("parser left %d bytes pending", len(parser.pending))
	}
}

// TestInterceptedControlBytesUnchanged verifies the bytes handled before the
// control-decode branch (\t, \r, \n, DEL) are unaffected by the fix.
func TestInterceptedControlBytesUnchanged(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want TypeEvent
	}{
		{"tab", []byte{0x09}, TypeEvent{Key: KeyTab}},
		{"CR", []byte{0x0d}, TypeEvent{Key: KeyEnter}},
		{"LF (Ctrl+Enter)", []byte{0x0a}, TypeEvent{Key: KeyEnter, Ctrl: true}},
		{"DEL/backspace", []byte{0x7f}, TypeEvent{Key: KeyBackspace}},
	}
	for _, tc := range cases {
		var parser inputParser
		events := parser.Feed(tc.in)
		if len(events) != 1 {
			t.Errorf("%s: got %d events, want 1: %#v", tc.name, len(events), events)
			continue
		}
		got := asTypeEvent(t, events[0])
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestParseOneInputEmpty checks the empty-input boundary returns need-more-bytes.
func TestParseOneInputEmpty(t *testing.T) {
	ev, consumed, ok := parseOneInput([]byte{})
	if ok || consumed != 0 || ev != nil {
		t.Fatalf("empty input: got ev=%#v consumed=%d ok=%v, want nil/0/false", ev, consumed, ok)
	}
}

// TestFeedInterleavesControlAndRunes checks a control byte immediately followed
// by a printable rune yields two distinct events in order (no merging, no
// wedging), and the printable rune is not marked Ctrl.
func TestFeedInterleavesControlAndRunes(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{0x1d, 'a'}) // Ctrl+] then plain 'a'
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %#v", len(events), events)
	}
	first := asTypeEvent(t, events[0])
	if first != wantCtrlEvent(']') {
		t.Errorf("first event = %+v, want Ctrl+]", first)
	}
	second := asTypeEvent(t, events[1])
	if want := (TypeEvent{Key: KeyRune, Rune: 'a'}); second != want {
		t.Errorf("second event = %+v, want plain 'a' (no Ctrl)", second)
	}
}

// TestBareBracketRuneIsNotCtrl discriminates the same rune typed bare (0x5D, no
// Ctrl) from Ctrl+] (0x1D): a typed ']' must carry no Ctrl modifier, while 0x1D
// must. Same rune, different modifier state.
func TestBareBracketRuneIsNotCtrl(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte{']'})
	if len(events) != 1 {
		t.Fatalf("bare ']' should produce 1 event, got %d", len(events))
	}
	got := asTypeEvent(t, events[0])
	want := TypeEvent{Key: KeyRune, Rune: ']'}
	if got != want {
		t.Fatalf("bare ']' = %+v, want %+v (no Ctrl)", got, want)
	}
}

// TestFeedSplitControlByte ensures a control byte fed across two Feed calls still
// decodes correctly (the parser buffers pending input between reads).
func TestFeedSplitControlByte(t *testing.T) {
	var parser inputParser
	// First chunk ends mid-stream; second delivers the Ctrl+] byte.
	if got := parser.Feed([]byte{0x0e}); len(got) != 1 {
		t.Fatalf("first Feed should emit Ctrl+N, got %d events", len(got))
	}
	events := parser.Feed([]byte{0x1d})
	if len(events) != 1 {
		t.Fatalf("second Feed should emit Ctrl+], got %d events: %#v", len(events), events)
	}
	got := asTypeEvent(t, events[0])
	if got != wantCtrlEvent(']') {
		t.Fatalf("Ctrl+] after a split = %+v, want %+v", got, wantCtrlEvent(']'))
	}
}
