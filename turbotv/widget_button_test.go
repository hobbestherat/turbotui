package tv

import (
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
