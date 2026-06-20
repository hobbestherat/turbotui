package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

// cellRendered reports whether any cell on the buffer holds ch. The raw Apply
// stream interleaves cursor-move escapes between every glyph, so multi-character
// substring checks on it are unreliable; reading the cell grid is not.
func cellRendered(app *tui.App, ch rune) bool {
	for y := 0; y < app.Height(); y++ {
		for x := 0; x < app.Width(); x++ {
			if app.ReadCell(x, y).Ch == ch {
				return true
			}
		}
	}
	return false
}

// TestShowConfirmYesNoLaysOutButtonsAndWiresThem is an end-to-end check that the
// HBox-based button row renders both buttons and that keyboard activation fires
// the result callback.
func TestShowConfirmYesNoLaysOutButtonsAndWiresThem(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})))

	result := "none"
	layer := ShowConfirmYesNo(desktop, "Confirm", "Apply values?", func(yes bool) {
		if yes {
			result = "yes"
		} else {
			result = "no"
		}
	})
	if layer == nil {
		t.Fatal("expected ShowConfirmYesNo to return a layer")
	}
	desktop.Redraw()

	// Both button labels render (distinctive uppercase letters not in the message).
	if !cellRendered(app, 'Y') {
		t.Fatal("expected the Yes button to render a 'Y'")
	}
	if !cellRendered(app, 'N') {
		t.Fatal("expected the No button to render an 'N'")
	}

	// "No" is focused on open; Enter should cancel.
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter})
	if result != "no" {
		t.Fatalf("expected Enter on focused 'No' to fire onResult(false), got %q", result)
	}
}

// TestShowConfirmYesNoTabToYesConfirms opens a fresh dialog, Tabs from the
// focused "No" to "Yes", and confirms via Enter.
func TestShowConfirmYesNoTabToYesConfirms(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})))

	result := "none"
	ShowConfirmYesNo(desktop, "Confirm", "Apply?", func(yes bool) {
		if yes {
			result = "yes"
		} else {
			result = "no"
		}
	})

	desktop.handleType(tui.TypeEvent{Key: tui.KeyTab})   // No -> Yes
	desktop.handleType(tui.TypeEvent{Key: tui.KeyEnter}) // Yes confirms
	if result != "yes" {
		t.Fatalf("expected Tab then Enter to confirm (yes), got %q", result)
	}
}

// TestShowConfirmYesNoButtonsDoNotOverlap checks the HBox gives the two buttons
// disjoint bounds (the whole point of replacing the hard-coded X:12/X:28).
func TestShowConfirmYesNoButtonsDoNotOverlap(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})))

	ShowConfirmYesNo(desktop, "Confirm", "Apply?", nil)
	desktop.Redraw()

	items := desktop.focusablesInTopLayer()
	if len(items) != 2 {
		t.Fatalf("expected exactly 2 focusable buttons, got %d", len(items))
	}
	a, b := items[0].AbsoluteBounds(), items[1].AbsoluteBounds()
	// Horizontally disjoint (the HBox packs them side by side).
	if a.Intersect(b).W > 0 {
		t.Fatalf("expected disjoint button bounds, got a=%+v b=%+v", a, b)
	}
}

// TestShowConfirmYesNoLongMessageWraps verifies the dialog's message label wraps a
// long message across its rows instead of clipping it to a single line (the root
// of the "text is not wrapping" popup bug). The 'Q' lives past the label's first
// row, so it is visible only when the message wraps.
func TestShowConfirmYesNoLongMessageWraps(t *testing.T) {
	app := tui.NewWithSize(80, 25, &bytes.Buffer{})
	desktop := NewDesktop(app)
	desktop.AddLayer(NewFullscreenLayer("base", NewComponent(Rect{X: 0, Y: 0, W: 80, H: 25})))

	// 50 A's fill the label's first row exactly; " Q" wraps onto the second row.
	message := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA Q"
	ShowConfirmYesNo(desktop, "Confirm", message, nil)
	desktop.Redraw()

	if !cellRendered(app, 'Q') {
		t.Fatal("expected the wrapped second line of the message to render a 'Q'")
	}
}
