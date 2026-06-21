package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the turbotui half of gogent #231: the "Disable shadows" preference
// (#215) flattened Window/Button/MenuBar because each gates its drop shadow
// behind a public Shadow field, but Select drew its dropdown-popup shadow
// UNCONDITIONALLY in drawPopup and exposed no field to toggle it. The fix adds a
// public Shadow bool (default true) and gates the DrawShadow call on it,
// mirroring Window/Button/MenuBar, so gogent can do
// `applySelectShadow(sel){ sel.Shadow = shadowsEnabled }`.
//
// These tests pin the contract from every angle: the default, the on/off draw
// paths, that the gate wraps ONLY the shadow (popup content is untouched), that
// Shadow is read at draw time (live toggle), and that suppressing the shadow
// never impairs opening, navigation or committing.

// shadowGlyphCells returns every cell carrying the shadow texture glyph — the
// only rune DrawShadow/drawShadowCell ever lays down — in the w×h back buffer.
// It is the theme-independent signature of "a drop shadow was composited": no
// other widget writes shadowGlyph, so the count is zero precisely when no
// shadow was drawn and non-zero precisely when one was.
func shadowGlyphCells(app *tui.App, w, h int) [][2]int {
	var out [][2]int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if app.ReadCell(x, y).Ch == shadowGlyph {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// shadowCellsInColour counts shadow-glyph cells painted in a specific foreground
// colour — used to assert the band uses activeTheme.WindowShadow (the colour is
// meant to stay as-is; only visibility became conditional).
func shadowCellsInColour(app *tui.App, want tui.Color, w, h int) int {
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := app.ReadCell(x, y)
			if c.Ch == shadowGlyph && c.FG == want {
				n++
			}
		}
	}
	return n
}

// openSelectWith builds a fresh desktop holding a 3-option Select, sets its
// Shadow field, opens the popup and composites one frame. It returns the widget,
// the app back buffer and the popup rect. The control sits mid-screen so the
// full default shadow band lands on-grid.
func openSelectWith(t *testing.T, shadow bool) (*Select, *tui.App, Rect) {
	t.Helper()
	const w, h = 60, 20
	desktop, s := setupSelect(w, h, []string{"alpha", "bravo", "charlie"},
		Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.Shadow = shadow
	s.open()
	if !s.IsOpen() {
		t.Fatalf("popup failed to open (Shadow=%v)", shadow)
	}
	desktop.Redraw()
	return s, desktop.App(), s.popupRect()
}

// TestSelectShadowDefaultsTrue pins the backwards-compatible default: NewSelect
// must leave Shadow == true so existing UIs keep their dropdown shadow until a
// caller (gogent's applySelectShadow) opts out.
func TestSelectShadowDefaultsTrue(t *testing.T) {
	desktop, s := setupSelect(60, 20, []string{"a", "b"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	if !s.Shadow {
		t.Fatalf("NewSelect must default Shadow to true; the default changed and would silently flatten existing UIs")
	}
	s.open()
	desktop.Redraw()
	if got := len(shadowGlyphCells(desktop.App(), 60, 20)); got == 0 {
		t.Fatalf("default Shadow=true popup drew no shadow; expected the drop-shadow band")
	}
}

// TestSelectPopupDrawsShadowWhenShadowTrue is the regression guard for the
// "still on" path: an explicitly Shadow=true opened popup composites a drop
// shadow, every band cell carries the theme's WindowShadow colour, and the band
// stays strictly outside the popup rect. This catches an inverted gate
// (if !s.Shadow) as well as a changed colour/style.
func TestSelectPopupDrawsShadowWhenShadowTrue(t *testing.T) {
	shadow := ActiveTheme().WindowShadow
	_, app, rect := openSelectWith(t, true)

	cells := shadowGlyphCells(app, 60, 20)
	if len(cells) == 0 {
		t.Fatalf("Shadow=true popup drew no shadow; the fix must not disable the default shadow")
	}
	if n := shadowCellsInColour(app, shadow, 60, 20); n != len(cells) {
		t.Errorf("shadow cells in WindowShadow colour = %d, want all %d; the band must keep activeTheme.WindowShadow", n, len(cells))
	}
	for _, c := range cells {
		if rect.Contains(c[0], c[1]) {
			t.Errorf("shadow cell %v landed inside the popup rect %v; the band must hug the outside only", c, rect)
		}
	}
}

// TestSelectPopupShadowUsesDefaultGeometry pins that the band keeps the classic
// DefaultShadowStyle shape — a 2-column right band and a 1-row bottom band — so
// the fix changed only the visibility, not the style.
func TestSelectPopupShadowUsesDefaultGeometry(t *testing.T) {
	_, app, rect := openSelectWith(t, true)
	right := rect.Right()
	bottom := rect.Bottom()

	// 2-column right band starting OffsetY (1) rows below the popup top.
	if app.ReadCell(right+1, rect.Y+1).Ch != shadowGlyph {
		t.Errorf("right band column %d at y=%d is %q, want shadow glyph", right+1, rect.Y+1, app.ReadCell(right+1, rect.Y+1).Ch)
	}
	if app.ReadCell(right+2, rect.Y+1).Ch != shadowGlyph {
		t.Errorf("right band column %d at y=%d is %q, want shadow glyph", right+2, rect.Y+1, app.ReadCell(right+2, rect.Y+1).Ch)
	}
	// 1-row bottom band starting OffsetX (1) columns in from the popup left.
	if app.ReadCell(rect.X+1, bottom+1).Ch != shadowGlyph {
		t.Errorf("bottom band at (%d,%d) is %q, want shadow glyph", rect.X+1, bottom+1, app.ReadCell(rect.X+1, bottom+1).Ch)
	}
}

// TestSelectPopupDrawsNoShadowWhenShadowFalse is the headline test for the #231
// fix: with Shadow=false the opened popup composites NO drop shadow — the
// dropdown renders flat. Against the pre-fix code (unconditional DrawShadow)
// this fails because the band is always drawn.
func TestSelectPopupDrawsNoShadowWhenShadowFalse(t *testing.T) {
	s, app, _ := openSelectWith(t, false)

	if got := shadowGlyphCells(app, 60, 20); len(got) != 0 {
		t.Fatalf("Shadow=false popup still drew a shadow at %v; the dropdown must be flat", got)
	}
	if n := shadowCellsInColour(app, ActiveTheme().WindowShadow, 60, 20); n != 0 {
		t.Errorf("Shadow=false popup painted %d cells in WindowShadow colour; expected none", n)
	}
	if !s.IsOpen() {
		t.Fatalf("Shadow=false popup is not open; the field must not affect opening")
	}
}

// TestSelectPopupContentIndependentOfShadow verifies the gate wraps ONLY the
// shadow: the popup's box border, fill and option text render identically whether
// the shadow is on or off. A regression that gated Fill/DrawBox/the option loop
// by mistake would diverge here.
func TestSelectPopupContentIndependentOfShadow(t *testing.T) {
	_, appOn, rectOn := openSelectWith(t, true)
	_, appOff, rectOff := openSelectWith(t, false)

	if rectOn != rectOff {
		t.Fatalf("popup geometry must not depend on Shadow: on=%v off=%v", rectOn, rectOff)
	}
	rect := rectOn
	for y := rect.Y; y <= rect.Bottom(); y++ {
		for x := rect.X; x <= rect.Right(); x++ {
			if on, off := appOn.ReadCell(x, y), appOff.ReadCell(x, y); on != off {
				t.Errorf("popup cell (%d,%d) differs: Shadow=true %+v vs Shadow=false %+v", x, y, on, off)
			}
		}
	}
}

// TestSelectShadowOnlyAddsBandOutsidePopup is the directional form of the
// content-integrity check: turning the shadow on must change ONLY cells outside
// the popup rect, and each changed cell must be a shadow glyph. Nothing inside
// the rect may move.
func TestSelectShadowOnlyAddsBandOutsidePopup(t *testing.T) {
	_, appOn, rect := openSelectWith(t, true)
	_, appOff, _ := openSelectWith(t, false)

	const w, h = 60, 20
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			on, off := appOn.ReadCell(x, y), appOff.ReadCell(x, y)
			if on == off {
				continue
			}
			if rect.Contains(x, y) {
				t.Errorf("cell (%d,%d) inside popup rect changed with Shadow: on=%+v off=%+v", x, y, on, off)
				continue
			}
			if on.Ch != shadowGlyph {
				t.Errorf("differing outside-rect cell (%d,%d) on-buffer glyph=%q, want shadow %q", x, y, on.Ch, shadowGlyph)
			}
		}
	}
}

// TestSelectShadowToggledAtDrawTime pins that Shadow is read on every redraw,
// not snapshotted when the popup opens. gogent's refreshTheme re-applies the
// preference by writing the field; because the popup recomposes each frame, a
// live toggle takes effect without re-opening. An implementation that cached the
// flag at open() time would fail here.
func TestSelectShadowToggledAtDrawTime(t *testing.T) {
	desktop, s := setupSelect(60, 20, []string{"a", "b"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.open()
	desktop.Redraw()
	if len(shadowGlyphCells(desktop.App(), 60, 20)) == 0 {
		t.Fatalf("baseline Shadow=true popup drew no shadow")
	}

	// Flip the preference off and recompose: the band must vanish.
	s.Shadow = false
	desktop.Redraw()
	if got := shadowGlyphCells(desktop.App(), 60, 20); len(got) != 0 {
		t.Fatalf("after Shadow=false + redraw the shadow persists at %v; the flag must be read at draw time", got)
	}

	// Flip it back on and recompose: the band must return.
	s.Shadow = true
	desktop.Redraw()
	if len(shadowGlyphCells(desktop.App(), 60, 20)) == 0 {
		t.Fatalf("after Shadow=true + redraw the shadow did not return; the flag must be read at draw time")
	}
}

// TestSelectShadowSetBeforeOpenSuppressesOnOpen mirrors gogent's construction
// path: the wrapper sets Shadow from the preference before the user ever opens
// the dropdown, and the very first open must render flat.
func TestSelectShadowSetBeforeOpenSuppressesOnOpen(t *testing.T) {
	desktop, s := setupSelect(60, 20, []string{"a", "b"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.Shadow = false
	s.open()
	desktop.Redraw()
	if got := shadowGlyphCells(desktop.App(), 60, 20); len(got) != 0 {
		t.Fatalf("popup opened with Shadow=false should have no shadow, got %v", got)
	}
}

// TestSelectShadowReflectsFieldOnReopen verifies the field is honoured across a
// close/re-open cycle: opening, closing, changing the field and reopening
// renders according to the new value each time.
func TestSelectShadowReflectsFieldOnReopen(t *testing.T) {
	desktop, s := setupSelect(60, 20, []string{"a", "b"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)

	s.open()
	desktop.Redraw()
	if len(shadowGlyphCells(desktop.App(), 60, 20)) == 0 {
		t.Fatalf("reopen baseline Shadow=true should draw a shadow")
	}

	s.close()
	s.Shadow = false
	s.open()
	desktop.Redraw()
	if got := shadowGlyphCells(desktop.App(), 60, 20); len(got) != 0 {
		t.Fatalf("reopen with Shadow=false should draw no shadow, got %v", got)
	}

	s.close()
	s.Shadow = true
	s.open()
	desktop.Redraw()
	if len(shadowGlyphCells(desktop.App(), 60, 20)) == 0 {
		t.Fatalf("reopen with Shadow=true should draw a shadow")
	}
}

// TestSelectShadowFalseDoesNotBlockCommit ensures suppressing the shadow does
// not impair the popup's interaction: clicking an item still commits the
// selection, fires OnChange and closes the popup.
func TestSelectShadowFalseDoesNotBlockCommit(t *testing.T) {
	desktop, s := setupSelect(60, 20, []string{"a", "b", "c"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.Shadow = false
	s.open()
	changed := -1
	s.OnChange = func(i int) { changed = i }

	rect := s.popupRect()
	inner := Rect{X: rect.X + 1, Y: rect.Y + 1, W: rect.W - 2, H: rect.H - 2}
	s.popupClick(nil, tui.ClickEvent{X: inner.X, Y: inner.Y + 1, Button: tui.MouseLeft, Down: true})
	s.popupClick(nil, tui.ClickEvent{X: inner.X, Y: inner.Y + 1, Button: tui.MouseLeft, Down: false})

	if s.Selected != 1 {
		t.Fatalf("Shadow=false popup should still commit clicks; Selected=%d want 1", s.Selected)
	}
	if changed != 1 {
		t.Fatalf("OnChange should fire with index 1, got %d", changed)
	}
	if s.IsOpen() {
		t.Fatalf("popup should close after committing an item")
	}
	_ = desktop
}

// TestSelectShadowFalseOpenedViaKeyboard drives the keyboard open path (Space)
// rather than calling open() directly, confirming the gate is honoured however
// the popup is summoned.
func TestSelectShadowFalseOpenedViaKeyboard(t *testing.T) {
	desktop, s := setupSelect(60, 20, []string{"a", "b"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
	s.Shadow = false
	if handled := s.handleType(s.Component, tui.TypeEvent{Key: tui.KeyRune, Rune: ' '}); !handled {
		t.Fatalf("Space should open the select")
	}
	if !s.IsOpen() {
		t.Fatalf("popup should be open after Space")
	}
	desktop.Redraw()
	if got := shadowGlyphCells(desktop.App(), 60, 20); len(got) != 0 {
		t.Fatalf("keyboard-opened Shadow=false popup drew a shadow at %v", got)
	}
}

// TestSelectCollapsedDrawsNoShadowRegardlessOfField locks that the shadow is a
// property of the OPENED popup only: a collapsed Select composites no drop
// shadow whether Shadow is true or false. A future change that added a shadow to
// the collapsed control would break the "Disable shadows" symmetry.
func TestSelectCollapsedDrawsNoShadowRegardlessOfField(t *testing.T) {
	for _, shadow := range []bool{true, false} {
		desktop, s := setupSelect(60, 20, []string{"a", "b"}, Rect{X: 5, Y: 5, W: 14, H: 1}, 0)
		s.Shadow = shadow
		if s.IsOpen() {
			t.Fatalf("select should start collapsed")
		}
		desktop.Redraw()
		if got := shadowGlyphCells(desktop.App(), 60, 20); len(got) != 0 {
			t.Errorf("collapsed Select (Shadow=%v) drew a shadow at %v; only the open popup may shadow", shadow, got)
		}
	}
}

// TestSelectShadowFalseNearScreenEdge exercises the gate when the popup sits at
// the screen edge — the position where, with shadows on, the band is partly
// clipped. With Shadow=false nothing is drawn and the popup composites without
// panic, even though popupRect has to flip up and shrink to fit.
func TestSelectShadowFalseNearScreenEdge(t *testing.T) {
	opts := make([]string, 8)
	desktop, s := setupSelect(20, 10, opts, Rect{X: 6, Y: 8, W: 12, H: 1}, 0)
	s.Shadow = false
	s.open()
	if !s.IsOpen() {
		t.Fatalf("edge popup should open")
	}
	desktop.Redraw()
	// Reaching these assertions means no out-of-bounds panic during compose; the
	// popup must still render flat.
	if got := shadowGlyphCells(desktop.App(), 20, 10); len(got) != 0 {
		t.Fatalf("edge popup with Shadow=false drew a shadow at %v", got)
	}
}
