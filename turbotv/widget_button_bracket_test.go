package tv

import (
	"bytes"
	"strconv"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// Tests for the flush-bracket Button.draw change (gogent#259): the '[' and ']'
// are pinned to the face bounds and only the caption floats between them, so
// face.W controls the visible box. They live in package tv so they can drive
// the unexported hasFocus / mnemonicActive flags for the chevron and mnemonic
// paths. Shared helpers (newRootSurface) come from the rest of the package's
// test files.

// drawButtonInto draws label as a button (shadow off) into app at face. focused
// selects the ►/◄ chevron chrome; mnemonicActive underlines the hot char.
func drawButtonInto(t *testing.T, app *tui.App, surface Surface, label string, face Rect, focused, mnemonicActive bool) {
	t.Helper()
	b := NewButton(label, face, nil)
	b.Shadow = false
	if focused {
		b.Component.hasFocus = true
	}
	if mnemonicActive {
		b.Component.mnemonicActive = true
	}
	b.draw(b.Component, surface)
}

// --- the bug repro: equal bounds, different labels => aligned brackets --------

// Two buttons given the SAME face bounds but DIFFERENT label lengths must paint
// their '[' at the same X and their ']' at the same X: face.W controls the box,
// not the caption width. (Pre-fix, the whole "[ caption ]" group was centred, so
// the right edges stair-stepped — gogent's Queue/Interject/Stop.)
func TestButtonFlushBracketsEqualBoundsDifferentLabels(t *testing.T) {
	app := tui.NewWithSize(24, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	face := Rect{X: 3, Y: 0, W: 14, H: 1}
	// "OK" (caption width 2) on row 0, "Interject" (caption width 9) on row 1 —
	// same X and W, deliberately different caption widths.
	drawButtonInto(t, app, surface, "OK", Rect{X: face.X, Y: 0, W: face.W, H: 1}, false, false)
	drawButtonInto(t, app, surface, "Interject", Rect{X: face.X, Y: 1, W: face.W, H: 1}, false, false)

	// Left bracket '[' sits at face.X for both, regardless of label length.
	if got := app.ReadCell(face.X, 0).Ch; got != '[' {
		t.Fatalf("OK: '[' at x=%d = %q, want '['", face.X, got)
	}
	if got := app.ReadCell(face.X, 1).Ch; got != '[' {
		t.Fatalf("Interject: '[' at x=%d = %q, want '['", face.X, got)
	}

	// Right bracket ']' sits on the last face column for both — the stair-step bug.
	lastX := face.X + face.W - 1
	if got := app.ReadCell(lastX, 0).Ch; got != ']' {
		t.Fatalf("OK: ']' at x=%d = %q, want ']'", lastX, got)
	}
	if got := app.ReadCell(lastX, 1).Ch; got != ']' {
		t.Fatalf("Interject: ']' at x=%d = %q, want ']'", lastX, got)
	}

	// The captions themselves centre at different X (they fill different fractions
	// of the same gap), proving the buttons really are different labels — only the
	// brackets align.
	// "OK": avail=10, captionW=2 => captionStart = 3+2+(10-2)/2 = 9.
	if got := app.ReadCell(9, 0).Ch; got != 'O' {
		t.Fatalf("OK caption 'O' at x=9 = %q", got)
	}
	// "Interject": avail=10, captionW=9 => captionStart = 3+2+(10-9)/2 = 5.
	if got := app.ReadCell(5, 1).Ch; got != 'I' {
		t.Fatalf("Interject caption 'I' at x=5 = %q", got)
	}
}

// --- sized-to-label buttons render identically to before --------------------

// A button whose face.W equals StringWidth(label)+4 (the natural outline width)
// renders with the brackets already at the edges and the caption filling the gap
// exactly — identical to the pre-fix centred layout. This is the "every button
// sized via buttonLabelWidth is unchanged" guarantee.
func TestButtonSizedToLabelIsUnchanged(t *testing.T) {
	app := tui.NewWithSize(20, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	label := "Hello" // clean width 5 -> outline width 9
	cleanW := tui.StringWidth(label)
	face := Rect{X: 0, Y: 0, W: cleanW + 4, H: 1} // face.W == displayW exactly
	drawButtonInto(t, app, surface, label, face, false, false)

	// '[' flush left, ']' flush right (last column), caption right after '['.
	if got := app.ReadCell(0, 0).Ch; got != '[' {
		t.Fatalf("'[' at x=0 = %q", got)
	}
	if got := app.ReadCell(face.W-1, 0).Ch; got != ']' {
		t.Fatalf("']' at x=%d = %q", face.W-1, got)
	}
	// caption fills avail exactly (avail==captionW) so it starts at face.X+leftW.
	for i, want := range label {
		if got := app.ReadCell(2+i, 0).Ch; got != want {
			t.Fatalf("caption rune %d at x=%d = %q, want %q", i, 2+i, got, want)
		}
	}
}

// --- caption X is invariant under the change (the safety property) -----------

// For any face at least as wide as the natural outline, the caption keeps the
// exact X the pre-fix whole-group centring gave it: old start+leftW equals the
// new face.X+leftW+(avail-captionW)/2. Only the brackets move outward. This is
// what makes the change safe for every existing button.
func TestButtonCaptionXMatchesPreFixCentering(t *testing.T) {
	cases := []struct {
		label string
		faceW int
	}{
		{"OK", 6}, {"OK", 7}, {"OK", 8}, {"OK", 10}, {"OK", 14},
		{"Stop", 8}, {"Stop", 10}, {"Stop", 14},
		{"Cancel", 10}, {"Cancel", 14},
		{"Running", 11}, {"Running", 14},
	}
	for _, tc := range cases {
		t.Run(tc.label+"_w"+strconv.Itoa(tc.faceW), func(t *testing.T) {
			clean, _ := parseMnemonic(tc.label)
			app := tui.NewWithSize(tc.faceW+2, 1, &bytes.Buffer{})
			surface := newRootSurface(app)
			face := Rect{X: 0, Y: 0, W: tc.faceW, H: 1}
			drawButtonInto(t, app, surface, tc.label, face, false, false)

			const leftW, rightW = 2, 2
			avail := tc.faceW - leftW - rightW
			if avail < 0 {
				avail = 0
			}
			captionW := tui.StringWidth(clean)
			if captionW > avail {
				captionW = avail
			}
			displayW := leftW + captionW + rightW
			if tc.faceW < displayW {
				t.Fatalf("test precondition: face.W %d < displayW %d", tc.faceW, displayW)
			}
			// Pre-fix caption X: start + leftW, where start = face.X + (face.W-displayW)/2.
			wantCaptionX := (tc.faceW-displayW)/2 + leftW

			got := app.ReadCell(wantCaptionX, 0).Ch
			wantRune := []rune(clean)[0]
			if got != wantRune {
				t.Fatalf("caption first rune at x=%d = %q, want %q (pre-fix centering)", wantCaptionX, got, wantRune)
			}
			// Brackets are now flush to the face edges regardless.
			if app.ReadCell(0, 0).Ch != '[' {
				t.Errorf("'[' not flush at x=0")
			}
			if app.ReadCell(tc.faceW-1, 0).Ch != ']' {
				t.Errorf("']' not flush at x=%d", tc.faceW-1)
			}
		})
	}
}

// --- focused chevrons are flush too ----------------------------------------

// The focused ►/◄ chrome also pins flush to the face, so equal-bounds focused
// buttons align regardless of label length.
func TestButtonFocusedChevronsFlushToFace(t *testing.T) {
	app := tui.NewWithSize(24, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	drawButtonInto(t, app, surface, "OK", Rect{X: 3, Y: 0, W: 14, H: 1}, true, false)
	drawButtonInto(t, app, surface, "Stop", Rect{X: 3, Y: 1, W: 14, H: 1}, true, false)

	// '►' at face.X on both rows; '◄' on the last face column on both rows.
	if got := app.ReadCell(3, 0).Ch; got != '►' {
		t.Fatalf("OK focused: '►' at x=3 = %q", got)
	}
	if got := app.ReadCell(3, 1).Ch; got != '►' {
		t.Fatalf("Stop focused: '►' at x=3 = %q", got)
	}
	lastX := 3 + 14 - 1
	if got := app.ReadCell(lastX, 0).Ch; got != '◄' {
		t.Fatalf("OK focused: '◄' at x=%d = %q", lastX, got)
	}
	if got := app.ReadCell(lastX, 1).Ch; got != '◄' {
		t.Fatalf("Stop focused: '◄' at x=%d = %q", lastX, got)
	}
	// Captions centre within the chevron gap (avail = 14-1-1 = 12): "OK" at
	// 3+1+(12-2)/2 = 9, "Stop" at 3+1+(12-4)/2 = 8.
	if got := app.ReadCell(9, 0).Ch; got != 'O' {
		t.Fatalf("OK caption 'O' at x=9 = %q", got)
	}
	if got := app.ReadCell(8, 1).Ch; got != 'S' {
		t.Fatalf("Stop caption 'S' at x=8 = %q", got)
	}
}

// --- mnemonic label: brackets flush + hot char underlined -------------------

// A mnemonic label still centres its caption between flush brackets and
// underlines the hot char (the existing mnemonic path is preserved).
func TestButtonMnemonicLabelBracketsFlushAndUnderlined(t *testing.T) {
	app := tui.NewWithSize(20, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	face := Rect{X: 0, Y: 0, W: 14, H: 1}
	drawButtonInto(t, app, surface, "&Queue", face, false, true) // clean "Queue", hot 'Q'

	// Brackets flush: '[' at 0, ']' at 13.
	if got := app.ReadCell(0, 0).Ch; got != '[' {
		t.Fatalf("'[' at x=0 = %q", got)
	}
	if got := app.ReadCell(13, 0).Ch; got != ']' {
		t.Fatalf("']' at x=13 = %q", got)
	}
	// avail=10, captionW=5 => captionStart = 0+2+(10-5)/2 = 4; 'Q' underlined there.
	q := app.ReadCell(4, 0)
	if q.Ch != 'Q' {
		t.Fatalf("'Q' at x=4 = %q", q.Ch)
	}
	if !q.Underline {
		t.Fatalf("mnemonic 'Q' not underlined: %+v", q)
	}
}

// --- pressed button offsets the face, brackets flush to the offset face ------

// A pressed button shifts its face down-right by one; the brackets must be flush
// to that offset face, not the original bounds.
func TestButtonPressedOffsetsFaceBracketsFlush(t *testing.T) {
	app := tui.NewWithSize(20, 3, &bytes.Buffer{})
	surface := newRootSurface(app)
	abs := Rect{X: 2, Y: 0, W: 12, H: 2}
	b := NewButton("OK", abs, nil)
	b.Shadow = false
	b.Pressed = true
	b.draw(b.Component, surface)

	// Pressed face = {abs.X+1, abs.Y+1, abs.W, abs.H}: '[' at x=3 on row 1.
	if got := app.ReadCell(3, 1).Ch; got != '[' {
		t.Fatalf("pressed '[' at (3,1) = %q", got)
	}
	// The un-offset corner stays blank (the whole face shifted).
	if got := app.ReadCell(2, 0).Ch; got != ' ' {
		t.Fatalf("un-offset corner (2,0) = %q, want blank (face shifted)", got)
	}
	// ']' on the last offset-face column: x = 3 + 12 - 1 = 14, row 1.
	if got := app.ReadCell(14, 1).Ch; got != ']' {
		t.Fatalf("pressed ']' at (14,1) = %q", got)
	}
}

// --- narrow faces: no panic, no bleed --------------------------------------

// A face narrower than the caption (or even narrower than the brackets) must not
// panic and must not bleed outside the face: the right bracket clamps to face.X
// and the face-clipped surface stops any overflow. Place the face at X=2 so a
// negative/leftward bracket offset would be visible as ink in columns 0–1.
func TestButtonNarrowFaceNoPanicNoBleed(t *testing.T) {
	label := "Stop" // clean width 4, outline width 8
	for _, w := range []int{0, 1, 2, 3, 4, 5, 7} {
		t.Run("w", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("face.W=%d panicked: %v", w, r)
				}
			}()
			const faceX = 2
			appW := 16
			app := tui.NewWithSize(appW, 1, &bytes.Buffer{})
			surface := newRootSurface(app)
			face := Rect{X: faceX, Y: 0, W: w, H: 1}
			drawButtonInto(t, app, surface, label, face, false, false)

			// No ink may land outside [faceX, faceX+w) on the button's row — not
			// left of the face (the clamp) and not right of it (the clip).
			lo := faceX + w
			if lo > appW {
				lo = appW
			}
			for x := 0; x < appW; x++ {
				if x >= faceX && x < faceX+w {
					continue // inside the face: anything goes (degraded rendering ok)
				}
				if ch := app.ReadCell(x, 0).Ch; ch != ' ' && ch != 0 {
					t.Errorf("face.W=%d: ink leaked to column %d = %q (outside face)", w, x, ch)
				}
			}
		})
	}
}

// --- bracket positions are exactly face.X and face.X+face.W-1 ---------------

// Direct formula check across focused/unfocused chrome: the left bracket glyph
// is always at face.X and the right bracket glyph at the last face column.
func TestButtonBracketPositionsAtFaceEdges(t *testing.T) {
	cases := []struct {
		name    string
		focused bool
		left    rune
		right   rune
	}{
		{"unfocused [ ]", false, '[', ']'},
		{"focused ► ◄", true, '►', '◄'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := tui.NewWithSize(20, 1, &bytes.Buffer{})
			surface := newRootSurface(app)
			face := Rect{X: 4, Y: 0, W: 11, H: 1}
			drawButtonInto(t, app, surface, "Hi", face, tc.focused, false)
			if got := app.ReadCell(face.X, 0).Ch; got != tc.left {
				t.Errorf("left bracket at x=%d = %q, want %q", face.X, got, tc.left)
			}
			if got := app.ReadCell(face.X+face.W-1, 0).Ch; got != tc.right {
				t.Errorf("right bracket at x=%d = %q, want %q", face.X+face.W-1, got, tc.right)
			}
		})
	}
}
