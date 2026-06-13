package tv

import (
	"testing"

	tui "github.com/hobbestherat/turbotui"
)

func TestTextBoxSubmitOnEnter(t *testing.T) {
	box := NewTextBox("abc", Rect{X: 0, Y: 0, W: 8, H: 1})
	submits := 0
	box.OnSubmit = func() {
		submits++
	}
	if !box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyEnter}) {
		t.Fatalf("expected Enter to be consumed when OnSubmit is set")
	}
	if submits != 1 {
		t.Fatalf("expected submit callback once, got %d", submits)
	}
}

func TestTextBoxEnterBubblesWithoutSubmit(t *testing.T) {
	box := NewTextBox("abc", Rect{X: 0, Y: 0, W: 8, H: 1})
	if box.handleType(box.Component, tui.TypeEvent{Key: tui.KeyEnter}) {
		t.Fatalf("expected Enter to bubble when OnSubmit is nil")
	}
}
