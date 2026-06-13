package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestParseMnemonic(t *testing.T) {
	text, index := parseMnemonic("&File")
	if text != "File" || index != 0 {
		t.Fatalf("unexpected mnemonic parse: %q %d", text, index)
	}
}

func TestMenuShortcutMatch(t *testing.T) {
	shortcut := &MenuShortcut{Key: tui.KeyRune, Rune: 'q', Ctrl: true}
	if !matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: 'Q', Ctrl: true}, shortcut) {
		t.Fatalf("expected shortcut match")
	}
	if matchShortcut(tui.TypeEvent{Key: tui.KeyRune, Rune: 'q', Ctrl: false}, shortcut) {
		t.Fatalf("expected ctrl mismatch to fail")
	}
}
