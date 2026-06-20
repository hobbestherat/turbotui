package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// TestCheckboxLongLabelTruncates verifies a label wider than the checkbox shows an
// ellipsis rather than being silently clipped.
func TestCheckboxLongLabelTruncates(t *testing.T) {
	app := tui.NewWithSize(12, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	cb := NewCheckbox("VeryLongCheckboxLabel", Rect{X: 0, Y: 0, W: 8, H: 1}, nil)
	cb.draw(cb.Component, surface)

	// "[ ] " occupies columns 0..3; the label starts at column 4 and is truncated
	// to the remaining 4 columns with an ellipsis on the last one.
	if got := app.ReadCell(4, 0).Ch; got != 'V' {
		t.Fatalf("expected label to start at column 4 with 'V', got %q", got)
	}
	if got := app.ReadCell(7, 0).Ch; got != '…' {
		t.Fatalf("expected ellipsis on the last column, got %q", got)
	}
}

// TestCheckboxFillsFullHeight ensures the focus background paints every row of the
// bounds (not just row 0), so the highlight matches the click hit area when a
// checkbox is sized taller than one row.
func TestCheckboxFillsFullHeight(t *testing.T) {
	app := tui.NewWithSize(12, 2, &bytes.Buffer{})
	surface := newRootSurface(app)
	cb := NewCheckbox("x", Rect{X: 0, Y: 0, W: 10, H: 2}, nil)
	cb.Component.hasFocus = true
	cb.draw(cb.Component, surface)

	blank := tui.DefaultCell().BG
	for y := 0; y < 2; y++ {
		cell := app.ReadCell(0, y)
		if cell.BG == blank {
			t.Fatalf("row %d was not painted with the focus background", y)
		}
		if cell.BG != cb.FocusBG {
			t.Fatalf("row %d BG mismatch: got focus bg mismatch (want %v)", y, cb.FocusBG)
		}
	}
}
