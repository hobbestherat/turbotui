package tv

import (
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Phase-3 tests for the terminal-deliverability service exposed on Chord
// (binding.go Chord.Deliverable, delegating to tui.Deliverability). These pin
// which combinations a capture UI must refuse/warn on (the known terminal
// ambiguities) and which are bindable.

// --- Undeliverable / ambiguous combos: ok=false with a human-readable reason. ---

func TestChordDeliverableUndeliverableCombos(t *testing.T) {
	cases := []struct {
		name      string
		chord     Chord
		reasonHas string // a substring the reason must contain, for the capture UI
	}{
		{"Ctrl+M==Enter", Chord{Key: tui.KeyRune, Rune: 'm', Ctrl: true}, "Enter"},
		{"Ctrl+I==Tab", Chord{Key: tui.KeyRune, Rune: 'i', Ctrl: true}, "Tab"},
		{"Ctrl+[==Esc", Chord{Key: tui.KeyRune, Rune: '[', Ctrl: true}, "Esc"},
		{"Ctrl+H==Backspace", Chord{Key: tui.KeyRune, Rune: 'h', Ctrl: true}, "Backspace"},
		{"Ctrl+Z==SIGTSTP", Chord{Key: tui.KeyRune, Rune: 'z', Ctrl: true}, "SIGTSTP"},
		{"Ctrl+S==XOFF", Chord{Key: tui.KeyRune, Rune: 's', Ctrl: true}, "flow control"},
		{"Ctrl+Q==XON", Chord{Key: tui.KeyRune, Rune: 'q', Ctrl: true}, "flow control"},
		{"Ctrl+Shift+A", Chord{Key: tui.KeyRune, Rune: 'a', Ctrl: true, Shift: true}, "indistinguishable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := tc.chord.Deliverable()
			if ok {
				t.Fatalf("%s must be reported undeliverable", tc.name)
			}
			if reason == "" {
				t.Fatalf("%s undeliverable must carry a reason for the capture UI", tc.name)
			}
			if !strings.Contains(reason, tc.reasonHas) {
				t.Errorf("%s reason %q should mention %q", tc.name, reason, tc.reasonHas)
			}
		})
	}
}

// The named-key forms of the conflated keys, carrying Ctrl, are also undeliverable:
// a binding captured as Ctrl+Tab / Ctrl+Backspace cannot fire distinctly.
func TestChordDeliverableNamedKeyCtrlFormsUndeliverable(t *testing.T) {
	cases := []struct {
		name  string
		chord Chord
	}{
		{"Ctrl+Tab", Chord{Key: tui.KeyTab, Ctrl: true}},
		{"Ctrl+Esc", Chord{Key: tui.KeyEscape, Ctrl: true}},
		{"Ctrl+Backspace", Chord{Key: tui.KeyBackspace, Ctrl: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if ok, _ := tc.chord.Deliverable(); ok {
				t.Fatalf("%s must be reported undeliverable", tc.name)
			}
		})
	}
}

// --- Deliverable combos: ok=true with an empty reason. ---

func TestChordDeliverableOrdinaryCombos(t *testing.T) {
	cases := []struct {
		name  string
		chord Chord
	}{
		{"Ctrl+N", Chord{Key: tui.KeyRune, Rune: 'n', Ctrl: true}},
		{"Ctrl+F", Chord{Key: tui.KeyRune, Rune: 'f', Ctrl: true}},
		{"Ctrl+W", Chord{Key: tui.KeyRune, Rune: 'w', Ctrl: true}},
		{"plain a", Chord{Key: tui.KeyRune, Rune: 'a'}},
		{"plain r", Chord{Key: tui.KeyRune, Rune: 'r'}},
		{"plain ?", Chord{Key: tui.KeyRune, Rune: '?'}},
		{"F1", Chord{Key: tui.KeyF1}},
		{"F12", Chord{Key: tui.KeyF12}},
		{"plain Enter", Chord{Key: tui.KeyEnter}},
		{"plain Tab", Chord{Key: tui.KeyTab}},
		{"plain Esc", Chord{Key: tui.KeyEscape}},
		{"plain Backspace", Chord{Key: tui.KeyBackspace}},
		{"Up arrow", Chord{Key: tui.KeyUp}},
		{"Shift+letter", Chord{Key: tui.KeyRune, Rune: 'a', Shift: true}},
		{"Alt+letter", Chord{Key: tui.KeyRune, Rune: 'a', Alt: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := tc.chord.Deliverable()
			if !ok {
				t.Fatalf("%s must be deliverable, reason=%q", tc.name, reason)
			}
			if reason != "" {
				t.Errorf("%s deliverable must have an empty reason, got %q", tc.name, reason)
			}
		})
	}
}

// Case-insensitivity: an upper-case rune for an ambiguous control letter is still
// undeliverable (the decoder lower-cases Ctrl+<letter>, and Deliverable must too).
func TestChordDeliverableCaseInsensitive(t *testing.T) {
	for _, r := range []rune{'M', 'I', 'Z', 'S', 'Q', 'H'} {
		c := Chord{Key: tui.KeyRune, Rune: r, Ctrl: true}
		if ok, _ := c.Deliverable(); ok {
			t.Errorf("Ctrl+%c (upper-case) must be undeliverable like its lower-case form", r)
		}
	}
}

// Ctrl+Shift on a punctuation key (not a-z) does not hit the Shift-letter rule but
// still resolves via the per-key table (Ctrl+Shift+[ is the Esc ambiguity).
func TestChordDeliverableCtrlShiftPunctuation(t *testing.T) {
	c := Chord{Key: tui.KeyRune, Rune: '[', Ctrl: true, Shift: true}
	if ok, reason := c.Deliverable(); ok {
		t.Fatalf("Ctrl+Shift+[ must be undeliverable (Esc ambiguity), got ok with reason %q", reason)
	}
}

// Deliverable must be a thin delegation to tui.Deliverability — the single source of
// truth next to the decoder. Verify they agree across a representative sample so the
// binding layer can never drift from the app's table.
func TestChordDeliverableMatchesTuiDeliverability(t *testing.T) {
	chords := []Chord{
		{Key: tui.KeyRune, Rune: 'm', Ctrl: true},
		{Key: tui.KeyRune, Rune: 'n', Ctrl: true},
		{Key: tui.KeyRune, Rune: 'a', Ctrl: true, Shift: true},
		{Key: tui.KeyRune, Rune: 'a'},
		{Key: tui.KeyEnter, Ctrl: true},
		{Key: tui.KeyF1},
	}
	for _, c := range chords {
		wantOK, wantReason := tui.Deliverability(c.Key, c.Rune, c.Ctrl, c.Shift, c.Alt)
		gotOK, gotReason := c.Deliverable()
		if gotOK != wantOK || gotReason != wantReason {
			t.Errorf("Chord%v.Deliverable()=(%v,%q), tui.Deliverability=(%v,%q): must match",
				c, gotOK, gotReason, wantOK, wantReason)
		}
	}
}

// REGRESSION GUARD (was a FINDING): the decoder surfaces LF/^J as a DISTINCT, usable
// submit key TypeEvent{Key: KeyEnter, Ctrl: true}, so the named-key Ctrl+Enter chord
// must be DELIVERABLE — it is distinguishable from a plain Enter and from a rune
// Ctrl+M. The earlier implementation folded `key == KeyEnter` into the Ctrl+M==Enter
// case and wrongly refused it; the fix distinguishes the named-key submit chord from
// the rune Ctrl+M. This guard pins the corrected behaviour so a regression re-trips.
func TestChordDeliverableCtrlEnterIsDeliverable(t *testing.T) {
	// The named-key Ctrl+Enter is a real, distinct submit key.
	if ok, reason := (Chord{Key: tui.KeyEnter, Ctrl: true}).Deliverable(); !ok {
		t.Fatalf("Ctrl+Enter (named-key) must be deliverable, got reason %q", reason)
	}
	// But the RUNE Ctrl+M still collapses to carriage return == plain Enter.
	ok, reason := (Chord{Key: tui.KeyRune, Rune: 'm', Ctrl: true}).Deliverable()
	if ok {
		t.Fatal("rune Ctrl+M must remain undeliverable (== Enter)")
	}
	if !strings.Contains(reason, "Enter") {
		t.Errorf("Ctrl+M reason %q expected to mention Enter", reason)
	}
}
