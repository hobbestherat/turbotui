package tui

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Tests for the cross-repo fix to gogent#464 (turbotui half): making
// Ctrl+Shift+<letter> distinguishable on Kitty/CSI-u-capable terminals instead of
// silently degrading to Ctrl+<letter>.
//
// The fix has three moving parts, each exercised here:
//
//  1. setupTerminal enables the Kitty keyboard protocol's *disambiguate* flag
//     (CSI > 1 u) and queries the terminal (CSI ? u); teardown pops it (CSI < u).
//     See setupSequence / teardownSequence.
//  2. The terminal's query reply (CSI ? <flags> u) is parsed into an internal
//     capabilityReport and consumed in dispatchEvent to flip the package-global
//     extendedKeyboardActive — the flag Deliverability reads. The flag reflects a
//     *confirmed* terminal (it answered), not merely that we asked, so a non-Kitty
//     terminal — or default tmux, which swallows both bytes — stays in legacy mode
//     and refuses Ctrl+Shift+<letter> with a reason rather than accepting a binding
//     that would never fire.
//  3. Deliverability gates only the Ctrl+Shift+<letter> branch on that flag; the
//     parser's CSI-u path already decoded the Shift bit (decodeCSIModifier).
//
// extendedKeyboardActive is package-global; every test that toggles it registers a
// Cleanup resetting it to false so no state leaks. The suite has no t.Parallel, so
// serial toggling is safe. Its zero value is false = legacy baseline, so every
// existing Deliverability/parser test stays green untouched.

// withLegacy resets extendedKeyboardActive to the legacy baseline now and on cleanup,
// so a test can flip it freely without leaking into siblings.
func withLegacy(t *testing.T) {
	t.Helper()
	extendedKeyboardActive.Store(false)
	t.Cleanup(func() { extendedKeyboardActive.Store(false) })
}

// --- Decoder: CSI-u for Ctrl+Shift+G (the headline #464 chord) ----------------

// TestExtendedKeyboardDecodeCtrlShiftG pins that a Kitty/CSI-u sequence for
// Ctrl+Shift+G decodes to a rune event carrying BOTH modifiers, through the per-byte
// parser and the buffering Feed path. Wire modifier 6 encodes the value-1 bitmask 5
// = Shift(1)+Ctrl(4); codepoint 103 = 'g'. This is the event gogent's capture UI
// must see on a capable terminal, and what Chord.Matches routes (see the tv tests).
func TestExtendedKeyboardDecodeCtrlShiftG(t *testing.T) {
	want := TypeEvent{Key: KeyRune, Rune: 'g', Shift: true, Ctrl: true}

	// Per-byte parser entry.
	ev, consumed, ok := parseOneInput([]byte("\x1b[103;6u"))
	if !ok {
		t.Fatal("CSI 103;6 u must decode (ok=true)")
	}
	if consumed != 8 { // ESC [ 1 0 3 ; 6 u
		t.Errorf("consumed = %d, want 8 (must advance to avoid wedging)", consumed)
	}
	if got := asTypeEvent(t, ev); got != want {
		t.Errorf("parseOneInput Ctrl+Shift+G = %+v, want %+v", got, want)
	}

	// Buffering Feed entry (what the Run loop drives).
	var parser inputParser
	events := parser.Feed([]byte("\x1b[103;6u"))
	if len(events) != 1 {
		t.Fatalf("Feed should emit 1 event, got %d: %#v", len(events), events)
	}
	if got := asTypeEvent(t, events[0]); got != want {
		t.Errorf("Feed Ctrl+Shift+G = %+v, want %+v", got, want)
	}
	if len(parser.pending) != 0 {
		t.Errorf("parser left %d bytes pending", len(parser.pending))
	}
}

// TestExtendedKeyboardDecodeModifiedLetters sweeps the whole alphabet at the wire
// modifier for Shift+Ctrl (6), confirming every Ctrl+Shift+<letter> decodes with
// both bits set — not just 'g'. Guards a future regression in decodeCSIModifier's
// bit math (shift = flags&1, ctrl = flags&4).
func TestExtendedKeyboardDecodeModifiedLetters(t *testing.T) {
	for r := rune('a'); r <= rune('z'); r++ {
		seq := "\x1b[" + strconv.Itoa(int(r)) + ";6u" // codepoint r, modifier 6 = Shift+Ctrl
		var parser inputParser
		events := parser.Feed([]byte(seq))
		if len(events) != 1 {
			t.Fatalf("Ctrl+Shift+%c: got %d events, want 1: %#v", r, len(events), events)
		}
		got := asTypeEvent(t, events[0])
		want := TypeEvent{Key: KeyRune, Rune: r, Shift: true, Ctrl: true}
		if got != want {
			t.Errorf("Ctrl+Shift+%c = %+v, want %+v", r, got, want)
		}
	}
}

// --- Decoder: the Ctrl+Space drop fix (review defect #1) ---------------------

// TestExtendedKeyboardDecodeCtrlSpaceNotDropped pins that a CSI-u Space (codepoint
// 0x20) is delivered, not silently dropped. The pre-fix guard keyCode > 0x20
// excluded 0x20 (Space), so Kitty's Ctrl+Space (CSI 32;5u) matched no arm and
// vanished. The widened guard (keyCode > 0) must deliver it.
func TestExtendedKeyboardDecodeCtrlSpaceNotDropped(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want TypeEvent
	}{
		{"Ctrl+Space (CSI 32;5u, mod 5=Ctrl)", "\x1b[32;5u", TypeEvent{Key: KeyRune, Rune: ' ', Ctrl: true}},
		{"plain Space (CSI 32u)", "\x1b[32u", TypeEvent{Key: KeyRune, Rune: ' '}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var parser inputParser
			events := parser.Feed([]byte(tc.in))
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1: %#v", len(events), events)
			}
			got := asTypeEvent(t, events[0])
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if len(parser.pending) != 0 {
				t.Errorf("parser left %d bytes pending", len(parser.pending))
			}
		})
	}
}

// TestExtendedKeyboardNoCSIUKeyDropped sweeps CSI-u codepoints 1..0x7f and asserts
// none is silently dropped (every reported key must decode). Only keyCode 0
// (malformed/empty) is allowed to fall through to nil — and even then it must
// advance so the parser cannot wedge.
func TestExtendedKeyboardNoCSIUKeyDropped(t *testing.T) {
	for code := 1; code <= 0x7f; code++ {
		seq := "\x1b[" + strconv.Itoa(code) + "u"
		ev, consumed, ok := parseOneInput([]byte(seq))
		if !ok {
			t.Errorf("codepoint %d (CSI %du): ok=false (would wedge the parser)", code, code)
			continue
		}
		if consumed == 0 {
			t.Errorf("codepoint %d (CSI %du): consumed=0 (would wedge)", code, code)
		}
		if ev == nil {
			t.Errorf("codepoint %d (CSI %du): silently dropped (nil) — every reported key must decode", code, code)
		}
	}

	// keyCode 0 (empty params, e.g. a bare "CSI u") is the one allowed nil, but it
	// must still consume its bytes so the stream advances.
	ev, consumed, ok := parseOneInput([]byte("\x1b[u"))
	if ev != nil {
		t.Errorf("CSI u (keyCode 0): expected nil, got %#v", ev)
	}
	if !ok || consumed == 0 {
		t.Errorf("CSI u (keyCode 0): ok=%v consumed=%d, want true/>0 (must advance even when malformed)", ok, consumed)
	}
}

// TestExtendedKeyboardSpecialCodepointsIntact pins that the widened rune guard did
// not shadow csiUSpecialKey: the C0 control codepoints and the Kitty functional-key
// range still map to their named keys (with modifiers carried through). A regression
// that reordered the special-key check before the guard, or dropped a case, would
// turn these into wrong runes.
func TestExtendedKeyboardSpecialCodepointsIntact(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want TypeEvent
	}{
		{"Tab (CSI 9u)", "\x1b[9u", TypeEvent{Key: KeyTab}},
		{"Enter (CSI 13u)", "\x1b[13u", TypeEvent{Key: KeyEnter}},
		{"Esc (CSI 27u)", "\x1b[27u", TypeEvent{Key: KeyEscape}},
		{"Backspace (CSI 127u)", "\x1b[127u", TypeEvent{Key: KeyBackspace}},
		{"F1 Kitty range (CSI 57364u)", "\x1b[57364u", TypeEvent{Key: KeyF1}},
		{"Shift+Tab as CSI-u (CSI 9;2u)", "\x1b[9;2u", TypeEvent{Key: KeyTab, Shift: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var parser inputParser
			events := parser.Feed([]byte(tc.in))
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1: %#v", len(events), events)
			}
			got := asTypeEvent(t, events[0])
			if got != tc.want {
				t.Errorf("got %+v, want %+v (special-key mapping must take precedence)", got, tc.want)
			}
		})
	}
}

// --- Handshake: the query reply is a control-plane signal, never a key --------

// TestExtendedKeyboardReplyIsCapabilityReport pins that the terminal's answer to the
// keyboard-protocol query (CSI ? <flags> u) is surfaced as the internal
// capabilityReport carrying the parsed flag bits — never as a TypeEvent key. The
// disambiguate bit is 0b1.
func TestExtendedKeyboardReplyIsCapabilityReport(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantFlags int
	}{
		{"disambiguate set (CSI ?1u)", "\x1b[?1u", 1},
		{"disambiguate clear (CSI ?0u)", "\x1b[?0u", 0},
		{"disambiguate + report-all (CSI ?3u)", "\x1b[?3u", 3},
		{"empty flags treated as 0 (CSI ?u)", "\x1b[?u", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, consumed, ok := parseOneInput([]byte(tc.in))
			if !ok || consumed == 0 {
				t.Fatalf("%s: ok=%v consumed=%d (must parse and advance)", tc.name, ok, consumed)
			}
			report, isReport := ev.(capabilityReport)
			if !isReport {
				t.Fatalf("%s: expected capabilityReport, got %T (%#v) — a query reply must never become a keypress", tc.name, ev, ev)
			}
			if report.flags != tc.wantFlags {
				t.Errorf("%s: flags=%d, want %d", tc.name, report.flags, tc.wantFlags)
			}
		})
	}
}

// TestExtendedKeyboardDispatchEventSetsFlag is the crux of "Deliverability reflects
// real capability": dispatchEvent turns a capabilityReport into the
// extendedKeyboardActive flag, set iff the reply carries the disambiguate bit. A
// reply with only other bits (e.g. report-all = 0b10) must leave it false.
func TestExtendedKeyboardDispatchEventSetsFlag(t *testing.T) {
	withLegacy(t)
	app := &App{} // dispatchEvent's capabilityReport case only touches the global flag

	cases := []struct {
		name       string
		flags      int
		wantActive bool
	}{
		{"disambiguate reply -> active", 1, true},
		{"disambiguate + report-all -> active", 3, true},
		{"report-all only -> inactive", 2, false},
		{"no flags -> inactive", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			extendedKeyboardActive.Store(false) // start each from the legacy baseline
			app.dispatchEvent(capabilityReport{flags: tc.flags})
			if got := extendedKeyboardActive.Load(); got != tc.wantActive {
				t.Errorf("flags=%d: extendedKeyboardActive=%v, want %v (bit 0b1 = disambiguate)", tc.flags, got, tc.wantActive)
			}
		})
	}
}

// TestExtendedKeyboardReplyDoesNotCorruptStream pins that a reply arriving in the
// middle of the input stream yields exactly one control-plane result plus the
// following key event, in order, with nothing dropped or merged. The reply must not
// be dispatched to handlers as a key.
func TestExtendedKeyboardReplyDoesNotCorruptStream(t *testing.T) {
	var parser inputParser
	events := parser.Feed([]byte("\x1b[?1u\x1b[103;6u")) // reply, then Ctrl+Shift+G
	if len(events) != 2 {
		t.Fatalf("got %d results, want 2 (reply + key): %#v", len(events), events)
	}
	if _, ok := events[0].(capabilityReport); !ok {
		t.Errorf("first result = %T, want capabilityReport (the reply must not become a key)", events[0])
	}
	got := asTypeEvent(t, events[1])
	if want := (TypeEvent{Key: KeyRune, Rune: 'g', Shift: true, Ctrl: true}); got != want {
		t.Errorf("second result = %+v, want Ctrl+Shift+G %+v", got, want)
	}
	if len(parser.pending) != 0 {
		t.Errorf("parser left %d bytes pending", len(parser.pending))
	}
}

// --- Deliverability: capability-gated Ctrl+Shift+<letter> ---------------------

// TestExtendedKeyboardDeliverabilityGated is the #464 headline behaviour at the
// deliverability table: Ctrl+Shift+<letter> is refused with the explanatory reason
// in legacy mode (and on any terminal that never confirmed capability) and
// deliverable with an empty reason once capability is confirmed. Sweeps every letter
// in both states.
func TestExtendedKeyboardDeliverabilityGated(t *testing.T) {
	withLegacy(t)
	const wantReason = "Ctrl+Shift+letter is indistinguishable from Ctrl+letter on most terminals"

	// Legacy mode (the default; also a terminal that ignored the push).
	extendedKeyboardActive.Store(false)
	for r := rune('a'); r <= rune('z'); r++ {
		ok, reason := Deliverability(KeyRune, r, true, true, false)
		if ok {
			t.Fatalf("legacy: Ctrl+Shift+%c must be refused", r)
		}
		if reason != wantReason {
			t.Errorf("legacy Ctrl+Shift+%c reason=%q, want %q", r, reason, wantReason)
		}
	}

	// Capability confirmed: deliverable with an empty reason.
	extendedKeyboardActive.Store(true)
	for r := rune('a'); r <= rune('z'); r++ {
		ok, reason := Deliverability(KeyRune, r, true, true, false)
		if !ok || reason != "" {
			t.Errorf("capable: Ctrl+Shift+%c = (ok=%v, reason=%q), want (true, \"\")", r, ok, reason)
		}
	}
}

// TestExtendedKeyboardFlagDoesNotLiftOtherVerdicts is the key regression guard for
// the design's stated scoping: ONLY Ctrl+Shift+<letter> is gated on capability. The
// per-key Ctrl ambiguities (Ctrl+M==Enter, Ctrl+I==Tab, …) and the flow/job-control
// captures must stay refused even when capability is confirmed — gating them would
// risk a false-positive on a key that doubles as Enter/Tab/Esc. If the flag ever
// leaked into those branches, this trips.
func TestExtendedKeyboardFlagDoesNotLiftOtherVerdicts(t *testing.T) {
	withLegacy(t)
	extendedKeyboardActive.Store(true) // even with capability confirmed …

	refused := []struct {
		name string
		key  KeyCode
		r    rune
	}{
		{"Ctrl+M==Enter", KeyRune, 'm'},
		{"Ctrl+I==Tab", KeyRune, 'i'},
		{"Ctrl+H==Backspace", KeyRune, 'h'},
		{"Ctrl+J==LF", KeyRune, 'j'},
		{"Ctrl+[==Esc", KeyRune, '['},
		{"Ctrl+Z==SIGTSTP", KeyRune, 'z'},
		{"Ctrl+S==XOFF", KeyRune, 's'},
		{"Ctrl+Q==XON", KeyRune, 'q'},
		{"Ctrl+Tab (named key)", KeyTab, 0},
		{"Ctrl+Esc (named key)", KeyEscape, 0},
		{"Ctrl+Backspace (named key)", KeyBackspace, 0},
	}
	for _, tc := range refused {
		ok, _ := Deliverability(tc.key, tc.r, true, false, false)
		if ok {
			t.Errorf("%s must stay refused even with capability active (only Ctrl+Shift+letter is gated)", tc.name)
		}
	}
}

// TestExtendedKeyboardFlagDefaultsFalse documents the zero-value baseline: with no
// terminal confirmation, Ctrl+Shift+<letter> is refused. This is the state every
// existing Deliverability test (and gogent's pre-Run config load) sees.
func TestExtendedKeyboardFlagDefaultsFalse(t *testing.T) {
	withLegacy(t)
	if ok, _ := Deliverability(KeyRune, 'g', true, true, false); ok {
		t.Fatal("default (no terminal confirmation): Ctrl+Shift+G must be refused")
	}
}

// --- Setup/teardown byte sequences (the four modes + Kitty push/query/pop) ----

// TestExtendedKeyboardSetupTeardownSequences pins that the four long-standing modes
// are still present (and first, in setup) and that the Kitty push+query on setup and
// the pop on teardown are appended without disturbing them — the no-regression
// property on legacy terminals (the Kitty bytes are no-ops there).
func TestExtendedKeyboardSetupTeardownSequences(t *testing.T) {
	for _, want := range []string{"\x1b[?1049h", "\x1b[?25l", "\x1b[?1002h", "\x1b[?1006h", "\x1b[?2004h"} {
		if !strings.Contains(setupSequence, want) {
			t.Errorf("setupSequence missing existing mode %q: %q", want, setupSequence)
		}
	}
	if !strings.Contains(setupSequence, "\x1b[>1u") {
		t.Errorf("setupSequence missing Kitty push \\x1b[>1u: %q", setupSequence)
	}
	if !strings.Contains(setupSequence, "\x1b[?u") {
		t.Errorf("setupSequence missing Kitty query \\x1b[?u: %q", setupSequence)
	}
	// The legacy mode group must precede the Kitty push.
	if i, j := strings.Index(setupSequence, "\x1b[?2004h"), strings.Index(setupSequence, "\x1b[>1u"); i < 0 || j < 0 || i > j {
		t.Errorf("setupSequence: legacy modes must precede the Kitty push (?2004h@%d, >1u@%d): %q", i, j, setupSequence)
	}

	for _, want := range []string{"\x1b[?2004l", "\x1b[?1002l", "\x1b[?1006l", "\x1b[?25h", "\x1b[?1049l"} {
		if !strings.Contains(teardownSequence, want) {
			t.Errorf("teardownSequence missing existing mode %q: %q", want, teardownSequence)
		}
	}
	if !strings.Contains(teardownSequence, "\x1b[<u") {
		t.Errorf("teardownSequence missing Kitty pop \\x1b[<u: %q", teardownSequence)
	}
}

// TestExtendedKeyboardTeardownClearsFlag proves every teardown path restores the
// legacy baseline (clears the flag) and writes the Kitty pop, so the user's shell is
// restored on normal exit and a stale "capable" state never leaks into the next Run.
// Covers Close (restoreTerminal) and CloseWithMessage's no-restoreState branch.
func TestExtendedKeyboardTeardownClearsFlag(t *testing.T) {
	withLegacy(t)

	t.Run("Close (restoreTerminal path)", func(t *testing.T) {
		pr, pw, _ := os.Pipe()
		defer pr.Close()
		defer pw.Close()
		var output bytes.Buffer
		app := NewWithSize(4, 1, &output)
		app.in = pr
		app.restoreState = &restoreStateForTest // pretend Run set raw mode

		extendedKeyboardActive.Store(true)
		app.Close()
		if extendedKeyboardActive.Load() {
			t.Error("Close must clear extendedKeyboardActive on teardown")
		}
		if !strings.Contains(output.String(), "\x1b[<u") {
			t.Errorf("teardown must write the Kitty pop \\x1b[<u, got %q", output.String())
		}
	})

	t.Run("CloseWithMessage (no-restoreState branch)", func(t *testing.T) {
		pr, pw, _ := os.Pipe()
		defer pr.Close()
		defer pw.Close()
		var output bytes.Buffer
		app := NewWithSize(4, 1, &output)
		app.in = pr
		// restoreState intentionally nil -> exercises CloseWithMessage's else branch.

		extendedKeyboardActive.Store(true)
		app.CloseWithMessage("") // empty message writes teardown then returns early
		if extendedKeyboardActive.Load() {
			t.Error("CloseWithMessage must clear extendedKeyboardActive on its no-restoreState path")
		}
		if !strings.Contains(output.String(), "\x1b[<u") {
			t.Errorf("teardown must write the Kitty pop \\x1b[<u, got %q", output.String())
		}
	})
}

// --- End-to-end handshake -> capability, and the legacy fallback --------------

// TestExtendedKeyboardHandshakeFlow wires the whole chain in one test: Feed the
// query reply, dispatch it as the Run loop does, and confirm Deliverability flips
// from refusing to accepting Ctrl+Shift+G — then back to refusing on a later
// reply that clears the bit. This is the behaviour gogent's customizer relies on.
func TestExtendedKeyboardHandshakeFlow(t *testing.T) {
	withLegacy(t)
	var parser inputParser
	app := &App{}

	// Before any reply: legacy mode, Ctrl+Shift+G refused.
	extendedKeyboardActive.Store(false)
	if ok, _ := Deliverability(KeyRune, 'g', true, true, false); ok {
		t.Fatal("before handshake reply: Ctrl+Shift+G must be refused")
	}

	// The terminal answers with disambiguate set; the loop dispatches each result.
	for _, ev := range parser.Feed([]byte("\x1b[?1u")) {
		app.dispatchEvent(ev)
	}
	if !extendedKeyboardActive.Load() {
		t.Fatal("handshake reply must flip extendedKeyboardActive to true")
	}
	if ok, reason := Deliverability(KeyRune, 'g', true, true, false); !ok || reason != "" {
		t.Errorf("after handshake: Ctrl+Shift+G = (ok=%v, reason=%q), want (true, \"\")", ok, reason)
	}

	// A later reply clearing the bit returns the table to legacy.
	for _, ev := range parser.Feed([]byte("\x1b[?0u")) {
		app.dispatchEvent(ev)
	}
	if extendedKeyboardActive.Load() {
		t.Fatal("a reply with the disambiguate bit clear must reset to legacy")
	}
	if ok, _ := Deliverability(KeyRune, 'g', true, true, false); ok {
		t.Fatal("after a clearing reply: Ctrl+Shift+G must be refused again")
	}
}

// TestExtendedKeyboardLegacyC0CtrlGFallback documents the no-regression property on
// legacy terminals: a terminal that ignores the Kitty push (Terminal.app, old xterm,
// default tmux) still sends Ctrl+Shift+G as the single C0 byte 0x07, which the
// unchanged C0 branch decodes to a Ctrl-only 'g'. The parser must keep doing this —
// the fix only helps terminals that answer the query. (Pinned exhaustively by
// TestParseOneInputControlRange; this ties it to #464.)
func TestExtendedKeyboardLegacyC0CtrlGFallback(t *testing.T) {
	ev, consumed, ok := parseOneInput([]byte{0x07})
	if !ok || consumed != 1 {
		t.Fatalf("0x07: ok=%v consumed=%d", ok, consumed)
	}
	got := asTypeEvent(t, ev)
	if want := (TypeEvent{Key: KeyRune, Rune: 'g', Ctrl: true}); got != want {
		t.Errorf("legacy Ctrl+G = %+v, want %+v (Shift is unrecoverable on the wire)", got, want)
	}
}
