package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestSetThemeSeedsNewWidgets(t *testing.T) {
	saved := ActiveTheme()
	defer SetTheme(saved)

	SetTheme(HighContrastTheme)
	if ActiveTheme().WindowBG != HighContrastTheme.WindowBG {
		t.Fatalf("ActiveTheme not updated by SetTheme")
	}

	label := NewLabel("&Hi", Rect{X: 0, Y: 0, W: 5, H: 1})
	if label.FG != HighContrastTheme.WindowFG || label.BG != HighContrastTheme.WindowBG {
		t.Fatalf("label did not seed from active theme: fg=%+v bg=%+v", label.FG, label.BG)
	}
	if label.HotFG != HighContrastTheme.MnemonicFG {
		t.Fatalf("label hot fg = %+v, want %+v", label.HotFG, HighContrastTheme.MnemonicFG)
	}
}

func TestMenuBarSeedsFromTheme(t *testing.T) {
	saved := ActiveTheme()
	defer SetTheme(saved)

	custom := DefaultTheme
	custom.MenuBarFG = tui.ANSIColor(12)
	custom.MenuBarBG = tui.ANSIColor(13)
	custom.MenuHotFG = tui.ANSIColor(14)
	custom.MenuHotBG = tui.ANSIColor(5)
	custom.MenuSelectFG = tui.ANSIColor(2)
	custom.MenuSelectBG = tui.ANSIColor(3)
	custom.MenuShadow = tui.ANSIColor(6)
	SetTheme(custom)

	bar := NewMenuBar(Rect{X: 0, Y: 0, W: 20, H: 1}, NewSubMenu("&File"))
	checks := []struct {
		name string
		got  tui.Color
		want tui.Color
	}{
		{"FG", bar.FG, custom.MenuBarFG},
		{"BG", bar.BG, custom.MenuBarBG},
		{"HotFG", bar.HotFG, custom.MenuHotFG},
		{"HotBG", bar.HotBG, custom.MenuHotBG},
		{"SelectFG", bar.SelectFG, custom.MenuSelectFG},
		{"SelectBG", bar.SelectBG, custom.MenuSelectBG},
		{"Shadow", bar.ShadowCol, custom.MenuShadow},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Fatalf("menu %s = %+v, want %+v", c.name, c.got, c.want)
		}
	}
}

func TestDefaultThemeContrast(t *testing.T) {
	// Mnemonic colours must not be the audited low-contrast picks: bright red on
	// blue, or yellow on the light dialog background.
	if DefaultTheme.MnemonicFG == tui.ANSIColor(9) {
		t.Fatalf("window mnemonic is still bright red (low contrast on blue)")
	}
	if DefaultTheme.DialogMnemonicFG == DefaultTheme.MnemonicFG {
		t.Fatalf("dialog mnemonic must differ so it is not yellow-on-light-grey")
	}
	// No key chrome pair may collapse to identical fg/bg.
	pairs := [][2]tui.Color{
		{DefaultTheme.WindowFG, DefaultTheme.WindowBG},
		{DefaultTheme.DialogFG, DefaultTheme.DialogBG},
		{DefaultTheme.MenuBarFG, DefaultTheme.MenuBarBG},
		{DefaultTheme.MenuSelectFG, DefaultTheme.MenuSelectBG},
		{DefaultTheme.SelectionFG, DefaultTheme.SelectionBG},
		{DefaultTheme.MnemonicFG, DefaultTheme.WindowBG},
		{DefaultTheme.DialogMnemonicFG, DefaultTheme.DialogBG},
	}
	for i, p := range pairs {
		if p[0] == p[1] {
			t.Fatalf("pair %d has identical fg/bg %+v", i, p[0])
		}
	}
}

func TestUnicodeLower(t *testing.T) {
	tests := []struct {
		in   rune
		want rune
	}{
		{'A', 'a'},
		{'z', 'z'},
		{'Ü', 'ü'},
		{'ẞ', 'ß'}, // capital sharp s -> sharp s (differs in code point)
		{'5', '5'},
	}
	for _, tc := range tests {
		if got := unicodeLower(tc.in); got != tc.want {
			t.Fatalf("unicodeLower(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Capital and lowercase forms must fold to the same rune so a label mnemonic
	// round-trips against the typed key.
	if labelMnemonic("&Über") != unicodeLower('ü') {
		t.Fatalf("mnemonic for &Über did not round-trip to 'ü'")
	}
}

func TestDrawMnemonicWideOffset(t *testing.T) {
	app := tui.NewWithSize(10, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	// A leading double-width glyph precedes the hot char: "世&X" -> text "世X",
	// hot rune index 1, but display column 2.
	style := tui.Cell{FG: tui.ANSIColor(15), BG: tui.ANSIColor(0)}
	drawMnemonic(surface, 0, 0, "世&X", style, true, tui.ANSIColor(11))

	cell := app.ReadCell(2, 0)
	if cell.Ch != 'X' {
		t.Fatalf("hot char at column 2 = %q, want X", cell.Ch)
	}
	if !cell.Bold || !cell.Underline || cell.FG != tui.ANSIColor(11) {
		t.Fatalf("hot char not highlighted: %+v", cell)
	}
}
