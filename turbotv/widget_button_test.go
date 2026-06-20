package tv

import (
	"bytes"
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestButtonPressedLifecycle(t *testing.T) {
	count := 0
	button := NewButton("OK", Rect{X: 2, Y: 2, W: 8, H: 1}, func() {
		count++
	})
	component := button.Component

	_ = button.handleClick(component, tui.ClickEvent{X: 3, Y: 2, Button: tui.MouseLeft, Down: true})
	if !button.Pressed {
		t.Fatalf("expected button to become pressed on mouse down")
	}

	_ = button.handleClick(component, tui.ClickEvent{X: 12, Y: 2, Button: tui.MouseLeft, Down: false})
	if button.Pressed {
		t.Fatalf("expected button to reset pressed state on mouse up")
	}
	if count != 0 {
		t.Fatalf("expected no callback when releasing outside")
	}

	_ = button.handleClick(component, tui.ClickEvent{X: 3, Y: 2, Button: tui.MouseLeft, Down: true})
	_ = button.handleClick(component, tui.ClickEvent{X: 3, Y: 2, Button: tui.MouseLeft, Down: false})
	if count != 1 {
		t.Fatalf("expected callback when releasing inside, got %d", count)
	}
}

// TestButtonLongCaptionDoesNotBleed draws a button whose caption is far wider than
// the button through an unclipped (parent-clip) surface — the condition that used
// to let the label overrun into neighbours — and asserts the caption is truncated
// with an ellipsis and stops at the button's right edge.
func TestButtonLongCaptionDoesNotBleed(t *testing.T) {
	app := tui.NewWithSize(20, 1, &bytes.Buffer{})
	surface := newRootSurface(app) // parent clip: the whole row (no button clipping)
	button := NewButton("ABCDEFGHIJ", Rect{X: 0, Y: 0, W: 6, H: 1}, nil)
	button.Shadow = false
	button.draw(button.Component, surface)

	// The caption was truncated, so an ellipsis is shown inside the button.
	if got := app.ReadCell(3, 0).Ch; got != '…' {
		t.Fatalf("expected ellipsis at column 3, got %q", got)
	}
	// The closing bracket sits on the last button column (5).
	if got := app.ReadCell(5, 0).Ch; got != ']' {
		t.Fatalf("expected ']' on the last button column, got %q", got)
	}
	// No caption glyph may land past the button's right edge: the tail of the long
	// caption must not bleed into columns 6..10.
	for x := 6; x <= 10; x++ {
		ch := app.ReadCell(x, 0).Ch
		if ch >= 'A' && ch <= 'J' {
			t.Fatalf("caption glyph %q bled past the button into column %d", ch, x)
		}
	}
}

// TestButtonShortCaptionCentred confirms a caption that fits renders fully
// (chevrons, text, no ellipsis) and is centred.
func TestButtonShortCaptionCentred(t *testing.T) {
	app := tui.NewWithSize(10, 1, &bytes.Buffer{})
	surface := newRootSurface(app)
	button := NewButton("OK", Rect{X: 0, Y: 0, W: 8, H: 1}, nil)
	button.Shadow = false
	button.draw(button.Component, surface)

	if got := app.ReadCell(3, 0).Ch; got != 'O' {
		t.Fatalf("expected 'O' at column 3, got %q", got)
	}
	if got := app.ReadCell(4, 0).Ch; got != 'K' {
		t.Fatalf("expected 'K' at column 4, got %q", got)
	}
	for x := 0; x < app.Width(); x++ {
		if app.ReadCell(x, 0).Ch == '…' {
			t.Fatalf("short caption should not be truncated with ellipsis")
		}
	}
}
