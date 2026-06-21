package tv

import (
	"bytes"
	"strings"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// These tests target the screen-artifact / shadow-leak behind gogent #213. The
// bug: DrawShadow's drawShadowCell used to read the underlying cell and, unless it
// was blank, RE-EMIT the underlying foreground glyph recoloured to the shadow
// colour. So the shadow band was not a pure function of its geometry — it mirrored
// whatever glyph happened to sit in the back buffer at that cell. When that glyph
// was stale (a letter from a widget drawn into the column on an earlier frame and
// never cleared), the shadow painted that letter into the band, producing the
// stray "e"s and corrupted divider reported in #213.
//
// The fix makes each shadow cell OWN the cell: it always lays down shadowGlyph in
// the shadow colour, preserving only the underlying background. The tests below
// would fail against the pre-fix drawShadowCell (which re-emitted 'e') and pass
// against the fixed one (which writes shadowGlyph).

// staleGlyph is the exact artifact character from the #213 report.
const staleGlyph = 'e'

// plantStaleGlyphs fills every cell of the back buffer with a stale glyph in a
// known foreground/background, simulating a previously-drawn layer (e.g. a
// "Session" label) whose content was never cleared before the shadow is composed
// over it. This is the precondition that produced the leak.
func plantStaleGlyphs(app *tui.App, w int, h int, ch rune) {
	stale := tui.Cell{Ch: ch, FG: tui.ANSIColor(15), BG: tui.ANSIColor(4)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			app.WriteCell(x, y, stale)
		}
	}
}

// TestDrawShadowLaysShadowGlyphOnCleanGrid is the normal case: over a clean
// (default) grid every shadow-band cell must carry the shadow glyph in the shadow
// colour. This pins the post-fix behaviour independently of any stale content.
func TestDrawShadowLaysShadowGlyphOnCleanGrid(t *testing.T) {
	const w, h = 12, 8
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	rect := Rect{X: 2, Y: 2, W: 5, H: 3}
	surface.DrawShadow(rect, shadow, DefaultShadowStyle)

	band := shadowCells(app, shadow, w, h)
	if len(band) == 0 {
		t.Fatal("expected shadow band cells, found none")
	}
	for cell := range band {
		got := app.ReadCell(cell[0], cell[1])
		if got.Ch != shadowGlyph {
			t.Errorf("clean-grid shadow cell %v Ch=%q, want %q", cell, got.Ch, shadowGlyph)
		}
		if got.FG != shadow {
			t.Errorf("clean-grid shadow cell %v FG=%v, want shadow colour", cell, got.FG)
		}
	}
}

// TestDrawShadowDoesNotLeakStaleGlyphIntoBand is the headline repro for #213: with
// the exact stale "e" sitting in every back-buffer cell where the band will land,
// the shadow must still render shadowGlyph everywhere — never the leaked letter.
// Against the pre-fix code this fails because drawShadowCell re-emitted the 'e'.
func TestDrawShadowDoesNotLeakStaleGlyphIntoBand(t *testing.T) {
	const w, h = 14, 8
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	// Pre-seed the whole buffer with the stale glyph, including the exact cells
	// the shadow is about to own — the condition under which the leak surfaced.
	plantStaleGlyphs(app, w, h, staleGlyph)

	rect := Rect{X: 2, Y: 2, W: 5, H: 3} // Right()=6, Bottom()=4
	surface.DrawShadow(rect, shadow, DefaultShadowStyle)

	band := shadowCells(app, shadow, w, h)
	if len(band) == 0 {
		t.Fatal("expected shadow band cells, found none")
	}
	for cell := range band {
		x, y := cell[0], cell[1]
		got := app.ReadCell(x, y)
		if got.Ch != shadowGlyph {
			t.Errorf("shadow cell (%d,%d) leaked stale glyph %q into the band; want %q",
				x, y, got.Ch, shadowGlyph)
		}
		if got.Ch == staleGlyph {
			t.Errorf("shadow cell (%d,%d) is the stale %q — the #213 artifact persisted", x, y, staleGlyph)
		}
	}
}

// TestDrawShadowDoesNotLeakRealisticLabel simulates the reported source more
// closely: a "Session"-style label drawn into the right-band columns and never
// cleared. None of its letters may survive into the shadow band.
func TestDrawShadowDoesNotLeakRealisticLabel(t *testing.T) {
	const w, h = 16, 6
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	// Element at (1,1) size 4x2 -> Right()=4, Bottom()=2. Default style right band
	// lands on columns 5,6 for rows 2,3. Paint a label across those columns.
	label := tui.Cell{Ch: 'x', FG: tui.ANSIColor(11), BG: tui.ANSIColor(2)}
	for x, r := range "Session" {
		app.WriteCell(5+x, 2, tui.Cell{Ch: r, FG: label.FG, BG: label.BG})
	}

	surface.DrawShadow(Rect{X: 1, Y: 1, W: 4, H: 2}, shadow, DefaultShadowStyle)

	band := shadowCells(app, shadow, w, h)
	for cell := range band {
		got := app.ReadCell(cell[0], cell[1])
		if got.Ch != shadowGlyph {
			t.Errorf("band cell %v = %q (label leaked); want %q", cell, got.Ch, shadowGlyph)
		}
	}
}

// TestDrawShadowPreservesUnderlyingBackground verifies the one thing the shadow
// intentionally keeps from the underlying cell: its background colour. The glyph
// and foreground are owned by the shadow; the background is preserved.
func TestDrawShadowPreservesUnderlyingBackground(t *testing.T) {
	const w, h = 10, 6
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)
	underBG := tui.RGBColor(10, 20, 30)

	// Plant a distinct background under the whole buffer, with a stale glyph on top.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			app.WriteCell(x, y, tui.Cell{Ch: 'Q', FG: tui.ANSIColor(11), BG: underBG})
		}
	}

	// style with 1-cell right and bottom bands so the band is small and exact.
	rect := Rect{X: 1, Y: 1, W: 4, H: 2} // Right()=4, Bottom()=2
	surface.DrawShadow(rect, shadow, ShadowStyle{OffsetX: 1, OffsetY: 1, RightWidth: 1, BottomHeight: 1})

	band := shadowCells(app, shadow, w, h)
	if len(band) == 0 {
		t.Fatal("expected shadow band cells, found none")
	}
	for cell := range band {
		got := app.ReadCell(cell[0], cell[1])
		if got.Ch != shadowGlyph {
			t.Errorf("band cell %v Ch=%q, want %q", cell, got.Ch, shadowGlyph)
		}
		if got.FG != shadow {
			t.Errorf("band cell %v FG=%v, want shadow colour (not the stale foreground)", cell, got.FG)
		}
		if got.BG != underBG {
			t.Errorf("band cell %v BG=%v, want underlying background %v preserved", cell, got.BG, underBG)
		}
	}
}

// TestDrawShadowCellOwnsGlyphDirectly drives drawShadowCell as a unit over a range
// of underlying glyphs (blank, NUL-coerced, ordinary letters) and asserts every one
// resolves to shadowGlyph. The space/NUL cases agreed with the old code too; the
// letter cases are where the old code leaked and the fix owns the cell.
func TestDrawShadowCellOwnsGlyphDirectly(t *testing.T) {
	app := tui.NewWithSize(4, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)
	underBG := tui.RGBColor(1, 2, 3)

	for _, under := range []rune{' ', 0, 'e', 'X', 'S', '5'} {
		app.WriteCell(0, 0, tui.Cell{Ch: under, FG: tui.ANSIColor(15), BG: underBG})
		surface.drawShadowCell(0, 0, shadow)
		got := app.ReadCell(0, 0)
		if got.Ch != shadowGlyph {
			t.Errorf("underlying %q: shadow cell Ch=%q, want %q", under, got.Ch, shadowGlyph)
		}
		if got.FG != shadow {
			t.Errorf("underlying %q: FG=%v, want shadow colour", under, got.FG)
		}
		if got.BG != underBG {
			t.Errorf("underlying %q: BG=%v, want preserved %v", under, got.BG, underBG)
		}
	}
}

// TestDrawShadowOverWideGlyphLeavesNoOrphan covers the edge case of a stale
// double-width glyph sitting where a shadow cell lands. The shadow must own the
// cell (shadowGlyph) and must not leave the wide glyph's orphaned right half
// behind. Against the pre-fix code the wide glyph itself leaked into the band.
func TestDrawShadowOverWideGlyphLeavesNoOrphan(t *testing.T) {
	app := tui.NewWithSize(4, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	// A wide glyph at column 0 occupies (0,0) and a continuation half at (1,0).
	app.WriteCell(0, 0, tui.Cell{Ch: '世', FG: tui.ANSIColor(15), BG: tui.ANSIColor(4)})

	surface.drawShadowCell(0, 0, shadow)

	if got := app.ReadCell(0, 0).Ch; got != shadowGlyph {
		t.Errorf("wide-underlying shadow cell Ch=%q, want %q", got, shadowGlyph)
	}
	// The continuation half must be blanked, not left holding the wide glyph.
	if got := app.ReadCell(1, 0).Ch; got != ' ' {
		t.Errorf("orphaned wide half at (1,0) = %q, want blank", got)
	}
}

// TestDrawShadowOverWideGlyphContinuationHalf covers the edge case where the shadow
// lands on the CONTINUATION (right) half of a wide glyph whose base sits beside the
// band. The shadow must own the cell (shadowGlyph) and the orphaned base must be
// blanked — no half-wide-glyph may linger on screen. Against the pre-fix code the
// continuation cell stored ' ' so both behaviours agreed here, but the orphan-blank
// contract is still worth pinning.
func TestDrawShadowOverWideGlyphContinuationHalf(t *testing.T) {
	app := tui.NewWithSize(5, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	// Wide glyph at (0,0) occupies (0,0) (base) and (1,0) (continuation).
	app.WriteCell(0, 0, tui.Cell{Ch: '世', FG: tui.ANSIColor(15), BG: tui.ANSIColor(4)})

	// Shadow the continuation cell directly.
	surface.drawShadowCell(1, 0, shadow)

	if got := app.ReadCell(1, 0).Ch; got != shadowGlyph {
		t.Errorf("continuation-half shadow cell Ch=%q, want %q", got, shadowGlyph)
	}
	if got := app.ReadCell(0, 0).Ch; got != ' ' {
		t.Errorf("orphaned wide base at (0,0) = %q, want blank", got)
	}
}

// TestDrawShadowRespectsSurfaceClip checks the clip guard in drawShadowCell: a
// shadow cell that falls outside the surface's clip rect is left untouched, so the
// shadow never overdraws a region it was not given. The untouched cell keeps its
// stale glyph (it is simply not the shadow's to paint).
func TestDrawShadowRespectsSurfaceClip(t *testing.T) {
	const w, h = 10, 6
	app := tui.NewWithSize(w, h, &bytes.Buffer{})
	// Clip the surface to columns 0..5; column 6 is outside.
	surface := newRootSurface(app).WithClip(Rect{X: 0, Y: 0, W: 6, H: h})
	shadow := tui.ANSIColor(8)
	plantStaleGlyphs(app, w, h, staleGlyph)

	// rect {1,1,4,2} -> Right()=4; default style right band is columns 5,6.
	surface.DrawShadow(Rect{X: 1, Y: 1, W: 4, H: 2}, shadow, DefaultShadowStyle)

	// Column 5 is inside the clip: the shadow owns it.
	if got := app.ReadCell(5, 2).Ch; got != shadowGlyph {
		t.Errorf("in-clip band cell (5,2) = %q, want %q", got, shadowGlyph)
	}
	// Column 6 is outside the clip: it must be untouched (still the stale glyph),
	// never painted by the shadow.
	got := app.ReadCell(6, 2)
	if got.Ch == shadowGlyph {
		t.Errorf("out-of-clip cell (6,2) was painted by the shadow; clip guard failed")
	}
	if got.FG == shadow {
		t.Errorf("out-of-clip cell (6,2) carries the shadow colour; clip guard failed")
	}
}

// TestShadowBandHealsOnOrdinaryApply ties the fix to the persistence signature in
// #213: the artifact "does NOT heal on an ordinary repaint but DOES clear when
// that exact cell is force-redrawn". With the fix, re-running DrawShadow over an
// already-corrupted cell writes a deterministic shadowGlyph that differs from the
// stale glyph the front buffer recorded, so an ORDINARY Apply (no Invalidate)
// repaints and heals it. Against the pre-fix code the band stayed the stale glyph
// and the Apply diff found nothing to repaint.
func TestShadowBandHealsOnOrdinaryApply(t *testing.T) {
	var buf bytes.Buffer
	app := tui.NewWithSize(8, 5, &buf)
	surface := newRootSurface(app)
	shadow := tui.ANSIColor(8)

	// rect {1,1,4,2} -> Right()=4; default style right band includes (5,2).
	const bandX, bandY = 5, 2

	// Plant the stale glyph and flush it, so the App's front buffer believes the
	// 'e' is genuinely on screen — the corrupted state.
	app.WriteCell(bandX, bandY, tui.Cell{Ch: staleGlyph, FG: tui.ANSIColor(15), BG: tui.ANSIColor(0)})
	if err := app.Apply(); err != nil {
		t.Fatalf("settle apply: %v", err)
	}
	buf.Reset()

	// Compose the shadow over the corrupted cell, then issue an ordinary flush.
	surface.DrawShadow(Rect{X: 1, Y: 1, W: 4, H: 2}, shadow, DefaultShadowStyle)
	if err := app.Apply(); err != nil {
		t.Fatalf("heal apply: %v", err)
	}

	// The ordinary Apply must have repainted the band cell to the shadow glyph.
	if !strings.Contains(buf.String(), string(shadowGlyph)) {
		t.Errorf("ordinary repaint did not heal the shadow cell; output=%q", buf.String())
	}
	// The stale glyph must be gone from the wire too: the band cell is repainted as
	// the shadow glyph, so the stray 'e' no longer reaches the terminal.
	if strings.Contains(buf.String(), string(staleGlyph)) {
		t.Errorf("stale glyph %q still present in the repaint output; not healed: %q", staleGlyph, buf.String())
	}
	if got := app.ReadCell(bandX, bandY).Ch; got != shadowGlyph {
		t.Errorf("band cell after heal = %q, want %q", got, shadowGlyph)
	}
}
