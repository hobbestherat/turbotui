package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// These tests cover the mnemonic hot-character equality guard (gogent#268): when
// the advertised hot-key colour is structurally identical to the background it
// would be painted over (e.g. a theme whose accent doubles as both MenuHotFG and
// the selected-row MenuSelectBG), the letter would render invisibly. The guard
// falls back to the label's own foreground in that case, and only in that case.

func TestMnemonicHotFGFallsBackOnlyOnCollision(t *testing.T) {
	accent := tui.RGBColor(0xFF, 0xCC, 0x00)
	white := tui.RGBColor(0xFF, 0xFF, 0xFF)
	style := tui.Cell{FG: white, BG: accent} // selected row: white-on-accent

	// Collision: hotFG == style.BG -> fall back to style.FG so it stays visible.
	if got := mnemonicHotFG(accent, style); got != white {
		t.Fatalf("collision: mnemonicHotFG = %+v, want style.FG %+v", got, white)
	}
	// No collision: a distinct hot colour is preserved untouched, even if it is a
	// deliberately low-contrast hue (the guard triggers on equality, not contrast).
	red := tui.RGBColor(0xAA, 0x00, 0x00)
	if got := mnemonicHotFG(red, style); got != red {
		t.Fatalf("non-collision: mnemonicHotFG = %+v, want hotFG %+v", got, red)
	}
}

func TestDrawMnemonicCollisionStaysVisible(t *testing.T) {
	accent := tui.RGBColor(0xFF, 0xCC, 0x00)
	white := tui.RGBColor(0xFF, 0xFF, 0xFF)
	style := tui.Cell{FG: white, BG: accent}

	app := tui.NewWithSize(20, 2, &bytes.Buffer{})
	surface := newRootSurface(app)

	// "&File": hot 'F' at rune index 0, painted at x=0.
	drawMnemonic(surface, 0, 0, "&File", style, true, accent)
	cell := app.ReadCell(0, 0)
	if cell.Ch != 'F' {
		t.Fatalf("drawMnemonic hot rune = %q, want 'F'", cell.Ch)
	}
	if cell.FG == cell.BG {
		t.Fatalf("drawMnemonic hot char invisible: FG==BG (%+v)", cell.FG)
	}
	if cell.FG != white {
		t.Fatalf("drawMnemonic hot FG = %+v, want fallback style.FG %+v", cell.FG, white)
	}
	if !cell.Bold || !cell.Underline {
		t.Fatalf("drawMnemonic lost affordance: bold=%v underline=%v", cell.Bold, cell.Underline)
	}

	// drawMnemonicClipped takes the same path; "&New" hot 'N' at x=0 on row 1.
	drawMnemonicClipped(surface, 0, 1, "&New", 10, style, true, accent)
	clipped := app.ReadCell(0, 1)
	if clipped.Ch != 'N' {
		t.Fatalf("drawMnemonicClipped hot rune = %q, want 'N'", clipped.Ch)
	}
	if clipped.FG == clipped.BG {
		t.Fatalf("drawMnemonicClipped hot char invisible: FG==BG")
	}
	if clipped.FG != white {
		t.Fatalf("drawMnemonicClipped hot FG = %+v, want fallback %+v", clipped.FG, white)
	}
}

func TestDrawMnemonicNonCollisionKeepsHotFG(t *testing.T) {
	bg := tui.RGBColor(0x00, 0x00, 0x00)
	fg := tui.RGBColor(0xCC, 0xCC, 0xCC)
	hot := tui.RGBColor(0xAA, 0x00, 0x00) // distinct from bg: must be preserved
	style := tui.Cell{FG: fg, BG: bg}

	app := tui.NewWithSize(20, 1, &bytes.Buffer{})
	surface := newRootSurface(app)

	drawMnemonic(surface, 0, 0, "&File", style, true, hot)
	if got := app.ReadCell(0, 0).FG; got != hot {
		t.Fatalf("non-collision hot FG = %+v, want intentional hotFG %+v", got, hot)
	}
}
