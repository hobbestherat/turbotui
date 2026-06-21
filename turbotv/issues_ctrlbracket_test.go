package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// gogent issue #208 (turbotui parser) — matchShortcut layer.
//
// gogent registers the Next-Session accelerator as
// WithShortcut("Ctrl+]", tui.KeyRune, ']', true). Desktop dispatch reaches
// matchShortcut correctly; the bug was that the parser handed it the wrong rune.
// This test pins the contract at the dispatch layer: the event the FIXED parser
// emits for byte 0x1D (Rune ']') matches the accelerator, while the event the OLD
// buggy parser emitted (Rune '}') does not — which is exactly why the chord was
// silently dropped.

func TestMatchShortcutCtrlBracketGogent208(t *testing.T) {
	shortcut := NewMenuItem("Ne&xt Session", nil).
		WithShortcut("Ctrl+]", tui.KeyRune, ']', true).Shortcut
	if shortcut == nil {
		t.Fatal("WithShortcut did not build a shortcut")
	}

	// Event the fixed parser produces for 0x1D (0x1D ^ 0x40 = 0x5D = ']').
	fixed := tui.TypeEvent{Key: tui.KeyRune, Rune: ']', Ctrl: true}
	if !matchShortcut(fixed, shortcut) {
		t.Fatalf("fixed Ctrl+] event %+v must match the accelerator", fixed)
	}

	// Event the pre-fix parser produced (0x1D + 'a' - 1 = 0x7D = '}').
	buggy := tui.TypeEvent{Key: tui.KeyRune, Rune: '}', Ctrl: true}
	if matchShortcut(buggy, shortcut) {
		t.Fatalf("old buggy '}' event must NOT match — that silent drop was gogent #208")
	}

	// The rest of the corrected punctuation range must also match their accelerators.
	for _, tc := range []struct {
		display string
		r       rune
	}{
		{"Ctrl+\\", '\\'},
		{"Ctrl+]", ']'},
		{"Ctrl+^", '^'},
		{"Ctrl+_", '_'},
	} {
		sc := NewMenuItem(tc.display, nil).WithShortcut(tc.display, tui.KeyRune, tc.r, true).Shortcut
		ev := tui.TypeEvent{Key: tui.KeyRune, Rune: tc.r, Ctrl: true}
		if !matchShortcut(ev, sc) {
			t.Errorf("%s: event %+v must match shortcut %+v", tc.display, ev, sc)
		}
	}

	// Missing the Ctrl modifier must still reject a ']' rune.
	if matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: ']'}, shortcut) {
		t.Fatalf("Ctrl+] accelerator must require the Ctrl modifier")
	}
}
