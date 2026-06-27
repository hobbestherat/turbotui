package tv

import (
	"bytes"
	"strconv"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the taller (2-row) button support (gogent#529): Button.draw renders
// at any bounds.H >= 1 — a solid "[ … ]" block filling every row, with the caption
// and the ►…◄ focus chevrons on the single vertically-centred row (round down on
// even heights), the drop shadow hugging the new bottom edge, and the whole face
// click-activatable. H == 1 must render byte-for-byte identically to before.
//
// They live in package tv so they can set the unexported hasFocus / mnemonicActive
// flags and reuse newRootSurface / drawButtonInto from the rest of the package's
// test files. Each test names the design gate it backs.

// ---------------------------------------------------------------------------
// Gate 1 (goal match): H == 1 renders byte-for-byte identically to today.
// ---------------------------------------------------------------------------

// TestTallButtonH1UnfocusedIsUnchanged renders a 1-row unfocused button and pins
// the exact cells: the single-row draw loop must reduce to the pre-change output.
func TestTallButtonH1UnfocusedIsUnchanged(t *testing.T) {
	app := tui.NewWithSize(10, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: 10, H: 1}, false, false)

	want := []rune{'[', ' ', ' ', ' ', 'O', 'K', ' ', ' ', ' ', ']'}
	for x, w := range want {
		if got := app.ReadCell(x, 0).Ch; got != w {
			t.Fatalf("H=1 unfocused cell (%d,0) = %q, want %q", x, got, w)
		}
	}
}

// TestTallButtonH1FocusedIsUnchanged pins the 1-row focused output: chevrons flush
// to the face and the caption centred between them.
func TestTallButtonH1FocusedIsUnchanged(t *testing.T) {
	app := tui.NewWithSize(10, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: 10, H: 1}, true, false)

	want := []rune{'►', ' ', ' ', ' ', 'O', 'K', ' ', ' ', ' ', '◄'}
	for x, w := range want {
		if got := app.ReadCell(x, 0).Ch; got != w {
			t.Fatalf("H=1 focused cell (%d,0) = %q, want %q", x, got, w)
		}
	}
}

// ---------------------------------------------------------------------------
// Gate 1 (goal match): caption + focus chrome land on the vertically-centred row.
// ---------------------------------------------------------------------------

// TestTallButtonH2CaptionOnCentreRow draws a 2-row button and asserts the caption
// sits on the lower row (centerY = face.Y + face.H/2 = face.Y+1), never on row 0,
// while the brackets span both rows.
func TestTallButtonH2CaptionOnCentreRow(t *testing.T) {
	app := tui.NewWithSize(10, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: 10, H: 2}, false, false)

	// Caption centred on the lower row (centerY = 0 + 2/2 = 1): avail = 10-2-2 = 6,
	// captionW = 2, captionStart = 0 + 2 + (6-2)/2 = 4.
	if got := app.ReadCell(4, 1).Ch; got != 'O' {
		t.Fatalf("caption 'O' should be on the centred row at (4,1), got %q", got)
	}
	if got := app.ReadCell(5, 1).Ch; got != 'K' {
		t.Fatalf("caption 'K' should be on the centred row at (5,1), got %q", got)
	}
	// Row 0 is a bracket-only row: no caption glyph there.
	if got := app.ReadCell(4, 0).Ch; got == 'O' {
		t.Fatalf("caption must not appear on the non-centred row 0, got 'O' at (4,0)")
	}
}

// TestTallButtonH3CaptionOnTrueMiddleRow draws a 3-row button and asserts the
// caption lands on the single true middle row (centerY = 0 + 3/2 = 1), with the
// other two rows carrying only brackets.
func TestTallButtonH3CaptionOnTrueMiddleRow(t *testing.T) {
	app := tui.NewWithSize(10, 3, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: 10, H: 3}, false, false)

	if got := app.ReadCell(4, 1).Ch; got != 'O' {
		t.Fatalf("H=3 caption 'O' should be on the middle row (4,1), got %q", got)
	}
	for _, y := range []int{0, 2} {
		if got := app.ReadCell(4, y).Ch; got == 'O' {
			t.Fatalf("H=3 caption must not appear on bracket row %d", y)
		}
	}
}

// ---------------------------------------------------------------------------
// Gate 1 (goal match) + Gate 2 (usability): full-height box — fill + brackets.
// ---------------------------------------------------------------------------

// TestTallButtonFaceFillsFullHeight asserts every row of an H=2 face is painted
// with the button background and bold style (a solid block), not just the caption
// row.
func TestTallButtonFaceFillsFullHeight(t *testing.T) {
	app := tui.NewWithSize(10, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	b := NewButton("OK", Rect{X: 0, Y: 0, W: 10, H: 2}, nil)
	b.Shadow = false
	b.draw(b.Component, surface)

	// Interior cells of BOTH rows (away from brackets/caption) carry the button bg
	// and bold — proof the fill ran on every row.
	for _, pos := range [][2]int{{2, 0}, {7, 0}, {2, 1}, {7, 1}} {
		c := app.ReadCell(pos[0], pos[1])
		if c.BG != b.BG {
			t.Errorf("cell (%d,%d) bg = %v, want button bg %v (fill did not cover this row)", pos[0], pos[1], c.BG, b.BG)
		}
		if !c.Bold {
			t.Errorf("cell (%d,%d) not bold (fill style lost on a tall face)", pos[0], pos[1])
		}
	}
}

// TestTallButtonBracketsSpanFullHeight asserts the '[' and ']' run the full height:
// at face.X on every row and on the last face column on every row.
func TestTallButtonBracketsSpanFullHeight(t *testing.T) {
	const w = 10
	for _, h := range []int{2, 3, 4, 5} {
		t.Run("h"+strconv.Itoa(h), func(t *testing.T) {
			app := tui.NewWithSize(w, h, &bytes.Buffer{})
			surface := newRootSurface(app)
			drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: w, H: h}, false, false)
			for y := 0; y < h; y++ {
				if got := app.ReadCell(0, y).Ch; got != '[' {
					t.Errorf("H=%d: '[' missing on row %d at (0,%d), got %q", h, y, y, got)
				}
				if got := app.ReadCell(w-1, y).Ch; got != ']' {
					t.Errorf("H=%d: ']' missing on row %d at (%d,%d), got %q", h, y, w-1, y, got)
				}
			}
		})
	}
}

// TestTallButtonFocusedChevronsOnlyOnCentreRow asserts a focused tall button keeps
// plain '[' / ']' on the non-centred rows and the ► / ◄ chevrons only on the
// centred caption row (design decision #1), with the box edges vertically aligned.
func TestTallButtonFocusedChevronsOnlyOnCentreRow(t *testing.T) {
	app := tui.NewWithSize(10, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: 10, H: 2}, true, false)

	// centerY = 1: chevrons + caption on row 1.
	if got := app.ReadCell(0, 1).Ch; got != '►' {
		t.Fatalf("focused '►' should be on centred row (0,1), got %q", got)
	}
	if got := app.ReadCell(9, 1).Ch; got != '◄' {
		t.Fatalf("focused '◄' should be on centred row (9,1), got %q", got)
	}
	if got := app.ReadCell(4, 1).Ch; got != 'O' {
		t.Fatalf("caption 'O' on centred row (4,1) = %q", got)
	}
	// Row 0 keeps the plain box brackets, NOT chevrons.
	if got := app.ReadCell(0, 0).Ch; got != '[' {
		t.Fatalf("non-centred row must keep '[' at (0,0), got %q (chevron leaked)", got)
	}
	if got := app.ReadCell(9, 0).Ch; got != ']' {
		t.Fatalf("non-centred row must keep ']' at (9,0), got %q (chevron leaked)", got)
	}
}

// TestTallButtonEdgesVerticallyAligned checks the left and right box edges line up
// across all rows whether the row carries a bracket or a chevron (so a focused tall
// button still reads as a clean rectangle).
func TestTallButtonEdgesVerticallyAligned(t *testing.T) {
	const w = 12
	for _, focused := range []bool{false, true} {
		t.Run("focused="+strconv.FormatBool(focused), func(t *testing.T) {
			app := tui.NewWithSize(w, 3, &bytes.Buffer{})
			surface := newRootSurface(app)
			drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: w, H: 3}, focused, false)
			// Column 0 and column w-1 are non-blank on every row.
			for y := 0; y < 3; y++ {
				if got := app.ReadCell(0, y).Ch; got == ' ' || got == 0 {
					t.Errorf("left edge (0,%d) is blank — edges not aligned", y)
				}
				if got := app.ReadCell(w-1, y).Ch; got == ' ' || got == 0 {
					t.Errorf("right edge (%d,%d) is blank — edges not aligned", w-1, y)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Gate 2 (usability): ellipsis + mnemonic still behave on a tall face.
// ---------------------------------------------------------------------------

// TestTallButtonLongCaptionEllipsisedOnCentreRow draws a tall button whose caption
// is far wider than the face and asserts it is ellipsised on the centred row, with
// no caption glyph bleeding past the face's right edge on either row.
func TestTallButtonLongCaptionEllipsisedOnCentreRow(t *testing.T) {
	app := tui.NewWithSize(16, 2, &bytes.Buffer{})
	surface := newRootSurface(app) // parent clip: full width (no button clipping)
	drawButtonInto(t, app, surface, "ABCDEFGHIJ", Rect{X: 0, Y: 0, W: 6, H: 2}, false, false)

	// avail = 6-2-2 = 2, captionW = 2, captionStart = 0 + 2 + 0 = 2 -> "A…" at cols 2,3.
	if got := app.ReadCell(3, 1).Ch; got != '…' {
		t.Fatalf("expected ellipsis on centred row at (3,1), got %q", got)
	}
	if got := app.ReadCell(5, 1).Ch; got != ']' {
		t.Fatalf("closing bracket must sit on the last face column (5,1), got %q", got)
	}
	// No caption glyph may bleed past the face's right edge (cols 6..15), any row.
	for x := 6; x <= 15; x++ {
		for _, y := range []int{0, 1} {
			if ch := app.ReadCell(x, y).Ch; ch >= 'A' && ch <= 'J' {
				t.Fatalf("caption glyph %q bled past the face to (%d,%d)", ch, x, y)
			}
		}
	}
	// The ellipsis/caption is on the centred row only; the bracket row has none.
	for _, x := range []int{2, 3} {
		if ch := app.ReadCell(x, 0).Ch; ch == 'A' || ch == '…' {
			t.Fatalf("caption/ellipsis must not appear on the bracket row 0 at (%d,0): %q", x, ch)
		}
	}
}

// TestTallButtonMnemonicUnderlinedOnCentreRow asserts the mnemonic hot character is
// underlined on the centred row only; the bracket rows carry no underline.
func TestTallButtonMnemonicUnderlinedOnCentreRow(t *testing.T) {
	app := tui.NewWithSize(12, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "&Queue", Rect{X: 0, Y: 0, W: 12, H: 2}, false, true)

	// avail = 12-2-2 = 8, captionW = 5, captionStart = 0 + 2 + (8-5)/2 = 3 -> 'Q' at 3.
	q := app.ReadCell(3, 1)
	if q.Ch != 'Q' {
		t.Fatalf("mnemonic 'Q' on centred row (3,1) = %q", q.Ch)
	}
	if !q.Underline {
		t.Fatalf("mnemonic 'Q' not underlined on the centred row: %+v", q)
	}
	// The bracket row must not carry an underlined hot char.
	if c := app.ReadCell(3, 0); c.Underline {
		t.Fatalf("bracket row 0 must not carry an underline at (3,0): %+v", c)
	}
	// Brackets still frame both rows.
	if got := app.ReadCell(0, 0).Ch; got != '[' {
		t.Fatalf("'[' at (0,0) = %q", got)
	}
	if got := app.ReadCell(11, 1).Ch; got != ']' {
		t.Fatalf("']' at (11,1) = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Acceptance: drop shadow hugs the new bottom edge at any height.
// ---------------------------------------------------------------------------

// TestTallButtonShadowHugsNewBottom draws a shadowed H=2 button and asserts the
// shadow sits just below the new bottom edge (and to the right of the new right
// edge), with no shadow glyph inside the face or above the top.
func TestTallButtonShadowHugsNewBottom(t *testing.T) {
	abs := Rect{X: 2, Y: 0, W: 8, H: 2}
	app := tui.NewWithSize(16, 6, &bytes.Buffer{})
	surface := newRootSurface(app)
	b := NewButton("OK", abs, nil) // Shadow defaults to true
	b.draw(b.Component, surface)

	// Bottom band one row below the new bottom edge: abs.Bottom()+1 = 2.
	if got := app.ReadCell(abs.X+1, abs.Bottom()+1).Ch; got != shadowGlyph {
		t.Fatalf("shadow glyph expected just below bottom at (%d,%d), got %q",
			abs.X+1, abs.Bottom()+1, got)
	}
	// Right band one column past the new right edge, on the lower rows.
	if got := app.ReadCell(abs.Right()+1, abs.Y+1).Ch; got != shadowGlyph {
		t.Fatalf("shadow glyph expected just right of the edge at (%d,%d), got %q",
			abs.Right()+1, abs.Y+1, got)
	}
	// No shadow glyph inside the face (it should be button content), nor above the
	// top row on the right band (the band starts OffsetY rows down).
	for _, pos := range [][2]int{{5, 0}, {5, 1}, {abs.Right() + 1, 0}} {
		if got := app.ReadCell(pos[0], pos[1]).Ch; got == shadowGlyph {
			t.Errorf("shadow glyph leaked to (%d,%d) — should be outside/below only", pos[0], pos[1])
		}
	}
}

// ---------------------------------------------------------------------------
// Acceptance: the full H-row face is click-activatable.
// ---------------------------------------------------------------------------

// TestTallButtonClickEitherRowActivates drives a down+up click on row 0 and on
// row 1 of an H=2 button and asserts both fire OnPress, while a click one row
// below the button does not.
func TestTallButtonClickEitherRowActivates(t *testing.T) {
	for _, row := range []int{0, 1} {
		t.Run("row"+strconv.Itoa(row), func(t *testing.T) {
			count := 0
			b := NewButton("OK", Rect{X: 2, Y: 0, W: 8, H: 2}, func() { count++ })
			c := b.Component
			x := 5
			_ = b.handleClick(c, tui.ClickEvent{X: x, Y: row, Button: tui.MouseLeft, Down: true})
			_ = b.handleClick(c, tui.ClickEvent{X: x, Y: row, Button: tui.MouseLeft, Down: false})
			if count != 1 {
				t.Fatalf("click on row %d of an H=2 button should activate (count=%d)", row, count)
			}
		})
	}
}

// TestTallButtonClickBelowDoesNotActivate asserts a click one row past the bottom
// edge is outside the face and does not activate the button.
func TestTallButtonClickBelowDoesNotActivate(t *testing.T) {
	count := 0
	b := NewButton("OK", Rect{X: 2, Y: 0, W: 8, H: 2}, func() { count++ })
	c := b.Component
	_ = b.handleClick(c, tui.ClickEvent{X: 5, Y: 2, Button: tui.MouseLeft, Down: true}) // row 2 is past H=2
	_ = b.handleClick(c, tui.ClickEvent{X: 5, Y: 2, Button: tui.MouseLeft, Down: false})
	if count != 0 {
		t.Fatalf("click below an H=2 button must not activate (count=%d)", count)
	}
}

// ---------------------------------------------------------------------------
// Acceptance: pressed tall button still offsets the face and fills every row.
// ---------------------------------------------------------------------------

// TestTallButtonPressedFillsOffsetFace draws a pressed H=2 button and asserts the
// full-height brackets and caption now sit on the down-right-offset face, with the
// un-offset corner blank. Extends the existing pressed-bracket test with the
// caption-on-centred-row and full-height properties.
func TestTallButtonPressedFillsOffsetFace(t *testing.T) {
	app := tui.NewWithSize(20, 4, &bytes.Buffer{})
	surface := newRootSurface(app)
	abs := Rect{X: 2, Y: 0, W: 12, H: 2}
	b := NewButton("OK", abs, nil)
	b.Shadow = false
	b.Pressed = true
	b.draw(b.Component, surface)

	// Pressed face = {abs.X+1, abs.Y+1, abs.W, abs.H} = {3,1,12,2}; centerY = 1+1 = 2.
	// Brackets span both pressed rows; the un-offset corner stays blank.
	if got := app.ReadCell(2, 0).Ch; got != ' ' {
		t.Fatalf("un-offset corner (2,0) = %q, want blank (face shifted)", got)
	}
	for _, y := range []int{1, 2} {
		if got := app.ReadCell(3, y).Ch; got != '[' {
			t.Errorf("pressed '[' missing on offset-face row %d at (3,%d), got %q", y, y, got)
		}
		if got := app.ReadCell(14, y).Ch; got != ']' {
			t.Errorf("pressed ']' missing on offset-face row %d at (14,%d), got %q", y, y, got)
		}
	}
	// Caption on the pressed centred row (centerY = 2): captionStart = 3+2+(8-2)/2 = 8.
	if got := app.ReadCell(8, 2).Ch; got != 'O' {
		t.Fatalf("pressed caption 'O' should be on centred row (8,2), got %q", got)
	}
	if got := app.ReadCell(8, 1).Ch; got == 'O' {
		t.Fatalf("pressed caption must not appear on the non-centred offset row 1")
	}
}

// ---------------------------------------------------------------------------
// Gate 1 (goal match): NewButtonRow propagates per-button height.
// ---------------------------------------------------------------------------

// TestNewButtonRowPropagatesTallButton asserts a button with H=2 yields a box of
// H=2 and every child laid out at H=2 (the HBox is AlignStretch).
func TestNewButtonRowPropagatesTallButton(t *testing.T) {
	tall := NewButton("OK", Rect{H: 2}, nil)
	row := NewButtonRow(2, 40, AlignStart, DefaultButtonGap, tall)
	if row.Component.Bounds.H != 2 {
		t.Fatalf("box H = %d, want 2 (should track the tallest button)", row.Component.Bounds.H)
	}
	row.Component.LayoutFn(row.Component)
	if tall.Component.Bounds.H != 2 {
		t.Fatalf("child H after layout = %d, want 2 (stretch should fill the row height)", tall.Component.Bounds.H)
	}
}

// TestNewButtonRowDefaultsToOneRow asserts buttons with a zero Rect still produce a
// 1-row box and 1-row children — the zero-regression default (decision #3).
func TestNewButtonRowDefaultsToOneRow(t *testing.T) {
	a := NewButton("OK", Rect{}, nil)
	b := NewButton("No", Rect{}, nil)
	row := NewButtonRow(2, 40, AlignCenter, DefaultButtonGap, a, b)
	if row.Component.Bounds.H != 1 {
		t.Fatalf("default box H = %d, want 1", row.Component.Bounds.H)
	}
	row.Component.LayoutFn(row.Component)
	for i, btn := range []*Button{a, b} {
		if btn.Component.Bounds.H != 1 {
			t.Fatalf("default child %d H = %d, want 1", i, btn.Component.Bounds.H)
		}
	}
}

// TestNewButtonRowMixedHeightsTallestWins asserts a row mixing H=2 and H=1 buttons
// normalises every child to the tallest (2) — the uniform-footer semantics the
// design commits to.
func TestNewButtonRowMixedHeightsTallestWins(t *testing.T) {
	tall := NewButton("OK", Rect{H: 2}, nil)
	short := NewButton("No", Rect{H: 1}, nil)
	row := NewButtonRow(2, 40, AlignStart, DefaultButtonGap, tall, short)
	if row.Component.Bounds.H != 2 {
		t.Fatalf("mixed row box H = %d, want 2 (tallest)", row.Component.Bounds.H)
	}
	row.Component.LayoutFn(row.Component)
	for _, btn := range []*Button{tall, short} {
		if btn.Component.Bounds.H != 2 {
			t.Fatalf("mixed-row child H = %d, want 2 (stretched to tallest): %+v", btn.Component.Bounds.H, btn.Component.Bounds)
		}
	}
}

// ---------------------------------------------------------------------------
// Gate 4 (holistic): ButtonHeight helper — height purely from bounds, default 1.
// ---------------------------------------------------------------------------

// TestButtonHeight pins the helper that documents "height comes from bounds; a
// caller opts into taller by setting a taller Rect, and 0/negative collapse to 1".
func TestButtonHeight(t *testing.T) {
	cases := []struct {
		bounds Rect
		want   int
	}{
		{Rect{}, 1},
		{Rect{H: 0}, 1},
		{Rect{H: -3}, 1},
		{Rect{H: 1}, 1},
		{Rect{H: 2}, 2},
		{Rect{W: 10, H: 5}, 5},
	}
	for _, tc := range cases {
		if got := ButtonHeight(tc.bounds); got != tc.want {
			t.Errorf("ButtonHeight(%+v) = %d, want %d", tc.bounds, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Gate 3 (no regressions): narrow / degenerate tall faces never panic or bleed.
// ---------------------------------------------------------------------------

// TestTallButtonNarrowFaceNoPanicNoBleed drives several sub-button-width faces at
// H=2 through draw and asserts no panic and no ink outside the face on either row.
// It is the tall analogue of TestButtonNarrowFaceNoPanicNoBleed.
func TestTallButtonNarrowFaceNoPanicNoBleed(t *testing.T) {
	label := "Stop" // clean width 4, outline width 8
	const faceX = 2
	const appW = 16
	for _, w := range []int{0, 1, 2, 3, 4, 5, 7} {
		t.Run("w"+strconv.Itoa(w), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("face.W=%d H=2 panicked: %v", w, r)
				}
			}()
			app := tui.NewWithSize(appW, 2, &bytes.Buffer{})
			surface := newRootSurface(app)
			face := Rect{X: faceX, Y: 0, W: w, H: 2}
			drawButtonInto(t, app, surface, label, face, false, false)

			// No ink may land outside [faceX, faceX+w) on EITHER row.
			for y := 0; y < 2; y++ {
				for x := 0; x < appW; x++ {
					if x >= faceX && x < faceX+w {
						continue
					}
					if ch := app.ReadCell(x, y).Ch; ch != ' ' && ch != 0 {
						t.Errorf("face.W=%d: ink leaked to (%d,%d) = %q (outside face)", w, x, y, ch)
					}
				}
			}
		})
	}
}

// TestTallButtonZeroHeightNoPanic drives a degenerate H=0 face (and an H=1 control)
// to ensure the height loop never panics on an out-of-contract bound.
func TestTallButtonZeroHeightNoPanic(t *testing.T) {
	for _, h := range []int{0, 1, 2} {
		t.Run("h"+strconv.Itoa(h), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("H=%d panicked: %v", h, r)
				}
			}()
			app := tui.NewWithSize(10, 3, &bytes.Buffer{})
			surface := newRootSurface(app)
			drawButtonInto(t, app, surface, "OK", Rect{X: 0, Y: 0, W: 8, H: h}, false, false)
		})
	}
}
