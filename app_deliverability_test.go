package tui

import (
	"strings"
	"testing"
)

// Phase-3 tests for the terminal-deliverability table (app.go Deliverability), the
// single source of truth that lives next to the byte->TypeEvent decoder. These pin
// the known terminal ambiguities and prove the table is consistent with what the
// decoder above it actually produces.

func TestDeliverabilityUndeliverableCtrlLetters(t *testing.T) {
	cases := []struct {
		name      string
		r         rune
		reasonHas string
	}{
		{"Ctrl+M", 'm', "Enter"},
		{"Ctrl+I", 'i', "Tab"},
		{"Ctrl+H", 'h', "Backspace"},
		{"Ctrl+J", 'j', "line feed"},
		{"Ctrl+Z", 'z', "SIGTSTP"},
		{"Ctrl+S", 's', "flow control"},
		{"Ctrl+Q", 'q', "flow control"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := Deliverability(KeyRune, tc.r, true, false, false)
			if ok {
				t.Fatalf("%s must be undeliverable", tc.name)
			}
			if !strings.Contains(reason, tc.reasonHas) {
				t.Errorf("%s reason %q should mention %q", tc.name, reason, tc.reasonHas)
			}
		})
	}
}

func TestDeliverabilityCtrlBracketIsEsc(t *testing.T) {
	ok, reason := Deliverability(KeyRune, '[', true, false, false)
	if ok {
		t.Fatal("Ctrl+[ must be undeliverable (== Esc)")
	}
	if !strings.Contains(reason, "Esc") {
		t.Errorf("Ctrl+[ reason %q should mention Esc", reason)
	}
}

func TestDeliverabilityCtrlShiftLetterIndistinguishable(t *testing.T) {
	ok, reason := Deliverability(KeyRune, 'a', true, true, false)
	if ok {
		t.Fatal("Ctrl+Shift+letter must be undeliverable")
	}
	if !strings.Contains(reason, "indistinguishable") {
		t.Errorf("reason %q should explain the Ctrl+letter indistinguishability", reason)
	}
}

func TestDeliverabilityOrdinaryCtrlCombosDeliverable(t *testing.T) {
	// The "safe" control letters used as real shortcuts must stay deliverable.
	for _, r := range []rune{'n', 'f', 'w', 'k', 'r', 'g', 'p', 'o', 'e', 't', 'u', 'y'} {
		ok, reason := Deliverability(KeyRune, r, true, false, false)
		if !ok {
			t.Errorf("Ctrl+%c must be deliverable, got reason %q", r, reason)
		}
		if reason != "" {
			t.Errorf("Ctrl+%c deliverable must have empty reason, got %q", r, reason)
		}
	}
}

// Without Ctrl there is no C0 collapsing or job/flow-control capture: every named
// key and every printable rune is deliverable. Exercise the named keys and a sweep
// of printable runes, including the ones that are ambiguous WITH Ctrl.
func TestDeliverabilityNonCtrlAlwaysDeliverable(t *testing.T) {
	namedKeys := []KeyCode{
		KeyEnter, KeyTab, KeyBackspace, KeyEscape, KeyUp, KeyDown, KeyLeft, KeyRight,
		KeyHome, KeyEnd, KeyPageUp, KeyPageDown, KeyInsert, KeyDelete,
		KeyF1, KeyF5, KeyF12,
	}
	for _, k := range namedKeys {
		if ok, reason := Deliverability(k, 0, false, false, false); !ok {
			t.Errorf("plain named key %d must be deliverable, got reason %q", k, reason)
		}
		// Shift/Alt without Ctrl are still deliverable (Shift+F1, etc).
		if ok, _ := Deliverability(k, 0, false, true, true); !ok {
			t.Errorf("Shift+Alt named key %d (no Ctrl) must be deliverable", k)
		}
	}
	for r := rune('!'); r <= rune('~'); r++ {
		if ok, reason := Deliverability(KeyRune, r, false, false, false); !ok {
			t.Errorf("plain printable %q must be deliverable, got reason %q", r, reason)
		}
	}
}

// Property: a non-empty reason must accompany every undeliverable verdict, and an
// empty reason every deliverable one, across a broad sweep of Ctrl combos. A capture
// UI relies on always having text to show when it refuses a combo.
func TestDeliverabilityReasonAccompaniesVerdict(t *testing.T) {
	for r := rune('a'); r <= rune('z'); r++ {
		for _, shift := range []bool{false, true} {
			ok, reason := Deliverability(KeyRune, r, true, shift, false)
			if ok && reason != "" {
				t.Errorf("Ctrl+%c shift=%v deliverable but carries reason %q", r, shift, reason)
			}
			if !ok && reason == "" {
				t.Errorf("Ctrl+%c shift=%v undeliverable but has no reason", r, shift)
			}
		}
	}
}

func TestDeliverabilityCaseInsensitiveLetters(t *testing.T) {
	// Upper- and lower-case must agree for the ambiguous control letters.
	for _, r := range []rune{'M', 'I', 'H', 'J', 'Z', 'S', 'Q'} {
		lo := r + ('a' - 'A')
		okU, _ := Deliverability(KeyRune, r, true, false, false)
		okL, _ := Deliverability(KeyRune, lo, true, false, false)
		if okU != okL {
			t.Errorf("Ctrl+%c and Ctrl+%c must agree (got %v vs %v)", r, lo, okU, okL)
		}
		if okU {
			t.Errorf("Ctrl+%c must be undeliverable", r)
		}
	}
}

// Consistency with the decoder: the bytes the decoder folds into Enter/Tab/Backspace
// are exactly the Ctrl chords the table calls undeliverable. Decode the raw byte and
// confirm the conflation the deliverability reason claims.
func TestDeliverabilityConsistentWithDecoderConflation(t *testing.T) {
	// Ctrl+M sends CR (0x0d), which the decoder turns into a plain Enter — proving
	// Ctrl+M is indistinguishable from Enter, exactly what Deliverability reports.
	evAny, _, ok := parseOneInput([]byte{'\r'})
	if !ok {
		t.Fatal("CR must decode")
	}
	if got, isType := evAny.(TypeEvent); !isType || got.Key != KeyEnter || got.Ctrl {
		t.Fatalf("CR must decode to a plain Enter, got %+v", evAny)
	}
	if dok, _ := Deliverability(KeyRune, 'm', true, false, false); dok {
		t.Fatal("Ctrl+M must be undeliverable, consistent with CR==Enter")
	}

	// Ctrl+I sends TAB (0x09) -> plain Tab.
	evAny, _, _ = parseOneInput([]byte{'\t'})
	if got, _ := evAny.(TypeEvent); got.Key != KeyTab || got.Ctrl {
		t.Fatalf("TAB must decode to a plain Tab, got %+v", evAny)
	}
	if dok, _ := Deliverability(KeyRune, 'i', true, false, false); dok {
		t.Fatal("Ctrl+I must be undeliverable, consistent with TAB==Tab")
	}
}

// REGRESSION GUARD (was a FINDING): the decoder surfaces LF/^J as a DISTINCT usable
// event TypeEvent{Key: KeyEnter, Ctrl: true}. The deliverability table must now AGREE
// with the decoder for the named-key submit chord: Ctrl+Enter (key==KeyEnter, ctrl) is
// DELIVERABLE. The raw rune Ctrl+J is still undeliverable (it is one of several bytes
// that fold to that single submit event, so a binding on the rune 'j' can't be told
// apart from a real Ctrl+Enter). This guard pins the reconciliation.
func TestDeliverabilityCtrlEnterReconciledWithDecoder(t *testing.T) {
	evAny, _, ok := parseOneInput([]byte{'\n'})
	if !ok {
		t.Fatal("LF must decode")
	}
	got, _ := evAny.(TypeEvent)
	if got.Key != KeyEnter || !got.Ctrl {
		t.Fatalf("LF must decode to a distinct Ctrl+Enter, got %+v", evAny)
	}
	// The named-key Ctrl+Enter the decoder produces is now deliverable, matching reality.
	if dok, reason := Deliverability(KeyEnter, 0, true, false, false); !dok {
		t.Fatalf("Ctrl+Enter (named-key) must be deliverable, got reason %q", reason)
	}
	// The raw rune Ctrl+J remains undeliverable (folds into the submit event).
	if dok, _ := Deliverability(KeyRune, 'j', true, false, false); dok {
		t.Fatal("rune Ctrl+J must remain undeliverable")
	}
	// And rune Ctrl+M still collapses to plain Enter.
	if dok, _ := Deliverability(KeyRune, 'm', true, false, false); dok {
		t.Fatal("rune Ctrl+M must remain undeliverable")
	}
}

// Alt is accepted but intentionally does NOT lift the Ctrl-level verdicts (the
// OS/terminal captures act on the raw control byte, which an Alt/ESC prefix leaves in
// the stream). Pin that design decision: Ctrl+Alt+S stays undeliverable, while an Alt
// modifier on an already-deliverable combo (Alt+N, no Ctrl) stays deliverable.
func TestDeliverabilityAltDoesNotLiftCtrlVerdict(t *testing.T) {
	if ok, _ := Deliverability(KeyRune, 's', true, false, true); ok {
		t.Fatal("Ctrl+Alt+S must remain undeliverable (Alt does not lift flow-control capture)")
	}
	if ok, _ := Deliverability(KeyRune, 'm', true, false, true); ok {
		t.Fatal("Ctrl+Alt+M must remain undeliverable")
	}
	if ok, reason := Deliverability(KeyRune, 'n', false, false, true); !ok {
		t.Fatalf("Alt+N (no Ctrl) must be deliverable, got reason %q", reason)
	}
}
