package tui

import (
	"bytes"
	"strings"
	"testing"
)

// These tests pin the Italic cell attribute's SGR emission. SGR 3 enables italic
// and SGR 23 disables it; the style emitter (appendStyle) must emit 3 alongside
// the existing Bold (1/22) and Underline (4/24) codes, in both the full-reset
// branch (cur invalid) and the differential branch (cur valid). Default colours
// are used throughout so the exact byte sequence is independent of the active
// colour level (ColorDefault renders as 39/49 at every level).

// full-reset branch: a Cell with Italic=true emits ";3" before the terminator.
func TestAppendStyleItalicFullResetEmitsSGR3(t *testing.T) {
	var buf []byte
	got := string(appendStyle(buf, styleState{}, Cell{Italic: true}))
	want := "\x1b[0;39;49;3m"
	if got != want {
		t.Fatalf("full-reset italic = %q, want %q", got, want)
	}
}

// full-reset must NOT emit any attribute code when Italic (and Bold/Underline)
// are all false — just the reset + default colours.
func TestAppendStyleFullResetNoAttributes(t *testing.T) {
	var buf []byte
	got := string(appendStyle(buf, styleState{}, Cell{}))
	if got != "\x1b[0;39;49m" {
		t.Fatalf("plain full-reset = %q, want %q", got, "\x1b[0;39;49m")
	}
}

// Bold+Underline+Italic together emit ;1;4;3 in that order on full reset.
func TestAppendStyleAllAttributesFullReset(t *testing.T) {
	var buf []byte
	cell := Cell{Bold: true, Underline: true, Italic: true}
	got := string(appendStyle(buf, styleState{}, cell))
	want := "\x1b[0;39;49;1;4;3m"
	if got != want {
		t.Fatalf("all-attr full reset = %q, want %q", got, want)
	}
}

// differential: turning italic ON from a valid non-italic state emits only 3.
func TestAppendStyleItalicDifferentialOn(t *testing.T) {
	cur := styleState{valid: true}
	var buf []byte
	if got := string(appendStyle(buf, cur, Cell{Italic: true})); got != "\x1b[3m" {
		t.Fatalf("italic-on diff = %q, want %q", got, "\x1b[3m")
	}
}

// differential: turning italic OFF from a valid italic state emits only 23.
func TestAppendStyleItalicDifferentialOff(t *testing.T) {
	cur := styleState{valid: true, italic: true}
	var buf []byte
	if got := string(appendStyle(buf, cur, Cell{Italic: false})); got != "\x1b[23m" {
		t.Fatalf("italic-off diff = %q, want %q", got, "\x1b[23m")
	}
}

// differential: identical italic state emits nothing at all.
func TestAppendStyleItalicNoChangeEmitsNothing(t *testing.T) {
	cur := styleState{valid: true, italic: true}
	var buf []byte
	if got := appendStyle(buf, cur, Cell{Italic: true}); len(got) != 0 {
		t.Fatalf("no-change italic diff = %q, want no bytes", got)
	}
}

// differential: italic can be toggled independently of bold/underline. Here bold
// is unchanged while italic turns on, so only 3 is emitted (not 1;3).
func TestAppendStyleItalicIndependentOfBold(t *testing.T) {
	cur := styleState{valid: true, bold: true}
	var buf []byte
	if got := string(appendStyle(buf, cur, Cell{Bold: true, Italic: true})); got != "\x1b[3m" {
		t.Fatalf("italic-on with bold held = %q, want %q", got, "\x1b[3m")
	}
}

// regression: Bold and Underline differential codes are unchanged by the Italic
// addition (1/22 and 4/24 still emitted exactly as before).
func TestAppendStyleBoldUnderlineDifferentialUnchanged(t *testing.T) {
	cur := styleState{valid: true}
	var buf []byte
	if got := string(appendStyle(buf, cur, Cell{Bold: true, Underline: true})); got != "\x1b[1;4m" {
		t.Fatalf("bold+underline on = %q, want %q", got, "\x1b[1;4m")
	}
	cur = styleState{valid: true, bold: true, underline: true}
	buf = nil
	if got := string(appendStyle(buf, cur, Cell{})); got != "\x1b[22;24m" {
		t.Fatalf("bold+underline off = %q, want %q", got, "\x1b[22;24m")
	}
}

// integration: a Cell with Italic=true actually reaches the wire as SGR 3 when
// flushed through Apply (exercises the per-cell styleState.italic population in
// the flush loop, not just appendStyle in isolation).
func TestApplyItalicCellEmitsSGR3(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(4, 1, &output)
	app.Clear(DefaultCell())
	if err := app.Apply(); err != nil { // settle the cleared frame
		t.Fatalf("settle apply: %v", err)
	}
	output.Reset()
	app.WriteCell(0, 0, Cell{Ch: 'X', Italic: true})
	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// Default colours -> "0;39;49" then ";3" for italic, then 'm'.
	needle := "\x1b[0;39;49;3m"
	if !strings.Contains(output.String(), needle) {
		t.Fatalf("italic Apply output %q does not contain %q", output.String(), needle)
	}
}

// integration: the italic SGR is absent when the cell is not italic, so the
// attribute is opt-in and does not leak into plain cells.
func TestApplyPlainCellDoesNotEmitSGR3(t *testing.T) {
	var output bytes.Buffer
	app := NewWithSize(4, 1, &output)
	app.Clear(DefaultCell())
	if err := app.Apply(); err != nil {
		t.Fatalf("settle apply: %v", err)
	}
	output.Reset()
	app.WriteCell(0, 0, Cell{Ch: 'X'})
	if err := app.Apply(); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Contains(output.String(), ";3m") {
		t.Fatalf("plain Apply unexpectedly emitted italic SGR 3: %q", output.String())
	}
}
