package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// gogent#464 cross-repo seam tests (turbotv side). gogent bumps the turbotui pin and
// relies on Chord.Deliverable() being capability-aware (it delegates to
// tui.Deliverability, which reads tui's private extendedKeyboardActive flag). These
// pin the public API the bump depends on and the dispatch contract once events carry
// Shift. The capability flag is package-private to tui and false in this test binary
// (tv can never set it, and runs in a separate test binary from package tui), so the
// active-mode behaviour is tested in package tui; here we pin the flag-independent
// invariants gogent sees, plus the headline chord at the public seam.

// Compile-time signature stability for the gogent#464 cross-repo seam. If either
// exported shape changes, this file stops compiling — turning gogent's clean pin bump
// ("go get …@<commit> && go mod tidy", no call-site edits) into a source-edit failure.
// This is criterion 4 made into a build-time check.
var (
	_ func(tui.KeyCode, rune, bool, bool, bool) (bool, string) = tui.Deliverability
	_ func() (bool, string)                                       = Chord{}.Deliverable
)

// TestChordDeliverableCtrlShiftGogent464 pins the exact #464 chord at the public
// seam gogent's customizer consults. In this test binary the capability flag is
// false (legacy view), so the chord is refused with the indistinguishability reason —
// the message a non-Kitty terminal / pre-Run config load sees.
func TestChordDeliverableCtrlShiftGogent464(t *testing.T) {
	ok, reason := (Chord{Key: tui.KeyRune, Rune: 'g', Ctrl: true, Shift: true}).Deliverable()
	if ok {
		t.Fatal("Ctrl+Shift+G must be refused in legacy mode (flag is false in this test binary)")
	}
	if reason == "" {
		t.Fatal("the refusal must carry a reason for the capture UI to surface")
	}
}

// TestChordDeliverableCtrlShiftGMatchesTui proves the binding layer is a thin
// delegation to the single source of truth, including for the #464 chord, so tv can
// never drift from the decoder's capability table. Flag-independent: both read the
// same flag at the same instant, so they must always agree.
func TestChordDeliverableCtrlShiftGMatchesTui(t *testing.T) {
	c := Chord{Key: tui.KeyRune, Rune: 'g', Ctrl: true, Shift: true}
	wantOK, wantReason := tui.Deliverability(c.Key, c.Rune, c.Ctrl, c.Shift, c.Alt)
	gotOK, gotReason := c.Deliverable()
	if gotOK != wantOK || gotReason != wantReason {
		t.Errorf("delegation drifted: Chord.Deliverable=(%v,%q), tui.Deliverability=(%v,%q)",
			gotOK, gotReason, wantOK, wantReason)
	}
}

// TestChordMatchesShiftExactForCtrlShiftG is the dispatch half of the fix (design
// point 3): once the parser delivers an event carrying Shift (which it does on a
// Kitty terminal via CSI 103;6u), Chord.Matches must compare Shift EXACTLY so the
// Ctrl+Shift+G binding routes — and must NOT match the legacy Ctrl+G event where
// Shift was lost. Without exact Shift comparison the binding would fire on the wrong
// chord (or the right chord would never fire).
func TestChordMatchesShiftExactForCtrlShiftG(t *testing.T) {
	binding := Chord{Key: tui.KeyRune, Rune: 'g', Ctrl: true, Shift: true}

	// The event a Kitty terminal sends for Ctrl+Shift+G (CSI 103;6u).
	capable := tui.TypeEvent{Key: tui.KeyRune, Rune: 'g', Ctrl: true, Shift: true}
	if !binding.Matches(capable) {
		t.Errorf("Ctrl+Shift+G binding must match the capable-terminal event %+v", capable)
	}

	// The event a legacy terminal sends (Shift lost on the wire) must NOT match.
	legacy := tui.TypeEvent{Key: tui.KeyRune, Rune: 'g', Ctrl: true}
	if binding.Matches(legacy) {
		t.Errorf("Ctrl+Shift+G binding must NOT match the legacy Ctrl+G event %+v (Shift must be exact)", legacy)
	}
}
